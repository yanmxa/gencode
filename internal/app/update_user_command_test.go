package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	appcommand "github.com/yanmxa/gencode/internal/ext/command"
	appconv "github.com/yanmxa/gencode/internal/app/output/conversation"
	appmode "github.com/yanmxa/gencode/internal/app/user/mode"
	appsystem "github.com/yanmxa/gencode/internal/app/system"
	"github.com/yanmxa/gencode/internal/app/user/sessionui"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/cron"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/ext/skill"
)

func TestHandlerRegistryMatchesBuiltinCommands(t *testing.T) {
	handlers := handlerRegistry()
	builtins := appcommand.BuiltinNames()

	if len(handlers) != len(builtins) {
		t.Fatalf("handler registry size mismatch: got %d, want %d", len(handlers), len(builtins))
	}

	for name := range builtins {
		if _, ok := handlers[name]; !ok {
			t.Fatalf("missing handler for builtin command %q", name)
		}
	}
}

func TestExecuteCommandExit(t *testing.T) {
	m := &model{}

	result, cmd, handled := executeCommand(context.Background(), m, "/exit")
	if !handled {
		t.Fatal("expected /exit to be handled")
	}
	if result != "" {
		t.Fatalf("expected empty result, got %q", result)
	}
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("expected quit command to produce a message")
	}
}

