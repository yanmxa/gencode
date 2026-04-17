package app

import (
	"encoding/json"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/app/conv"
	"github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

// hookPermissionResultMsg carries the result of an async PermissionRequest hook execution.
type hookPermissionResultMsg struct {
	Request *perm.PermissionRequest
	Blocked bool
	Allowed bool
	Reason  string
	Outcome hook.HookOutcome // full outcome for applying permission updates
}

// updateApproval routes permission request messages.
// Note: response messages are handled directly in delegateToActiveModal.
func (m *model) updateApproval(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case input.ApprovalRequestMsg:
		c := m.handlePermissionRequest(msg)
		return c, true
	case hookPermissionResultMsg:
		c := m.handleHookPermissionResult(msg)
		return c, true
	}
	return nil, false
}

func (m *model) handlePermissionRequest(msg input.ApprovalRequestMsg) tea.Cmd {
	// If there's a PermissionRequest hook configured, run it asynchronously
	// to avoid blocking the Bubble Tea event loop (which freezes the TUI).
	if m.runtime.HookEngine != nil && m.runtime.HookEngine.HasHooks(hook.PermissionRequest) && msg.Request != nil {
		return tea.Batch(
			m.showApprovalModal(msg.Request),
			m.dispatchPermissionHookAsync(msg.Request),
		)
	}

	// No hook — show approval modal directly
	return m.showApprovalModal(msg.Request)
}

// dispatchPermissionHookAsync runs PermissionRequest hooks in a goroutine,
// keeping the TUI responsive while waiting for external hook responses (e.g. FIFO-based monitors).
func (m *model) dispatchPermissionHookAsync(req *perm.PermissionRequest) tea.Cmd {
	hookEngine := m.runtime.HookEngine
	ctx := m.tool.Context()

	hookInput := hook.HookInput{
		ToolName:  req.ToolName,
		ToolInput: m.fullToolInputForHook(req),
	}
	hookInput.PermissionSuggestions = m.buildPermissionSuggestions(req)

	return func() tea.Msg {
		outcome := hookEngine.Execute(ctx, hook.PermissionRequest, hookInput)

		blocked := outcome.ShouldBlock
		allowed := outcome.PermissionAllow

		return hookPermissionResultMsg{
			Request: req,
			Blocked: blocked,
			Allowed: allowed,
			Reason:  outcome.BlockReason,
			Outcome: outcome,
		}
	}
}

// handleHookPermissionResult processes the async hook result and decides
// whether to auto-approve or show the approval modal.
func (m *model) handleHookPermissionResult(msg hookPermissionResultMsg) tea.Cmd {
	if !m.isCurrentPermissionRequest(msg.Request) {
		return nil
	}

	if msg.Blocked {
		m.userInput.Approval.Hide()
		return m.abortToolWithError("Blocked by hook: "+msg.Reason, false)
	}

	if msg.Allowed {
		// Apply structured permission updates from hook
		m.applyPermissionUpdates(msg.Outcome.UpdatedPermissions)
		// Propagate updated input back to the pending tool call
		if msg.Outcome.UpdatedInput != nil {
			m.applyUpdatedToolInput(msg.Outcome.UpdatedInput)
		}

		// Hook wants to allow — validate against safety invariant
		args := m.buildPermissionArgs(msg.Request)
		if m.runtime.Settings != nil && m.runtime.Settings.ResolveHookAllow(msg.Request.ToolName, args, m.runtime.SessionPermissions) {
			// Hook allow is valid, skip permission prompt
			m.userInput.Approval.Hide()
			return conv.ExecuteApproved(m.tool.Context(), m.agentOutput.ProgressHub, m.tool.PendingCalls, m.tool.CurrentIdx, m.cwd)
		}
		// Safety invariant denied the hook allow — fall through to normal approval modal
	}

	// Show approval modal
	return m.showApprovalModal(msg.Request)
}

