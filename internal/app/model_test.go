package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yanmxa/gencode/internal/ext/subagent"
	appapproval "github.com/yanmxa/gencode/internal/app/approval"
	appconv "github.com/yanmxa/gencode/internal/app/conversation"
	"github.com/yanmxa/gencode/internal/app/mcpui"
	appmode "github.com/yanmxa/gencode/internal/app/mode"
	appoutput "github.com/yanmxa/gencode/internal/app/output"
	"github.com/yanmxa/gencode/internal/app/providerui"
	"github.com/yanmxa/gencode/internal/app/toolui"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/ext/mcp"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/ext/skill"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/perm"
	"github.com/yanmxa/gencode/internal/tracker"
	"github.com/yanmxa/gencode/internal/ui/progress"
)

type testLLMProvider struct{}

func (testLLMProvider) Stream(_ context.Context, _ provider.CompletionOptions) <-chan message.StreamChunk {
	ch := make(chan message.StreamChunk)
	close(ch)
	return ch
}

func (testLLMProvider) ListModels(_ context.Context) ([]provider.ModelInfo, error) { return nil, nil }

func (testLLMProvider) Name() string { return "test" }

func TestFireSessionEndClearsSessionHooks(t *testing.T) {
	engine := hooks.NewEngine(config.NewSettings(), "test-session", t.TempDir(), "")
	engine.AddSessionFunctionHook(hooks.Stop, "", hooks.FunctionHook{
		Callback: func(_ context.Context, _ hooks.HookInput) (hooks.HookOutput, error) {
			return hooks.HookOutput{}, nil
		},
	})

	m := &model{hookEngine: engine}
	m.fireSessionEnd("other")

	if engine.HasHooks(hooks.Stop) {
		t.Fatal("expected session-scoped hooks to be cleared after SessionEnd")
	}
}

func TestInitFiresSetupHook(t *testing.T) {
	engine := hooks.NewEngine(config.NewSettings(), "test-session", t.TempDir(), "")
	triggered := make(chan string, 1)
	engine.AddSessionFunctionHook(hooks.Setup, "init", hooks.FunctionHook{
		Callback: func(_ context.Context, input hooks.HookInput) (hooks.HookOutput, error) {
			triggered <- input.Trigger
			return hooks.HookOutput{}, nil
		},
	})

	m := model{hookEngine: engine}
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected init command batch")
	}

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
	m := appconv.Model{
		Messages: []message.ChatMessage{
			{
				Role: message.RoleAssistant,
				ToolCalls: []message.ToolCall{
					{ID: "tc-1", Name: "Agent"},
				},
			},
			{Role: message.RoleNotice, Content: "background policy update"},
			{
				Role:       message.RoleUser,
				ToolResult: &message.ToolResult{ToolCallID: "tc-1", Content: "done"},
			},
		},
	}

	if !m.HasAllToolResults(0) {
		t.Fatal("expected notice between tool call and tool result to still count as complete")
	}
}

