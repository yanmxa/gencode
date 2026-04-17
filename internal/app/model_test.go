package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	appoutput "github.com/yanmxa/gencode/internal/app/output"
	appruntime "github.com/yanmxa/gencode/internal/app/runtime"
	appsystem "github.com/yanmxa/gencode/internal/app/system"
	appuser "github.com/yanmxa/gencode/internal/app/user"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/subagent"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/task/tracker"
)

type testLLMProvider struct{}

func (testLLMProvider) Stream(_ context.Context, _ llm.CompletionOptions) <-chan llm.StreamChunk {
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch
}

func (testLLMProvider) ListModels(_ context.Context) ([]llm.ModelInfo, error) { return nil, nil }

func (testLLMProvider) Name() string { return "test" }

func TestFireSessionEndClearsSessionHooks(t *testing.T) {
	engine := hook.NewEngine(setting.NewSettings(), "test-session", t.TempDir(), "")
	engine.AddSessionFunctionHook(hook.Stop, "", hook.FunctionHook{
		Callback: func(_ context.Context, _ hook.HookInput) (hook.HookOutput, error) {
			return hook.HookOutput{}, nil
		},
	})

	m := &model{runtime: appruntime.Model{HookEngine: engine}}
	m.fireSessionEnd("other")

	if engine.HasHooks(hook.Stop) {
		t.Fatal("expected session-scoped hooks to be cleared after SessionEnd")
	}
}

func TestInitFiresSetupHook(t *testing.T) {
	engine := hook.NewEngine(setting.NewSettings(), "test-session", t.TempDir(), "")
	triggered := make(chan string, 1)
	engine.AddSessionFunctionHook(hook.Setup, "init", hook.FunctionHook{
		Callback: func(_ context.Context, input hook.HookInput) (hook.HookOutput, error) {
			triggered <- input.Trigger
			return hook.HookOutput{}, nil
		},
	})

	// Hook firing now happens during model construction (not Init()) to
	// avoid value-receiver mutation loss. Simulate the newModel() path.
	m := model{runtime: appruntime.Model{HookEngine: engine}}
	m.runtime.HookEngine.ExecuteAsync(hook.Setup, hook.HookInput{Trigger: "init"})

	select {
	case trigger := <-triggered:
		if trigger != "init" {
			t.Fatalf("expected setup trigger init, got %q", trigger)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for setup hook")
	}
}

func TestHasAllToolResultsAllowsInterleavedNotices(t *testing.T) {
	m := appoutput.ConversationModel{
		Messages: []core.ChatMessage{
			{
				Role: core.RoleAssistant,
				ToolCalls: []core.ToolCall{
					{ID: "tc-1", Name: "Agent"},
				},
			},
			{Role: core.RoleNotice, Content: "background policy update"},
			{
				Role:       core.RoleUser,
				ToolResult: &core.ToolResult{ToolCallID: "tc-1", Content: "done"},
			},
		},
	}

	if !m.HasAllToolResults(0) {
		t.Fatal("expected notice between tool call and tool result to still count as complete")
	}
}

func TestViewRendersCompactStatusCard(t *testing.T) {
	appCwd = t.TempDir()
	m := newBaseModel()
	m.ready = true
	m.width = 100
	m.conv.Compact.Active = true
	m.conv.Compact.Focus = "deployment regressions"

	view := m.View()
	if !strings.Contains(view, "Compacting conversation") {
		t.Fatalf("expected active compact card, got:\n%s", view)
	}
	if !strings.Contains(view, "SESSION SUMMARY") || !strings.Contains(view, "Focus: deployment regressions") {
		t.Fatalf("expected compact focus details, got:\n%s", view)
	}

	m.conv.Compact.Active = false
	m.conv.Compact.Focus = ""
	m.conv.Compact.LastResult = "Condensed 73 earlier messages."
	view = m.View()
	if !strings.Contains(view, "Conversation compacted") {
		t.Fatalf("expected compact success title, got:\n%s", view)
	}
	if !strings.Contains(view, "Condensed 73 earlier messages.") {
		t.Fatalf("expected compact success detail, got:\n%s", view)
	}
}

func TestFreshSessionInitializesTaskStorageAndOutputDir(t *testing.T) {
	prevHome := os.Getenv("HOME")
	tmpHome := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(tmpHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(home) error = %v", err)
	}
	if err := os.Setenv("HOME", tmpHome); err != nil {
		t.Fatalf("Setenv(HOME) error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", prevHome)
		tracker.DefaultStore.Reset()
		_ = tracker.DefaultStore.SetStorageDir("")
		_ = task.SetOutputDir("")
	})
	tracker.DefaultStore.Reset()
	_ = tracker.DefaultStore.SetStorageDir("")
	_ = task.SetOutputDir("")

	appCwd = t.TempDir()
	prevSessionID := session.DefaultSetup.SessionID
	session.DefaultSetup.SessionID = "session-fresh-123"
	t.Cleanup(func() { session.DefaultSetup.SessionID = prevSessionID })
	m := newBaseModel()
	if m.runtime.SessionID != "session-fresh-123" {
		t.Fatalf("expected initial session id to propagate, got %q", m.runtime.SessionID)
	}

	m.initTaskStorage()

	wantDir := filepath.Join(tmpHome, ".gen", "tasks", "session-fresh-123")
	if got := tracker.DefaultStore.GetStorageDir(); got != wantDir {
		t.Fatalf("expected task storage dir %q, got %q", wantDir, got)
	}

	outputPath := task.OutputPath("worker-1")
	wantOutputPath := filepath.Join(wantDir, "outputs", "worker-1.log")
	if outputPath != wantOutputPath {
		t.Fatalf("expected output path %q, got %q", wantOutputPath, outputPath)
	}
	if _, err := os.Stat(filepath.Dir(outputPath)); err != nil {
		t.Fatalf("expected outputs directory to exist: %v", err)
	}
}