func (m *model) isCurrentPermissionRequest(req *perm.PermissionRequest) bool {
	if req == nil || !m.userInput.Approval.IsActive() {
		return false
	}
	current := m.userInput.Approval.GetRequest()
	if current == nil {
		return false
	}
	return current.ID != "" && current.ID == req.ID
}

// showApprovalModal generates suggestions, shows the approval UI, and fires notification.
func (m *model) showApprovalModal(req *perm.PermissionRequest) tea.Cmd {
	// Generate smart allow rule suggestions for the approval UI
	if req != nil {
		req.SuggestedRules = setting.GenerateSuggestions(req.ToolName, m.buildPermissionArgs(req), 5)
	}

	m.userInput.Approval.Show(req, m.width, m.height)

	// Fire Notification hook when permission prompt is shown
	if m.runtime.HookEngine != nil {
		m.runtime.HookEngine.ExecuteAsync(hook.Notification, hook.HookInput{
			Message:          "Permission required for " + req.ToolName,
			NotificationType: "permission_prompt",
		})
	}

	return nil
}

func (m *model) abortToolWithError(errorMsg string, retry bool) tea.Cmd {
	if m.tool.PendingCalls == nil || m.tool.CurrentIdx >= len(m.tool.PendingCalls) {
		m.tool.Reset()
		m.conv.Stream.Stop()
		return tea.Batch(m.commitMessages()...)
	}
	tc := m.tool.PendingCalls[m.tool.CurrentIdx]
	m.conv.Append(core.ChatMessage{
		Role:     core.RoleUser,
		ToolName: tc.Name,
		ToolResult: &core.ToolResult{
			ToolCallID: tc.ID,
			Content:    errorMsg,
			IsError:    true,
		},
	})
	m.cancelRemainingToolCalls(m.tool.CurrentIdx + 1)
	m.tool.Reset()
	m.conv.Stream.Stop()
	commitCmds := m.commitMessages()
	if retry {
		commitCmds = append(commitCmds, m.continueOutbox())
	}
	return tea.Batch(commitCmds...)
}

// buildPermissionSuggestions generates permission suggestions for hook input,
// matching Claude Code's permission_suggestions field format.
func (m *model) buildPermissionSuggestions(req *perm.PermissionRequest) []hook.PermissionSuggestion {
	var suggestions []hook.PermissionSuggestion

	// Suggest addDirectories if the file is in a recognizable directory
	if req.FilePath != "" {
		dir := req.FilePath
		if i := strings.LastIndex(dir, "/"); i > 0 {
			dir = dir[:i]
		}
		suggestions = append(suggestions, hook.PermissionSuggestion{
			Type:        "addDirectories",
			Directories: []string{dir},
			Destination: "session",
		})
	}

	// Suggest acceptEdits mode for write-type tools
	if req.ToolName == "Edit" || req.ToolName == "Write" {
		suggestions = append(suggestions, hook.PermissionSuggestion{
			Type:        "setMode",
			Mode:        "acceptEdits",
			Destination: "session",
		})
	}

	return suggestions
}

// buildPermissionArgs constructs a tool args map from a permission request.
func (m *model) buildPermissionArgs(req *perm.PermissionRequest) map[string]any {
	args := make(map[string]any)
	if req == nil {
		return args
	}
	if req.FilePath != "" {
		args["file_path"] = req.FilePath
	}
	if req.BashMeta != nil {
		args["command"] = req.BashMeta.Command
	}
	if req.SkillMeta != nil {
		args["skill"] = req.SkillMeta.SkillName
	}
	return args
}

// fullToolInputForHook returns the full tool input params from the pending tool call
// for use in hook events (matching CC's behavior of sending all params).
// Falls back to buildPermissionArgs if the pending call can't be parsed.
func (m *model) fullToolInputForHook(req *perm.PermissionRequest) map[string]any {
	if m.tool.PendingCalls != nil && m.tool.CurrentIdx < len(m.tool.PendingCalls) {
		tc := m.tool.PendingCalls[m.tool.CurrentIdx]
		if tc.Name == req.ToolName {
			if params, err := core.ParseToolInput(tc.Input); err == nil {
				return params
			}
		}
	}
	return m.buildPermissionArgs(req)
}