func TestExecuteCommandUnknown(t *testing.T) {
	m := &model{}

	result, cmd, handled := executeCommand(context.Background(), m, "/definitely-unknown")
	if !handled {
		t.Fatal("expected unknown command to be handled")
	}
	if cmd != nil {
		t.Fatal("did not expect follow-up command")
	}
	if result != "Unknown command: /definitely-unknown\nType /help for available commands." {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestExecuteCommandPlanUsageAndState(t *testing.T) {
	t.Run("usage shown when task missing", func(t *testing.T) {
		m := &model{}

		result, cmd, handled := executeCommand(context.Background(), m, "/plan")
		if !handled {
			t.Fatal("expected /plan to be handled")
		}
		if cmd != nil {
			t.Fatal("did not expect follow-up command")
		}
		if result == "" || result[:12] != "Usage: /plan" {
			t.Fatalf("unexpected usage result: %q", result)
		}
	})

	t.Run("plan mode enabled when task provided", func(t *testing.T) {
		m := &model{
			mode: appmode.State{
				SessionPermissions: config.NewSessionPermissions(),
			},
		}

		result, cmd, handled := executeCommand(context.Background(), m, "/plan audit regression coverage")
		if !handled {
			t.Fatal("expected /plan to be handled")
		}
		if cmd != nil {
			t.Fatal("did not expect follow-up command")
		}
		if m.mode.Operation != config.ModePlan || !m.mode.Enabled {
			t.Fatalf("expected plan mode enabled, got operation=%v enabled=%v", m.mode.Operation, m.mode.Enabled)
		}
		if m.mode.Task != "audit regression coverage" {
			t.Fatalf("unexpected plan task %q", m.mode.Task)
		}
		if m.mode.Store == nil {
			t.Fatal("expected plan store to be initialized")
		}
		if !strings.Contains(result, "Entering plan mode for: audit regression coverage") {
			t.Fatalf("unexpected result: %q", result)
		}
		if m.mode.SessionPermissions.AllowAllEdits || m.mode.SessionPermissions.AllowAllWrites || m.mode.SessionPermissions.AllowAllBash || m.mode.SessionPermissions.AllowAllSkills {
			t.Fatal("plan mode should reset permissive session flags")
		}
	})
}

func TestExecuteCommandOpenSelectors(t *testing.T) {
	t.Run("tools opens selector", func(t *testing.T) {
		m := &model{width: 80, height: 24}

		result, cmd, handled := executeCommand(context.Background(), m, "/tools")
		if !handled {
			t.Fatal("expected /tools to be handled")
		}
		if result != "" || cmd != nil {
			t.Fatalf("unexpected command outputs: result=%q cmd=%v", result, cmd != nil)
		}
		if !m.tool.Selector.IsActive() {
			t.Fatal("expected tool selector to become active")
		}
	})

	t.Run("resume opens session selector when sessions exist", func(t *testing.T) {
		tmpHome := t.TempDir()
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpHome)

		store, err := session.NewStore(tmpDir)
		if err != nil {
			t.Fatalf("NewStore(): %v", err)
		}
		err = store.Save(&session.Snapshot{
			Metadata: session.SessionMetadata{
				Title: "Resume me",
				Cwd:   tmpDir,
			},
			Entries: []session.Entry{
				{
					Type: session.EntryUser,
					Message: &session.EntryMessage{
						Role: "user",
						Content: []session.ContentBlock{
							{Type: "text", Text: "hello"},
						},
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("Save(): %v", err)
		}

		m := &model{
			cwd:     tmpDir,
			width:   80,
			height:  24,
			session: sessionui.State{Store: store},
		}

		result, cmd, handled := executeCommand(context.Background(), m, "/resume")
		if !handled {
			t.Fatal("expected /resume to be handled")
		}
		if result != "" || cmd != nil {
			t.Fatalf("unexpected command outputs: result=%q cmd=%v", result, cmd != nil)
		}
		if !m.session.Selector.IsActive() {
			t.Fatal("expected session selector to become active")
		}
	})
}

func TestExecuteCommandReloadPlugins(t *testing.T) {
	prev := plugin.DefaultRegistry
	plugin.DefaultRegistry = plugin.NewRegistry()
	t.Cleanup(func() { plugin.DefaultRegistry = prev })

	tmpHome := t.TempDir()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpHome)

	m := &model{cwd: tmpDir}

	result, cmd, handled := executeCommand(context.Background(), m, "/reload-plugins")
	if !handled {
		t.Fatal("expected /reload-plugins to be handled")
	}
	if cmd != nil {
		t.Fatal("did not expect follow-up command")
	}
	if !strings.Contains(result, "Reloaded plugins") {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestExecuteCommandLoopSchedulesRecurringPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".gen"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.gen): %v", err)
	}

	prevStore := cron.DefaultStore
	cron.DefaultStore = cron.NewStore()
	cron.DefaultStore.SetStoragePath(filepath.Join(tmpDir, ".gen", "scheduled_tasks.json"))
	t.Cleanup(func() { cron.DefaultStore = prevStore })

	m := &model{
		cwd:  tmpDir,
		conv: appconv.New(),
	}

	result, cmd, handled := executeCommand(context.Background(), m, "/loop 5m check the deploy")
	if !handled {
		t.Fatal("expected /loop to be handled")
	}
	if result != "" {
		t.Fatalf("expected empty result, got %q", result)
	}
	if cmd == nil {
		t.Fatal("expected follow-up command to execute prompt immediately")
	}

	jobs := cron.DefaultStore.List()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 scheduled job, got %d", len(jobs))
	}
	if jobs[0].Cron != "*/5 * * * *" {
		t.Fatalf("unexpected cron expression %q", jobs[0].Cron)
	}
	if jobs[0].Prompt != "check the deploy" {
		t.Fatalf("unexpected scheduled prompt %q", jobs[0].Prompt)
	}

	foundImmediatePrompt := false
	for _, msg := range m.conv.Messages {
		if msg.Content == "check the deploy" {
			foundImmediatePrompt = true
			break
		}
	}
	if !foundImmediatePrompt {
		t.Fatal("expected immediate prompt to be appended to conversation")
	}
}

func TestExecuteCommandLoopParsesTrailingEveryClause(t *testing.T) {
	prevStore := cron.DefaultStore
	cron.DefaultStore = cron.NewStore()
	t.Cleanup(func() { cron.DefaultStore = prevStore })

	m := &model{conv: appconv.New()}

	result, cmd, handled := executeCommand(context.Background(), m, "/loop check the deploy every 20m")
	if !handled {
		t.Fatal("expected /loop to be handled")
	}
	if result != "" {
		t.Fatalf("expected empty result, got %q", result)
	}
	if cmd == nil {
		t.Fatal("expected follow-up command")
	}

	jobs := cron.DefaultStore.List()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 scheduled job, got %d", len(jobs))
	}
	if jobs[0].Cron != "*/20 * * * *" {
		t.Fatalf("unexpected cron expression %q", jobs[0].Cron)
	}
	if jobs[0].Prompt != "check the deploy" {
		t.Fatalf("unexpected scheduled prompt %q", jobs[0].Prompt)
	}
}

func TestExecuteCommandLoopOnceSchedulesOneShot(t *testing.T) {
	prevStore := cron.DefaultStore
	cron.DefaultStore = cron.NewStore()
	t.Cleanup(func() { cron.DefaultStore = prevStore })

	m := &model{conv: appconv.New()}

	result, cmd, handled := executeCommand(context.Background(), m, "/loop once 20m check the deploy")
	if !handled {
		t.Fatal("expected /loop once to be handled")
	}
	if result != "" {
		t.Fatalf("expected empty result, got %q", result)
	}
	if cmd != nil {
		t.Fatal("did not expect immediate follow-up command for one-shot schedule")
	}

	jobs := cron.DefaultStore.List()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 scheduled job, got %d", len(jobs))
	}
	if jobs[0].Recurring {
		t.Fatal("expected one-shot task")
	}
	if jobs[0].Prompt != "check the deploy" {
		t.Fatalf("unexpected scheduled prompt %q", jobs[0].Prompt)
	}
}

