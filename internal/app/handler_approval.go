package app

import (
	"encoding/json"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	appapproval "github.com/yanmxa/gencode/internal/app/approval"
	apptool "github.com/yanmxa/gencode/internal/app/tool"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/permission"
)

// hookPermissionResultMsg carries the result of an async PermissionRequest hook execution.
type hookPermissionResultMsg struct {
	Request *permission.PermissionRequest
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
		return m.dispatchPermissionHookAsync(msg.Request)
	}

	// No hook — show approval modal directly
	return m.showApprovalModal(msg.Request)
}

// dispatchPermissionHookAsync runs PermissionRequest hooks in a goroutine,
// keeping the TUI responsive while waiting for external hook responses (e.g. FIFO-based monitors).
func (m *model) dispatchPermissionHookAsync(req *permission.PermissionRequest) tea.Cmd {
	hookEngine := m.hookEngine
	ctx := m.tool.Context()

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
	if msg.Blocked {
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
		if m.settings != nil && m.settings.ResolveHookAllow(msg.Request.ToolName, args, m.mode.SessionPermissions) {
			// Hook allow is valid, skip permission prompt
			return apptool.ExecuteApproved(m.tool.Context(), m.output.ProgressHub, m.tool.PendingCalls, m.tool.CurrentIdx, m.cwd)
		}
		// Safety invariant denied the hook allow — fall through to normal approval modal
	}

	// Show approval modal
	return m.showApprovalModal(msg.Request)
}

// showApprovalModal generates suggestions, shows the approval UI, and fires notification.
func (m *model) showApprovalModal(req *permission.PermissionRequest) tea.Cmd {
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
	tc := m.tool.PendingCalls[m.tool.CurrentIdx]
	m.conv.Append(message.ChatMessage{
		Role:     message.RoleUser,
		ToolName: tc.Name,
		ToolResult: &message.ToolResult{
			ToolCallID: tc.ID,
			Content:    errorMsg,
			IsError:    true,
		},
	})
	m.tool.Reset()
	m.conv.Stream.Active = false
	commitCmds := m.commitMessages()
	if retry {
		commitCmds = append(commitCmds, m.startContinueStream())
	}
	return tea.Batch(commitCmds...)
}

// buildPermissionSuggestions generates permission suggestions for hook input,
// matching Claude Code's permission_suggestions field format.
func (m *model) buildPermissionSuggestions(req *permission.PermissionRequest) []hooks.PermissionSuggestion {
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
func (m *model) buildPermissionArgs(req *permission.PermissionRequest) map[string]any {
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
func (m *model) fullToolInputForHook(req *permission.PermissionRequest) map[string]any {
	if m.tool.PendingCalls != nil && m.tool.CurrentIdx < len(m.tool.PendingCalls) {
		tc := m.tool.PendingCalls[m.tool.CurrentIdx]
		if tc.Name == req.ToolName {
			if params, err := message.ParseToolInput(tc.Input); err == nil {
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
			if m.mode.SessionPermissions != nil {
				switch pu.Mode {
				case "bypassPermissions":
					m.mode.SessionPermissions.Mode = config.ModeBypassPermissions
				case "acceptEdits":
					m.mode.SessionPermissions.Mode = config.ModeAutoAccept
				case "dontAsk":
					m.mode.SessionPermissions.Mode = config.ModeDontAsk
				case "plan":
					m.mode.SessionPermissions.Mode = config.ModePlan
				case "normal":
					m.mode.SessionPermissions.Mode = config.ModeNormal
				}
			}

		case "addRules":
			for _, rule := range pu.Rules {
				ruleStr := buildRuleString(rule)
				if ruleStr == "" {
					continue
				}
				if pu.Destination == "persistent" {
					if err := config.AddAllowRuleDirectly(ruleStr); err != nil {
						log.Logger().Warn("failed to persist hook rule", zap.Error(err))
					}
					needReload = true
				} else if m.mode.SessionPermissions != nil {
					// Session-scoped (default)
					m.mode.SessionPermissions.AllowPattern(ruleStr)
				}
			}

		case "addDirectories":
			if m.mode.SessionPermissions != nil {
				for _, dir := range pu.Directories {
					m.mode.SessionPermissions.AddWorkingDirectory(dir)
				}
			}
		}
	}
	if needReload {
		_, _ = config.Reload()
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
	if !msg.Approved {
		retry := false
		if m.hookEngine != nil && msg.Request != nil {
			outcome := m.hookEngine.Execute(m.tool.Context(), hooks.PermissionDenied, hooks.HookInput{
				ToolName:  msg.Request.ToolName,
				ToolInput: m.buildPermissionArgs(msg.Request),
			})
			m.applyRuntimeHookOutcome(outcome)
			retry = outcome.Retry
		}
		return m.abortToolWithError("User denied permission", retry)
	}

	if msg.AllowAll && m.mode.SessionPermissions != nil && msg.Request != nil {
		m.applyAllowAllPermission(msg.Request.ToolName)
	}

	if msg.Persist && msg.Request != nil {
		m.persistAllowRule(msg.Request)
	}

	if msg.Request != nil && msg.Request.ToolName == tool.ToolAgent {
		m.output.TaskProgress = nil
		return tea.Batch(
			apptool.ExecuteApproved(m.tool.Context(), m.output.ProgressHub, m.tool.PendingCalls, m.tool.CurrentIdx, m.cwd),
			m.output.HandleProgressTick(true),
		)
	}

	return apptool.ExecuteApproved(m.tool.Context(), m.output.ProgressHub, m.tool.PendingCalls, m.tool.CurrentIdx, m.cwd)
}

func (m *model) applyAllowAllPermission(toolName string) {
	switch toolName {
	case "Edit":
		m.mode.SessionPermissions.AllowAllEdits = true
	case "Write":
		m.mode.SessionPermissions.AllowAllWrites = true
	case "Bash":
		m.mode.SessionPermissions.AllowAllBash = true
	case tool.ToolSkill:
		m.mode.SessionPermissions.AllowAllSkills = true
	case tool.ToolAgent:
		m.mode.SessionPermissions.AllowAllTasks = true
	default:
		m.mode.SessionPermissions.AllowTool(toolName)
	}
}

// persistAllowRule writes a permission allow rule to project settings.
// Uses smart suggested rules (prefix-based) when available instead of exact rules.
func (m *model) persistAllowRule(req *permission.PermissionRequest) {
	if len(req.SuggestedRules) > 0 {
		// Use the best suggestion (prefix-based, reusable)
		if err := config.AddAllowRuleDirectly(req.SuggestedRules[0]); err != nil {
			log.Logger().Warn("failed to persist allow rule", zap.Error(err))
		}
	} else {
		// Fallback to exact rule
		if err := config.AddAllowRule(req.ToolName, m.buildPermissionArgs(req)); err != nil {
			log.Logger().Warn("failed to persist allow rule", zap.Error(err))
		}
	}
	// Reload settings so the rule takes effect immediately
	_, _ = config.Reload()
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
	m.approval.TogglePreview()
}
