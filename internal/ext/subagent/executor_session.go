package subagent

import (
	"fmt"

	"github.com/yanmxa/gencode/internal/util/log"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/loop"
	"go.uber.org/zap"
)

// persistSubagentSession saves the subagent conversation to disk if a session store is configured.
// Returns the session ID and transcript path (both empty if not persisted).
func (e *Executor) persistSubagentSession(agentName, modelID, description string, messages []core.Message) (string, string) {
	if e.sessionStore == nil || e.parentSessionID == "" {
		return "", ""
	}

	title := description
	if title == "" {
		title = agentName
	}
	sessionID, transcriptPath, err := e.sessionStore.SaveSubagentConversation(e.parentSessionID, title, modelID, e.cwd, messages)
	if err != nil {
		log.Logger().Warn("Failed to persist subagent session",
			zap.String("agent", agentName),
			zap.Error(err),
		)
		return "", ""
	}
	return sessionID, transcriptPath
}

// resumeFromSession loads a previous subagent session and restores its conversation,
// then appends the new prompt as a continuation.
func (e *Executor) resumeFromSession(lp *loop.Loop, agentID, newPrompt string) error {
	if e.sessionStore == nil {
		return fmt.Errorf("session store not configured, cannot resume")
	}

	prevMessages, err := e.sessionStore.LoadSubagentMessages(agentID)
	if err != nil {
		return fmt.Errorf("load subagent messages for %s: %w", agentID, err)
	}

	// Restore previous conversation and append continuation prompt
	lp.SetMessages(prevMessages)
	lp.AddUser(newPrompt, nil)

	log.Logger().Info("Resumed agent from previous session",
		zap.String("agentID", agentID),
		zap.Int("previousMessages", len(prevMessages)),
	)
	return nil
}