func TestExecuteCommandLoopListAndDelete(t *testing.T) {
	prevStore := cron.DefaultStore
	cron.DefaultStore = cron.NewStore()
	t.Cleanup(func() { cron.DefaultStore = prevStore })

	job, err := cron.DefaultStore.Create("*/5 * * * *", "check deploy", true, false)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	m := &model{conv: appconv.New()}

	result, cmd, handled := executeCommand(context.Background(), m, "/loop list")
	if !handled {
		t.Fatal("expected /loop list to be handled")
	}
	if cmd != nil {
		t.Fatal("did not expect follow-up command for /loop list")
	}
	if !strings.Contains(result, job.ID) || !strings.Contains(result, "check deploy") {
		t.Fatalf("unexpected list output %q", result)
	}

	result, cmd, handled = executeCommand(context.Background(), m, "/loop delete "+job.ID)
	if !handled {
		t.Fatal("expected /loop delete to be handled")
	}
	if cmd != nil {
		t.Fatal("did not expect follow-up command for /loop delete")
	}
	if !strings.Contains(result, job.ID) {
		t.Fatalf("unexpected delete output %q", result)
	}
	if len(cron.DefaultStore.List()) != 0 {
		t.Fatal("expected scheduled task to be deleted")
	}
}

func TestExecuteCommandLoopDeleteAll(t *testing.T) {
	prevStore := cron.DefaultStore
	cron.DefaultStore = cron.NewStore()
	t.Cleanup(func() { cron.DefaultStore = prevStore })

	if _, err := cron.DefaultStore.Create("*/5 * * * *", "check deploy", true, false); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if _, err := cron.DefaultStore.Create("*/10 * * * *", "check logs", true, false); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	m := &model{conv: appconv.New()}

	result, cmd, handled := executeCommand(context.Background(), m, "/loop delete all")
	if !handled {
		t.Fatal("expected /loop delete all to be handled")
	}
	if cmd != nil {
		t.Fatal("did not expect follow-up command")
	}
	if !strings.Contains(result, "Cancelled 2 scheduled task(s).") {
		t.Fatalf("unexpected result %q", result)
	}
	if len(cron.DefaultStore.List()) != 0 {
		t.Fatal("expected all scheduled tasks to be deleted")
	}
}