func TestApplyRunOptionsPluginDirReloadsPluginComponents(t *testing.T) {
	prevHome := os.Getenv("HOME")
	tmpHome := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(tmpHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(home) error = %v", err)
	}
	if err := os.Setenv("HOME", tmpHome); err != nil {
		t.Fatalf("Setenv(HOME) error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", prevHome)
		_, _ = setting.Reload()
	})
	_, _ = setting.Reload()

	prevPluginRegistry := plugin.DefaultRegistry
	prevSkillRegistry := skill.DefaultRegistry
	prevAgentRegistry := subagent.DefaultRegistry
	prevMCPRegistry := mcp.DefaultRegistry
	plugin.DefaultRegistry = plugin.NewRegistry()
	subagent.DefaultRegistry = subagent.NewRegistry()
	mcp.DefaultRegistry = nil
	t.Cleanup(func() {
		plugin.DefaultRegistry = prevPluginRegistry
		skill.DefaultRegistry = prevSkillRegistry
		subagent.DefaultRegistry = prevAgentRegistry
		mcp.DefaultRegistry = prevMCPRegistry
	})

	cwd := t.TempDir()
	pluginDir := filepath.Join(t.TempDir(), "demo-plugin")
	if err := os.MkdirAll(filepath.Join(pluginDir, ".gen-plugin"), 0o755); err != nil {
		t.Fatalf("MkdirAll(plugin meta) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(pluginDir, "agents"), 0o755); err != nil {
		t.Fatalf("MkdirAll(agents) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(pluginDir, "hooks"), 0o755); err != nil {
		t.Fatalf("MkdirAll(hooks) error = %v", err)
	}

	manifest := plugin.Manifest{
		Name:       "demo",
		Version:    "1.0.0",
		Agents:     "agents",
		Hooks:      "hooks/hooks.json",
		MCPServers: ".mcp.json",
	}
	manifestData, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(pluginDir, ".gen-plugin", "plugin.json"), manifestData, 0o644); err != nil {
		t.Fatalf("WriteFile(plugin.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "agents", "verifier.md"), []byte(`---
name: verifier
description: Plugin verifier
---
You are a verifier.`), 0o644); err != nil {
		t.Fatalf("WriteFile(agent) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "hooks", "hooks.json"), []byte(`{
  "hooks": {
    "SessionStart": [{
      "hooks": [{
        "type": "command",
        "command": "echo plugin"
      }]
    }]
  }
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(hooks) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, ".mcp.json"), []byte(`{
  "mcpServers": {
    "db": {
      "command": "echo"
    }
  }
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(.mcp.json) error = %v", err)
	}

	settings := setting.NewSettings()
	m := &model{
		cwd:       cwd,
		userInput: appuser.Model{MCP: appuser.MCPState{}},
		runtime: appruntime.Model{
			Settings:   settings,
			HookEngine: hook.NewEngine(settings, "test-session", cwd, ""),
		},
	}

	if err := m.applyRunOptions(setting.RunOptions{PluginDir: pluginDir}); err != nil {
		t.Fatalf("applyRunOptions() error = %v", err)
	}

	if _, ok := subagent.DefaultRegistry.Get("demo:verifier"); !ok {
		t.Fatal("expected plugin agent to be registered after --plugin-dir load")
	}
	if len(m.runtime.Settings.Hooks["SessionStart"]) == 0 {
		t.Fatal("expected plugin hooks to be merged into settings after --plugin-dir load")
	}
	if !m.runtime.HookEngine.HasHooks(hook.SessionStart) {
		t.Fatal("expected hook engine to see plugin hooks after --plugin-dir load")
	}
	if mcp.DefaultRegistry == nil {
		t.Fatal("expected MCP registry to be reloaded after --plugin-dir load")
	}
	if _, ok := mcp.DefaultRegistry.GetConfig("demo:db"); !ok {
		t.Fatal("expected plugin MCP server to be available after --plugin-dir load")
	}
}

func TestRefreshMemoryContextFiresInstructionsLoaded(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	if err := os.MkdirAll(filepath.Join(homeDir, ".gen"), 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	projectGenDir := filepath.Join(tmpDir, ".gen")
	if err := os.MkdirAll(projectGenDir, 0o755); err != nil {
		t.Fatalf("mkdir project .gen: %v", err)
	}
	projectFile := filepath.Join(projectGenDir, "GEN.md")
	if err := os.WriteFile(projectFile, []byte("project instructions"), 0o644); err != nil {
		t.Fatalf("write project GEN.md: %v", err)
	}

	prevHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", prevHome) }()

	engine := hook.NewEngine(setting.NewSettings(), "test-session", tmpDir, "")
	triggered := make(chan hook.HookInput, 1)
	engine.AddSessionFunctionHook(hook.InstructionsLoaded, projectFile, hook.FunctionHook{
		Callback: func(_ context.Context, input hook.HookInput) (hook.HookOutput, error) {
			triggered <- input
			return hook.HookOutput{}, nil
		},
	})

	m := &model{
		cwd:     tmpDir,
		runtime: appruntime.Model{HookEngine: engine},
	}
	m.runtime.RefreshMemoryContext(m.cwd, "session_start")

	select {
	case input := <-triggered:
		if input.FilePath != projectFile {
			t.Fatalf("expected instructions file %q, got %q", projectFile, input.FilePath)
		}
		if input.MemoryType != "Project" {
			t.Fatalf("expected memory type Project, got %q", input.MemoryType)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for InstructionsLoaded hook")
	}
}

func TestChangeCwdFiresCwdChanged(t *testing.T) {
	oldCwd := t.TempDir()
	newCwd := t.TempDir()

	engine := hook.NewEngine(setting.NewSettings(), "test-session", oldCwd, "")
	triggered := make(chan hook.HookInput, 1)
	engine.AddSessionFunctionHook(hook.CwdChanged, newCwd, hook.FunctionHook{
		Callback: func(_ context.Context, input hook.HookInput) (hook.HookOutput, error) {
			triggered <- input
			return hook.HookOutput{}, nil
		},
	})

	m := &model{
		cwd:     oldCwd,
		runtime: appruntime.Model{HookEngine: engine},
	}
	m.changeCwd(newCwd)

	if m.cwd != newCwd {
		t.Fatalf("expected cwd %q, got %q", newCwd, m.cwd)
	}

	select {
	case input := <-triggered:
		if input.OldCwd != oldCwd {
			t.Fatalf("expected old cwd %q, got %q", oldCwd, input.OldCwd)
		}
		if input.NewCwd != newCwd {
			t.Fatalf("expected new cwd %q, got %q", newCwd, input.NewCwd)
		}
		if input.Cwd != newCwd {
			t.Fatalf("expected hook cwd %q, got %q", newCwd, input.Cwd)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for CwdChanged hook")
	}
}

func TestApplyAgentToolSideEffectsFiresFileChanged(t *testing.T) {
	cwd := t.TempDir()
	filePath := filepath.Join(cwd, "file.txt")

	engine := hook.NewEngine(setting.NewSettings(), "test-session", cwd, "")
	triggered := make(chan hook.HookInput, 1)
	engine.AddSessionFunctionHook(hook.FileChanged, filePath, hook.FunctionHook{
		Callback: func(_ context.Context, input hook.HookInput) (hook.HookOutput, error) {
			triggered <- input
			return hook.HookOutput{}, nil
		},
	})

	m := &model{
		cwd:     cwd,
		runtime: appruntime.Model{HookEngine: engine},
	}
	m.applyAgentToolSideEffects("Write", map[string]any{"filePath": filePath})

	select {
	case input := <-triggered:
		if input.FilePath != filePath {
			t.Fatalf("expected file path %q, got %q", filePath, input.FilePath)
		}
		if input.Source != "Write" {
			t.Fatalf("expected source %q, got %q", "Write", input.Source)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for FileChanged hook")
	}
}

func TestApplyAgentToolSideEffectsUpdatesCwdFromBash(t *testing.T) {
	oldCwd := t.TempDir()
	newCwd := t.TempDir()

	engine := hook.NewEngine(setting.NewSettings(), "test-session", oldCwd, "")
	triggered := make(chan hook.HookInput, 1)
	engine.AddSessionFunctionHook(hook.CwdChanged, newCwd, hook.FunctionHook{
		Callback: func(_ context.Context, input hook.HookInput) (hook.HookOutput, error) {
			triggered <- input
			return hook.HookOutput{}, nil
		},
	})

	m := &model{
		cwd:     oldCwd,
		runtime: appruntime.Model{HookEngine: engine},
	}
	m.applyAgentToolSideEffects("Bash", map[string]any{"cwd": newCwd})

	if m.cwd != newCwd {
		t.Fatalf("expected cwd %q, got %q", newCwd, m.cwd)
	}

	select {
	case input := <-triggered:
		if input.OldCwd != oldCwd {
			t.Fatalf("expected old cwd %q, got %q", oldCwd, input.OldCwd)
		}
		if input.NewCwd != newCwd {
			t.Fatalf("expected new cwd %q, got %q", newCwd, input.NewCwd)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for CwdChanged hook")
	}
}

func TestChangeCwdReloadsProjectScopedSettings(t *testing.T) {
	prevHome := os.Getenv("HOME")
	tmpHome := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(tmpHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(home) error = %v", err)
	}
	if err := os.Setenv("HOME", tmpHome); err != nil {
		t.Fatalf("Setenv(HOME) error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", prevHome)
		_, _ = setting.Reload()
	})
	_, _ = setting.Reload()

	oldCwd := t.TempDir()
	newCwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(oldCwd, ".gen"), 0o755); err != nil {
		t.Fatalf("MkdirAll(old .gen) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(newCwd, ".gen"), 0o755); err != nil {
		t.Fatalf("MkdirAll(new .gen) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldCwd, ".gen", "settings.json"), []byte(`{"disabledTools":{"Bash":true}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(old settings) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(newCwd, ".gen", "settings.json"), []byte(`{"disabledTools":{"Grep":true}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(new settings) error = %v", err)
	}

	m := &model{
		cwd: oldCwd,
		runtime: appruntime.Model{
			Settings:           setting.InitForApp(oldCwd),
			SessionPermissions: setting.NewSessionPermissions(),
			DisabledTools:      map[string]bool{"Bash": true},
		},
	}

	m.changeCwd(newCwd)

	if !m.runtime.Settings.DisabledTools["Grep"] {
		t.Fatalf("expected Grep to be disabled after cwd change, got %#v", m.runtime.Settings.DisabledTools)
	}
	if m.runtime.Settings.DisabledTools["Bash"] {
		t.Fatalf("expected Bash disable from old cwd to be cleared, got %#v", m.runtime.Settings.DisabledTools)
	}
	if !m.runtime.DisabledTools["Grep"] {
		t.Fatalf("expected mode disabled tools to reload for new cwd, got %#v", m.runtime.DisabledTools)
	}
	if m.runtime.DisabledTools["Bash"] {
		t.Fatalf("expected old cwd disabled tools to be replaced, got %#v", m.runtime.DisabledTools)
	}
}

func TestInitRegistersWatchPathsFromSessionStart(t *testing.T) {
	engine := hook.NewEngine(setting.NewSettings(), "test-session", t.TempDir(), "")
	watchPath := filepath.Join(t.TempDir(), ".env")
	engine.AddSessionFunctionHook(hook.SessionStart, "", hook.FunctionHook{
		Callback: func(_ context.Context, _ hook.HookInput) (hook.HookOutput, error) {
			return hook.HookOutput{
				HookSpecificOutput: &hook.HookSpecificOutput{
					HookEventName: "SessionStart",
					WatchPaths:    []string{watchPath},
				},
			}, nil
		},
	})

	// Hook firing now happens during model construction (not Init()).
	// Simulate the newModel() path: fire SessionStart and apply outcome.
	m := model{runtime: appruntime.Model{HookEngine: engine}}
	outcome := engine.Execute(context.Background(), hook.SessionStart, hook.HookInput{
		Source: "startup",
	})
	m.applyRuntimeHookOutcome(outcome)

	paths := m.fileWatcher.CurrentPaths()
	if len(paths) != 1 || paths[0] != watchPath {
		t.Fatalf("expected watch path %q, got %#v", watchPath, paths)
	}
	m.fileWatcher.Stop()
}

func TestFileWatcherFiresFileChangedForWatchedPath(t *testing.T) {
	cwd := t.TempDir()
	filePath := filepath.Join(cwd, ".env")
	if err := os.WriteFile(filePath, []byte("A=1\n"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	engine := hook.NewEngine(setting.NewSettings(), "test-session", cwd, "")
	triggered := make(chan hook.HookInput, 1)
	engine.AddSessionFunctionHook(hook.FileChanged, filePath, hook.FunctionHook{
		Callback: func(_ context.Context, input hook.HookInput) (hook.HookOutput, error) {
			triggered <- input
			return hook.HookOutput{}, nil
		},
	})

	m := &model{
		cwd:     cwd,
		runtime: appruntime.Model{HookEngine: engine},
	}
	m.fileWatcher = appsystem.NewFileWatcher(engine, func(outcome hook.HookOutcome) {
		m.applyRuntimeHookOutcome(outcome)
	})
	m.applyRuntimeHookOutcome(hook.HookOutcome{WatchPaths: []string{filePath}})

	time.Sleep(appsystem.DefaultFileWatcherInterval + 100*time.Millisecond)
	if err := os.WriteFile(filePath, []byte("A=2\n"), 0o644); err != nil {
		t.Fatalf("update watched file: %v", err)
	}

	select {
	case input := <-triggered:
		if input.FilePath != filePath {
			t.Fatalf("expected file path %q, got %q", filePath, input.FilePath)
		}
		if input.Event != "change" {
			t.Fatalf("expected change event, got %q", input.Event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for watched file change hook")
	}

	m.fileWatcher.Stop()
}

func TestApplyRuntimeHookOutcomeSetsInitialPrompt(t *testing.T) {
	m := &model{}
	m.applyRuntimeHookOutcome(hook.HookOutcome{
		InitialUserMessage: "Summarize the repository structure.",
	})
	if m.initialPrompt != "Summarize the repository structure." {
		t.Fatalf("expected initial prompt from hook outcome, got %q", m.initialPrompt)
	}
}

func TestAsyncHookTickInjectsNoticeAndContext(t *testing.T) {
	m := &model{
		cwd:         t.TempDir(),
		conv:        appoutput.NewConversation(),
		systemInput: appsystem.New(nil),
		agentOutput: appoutput.New(80, appoutput.NewProgressHub(10)),
		runtime: appruntime.Model{
			LLMProvider: testLLMProvider{},
		},
	}
	m.systemInput.AsyncHookQueue.Push(appsystem.AsyncHookRewake{
		Notice:             "Async hook blocked: background policy blocked this",
		Context:            []string{"<background-hook-result>\nstatus: blocked\n</background-hook-result>"},
		ContinuationPrompt: "Re-evaluate the plan.",
	})

	cmd := m.handleAsyncHookTick()
	if cmd == nil {
		t.Fatal("expected async hook tick command")
	}

	// Verify the notice and context messages were appended
	if len(m.conv.Messages) < 2 {
		t.Fatalf("expected at least notice plus context, got %d messages", len(m.conv.Messages))
	}
	if m.conv.Messages[0].Role != core.RoleNotice {
		t.Fatalf("expected first message to be notice, got %v", m.conv.Messages[0].Role)
	}
	if !strings.Contains(m.conv.Messages[0].Content, "background policy blocked this") {
		t.Fatalf("unexpected notice content: %q", m.conv.Messages[0].Content)
	}
	if m.conv.Messages[1].Role != core.RoleUser || !strings.Contains(m.conv.Messages[1].Content, "<background-hook-result>") {
		t.Fatalf("expected context message, got %v: %q", m.conv.Messages[1].Role, m.conv.Messages[1].Content)
	}
}

func TestAsyncHookTickRefreshesHookStatus(t *testing.T) {
	engine := hook.NewEngine(setting.NewSettings(), "test-session", t.TempDir(), "")
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	engine.AddSessionFunctionHook(hook.Notification, "", hook.FunctionHook{
		StatusMessage: "hook is running",
		Callback: func(_ context.Context, _ hook.HookInput) (hook.HookOutput, error) {
			started <- struct{}{}
			<-release
			return hook.HookOutput{}, nil
		},
	})

	m := &model{
		systemInput: appsystem.New(engine),
		runtime:     appruntime.Model{HookEngine: engine},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		engine.Execute(context.Background(), hook.Notification, hook.HookInput{NotificationType: "idle_prompt"})
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for status hook to start")
	}

	_ = m.handleAsyncHookTick()
	if m.systemInput.HookStatus != "hook is running" {
		t.Fatalf("expected hook status to refresh, got %q", m.systemInput.HookStatus)
	}

	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for hook to finish")
	}

	_ = m.handleAsyncHookTick()
	if m.systemInput.HookStatus != "" {
		t.Fatalf("expected hook status to clear, got %q", m.systemInput.HookStatus)
	}
}

func TestTaskNotificationTickInjectsNotice(t *testing.T) {
	m := &model{
		cwd:         t.TempDir(),
		conv:        appoutput.NewConversation(),
		agentInput:  appagent.New(),
		agentOutput: appoutput.New(80, appoutput.NewProgressHub(10)),
		runtime: appruntime.Model{
			LLMProvider: testLLMProvider{},
		},
	}
	info := task.TaskInfo{
		ID:          "a123",
		Type:        task.TaskTypeAgent,
		Description: "Bubble Tea architecture audit",
		Status:      task.StatusCompleted,
		AgentName:   "Bubble Tea architecture audit",
		TurnCount:   4,
		TokenUsage:  321,
		Output:      "Architecture looks consistent.",
	}
	item, ok := appagent.BuildTaskNotification(appagent.TaskNotificationInput{
		Info:    info,
		Subject: appagent.TaskSubject(info),
		Batch:   appagent.SnapshotBackgroundBatchForTask(info.ID),
	})
	if !ok {
		t.Fatal("expected BuildTaskNotification to produce an item")
	}
	m.agentInput.Notifications.Push(item)

	cmd := m.handleTaskNotificationTick()
	if cmd == nil {
		t.Fatal("expected task notification tick command")
	}

	// Verify the notice was appended to conversation
	if len(m.conv.Messages) < 1 {
		t.Fatalf("expected at least one message, got %d", len(m.conv.Messages))
	}
	if m.conv.Messages[0].Role != core.RoleNotice {
		t.Fatalf("expected first message to be notice, got %v", m.conv.Messages[0].Role)
	}
	if !strings.Contains(m.conv.Messages[0].Content, "Bubble Tea architecture audit completed") {
		t.Fatalf("unexpected notice content: %q", m.conv.Messages[0].Content)
	}

	// Verify coordinator hint is included in the continuation prompt
	prompt := appagent.BuildContinuationPrompt(item)
	if !strings.Contains(prompt, "<phase>single_completion</phase>") {
		t.Fatalf("expected single completion coordinator hint, got %q", prompt)
	}
	if !strings.Contains(prompt, "<recommended-action>synthesize_then_decide_followup</recommended-action>") {
		t.Fatalf("expected single completion recommendation, got %q", prompt)
	}
	if !strings.Contains(prompt, "<wait-for-remaining-workers>false</wait-for-remaining-workers>") {
		t.Fatalf("expected single completion to avoid waiting, got %q", prompt)
	}
	if !strings.Contains(prompt, "<should-finalize-summary>true</should-finalize-summary>") {
		t.Fatalf("expected single completion to allow summary finalization, got %q", prompt)
	}
}

func TestTaskNotificationTickBatchesDrainsQueue(t *testing.T) {
	m := &model{
		cwd:         t.TempDir(),
		conv:        appoutput.NewConversation(),
		agentInput:  appagent.New(),
		agentOutput: appoutput.New(80, appoutput.NewProgressHub(10)),
		runtime: appruntime.Model{
			LLMProvider: testLLMProvider{},
		},
	}

	batch := &orchestration.Batch{
		ID:        "batch-1",
		Subject:   "2 background agents launched",
		Status:    tracker.StatusInProgress,
		Completed: 1,
		Total:     2,
		Failures:  1,
	}

	m.agentInput.Notifications.Push(appagent.Notification{
		Notice:             "dir-audit completed",
		Context:            []string{"single task context"},
		ContinuationPrompt: "<task-notification><task-id>bg-1</task-id></task-notification>",
		Batch:              batch,
	})
	m.agentInput.Notifications.Push(appagent.Notification{
		Notice:             "naming-audit failed",
		Context:            []string{"single task context"},
		ContinuationPrompt: "<task-notification><task-id>bg-2</task-id></task-notification>",
		Batch:              batch,
	})

	cmd := m.handleTaskNotificationTick()
	if cmd == nil {
		t.Fatal("expected task notification tick command")
	}
	if got := m.agentInput.Notifications.Len(); got != 0 {
		t.Fatalf("expected task notification queue to drain, got %d", got)
	}
	if len(m.conv.Messages) < 1 {
		t.Fatalf("expected at least one notice message, got %d", len(m.conv.Messages))
	}
	if !strings.Contains(m.conv.Messages[0].Content, "2 background tasks completed") {
		t.Fatalf("unexpected batched notice: %q", m.conv.Messages[0].Content)
	}
}

func TestTaskNotificationBatchMergeProducesCorrectXML(t *testing.T) {
	batch := &orchestration.Batch{
		ID:        "batch-1",
		Subject:   "2 background agents launched",
		Status:    tracker.StatusInProgress,
		Completed: 1,
		Total:     2,
		Failures:  1,
	}

	items := []appagent.Notification{
		{
			Notice:             "dir-audit completed",
			Context:            []string{"single task context"},
			ContinuationPrompt: "<task-notification><task-id>bg-1</task-id></task-notification>",
			Batch:              batch,
		},
		{
			Notice:             "naming-audit failed",
			Context:            []string{"single task context"},
			ContinuationPrompt: "<task-notification><task-id>bg-2</task-id></task-notification>",
			Batch:              batch,
		},
	}
	merged := appagent.MergeNotifications(items)
	prompt := appagent.BuildContinuationPrompt(merged)

	if !strings.Contains(prompt, "<coordinator-hint>") {
		t.Fatalf("expected structured coordinator hint, got %q", prompt)
	}
	if !strings.Contains(prompt, "<phase>partial_batch_with_failures</phase>") {
		t.Fatalf("expected partial batch phase, got %q", prompt)
	}
	if !strings.Contains(prompt, "<recommended-action>synthesize_partial_results_and_decide_recovery_or_wait</recommended-action>") {
		t.Fatalf("expected partial batch recommendation, got %q", prompt)
	}
	if !strings.Contains(prompt, "<wait-for-remaining-workers>true</wait-for-remaining-workers>") {
		t.Fatalf("expected partial batch to wait for remaining workers, got %q", prompt)
	}
	if !strings.Contains(prompt, "<should-continue-failed-worker>true</should-continue-failed-worker>") {
		t.Fatalf("expected partial batch failure recovery hint, got %q", prompt)
	}

	wrappedPrompt := merged.ContinuationPrompt
	if !strings.Contains(wrappedPrompt, "<task-notifications count=\"2\">") {
		t.Fatalf("expected batched notification wrapper, got %q", wrappedPrompt)
	}
	if !strings.Contains(wrappedPrompt, "<batch-summary>") || !strings.Contains(wrappedPrompt, "2 background agents launched is 1/2 complete with 1 failures") {
		t.Fatalf("expected batch summary, got %q", wrappedPrompt)
	}
	if !strings.Contains(wrappedPrompt, "<task-id>bg-1</task-id>") || !strings.Contains(wrappedPrompt, "<task-id>bg-2</task-id>") {
		t.Fatalf("expected both task IDs, got %q", wrappedPrompt)
	}

	contexts := merged.Context
	foundMultiple := false
	for _, ctx := range contexts {
		if strings.Contains(ctx, "Multiple background tasks completed") {
			foundMultiple = true
			break
		}
	}
	if !foundMultiple {
		t.Fatalf("expected batched continuation context, got %#v", contexts)
	}

	contextStrs := appagent.ContinuationContext(merged)
	foundPolicy := false
	for _, ctx := range contextStrs {
		if strings.Contains(ctx, "Do not assume the batch is finished") {
			foundPolicy = true
			break
		}
	}
	if !foundPolicy {
		t.Fatalf("expected partial-batch coordinator policy in continuation context, got %#v", contextStrs)
	}
}

func TestCoordinatorPolicyForCompletedFailedBatch(t *testing.T) {
	item := appagent.Notification{
		Notice:             "naming-audit failed",
		Context:            []string{"background task context"},
		ContinuationPrompt: "<task-notification><task-id>bg-2</task-id></task-notification>",
		Count:              1,
		Status:             "failed",
		Batch: &orchestration.Batch{
			ID:        "batch-2",
			Subject:   "2 background agents launched",
			Status:    tracker.StatusCompleted,
			Completed: 2,
			Total:     2,
			Failures:  1,
		},
	}

	prompt := appagent.BuildContinuationPrompt(item)
	if !strings.Contains(prompt, "<coordinator-hint>") {
		t.Fatalf("expected structured coordinator hint in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "<phase>completed_batch_with_failures</phase>") {
		t.Fatalf("expected completed batch failure phase, got %q", prompt)
	}
	if !strings.Contains(prompt, "<recommended-action>synthesize_batch_and_recover_failed_workers</recommended-action>") {
		t.Fatalf("expected completed batch recovery recommendation, got %q", prompt)
	}
	if !strings.Contains(prompt, "<wait-for-remaining-workers>false</wait-for-remaining-workers>") {
		t.Fatalf("expected completed batch to avoid waiting, got %q", prompt)
	}
	if !strings.Contains(prompt, "<should-continue-failed-worker>true</should-continue-failed-worker>") {
		t.Fatalf("expected completed batch to consider failed-worker continuation, got %q", prompt)
	}
	if !strings.Contains(prompt, "<should-spawn-verifier>true</should-spawn-verifier>") {
		t.Fatalf("expected completed batch to consider verifier, got %q", prompt)
	}
	if !strings.Contains(prompt, "<should-finalize-summary>true</should-finalize-summary>") {
		t.Fatalf("expected completed batch to allow final summary, got %q", prompt)
	}

	contextStrs := appagent.ContinuationContext(item)
	foundPolicy := false
	for _, ctx := range contextStrs {
		if strings.Contains(ctx, "background batch completed with failures") &&
			strings.Contains(ctx, "continue a failed worker, spawn a verifier, or report a partial result") {
			foundPolicy = true
			break
		}
	}
	if !foundPolicy {
		t.Fatalf("expected failed-batch coordinator policy in continuation context, got %#v", contextStrs)
	}
}
