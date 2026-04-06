package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yanmxa/gencode/internal/agent"
	appapproval "github.com/yanmxa/gencode/internal/app/approval"
	appconv "github.com/yanmxa/gencode/internal/app/conversation"
	appmcp "github.com/yanmxa/gencode/internal/app/mcp"
	appoutput "github.com/yanmxa/gencode/internal/app/output"
	appprovider "github.com/yanmxa/gencode/internal/app/provider"
	apptool "github.com/yanmxa/gencode/internal/app/tool"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/options"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/tool/permission"
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
	prevAgentRegistry := agent.DefaultRegistry
	prevMCPRegistry := mcp.DefaultRegistry
	plugin.DefaultRegistry = plugin.NewRegistry()
	agent.DefaultRegistry = agent.NewRegistry()
	mcp.DefaultRegistry = nil
	t.Cleanup(func() {
		plugin.DefaultRegistry = prevPluginRegistry
		skill.DefaultRegistry = prevSkillRegistry
		agent.DefaultRegistry = prevAgentRegistry
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
		mcp:        appmcp.State{},
	}

	if err := m.applyRunOptions(options.RunOptions{PluginDir: pluginDir}); err != nil {
		t.Fatalf("applyRunOptions() error = %v", err)
	}

	if _, ok := agent.DefaultRegistry.Get("demo:verifier"); !ok {
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
	m.applyToolResultSideEffects(apptool.ExecResultMsg{
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

	runtime := &fakeConversationRuntime{}
	m := &model{
		cwd:        cwd,
		hookEngine: engine,
		runtime:    runtime,
		conv:       appconv.New(),
		loop:       &core.Loop{},
		tool: apptool.State{
			ExecState: apptool.ExecState{
				PendingCalls: []message.ToolCall{{ID: "tc-1", Name: "Write"}},
				CurrentIdx:   0,
			},
		},
		output: appoutput.New(80, progress.NewHub(10)),
	}

	cmd := m.handlePermissionResponse(appapproval.ResponseMsg{
		Approved: false,
		Request: &permission.PermissionRequest{
			ToolName: "Write",
			FilePath: filepath.Join(cwd, "a.txt"),
		},
	})
	if cmd == nil {
		t.Fatal("expected retry denial to start a follow-up command")
	}
	_ = cmd()

	if !runtime.startCalled {
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
	runtime := &fakeConversationRuntime{}
	m := &model{
		cwd:            t.TempDir(),
		runtime:        runtime,
		conv:           appconv.New(),
		asyncHookQueue: newAsyncHookQueue(),
		loop:           &core.Loop{},
		provider:       appprovider.State{LLM: testLLMProvider{}},
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

	if !runtime.startCalled {
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
	if runtime.lastStreamReq.Loop == nil || runtime.lastStreamReq.Loop.System == nil {
		t.Fatal("expected async hook rewake to build loop system")
	}
	if len(runtime.lastStreamReq.Loop.System.Extra) == 0 {
		t.Fatal("expected async hook rewake to pass internal continuation context")
	}
	if runtime.lastStreamReq.Loop.System.Extra[0] != "<background-hook-result>\nstatus: blocked\n</background-hook-result>" {
		t.Fatalf("unexpected async hook continuation context: %#v", runtime.lastStreamReq.Loop.System.Extra)
	}
	if got := runtime.lastStreamReq.Messages[len(runtime.lastStreamReq.Messages)-1]; got.Role != message.RoleUser || got.Content != "Re-evaluate the plan." {
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