func TestHandleClearCommand_PreservesScheduledTasks(t *testing.T) {
	prevStore := cron.DefaultStore
	cron.DefaultStore = cron.NewStore()
	t.Cleanup(func() { cron.DefaultStore = prevStore })

	if _, err := cron.DefaultStore.Create("*/5 * * * *", "check deploy", true, false); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	m := &model{conv: appconv.New()}
	_, _, err := handleClearCommand(context.Background(), m, "")
	if err != nil {
		t.Fatalf("handleClearCommand() failed: %v", err)
	}

	if len(cron.DefaultStore.List()) != 1 {
		t.Fatal("expected scheduled tasks to survive /clear")
	}
}

func TestTriggerCronTickNow_ReturnsCronTickMsg(t *testing.T) {
	cmd := appsystem.TriggerCronTickNow()
	if cmd == nil {
		t.Fatal("expected TriggerCronTickNow to return a command")
	}
	msg := cmd()
	if _, ok := msg.(appsystem.CronTickMsg); !ok {
		t.Fatalf("expected CronTickMsg, got %T", msg)
	}
}

func TestShouldPreserveCommandInConversation_PreservesLoopKeyword(t *testing.T) {
	if !shouldPreserveCommandInConversation("/loop 5m check deploy", "", func() tea.Msg { return nil }) {
		t.Fatal("expected /loop command to be preserved in conversation")
	}
	if !shouldPreserveCommandInConversation("/loop once 20m check deploy", "", nil) {
		t.Fatal("expected /loop once command to be preserved in conversation")
	}
	if !shouldPreserveCommandInConversation("/tools", "", nil) {
		t.Fatal("expected selector slash command to be preserved")
	}
	if !shouldPreserveCommandInConversation("/plan audit coverage", "", nil) {
		t.Fatal("expected slash command arguments to be preserved")
	}
	if shouldPreserveCommandInConversation("/clear", "", nil) {
		t.Fatal("did not expect /clear to be preserved")
	}
}

func TestHandleCommandSubmit_LoopDeleteAllPreservesLiteralInputAndDeletesJobs(t *testing.T) {
	prevStore := cron.DefaultStore
	cron.DefaultStore = cron.NewStore()
	t.Cleanup(func() { cron.DefaultStore = prevStore })

	if _, err := cron.DefaultStore.Create("*/5 * * * *", "check deploy", true, false); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if _, err := cron.DefaultStore.Create("*/10 * * * *", "check logs", true, false); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	tmpDir := t.TempDir()
	base := newBaseModel(tmpDir, modelInfra{})
	m := &model{
		cwd:   tmpDir,
		userInput: base.userInput,
		conv:      appconv.New(),
	}

	cmd, handled := m.handleCommandSubmit("/loop delete all")
	if !handled {
		t.Fatal("expected /loop delete all to be handled")
	}
	if cmd == nil {
		t.Fatal("expected commit command for handled slash command")
	}
	if len(cron.DefaultStore.List()) != 0 {
		t.Fatal("expected all scheduled jobs to be deleted")
	}
	if len(m.conv.Messages) != 2 {
		t.Fatalf("expected preserved user command and notice, got %d messages", len(m.conv.Messages))
	}
	if m.conv.Messages[0].Role != core.RoleUser || m.conv.Messages[0].Content != "/loop delete all" {
		t.Fatalf("expected preserved literal slash command, got %#v", m.conv.Messages[0])
	}
	if m.conv.Messages[1].Role != core.RoleNotice || !strings.Contains(m.conv.Messages[1].Content, "Cancelled 2 scheduled task(s).") {
		t.Fatalf("expected delete-all notice, got %#v", m.conv.Messages[1])
	}
}