// applyPermissionUpdates processes structured permission updates from hook responses.
// Supports setMode, addRules, and addDirectories with session or persistent destination.
func (m *model) applyPermissionUpdates(updates []hook.PermissionUpdate) {
	needReload := false
	for _, pu := range updates {
		switch pu.Type {
		case "setMode":
			if m.runtime.SessionPermissions != nil {
				switch pu.Mode {
				case "bypassPermissions":
					// Hooks cannot escalate to bypassPermissions — ignore
					log.Logger().Warn("hook attempted to set bypassPermissions mode, denied")
				case "acceptEdits":
					m.runtime.SessionPermissions.Mode = setting.ModeAutoAccept
					m.runtime.OperationMode = setting.ModeAutoAccept
				case "dontAsk":
					m.runtime.SessionPermissions.Mode = setting.ModeDontAsk
				case "plan":
					m.runtime.SessionPermissions.Mode = setting.ModePlan
					m.runtime.OperationMode = setting.ModePlan
				case "normal":
					m.runtime.SessionPermissions.Mode = setting.ModeNormal
					m.runtime.OperationMode = setting.ModeNormal
				}
			}

		case "addRules":
			for _, rule := range pu.Rules {
				ruleStr := buildRuleString(rule)
				if ruleStr == "" {
					continue
				}
				// Block catch-all patterns in persistent rules to prevent privilege escalation
				if strings.Contains(ruleStr, "(**)") {
					if pu.Destination == "persistent" {
						log.Logger().Warn("hook attempted to persist catch-all rule, denied", zap.String("rule", ruleStr))
					} else {
						log.Logger().Warn("hook attempted session-scoped catch-all rule, denied", zap.String("rule", ruleStr))
					}
					continue
				}
				if pu.Destination == "persistent" {
					if err := setting.AddAllowRuleDirectlyAt(ruleStr, m.cwd); err != nil {
						log.Logger().Warn("failed to persist hook rule", zap.Error(err))
					}
					needReload = true
				} else if m.runtime.SessionPermissions != nil {
					// Session-scoped (default)
					m.runtime.SessionPermissions.AllowPattern(ruleStr)
				}
			}

		case "addDirectories":
			if m.runtime.SessionPermissions != nil {
				for _, dir := range pu.Directories {
					m.runtime.SessionPermissions.AddWorkingDirectory(dir)
				}
			}
		}
	}
	if needReload {
		m.reloadProjectContext(m.cwd)
	}
}

// buildRuleString constructs a permission rule string from a PermissionRule.
// E.g. {ToolName: "Bash", RuleContent: "git"} → "Bash(git:*)"
func buildRuleString(rule hook.PermissionRule) string {
	if rule.RuleContent != "" && rule.ToolName != "" {
		return rule.ToolName + "(" + rule.RuleContent + ":*)"
	}
	if rule.ToolName != "" {
		return rule.ToolName + "(**)"
	}
	if rule.RuleContent != "" {
		// Legacy: ruleContent is already a full rule string
		return rule.RuleContent
	}
	return ""
}

func (m *model) handlePermissionResponse(msg input.ApprovalResponseMsg) tea.Cmd {
	return m.handlePermBridgeDecision(permissionDecision{
		Approved: msg.Approved,
		AllowAll: msg.AllowAll,
		Request:  msg.Request,
	})
}


// applyUpdatedToolInput marshals the hook-provided input and updates the current
// pending tool call so the executor uses the modified arguments.
func (m *model) applyUpdatedToolInput(updated map[string]any) {
	if m.tool.PendingCalls == nil || m.tool.CurrentIdx >= len(m.tool.PendingCalls) {
		return
	}
	data, err := json.Marshal(updated)
	if err != nil {
		return
	}
	m.tool.PendingCalls[m.tool.CurrentIdx].Input = string(data)
}

// togglePermissionPreview toggles the expand state of permission prompt previews.
func (m *model) togglePermissionPreview() {
	m.userInput.Approval.TogglePreview()
}
