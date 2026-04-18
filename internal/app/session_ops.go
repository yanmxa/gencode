package app

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

type sessionSaveDeps struct {
	env             *Env
	cwd             string
	messages        []core.ChatMessage
	reconfigureTool func()
}

type sessionLoadDeps struct {
	env             *Env
	cwd             string
	restoreMessages func([]core.ChatMessage)
}

func saveSession(deps sessionSaveDeps) error {
	if err := deps.env.EnsureSessionStore(deps.cwd); err != nil {
		return err
	}

	if len(deps.messages) == 0 {
		return nil
	}

	entries := session.ConvertToEntries(deps.messages)

	providerName := ""
	modelID := ""
	if deps.env.CurrentModel != nil {
		providerName = string(deps.env.CurrentModel.Provider)
		modelID = deps.env.CurrentModel.ModelID
	}

	sess := &session.Snapshot{
		Metadata: session.SessionMetadata{
			ID:         deps.env.SessionID,
			Provider:   providerName,
			Model:      modelID,
			Cwd:        deps.cwd,
			LastPrompt: session.ExtractLastUserText(entries),
			Summary:    deps.env.SessionSummary,
			Mode:       deps.env.SessionMode(),
		},
		Entries: entries,
		Tasks:   tracker.DefaultStore.Export(),
	}

	if sess.Metadata.Title == "" || sess.Metadata.ID == "" {
		sess.Metadata.Title = session.GenerateTitle(sess.Entries)
	}

	if err := deps.env.SessionStore.Save(sess); err != nil {
		return err
	}

	deps.env.SessionID = sess.Metadata.ID
	initTaskStorage(deps.env.SessionID)

	if deps.env.HookEngine != nil {
		deps.env.HookEngine.SetTranscriptPath(deps.env.SessionStore.SessionPath(sess.Metadata.ID))
	}
	if deps.reconfigureTool != nil {
		deps.reconfigureTool()
	}

	return nil
}

func loadSession(deps sessionLoadDeps, id string) error {
	if err := deps.env.EnsureSessionStore(deps.cwd); err != nil {
		return err
	}

	sess, err := deps.env.SessionStore.Load(id)
	if err != nil {
		return err
	}

	tracker.DefaultStore.SetStorageDir("")
	restoreSessionData(deps.env, sess, deps.restoreMessages)

	if len(sess.Tasks) == 0 {
		tracker.DefaultStore.Reset()
	}
	tool.ResetFetched()

	deps.env.InputTokens = 0
	deps.env.OutputTokens = 0

	return nil
}

func restoreSessionData(e *Env, sess *session.Snapshot, restoreMessages func([]core.ChatMessage)) {
	restoreMessages(session.ConvertFromEntries(sess.Entries))
	e.SessionID = sess.Metadata.ID

	if sess.Metadata.Summary != "" {
		e.SessionSummary = sess.Metadata.Summary
	} else if e.SessionStore != nil {
		if mem, err := e.SessionStore.LoadSessionMemory(sess.Metadata.ID); err == nil && mem != "" {
			e.SessionSummary = mem
		}
	}

	initTaskStorage(e.SessionID)

	if len(sess.Tasks) > 0 {
		tracker.DefaultStore.Import(sess.Tasks)
	}
}

func initTaskStorage(sessionID string) {
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
