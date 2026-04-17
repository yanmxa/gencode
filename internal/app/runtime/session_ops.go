package runtime

import (
	"os"
	"path/filepath"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/task/tracker"
	"github.com/yanmxa/gencode/internal/tool"
	gozap "go.uber.org/zap"
)

type SessionSaveDeps struct {
	Runtime         *Model
	Cwd             string
	Messages        []core.ChatMessage
	ReconfigureTool func()
}

type SessionLoadDeps struct {
	Runtime         *Model
	Cwd             string
	RestoreMessages func([]core.ChatMessage)
}

func SaveSession(deps SessionSaveDeps) error {
	if err := deps.Runtime.EnsureSessionStore(deps.Cwd); err != nil {
		return err
	}

	if len(deps.Messages) == 0 {
		return nil
	}

	entries := session.ConvertToEntries(deps.Messages)

	providerName := ""
	modelID := ""
	if deps.Runtime.CurrentModel != nil {
		providerName = string(deps.Runtime.CurrentModel.Provider)
		modelID = deps.Runtime.CurrentModel.ModelID
	}

	sess := &session.Snapshot{
		Metadata: session.SessionMetadata{
			ID:         deps.Runtime.SessionID,
			Provider:   providerName,
			Model:      modelID,
			Cwd:        deps.Cwd,
			LastPrompt: session.ExtractLastUserText(entries),
			Summary:    deps.Runtime.SessionSummary,
			Mode:       deps.Runtime.SessionMode(),
		},
		Entries: entries,
		Tasks:   tracker.DefaultStore.Export(),
	}

	if sess.Metadata.Title == "" || sess.Metadata.ID == "" {
		sess.Metadata.Title = session.GenerateTitle(sess.Entries)
	}

	if err := deps.Runtime.SessionStore.Save(sess); err != nil {
		return err
	}

	deps.Runtime.SessionID = sess.Metadata.ID
	InitTaskStorage(deps.Runtime.SessionID)

	if deps.Runtime.HookEngine != nil {
		deps.Runtime.HookEngine.SetTranscriptPath(deps.Runtime.SessionStore.SessionPath(sess.Metadata.ID))
	}
	if deps.ReconfigureTool != nil {
		deps.ReconfigureTool()
	}

	return nil
}

func LoadSession(deps SessionLoadDeps, id string) error {
	if err := deps.Runtime.EnsureSessionStore(deps.Cwd); err != nil {
		return err
	}

	sess, err := deps.Runtime.SessionStore.Load(id)
	if err != nil {
		return err
	}

	tracker.DefaultStore.SetStorageDir("")
	RestoreSessionData(deps.Runtime, sess, deps.RestoreMessages)

	if len(sess.Tasks) == 0 {
		tracker.DefaultStore.Reset()
	}
	tool.ResetFetched()

	deps.Runtime.InputTokens = 0
	deps.Runtime.OutputTokens = 0

	return nil
}

func RestoreSessionData(rt *Model, sess *session.Snapshot, restoreMessages func([]core.ChatMessage)) {
	restoreMessages(session.ConvertFromEntries(sess.Entries))
	rt.SessionID = sess.Metadata.ID

	if sess.Metadata.Summary != "" {
		rt.SessionSummary = sess.Metadata.Summary
	} else if rt.SessionStore != nil {
		if mem, err := rt.SessionStore.LoadSessionMemory(sess.Metadata.ID); err == nil && mem != "" {
			rt.SessionSummary = mem
		}
	}

	InitTaskStorage(rt.SessionID)

	if len(sess.Tasks) > 0 {
		tracker.DefaultStore.Import(sess.Tasks)
	}
}

func InitTaskStorage(sessionID string) {
	if tracker.DefaultStore.GetStorageDir() != "" {
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Logger().Warn("failed to get home directory for task storage", gozap.Error(err))
		return
	}

	taskListID := os.Getenv("GEN_TASK_LIST_ID")
	if taskListID != "" {
		dir := filepath.Join(homeDir, ".gen", "tasks", taskListID)
		tracker.DefaultStore.SetStorageDir(dir)
		_ = task.SetOutputDir(filepath.Join(dir, "outputs"))
		return
	}

	if sessionID == "" {
		return
	}
	dir := filepath.Join(homeDir, ".gen", "tasks", sessionID)
	tracker.DefaultStore.SetStorageDir(dir)
	_ = task.SetOutputDir(filepath.Join(dir, "outputs"))
}