func TestViewRendersCompactStatusCard(t *testing.T) {
	m := newBaseModel(t.TempDir(), modelInfra{})
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

	m := newBaseModel(t.TempDir(), modelInfra{initialSessionID: "session-fresh-123"})
	if m.session.CurrentID != "session-fresh-123" {
		t.Fatalf("expected initial session id to propagate, got %q", m.session.CurrentID)
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

func TestAsyncHookTickDoesNotInjectWhileToolExecutionPending(t *testing.T) {
	m := &model{
		asyncHookQueue: &asyncHookQueue{},
		tool: toolui.State{
			ExecState: toolui.ExecState{
				PendingCalls: []message.ToolCall{{ID: "tc-1", Name: "Agent"}},
				CurrentIdx:   0,
			},
		},
		conv: appconv.New(),
	}

	m.asyncHookQueue.Push(asyncHookRewake{
		Notice:             "Async hook blocked: test",
		Context:            []string{"extra context"},
		ContinuationPrompt: "continue",
	})

	cmd := m.handleAsyncHookTick()
	if cmd == nil {
		t.Fatal("expected ticker command")
	}
	if len(m.conv.Messages) != 0 {
		t.Fatalf("expected no async hook notice while tool execution is pending, got %#v", m.conv.Messages)
	}
}

func TestCronTickDoesNotDrainQueueWhileToolExecutionPending(t *testing.T) {
	m := &model{
		cronQueue: []string{"check background task"},
		tool: toolui.State{
			ExecState: toolui.ExecState{
				PendingCalls: []message.ToolCall{{ID: "tc-1", Name: "Agent"}},
				CurrentIdx:   0,
			},
		},
		conv: appconv.New(),
	}

	cmd := m.handleCronTick()
	if cmd == nil {
		t.Fatal("expected cron tick command")
	}
	if len(m.cronQueue) != 1 {
		t.Fatalf("expected queued cron prompt to remain queued while tool execution is pending, got %d", len(m.cronQueue))
	}
	if len(m.conv.Messages) != 0 {
		t.Fatalf("expected cron not to inject messages while tool execution is pending, got %#v", m.conv.Messages)
	}
}

func TestRenderActiveContentDoesNotDuplicatePendingAgentTitle(t *testing.T) {
	m := newBaseModel(t.TempDir(), modelInfra{})
	m.width = 100

	tc := message.ToolCall{
		ID:    "tc-1",
		Name:  tool.ToolAgent,
		Input: `{"subagent_type":"Deep","description":"analyze project structure","prompt":"inspect files"}`,
	}
	m.conv.Messages = []message.ChatMessage{{
		Role:      message.RoleAssistant,
		ToolCalls: []message.ToolCall{tc},
	}}
	m.tool.PendingCalls = []message.ToolCall{tc}
	m.tool.CurrentIdx = 0
	m.output.TaskProgress = map[int][]string{
		0: {"Agent: Check session and duplicate packages"},
	}

	rendered := m.renderActiveContent()
	if strings.Count(rendered, "Agent: Deep analyze project structure") != 1 {
		t.Fatalf("expected agent title once, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Agent: Check session and duplicate packages") {
		t.Fatalf("expected child task progress, got:\n%s", rendered)
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
		_, _ = config.Reload()
	})
	_, _ = config.Reload()

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

	settings := config.NewSettings()
	m := &model{
		cwd:        cwd,
		settings:   settings,
		hookEngine: hooks.NewEngine(settings, "test-session", cwd, ""),
		mcp:        mcpui.State{},
	}

	if err := m.applyRunOptions(config.RunOptions{PluginDir: pluginDir}); err != nil {
		t.Fatalf("applyRunOptions() error = %v", err)
	}

	if _, ok := subagent.DefaultRegistry.Get("demo:verifier"); !ok {
		t.Fatal("expected plugin agent to be registered after --plugin-dir load")
	}
	if len(m.settings.Hooks["SessionStart"]) == 0 {
		t.Fatal("expected plugin hooks to be merged into settings after --plugin-dir load")
	}
	if !m.hookEngine.HasHooks(hooks.SessionStart) {
		t.Fatal("expected hook engine to see plugin hooks after --plugin-dir load")
	}
	if m.mcp.Registry == nil {
		t.Fatal("expected MCP registry to be reloaded after --plugin-dir load")
	}
	if _, ok := m.mcp.Registry.GetConfig("demo:db"); !ok {
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

	engine := hooks.NewEngine(config.NewSettings(), "test-session", tmpDir, "")
	triggered := make(chan hooks.HookInput, 1)
	engine.AddSessionFunctionHook(hooks.InstructionsLoaded, projectFile, hooks.FunctionHook{
		Callback: func(_ context.Context, input hooks.HookInput) (hooks.HookOutput, error) {
			triggered <- input
			return hooks.HookOutput{}, nil
		},
	})

	m := &model{
		cwd:        tmpDir,
		hookEngine: engine,
	}
	m.refreshMemoryContext("session_start")

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

	engine := hooks.NewEngine(config.NewSettings(), "test-session", oldCwd, "")
	triggered := make(chan hooks.HookInput, 1)
	engine.AddSessionFunctionHook(hooks.CwdChanged, newCwd, hooks.FunctionHook{
		Callback: func(_ context.Context, input hooks.HookInput) (hooks.HookOutput, error) {
			triggered <- input
			return hooks.HookOutput{}, nil
		},
	})

	m := &model{
		cwd:        oldCwd,
		hookEngine: engine,
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

func TestApplyToolResultSideEffectsFiresFileChanged(t *testing.T) {
	cwd := t.TempDir()
	filePath := filepath.Join(cwd, "file.txt")

	engine := hooks.NewEngine(config.NewSettings(), "test-session", cwd, "")
	triggered := make(chan hooks.HookInput, 1)
	engine.AddSessionFunctionHook(hooks.FileChanged, filePath, hooks.FunctionHook{
		Callback: func(_ context.Context, input hooks.HookInput) (hooks.HookOutput, error) {
			triggered <- input
			return hooks.HookOutput{}, nil
		},
	})

	m := &model{
		cwd:        cwd,
		hookEngine: engine,
	}
	m.applyToolResultSideEffects(toolui.ExecResultMsg{
		ToolName: "Write",
		Result: message.ToolResult{
			ToolCallID:   "tool-1",
			Content:      "ok",
			HookResponse: map[string]any{"filePath": filePath},
		},
	})

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

func TestApplyToolResultSideEffectsUpdatesCwdFromBash(t *testing.T) {
	oldCwd := t.TempDir()
	newCwd := t.TempDir()

	engine := hooks.NewEngine(config.NewSettings(), "test-session", oldCwd, "")
	triggered := make(chan hooks.HookInput, 1)
	engine.AddSessionFunctionHook(hooks.CwdChanged, newCwd, hooks.FunctionHook{
		Callback: func(_ context.Context, input hooks.HookInput) (hooks.HookOutput, error) {
			triggered <- input
			return hooks.HookOutput{}, nil
		},
	})

	m := &model{
		cwd:        oldCwd,
		hookEngine: engine,
	}
	m.applyToolResultSideEffects(toolui.ExecResultMsg{
		ToolName: "Bash",
		Result: message.ToolResult{
			ToolCallID:   "tool-1",
			Content:      "ok",
			HookResponse: map[string]any{"cwd": newCwd},
		},
	})

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
		_, _ = config.Reload()
	})
	_, _ = config.Reload()

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
		cwd:      oldCwd,
		settings: loadSettingsForCwd(oldCwd),
		mode: appmode.State{
			SessionPermissions: config.NewSessionPermissions(),
			DisabledTools:      map[string]bool{"Bash": true},
		},
	}

	m.changeCwd(newCwd)

	if !m.settings.DisabledTools["Grep"] {
		t.Fatalf("expected Grep to be disabled after cwd change, got %#v", m.settings.DisabledTools)
	}
	if m.settings.DisabledTools["Bash"] {
		t.Fatalf("expected Bash disable from old cwd to be cleared, got %#v", m.settings.DisabledTools)
	}
	if !m.mode.DisabledTools["Grep"] {
		t.Fatalf("expected mode disabled tools to reload for new cwd, got %#v", m.mode.DisabledTools)
	}
	if m.mode.DisabledTools["Bash"] {
		t.Fatalf("expected old cwd disabled tools to be replaced, got %#v", m.mode.DisabledTools)
	}
}

func TestInitRegistersWatchPathsFromSessionStart(t *testing.T) {
	engine := hooks.NewEngine(config.NewSettings(), "test-session", t.TempDir(), "")
	watchPath := filepath.Join(t.TempDir(), ".env")
	engine.AddSessionFunctionHook(hooks.SessionStart, "", hooks.FunctionHook{
		Callback: func(_ context.Context, _ hooks.HookInput) (hooks.HookOutput, error) {
			return hooks.HookOutput{
				HookSpecificOutput: &hooks.HookSpecificOutput{
					HookEventName: "SessionStart",
					WatchPaths:    []string{watchPath},
				},
			}, nil
		},
	})

	m := model{
		hookEngine:  engine,
		fileWatcher: newFileWatcher(engine, nil),
	}
	m.fileWatcher.onOutcome = func(outcome hooks.HookOutcome) {
		m.applyRuntimeHookOutcome(outcome)
	}

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected init command batch")
	}

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

	engine := hooks.NewEngine(config.NewSettings(), "test-session", cwd, "")
	triggered := make(chan hooks.HookInput, 1)
	engine.AddSessionFunctionHook(hooks.FileChanged, filePath, hooks.FunctionHook{
		Callback: func(_ context.Context, input hooks.HookInput) (hooks.HookOutput, error) {
			triggered <- input
			return hooks.HookOutput{}, nil
		},
	})

	m := &model{
		cwd:         cwd,
		hookEngine:  engine,
		fileWatcher: newFileWatcher(engine, nil),
	}
	m.fileWatcher.onOutcome = func(outcome hooks.HookOutcome) {
		m.applyRuntimeHookOutcome(outcome)
	}
	m.applyRuntimeHookOutcome(hooks.HookOutcome{WatchPaths: []string{filePath}})

	time.Sleep(defaultFileWatcherInterval + 100*time.Millisecond)
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
	m.applyRuntimeHookOutcome(hooks.HookOutcome{
		InitialUserMessage: "Summarize the repository structure.",
	})
	if m.initialPrompt != "Summarize the repository structure." {
		t.Fatalf("expected initial prompt from hook outcome, got %q", m.initialPrompt)
	}
}

func TestPermissionDeniedRetryContinuesStream(t *testing.T) {
	cwd := t.TempDir()
	engine := hooks.NewEngine(config.NewSettings(), "test-session", cwd, "")
	engine.AddSessionFunctionHook(hooks.PermissionDenied, "", hooks.FunctionHook{
		Callback: func(_ context.Context, _ hooks.HookInput) (hooks.HookOutput, error) {
			return hooks.HookOutput{
				HookSpecificOutput: &hooks.HookSpecificOutput{
					HookEventName: "PermissionDenied",
					Retry:         true,
				},
			}, nil
		},
	})

	rt := &fakeConversationRuntime{}
	m := &model{
		cwd:        cwd,
		hookEngine: engine,
		runtime:    rt,
		conv:       appconv.New(),
		tool: toolui.State{
			ExecState: toolui.ExecState{
				PendingCalls: []message.ToolCall{{ID: "tc-1", Name: "Write"}},
				CurrentIdx:   0,
			},
		},
		provider: providerui.State{LLM: testLLMProvider{}},
		output:   appoutput.New(80, progress.NewHub(10)),
	}

	cmd := m.handlePermissionResponse(appapproval.ResponseMsg{
		Approved: false,
		Request: &perm.PermissionRequest{
			ToolName: "Write",
			FilePath: filepath.Join(cwd, "a.txt"),
		},
	})
	if cmd == nil {
		t.Fatal("expected retry denial to start a follow-up command")
	}
	_ = cmd()

	if !rt.startCalled {
		t.Fatal("expected permission denied retry to continue the model stream")
	}
	foundDeniedResult := false
	for _, msg := range m.conv.Messages {
		if msg.ToolResult != nil && msg.ToolResult.IsError && msg.ToolResult.Content == "User denied permission" {
			foundDeniedResult = true
			break
		}
	}
	if !foundDeniedResult {
		t.Fatal("expected denial to append an error tool result before retry")
	}
}

func TestAsyncHookTickRewakesModel(t *testing.T) {
	rt := &fakeConversationRuntime{}
	m := &model{
		cwd:            t.TempDir(),
		runtime:        rt,
		conv:           appconv.New(),
		asyncHookQueue: newAsyncHookQueue(),
		provider:       providerui.State{LLM: testLLMProvider{}},
		output:         appoutput.New(80, progress.NewHub(10)),
	}
	m.asyncHookQueue.Push(asyncHookRewake{
		Notice:             "Async hook blocked: background policy blocked this",
		Context:            []string{"<background-hook-result>\nstatus: blocked\n</background-hook-result>"},
		ContinuationPrompt: "Re-evaluate the plan.",
	})

	cmd := m.handleAsyncHookTick()
	if cmd == nil {
		t.Fatal("expected async hook tick command")
	}
	_ = cmd()

	if !rt.startCalled {
		t.Fatal("expected async hook rewake to start a follow-up stream")
	}
	if len(m.conv.Messages) != 2 {
		t.Fatalf("expected notice plus assistant placeholder, got %d", len(m.conv.Messages))
	}
	if m.conv.Messages[0].Role != message.RoleNotice {
		t.Fatalf("expected first message to be notice, got %v", m.conv.Messages[0].Role)
	}
	if m.conv.Messages[1].Role != message.RoleAssistant {
		t.Fatalf("expected second message to be assistant placeholder, got %v", m.conv.Messages[1].Role)
	}
	if rt.lastStreamReq.System == "" {
		t.Fatal("expected async hook rewake to build system prompt")
	}
	if !strings.Contains(rt.lastStreamReq.System, "<background-hook-result>") {
		t.Fatal("expected async hook rewake to pass internal continuation context in system prompt")
	}
	if got := rt.lastStreamReq.Messages[len(rt.lastStreamReq.Messages)-1]; got.Role != message.RoleUser || got.Content != "Re-evaluate the plan." {
		t.Fatalf("expected provider-only continuation prompt, got %#v", got)
	}
}

func TestAsyncHookTickRefreshesHookStatus(t *testing.T) {
	engine := hooks.NewEngine(config.NewSettings(), "test-session", t.TempDir(), "")
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	engine.AddSessionFunctionHook(hooks.Notification, "", hooks.FunctionHook{
		StatusMessage: "hook is running",
		Callback: func(_ context.Context, _ hooks.HookInput) (hooks.HookOutput, error) {
			started <- struct{}{}
			<-release
			return hooks.HookOutput{}, nil
		},
	})

	m := &model{
		hookEngine:     engine,
		asyncHookQueue: newAsyncHookQueue(),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		engine.Execute(context.Background(), hooks.Notification, hooks.HookInput{NotificationType: "idle_prompt"})
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for status hook to start")
	}

	_ = m.handleAsyncHookTick()
	if m.hookStatus != "hook is running" {
		t.Fatalf("expected hook status to refresh, got %q", m.hookStatus)
	}

	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for hook to finish")
	}

	_ = m.handleAsyncHookTick()
	if m.hookStatus != "" {
		t.Fatalf("expected hook status to clear, got %q", m.hookStatus)
	}
}

func TestTaskNotificationTickRewakesModel(t *testing.T) {
	rt := &fakeConversationRuntime{}
	m := &model{
		cwd:               t.TempDir(),
		runtime:           rt,
		conv:              appconv.New(),
		taskNotifications: newTaskNotificationQueue(),
		provider:          providerui.State{LLM: testLLMProvider{}},
		output:            appoutput.New(80, progress.NewHub(10)),
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
	item, ok := buildTaskNotification(info)
	if !ok {
		t.Fatal("expected buildTaskNotification to produce an item")
	}
	m.taskNotifications.Push(item)

	cmd := m.handleTaskNotificationTick()
	if cmd == nil {
		t.Fatal("expected task notification tick command")
	}
	_ = cmd()

	if !rt.startCalled {
		t.Fatal("expected task notification to start a follow-up stream")
	}
	if len(m.conv.Messages) != 2 {
		t.Fatalf("expected notice plus assistant placeholder, got %d", len(m.conv.Messages))
	}
	if m.conv.Messages[0].Role != message.RoleNotice {
		t.Fatalf("expected first message to be notice, got %v", m.conv.Messages[0].Role)
	}
	if !strings.Contains(m.conv.Messages[0].Content, "Bubble Tea architecture audit completed") {
		t.Fatalf("unexpected notice content: %q", m.conv.Messages[0].Content)
	}
	if got := rt.lastStreamReq.Messages[len(rt.lastStreamReq.Messages)-1]; got.Role != message.RoleUser || !strings.Contains(got.Content, "<task-notification>") {
		t.Fatalf("expected provider-only task notification prompt, got %#v", got)
	}
	prompt := rt.lastStreamReq.Messages[len(rt.lastStreamReq.Messages)-1].Content
	if !strings.Contains(prompt, "<coordinator-hint>") || !strings.Contains(prompt, "<phase>single_completion</phase>") {
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
	if rt.lastStreamReq.System == "" {
		t.Fatal("expected task notification to build system prompt")
	}
	if !strings.Contains(rt.lastStreamReq.System, "<task-notification>") {
		t.Fatal("expected task notification continuation context in system prompt")
	}
}

func TestTaskNotificationTickDoesNotInjectWhileToolExecutionPending(t *testing.T) {
	m := &model{
		taskNotifications: newTaskNotificationQueue(),
		tool: toolui.State{
			ExecState: toolui.ExecState{
				PendingCalls: []message.ToolCall{{ID: "tc-1", Name: "Agent"}},
				CurrentIdx:   0,
			},
		},
		conv: appconv.New(),
	}
	m.taskNotifications.Push(taskNotification{
		Notice:             "Background agent completed",
		Context:            []string{"background task context"},
		ContinuationPrompt: "<task-notification></task-notification>",
	})

	cmd := m.handleTaskNotificationTick()
	if cmd == nil {
		t.Fatal("expected ticker command")
	}
	if len(m.conv.Messages) != 0 {
		t.Fatalf("expected no task notification while tool execution is pending, got %#v", m.conv.Messages)
	}
	if got := m.taskNotifications.Len(); got != 1 {
		t.Fatalf("expected queued task notification to remain queued, got %d", got)
	}
}

func TestTaskNotificationTickBatchesQueuedNotifications(t *testing.T) {
	rt := &fakeConversationRuntime{}
	m := &model{
		cwd:               t.TempDir(),
		runtime:           rt,
		conv:              appconv.New(),
		taskNotifications: newTaskNotificationQueue(),
		provider:          providerui.State{LLM: testLLMProvider{}},
		output:            appoutput.New(80, progress.NewHub(10)),
	}

	batch := &backgroundBatchSnapshot{
		BatchID:   "batch-1",
		Subject:   "2 background agents launched",
		Status:    tracker.StatusInProgress,
		Completed: 1,
		Total:     2,
		Failures:  1,
	}

	m.taskNotifications.Push(taskNotification{
		Notice:             "dir-audit completed",
		Context:            []string{"single task context"},
		ContinuationPrompt: "<task-notification><task-id>bg-1</task-id></task-notification>",
		Batch:              batch,
	})
	m.taskNotifications.Push(taskNotification{
		Notice:             "naming-audit failed",
		Context:            []string{"single task context"},
		ContinuationPrompt: "<task-notification><task-id>bg-2</task-id></task-notification>",
		Batch:              batch,
	})

	cmd := m.handleTaskNotificationTick()
	if cmd == nil {
		t.Fatal("expected task notification tick command")
	}
	_ = cmd()

	if !rt.startCalled {
		t.Fatal("expected batched task notifications to start a follow-up stream")
	}
	if got := m.taskNotifications.Len(); got != 0 {
		t.Fatalf("expected task notification queue to drain, got %d", got)
	}
	if len(m.conv.Messages) != 2 {
		t.Fatalf("expected notice plus assistant placeholder, got %d", len(m.conv.Messages))
	}
	if !strings.Contains(m.conv.Messages[0].Content, "2 background tasks completed") {
		t.Fatalf("unexpected batched notice: %q", m.conv.Messages[0].Content)
	}
	got := rt.lastStreamReq.Messages[len(rt.lastStreamReq.Messages)-1]
	if got.Role != message.RoleUser || !strings.Contains(got.Content, "<task-notifications count=\"2\">") {
		t.Fatalf("expected batched task notification wrapper, got %#v", got)
	}
	if !strings.Contains(got.Content, "<coordinator-hint>") {
		t.Fatalf("expected structured coordinator hint, got %q", got.Content)
	}
	if !strings.Contains(got.Content, "<phase>partial_batch_with_failures</phase>") {
		t.Fatalf("expected partial batch phase, got %q", got.Content)
	}
	if !strings.Contains(got.Content, "<recommended-action>synthesize_partial_results_and_decide_recovery_or_wait</recommended-action>") {
		t.Fatalf("expected partial batch recommendation, got %q", got.Content)
	}
	if !strings.Contains(got.Content, "<wait-for-remaining-workers>true</wait-for-remaining-workers>") {
		t.Fatalf("expected partial batch to wait for remaining workers, got %q", got.Content)
	}
	if !strings.Contains(got.Content, "<should-continue-failed-worker>true</should-continue-failed-worker>") {
		t.Fatalf("expected partial batch failure recovery hint, got %q", got.Content)
	}
	if !strings.Contains(got.Content, "<batch-summary>") || !strings.Contains(got.Content, "2 background agents launched is 1/2 complete with 1 failures") {
		t.Fatalf("expected batch summary in provider prompt, got %q", got.Content)
	}
	if !strings.Contains(got.Content, "<task-id>bg-1</task-id>") || !strings.Contains(got.Content, "<task-id>bg-2</task-id>") {
		t.Fatalf("expected both task notifications in provider prompt, got %q", got.Content)
	}
	if !strings.Contains(rt.lastStreamReq.System, "Multiple background tasks completed") {
		t.Fatalf("expected batched continuation context in system prompt, got %q", rt.lastStreamReq.System)
	}
	if !strings.Contains(rt.lastStreamReq.System, "Do not assume the batch is finished") {
		t.Fatalf("expected partial-batch coordinator policy in system prompt, got %q", rt.lastStreamReq.System)
	}
}

func TestTaskNotificationTickAddsCoordinatorPolicyForCompletedFailedBatch(t *testing.T) {
	rt := &fakeConversationRuntime{}
	m := &model{
		cwd:               t.TempDir(),
		runtime:           rt,
		conv:              appconv.New(),
		taskNotifications: newTaskNotificationQueue(),
		provider:          providerui.State{LLM: testLLMProvider{}},
		output:            appoutput.New(80, progress.NewHub(10)),
	}

	m.taskNotifications.Push(taskNotification{
		Notice:             "naming-audit failed",
		Context:            []string{"background task context"},
		ContinuationPrompt: "<task-notification><task-id>bg-2</task-id></task-notification>",
		Batch: &backgroundBatchSnapshot{
			BatchID:   "batch-2",
			Subject:   "2 background agents launched",
			Status:    tracker.StatusCompleted,
			Completed: 2,
			Total:     2,
			Failures:  1,
		},
	})

	cmd := m.handleTaskNotificationTick()
	if cmd == nil {
		t.Fatal("expected task notification tick command")
	}
	_ = cmd()

	if !rt.startCalled {
		t.Fatal("expected task notification to start a follow-up stream")
	}
	joined := rt.lastStreamReq.System
	prompt := rt.lastStreamReq.Messages[len(rt.lastStreamReq.Messages)-1].Content
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
	if !strings.Contains(joined, "background batch completed with failures") {
		t.Fatalf("expected failed-batch coordinator policy in system prompt, got %q", joined)
	}
	if !strings.Contains(joined, "continue a failed worker, spawn a verifier, or report a partial result") {
		t.Fatalf("expected failed-batch follow-up guidance in system prompt, got %q", joined)
	}
}
