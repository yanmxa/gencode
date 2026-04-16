package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	appcommand "github.com/yanmxa/gencode/internal/ext/command"
	"github.com/yanmxa/gencode/internal/core"
)

// commandController owns slash-command execution and transcript insertion rules.
type commandController struct {
	model *model
}

func (m *model) commands() commandController {
	return commandController{model: m}
}

func (c commandController) execute(ctx context.Context, input string) (string, tea.Cmd, bool) {
	cmd, args, isCmd := appcommand.ParseCommand(input)
	if !isCmd {
		return "", nil, false
	}

	if result, followUp, handled := executeExitCommand(c.model, cmd); handled {
		return result, followUp, true
	}

	if result, followUp, handled := executeBuiltinCommand(ctx, c.model, cmd, args); handled {
		return result, followUp, true
	}

	if sk, ok := lookupSkillCommand(cmd); ok {
		return executeSkillSlashCommand(c.model, sk, args), c.model.handleSkillInvocation(), true
	}

	if pc, ok := appcommand.IsCustomCommand(cmd); ok {
		return executeCustomCommand(c.model, pc, args), c.model.handleSkillInvocation(), true
	}

	return unknownCommandResult(cmd), nil, true
}

func (c commandController) handleSubmit(input string) (tea.Cmd, bool) {
	preserve := shouldPreserveCommandInConversation(input, "", nil)
	preAppended := false
	if preserve && shouldPreserveBeforeCommandExecution(input) {
		c.model.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: input})
		preAppended = true
	}

	insertAt := len(c.model.conv.Messages)
	result, cmd, isCmd := c.execute(context.Background(), input)
	if !isCmd {
		if preAppended && len(c.model.conv.Messages) > 0 {
			c.model.conv.Messages = c.model.conv.Messages[:len(c.model.conv.Messages)-1]
		}
		return nil, false
	}

	c.model.resetInputField()

	// Slash commands should remain visible in the conversation so the transcript
	// reflects the user's literal input and arguments. Skill commands are the
	// exception here because handleSkillInvocation appends the full slash
	// invocation itself before starting the provider turn.
	if preserve && !preAppended {
		c.insertConversationMessage(insertAt, core.ChatMessage{Role: core.RoleUser, Content: input})
	}
	if result != "" {
		c.model.conv.AddNotice(result)
	}

	cmds := c.model.commitMessages()
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...), true
}

func (c commandController) insertConversationMessage(idx int, msg core.ChatMessage) {
	if idx < 0 || idx >= len(c.model.conv.Messages) {
		c.model.conv.Append(msg)
		return
	}

	c.model.conv.Messages = append(c.model.conv.Messages, core.ChatMessage{})
	copy(c.model.conv.Messages[idx+1:], c.model.conv.Messages[idx:])
	c.model.conv.Messages[idx] = msg
	if idx < c.model.conv.CommittedCount {
		c.model.conv.CommittedCount++
	}
}
