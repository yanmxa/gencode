package input

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/conv"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/plugin"
)

type SubmitRequest struct {
	Input string
}

// SubmitRuntime provides app-level operations that the submit flow needs.
type SubmitRuntime interface {
	CommitMessages() []tea.Cmd
	QuitWithCancel() (tea.Cmd, bool)
	StartProviderTurn(content string) tea.Cmd
}

type SubmitDeps struct {
	Actions        SubmitRuntime
	Input          *Model
	Conversation   *conv.ConversationModel
	CheckPromptHook func(context.Context, string) (bool, string)
	Cwd            string
	HandleCommand  func(string) (tea.Cmd, bool)
}

func HandleSubmit(deps SubmitDeps) tea.Cmd {
	deps.Input.PromptSuggestion.Clear()

	input := strings.TrimSpace(deps.Input.FullValue())
	if input == "" && len(deps.Input.Images.Pending) == 0 {
		return nil
	}

	if deps.Conversation.Stream.Active {
		return enqueueCurrentInput(deps, input)
	}

	deps.Conversation.Compact.ClearResult()
	return ExecuteSubmitRequest(deps, SubmitRequest{Input: input})
}

func enqueueCurrentInput(deps SubmitDeps, input string) tea.Cmd {
	if deps.Input.Queue.Enqueue(input, deps.Input.PendingImages()) < 0 {
		deps.Conversation.AddNotice("Input queue is full. Please wait for the current turn to complete.")
		return nil
	}
	deps.Input.Reset()
	return nil
}

func DrainInputQueue(deps SubmitDeps) tea.Cmd {
	item, ok := deps.Input.Queue.Dequeue()
	if !ok {
		return nil
	}

	deps.Conversation.Compact.ClearResult()
	deps.Input.RestoreImages(item.Images)
	return ExecuteSubmitRequest(deps, SubmitRequest{Input: item.Content})
}

func ExecuteSubmitRequest(deps SubmitDeps, req SubmitRequest) tea.Cmd {
	if isExitRequest(req.Input) {
		cmd, _ := deps.Actions.QuitWithCancel()
		return cmd
	}

	if blocked, reason := deps.CheckPromptHook(context.Background(), req.Input); blocked {
		return BlockPromptSubmission(deps, reason)
	}

	deps.Input.RecordSubmission(deps.Cwd, req.Input)

	if cmd, handled := deps.HandleCommand(req.Input); handled {
		return cmd
	}

	deps.Input.Skill.ActiveInvocation = ""
	plugin.ClearActivePluginRoot()

	userMsg, cmd, handled := PrepareSubmittedUserMessage(deps, req.Input)
	if handled {
		return cmd
	}
	deps.Conversation.Append(userMsg)
	deps.Input.Reset()
	return deps.Actions.StartProviderTurn(userMsg.Content)
}

func BlockPromptSubmission(deps SubmitDeps, reason string) tea.Cmd {
	deps.Conversation.Append(core.ChatMessage{Role: core.RoleNotice, Content: "Prompt blocked: " + reason})
	deps.Input.Reset()
	return tea.Batch(deps.Actions.CommitMessages()...)
}

func PrepareSubmittedUserMessage(deps SubmitDeps, rawInput string) (core.ChatMessage, tea.Cmd, bool) {
	content, fileImages, err := ProcessImageRefs(deps.Cwd, rawInput)
	if err != nil {
		deps.Conversation.Append(core.ChatMessage{Role: core.RoleNotice, Content: "Image error: " + err.Error()})
		return core.ChatMessage{}, tea.Batch(deps.Actions.CommitMessages()...), true
	}

	displayContent := content
	content, inlineImages := deps.Input.ExtractInlineImages(content)
	allImages := make([]core.Image, 0, len(inlineImages)+len(fileImages))
	allImages = append(allImages, inlineImages...)
	allImages = append(allImages, fileImages...)

	return core.ChatMessage{
		Role:           core.RoleUser,
		Content:        content,
		DisplayContent: displayContent,
		Images:         allImages,
	}, nil, false
}

func isExitRequest(input string) bool {
	return strings.EqualFold(input, "exit")
}