func TestHandleCommandSubmit_ToolsPreservesLiteralInput(t *testing.T) {
	tmpDir := t.TempDir()
	base := newBaseModel(tmpDir, modelInfra{})
	m := &model{
		cwd:    tmpDir,
		userInput: base.userInput,
		conv:      appconv.New(),
		width:     80,
		height:    24,
	}

	cmd, handled := m.handleCommandSubmit("/tools")
	if !handled {
		t.Fatal("expected /tools to be handled")
	}
	if cmd == nil {
		t.Fatal("expected commit command for handled slash command")
	}
	if !m.tool.Selector.IsActive() {
		t.Fatal("expected tool selector to open")
	}
	if len(m.conv.Messages) != 1 {
		t.Fatalf("expected preserved user command only, got %d messages", len(m.conv.Messages))
	}
	if m.conv.Messages[0].Role != core.RoleUser || m.conv.Messages[0].Content != "/tools" {
		t.Fatalf("expected preserved /tools command, got %#v", m.conv.Messages[0])
	}
}

func TestHandleCommandSubmit_LoopOncePlacesCommandBeforeNotice(t *testing.T) {
	prevStore := cron.DefaultStore
	cron.DefaultStore = cron.NewStore()
	t.Cleanup(func() { cron.DefaultStore = prevStore })

	tmpDir := t.TempDir()
	base := newBaseModel(tmpDir, modelInfra{})
	m := &model{
		cwd:       tmpDir,
		userInput: base.userInput,
		conv:      appconv.New(),
	}

	cmd, handled := m.handleCommandSubmit("/loop once 20m check the deploy")
	if !handled {
		t.Fatal("expected /loop once to be handled")
	}
	if cmd == nil {
		t.Fatal("expected commit command")
	}
	if len(m.conv.Messages) != 2 {
		t.Fatalf("expected command and notice, got %d messages", len(m.conv.Messages))
	}
	if m.conv.Messages[0].Role != core.RoleUser || m.conv.Messages[0].Content != "/loop once 20m check the deploy" {
		t.Fatalf("expected literal slash command first, got %#v", m.conv.Messages[0])
	}
	if m.conv.Messages[1].Role != core.RoleNotice || !strings.Contains(m.conv.Messages[1].Content, "Scheduled one-shot task") {
		t.Fatalf("expected one-shot notice second, got %#v", m.conv.Messages[1])
	}
}

func TestHandleCommandSubmit_LoopRecurringPlacesCommandBeforeNoticeAndPrompt(t *testing.T) {
	prevStore := cron.DefaultStore
	cron.DefaultStore = cron.NewStore()
	t.Cleanup(func() { cron.DefaultStore = prevStore })

	tmpDir := t.TempDir()
	base := newBaseModel(tmpDir, modelInfra{})
	m := &model{
		cwd:       tmpDir,
		userInput: base.userInput,
		conv:      appconv.New(),
	}

	cmd, handled := m.handleCommandSubmit("/loop 5m check the deploy")
	if !handled {
		t.Fatal("expected /loop to be handled")
	}
	if cmd == nil {
		t.Fatal("expected commit command")
	}
	if len(m.conv.Messages) < 3 {
		t.Fatalf("expected at least command, notice, and parsed prompt, got %d messages", len(m.conv.Messages))
	}
	if m.conv.Messages[0].Role != core.RoleUser || m.conv.Messages[0].Content != "/loop 5m check the deploy" {
		t.Fatalf("expected literal slash command first, got %#v", m.conv.Messages[0])
	}
	if m.conv.Messages[1].Role != core.RoleNotice || !strings.Contains(m.conv.Messages[1].Content, "Scheduled recurring task") {
		t.Fatalf("expected recurring notice second, got %#v", m.conv.Messages[1])
	}
	if m.conv.Messages[2].Role != core.RoleUser || m.conv.Messages[2].Content != "check the deploy" {
		t.Fatalf("expected parsed prompt third, got %#v", m.conv.Messages[2])
	}
}

func TestExecuteSkillCommand_PreservesFullSlashInvocation(t *testing.T) {
	m := &model{}
	sk := &skill.Skill{Name: "commit", Namespace: "git"}

	executeSkillCommand(m, sk, "fix release notes")

	if got := m.skill.PendingArgs; got != "/git:commit fix release notes" {
		t.Fatalf("expected full slash invocation, got %q", got)
	}
}
