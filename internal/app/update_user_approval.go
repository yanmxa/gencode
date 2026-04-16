package app

import (
	"encoding/json"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	appapproval "github.com/yanmxa/gencode/internal/app/user/approval"
	"github.com/yanmxa/gencode/internal/app/output/toolexec"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/util/log"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

// hookPermissionResultMsg carries the result of an async PermissionRequest hook execution.
type hookPermissionResultMsg struct {
	Request *perm.PermissionRequest
	Blocked bool
	Allowed bool
	Reason  string
	Outcome hooks.HookOutcome // full outcome for applying permission updates
}

// updateApproval routes permission request messages.
// Note: response messages are handled directly in delegateToActiveModal.
func (m *model) updateApproval(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case appapproval.RequestMsg:
		c := m.handlePermissionRequest(msg)
		return c, true
	case hookPermissionResultMsg:
		c := m.handleHookPermissionResult(msg)
		return c, true
	}
	return nil, false
}

func (m *model) handlePermissionRequest(msg appapproval.RequestMsg) tea.Cmd {
	// If there's a PermissionRequest hook configured, run it asynchronously
	// to avoid blocking the Bubble Tea event loop (which freezes the TUI).
	if m.hookEngine != nil && m.hookEngine.HasHooks(hooks.PermissionRequest) && msg.Request != nil {
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
	hookEngine := m.hookEngine
	ctx := m.toolExec.Context()

	hookInput := hooks.HookInput{
		ToolName:  req.ToolName,
		ToolInput: m.fullToolInputForHook(req),
	}
	hookInput.PermissionSuggestions = m.buildPermissionSuggestions(req)

	return func() tea.Msg {
		outcome := hookEngine.Execute(ctx, hooks.PermissionRequest, hookInput)

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
		m.approval.Hide()
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
		if m.settings != nil && m.settings.ResolveHookAllow(msg.Request.ToolName, args, m.sessionPermissions) {
			// Hook allow is valid, skip permission prompt
			m.approval.Hide()
			return toolexec.ExecuteApproved(m.toolExec.Context(), m.agentOutput.ProgressHub, m.toolExec.PendingCalls, m.toolExec.CurrentIdx, m.cwd)
		}
		// Safety invariant denied the hook allow — fall through to normal approval modal
	}

	// Show approval modal
	return m.showApprovalModal(msg.Request)
}

func (m *model) isCurrentPermissionRequest(req *perm.PermissionRequest) bool {
	if req == nil || !m.approval.IsActive() {
		return false
	}
	current := m.approval.GetRequest()
	if current == nil {
		return false
	}
	return current.ID != "" && current.ID == req.ID
}

// showApprovalModal generates suggestions, shows the approval UI, and fires notification.
func (m *model) showApprovalModal(req *perm.PermissionRequest) tea.Cmd {
	// Generate smart allow rule suggestions for the approval UI
	if req != nil {
		req.SuggestedRules = config.GenerateSuggestions(req.ToolName, m.buildPermissionArgs(req), 5)
	}

	m.approval.Show(req, m.width, m.height)

	// Fire Notification hook when permission prompt is shown
	if m.hookEngine != nil {
		m.hookEngine.ExecuteAsync(hooks.Notification, hooks.HookInput{
			Message:          "Permission required for " + req.ToolName,
			NotificationType: "permission_prompt",
		})
	}

	return nil
}

func (m *model) abortToolWithError(errorMsg string, retry bool) tea.Cmd {
	if m.toolExec.PendingCalls == nil || m.toolExec.CurrentIdx >= len(m.toolExec.PendingCalls) {
		m.toolExec.Reset()
		m.conv.Stream.Stop()
		return tea.Batch(m.commitMessages()...)
	}
	tc := m.toolExec.PendingCalls[m.toolExec.CurrentIdx]
	m.conv.Append(core.ChatMessage{
		Role:     core.RoleUser,
		ToolName: tc.Name,
		ToolResult: &core.ToolResult{
			ToolCallID: tc.ID,
			Content:    errorMsg,
			IsError:    true,
		},
	})
	m.cancelRemainingToolCalls(m.toolExec.CurrentIdx + 1)
	m.toolExec.Reset()
	m.conv.Stream.Stop()
	commitCmds := m.commitMessages()
	if retry {
		commitCmds = append(commitCmds, m.continueOutbox())
	}
	return tea.Batch(commitCmds...)
}

// buildPermissionSuggestions generates permission suggestions for hook input,
// matching Claude Code's permission_suggestions field format.
func (m *model) buildPermissionSuggestions(req *perm.PermissionRequest) []hooks.PermissionSuggestion {
	var suggestions []hooks.PermissionSuggestion

	// Suggest addDirectories if the file is in a recognizable directory
	if req.FilePath != "" {
		dir := req.FilePath
		if i := strings.LastIndex(dir, "/"); i > 0 {
			dir = dir[:i]
		}
		suggestions = append(suggestions, hooks.PermissionSuggestion{
			Type:        "addDirectories",
			Directories: []string{dir},
			Destination: "session",
		})
	}

	// Suggest acceptEdits mode for write-type tools
	if req.ToolName == "Edit" || req.ToolName == "Write" {
		suggestions = append(suggestions, hooks.PermissionSuggestion{
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
	if m.toolExec.PendingCalls != nil && m.toolExec.CurrentIdx < len(m.toolExec.PendingCalls) {
		tc := m.toolExec.PendingCalls[m.toolExec.CurrentIdx]
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
func (m *model) applyPermissionUpdates(updates []hooks.PermissionUpdate) {
	needReload := false
	for _, pu := range updates {
		switch pu.Type {
		case "setMode":
			if m.sessionPermissions != nil {
				switch pu.Mode {
				case "bypassPermissions":
					// Hooks cannot escalate to bypassPermissions — ignore
					log.Logger().Warn("hook attempted to set bypassPermissions mode, denied")
				case "acceptEdits":
					m.sessionPermissions.Mode = config.ModeAutoAccept
					m.operationMode = config.ModeAutoAccept
				case "dontAsk":
					m.sessionPermissions.Mode = config.ModeDontAsk
				case "plan":
					m.sessionPermissions.Mode = config.ModePlan
					m.operationMode = config.ModePlan
				case "normal":
					m.sessionPermissions.Mode = config.ModeNormal
					m.operationMode = config.ModeNormal
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
					if err := config.AddAllowRuleDirectlyAt(ruleStr, m.cwd); err != nil {
						log.Logger().Warn("failed to persist hook rule", zap.Error(err))
					}
					needReload = true
				} else if m.sessionPermissions != nil {
					// Session-scoped (default)
					m.sessionPermissions.AllowPattern(ruleStr)
				}
			}

		case "addDirectories":
			if m.sessionPermissions != nil {
				for _, dir := range pu.Directories {
					m.sessionPermissions.AddWorkingDirectory(dir)
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
func buildRuleString(rule hooks.PermissionRule) string {
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

func (m *model) handlePermissionResponse(msg appapproval.ResponseMsg) tea.Cmd {
	return m.handlePermBridgeResponse(msg)
}


// applyUpdatedToolInput marshals the hook-provided input and updates the current
// pending tool call so the executor uses the modified arguments.
func (m *model) applyUpdatedToolInput(updated map[string]any) {
	if m.toolExec.PendingCalls == nil || m.toolExec.CurrentIdx >= len(m.toolExec.PendingCalls) {
		return
	}
	data, err := json.Marshal(updated)
	if err != nil {
		return
	}
	m.toolExec.PendingCalls[m.toolExec.CurrentIdx].Input = string(data)
}

// togglePermissionPreview toggles the expand state of permission prompt previews.
func (m *model) togglePermissionPreview() {
	m.approval.TogglePreview()
}
