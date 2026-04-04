package app

import (
	"context"
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

// updateApproval routes permission request messages.
// Note: response messages are handled directly in delegateToActiveModal.
func (m *model) updateApproval(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case appapproval.RequestMsg:
		c := m.handlePermissionRequest(msg)
		return c, true
	}
	return nil, false
}

func (m *model) handlePermissionRequest(msg appapproval.RequestMsg) tea.Cmd {
	blocked, allowed, reason := m.checkPermissionHook(msg.Request)

	if blocked {
		return m.abortToolWithError("Blocked by hook: " + reason)
	}

	if allowed {
		// Hook wants to allow — validate against safety invariant
		args := m.buildPermissionArgs(msg.Request)
		if m.settings != nil && m.settings.ResolveHookAllow(msg.Request.ToolName, args, m.mode.SessionPermissions) {
			// Hook allow is valid, skip permission prompt
			return apptool.ExecuteApproved(m.tool.Ctx, m.output.ProgressHub, m.tool.PendingCalls, m.tool.CurrentIdx, m.cwd)
		}
		// Safety invariant denied the hook allow — fall through to normal approval modal
	}

	// Generate smart allow rule suggestions for the approval UI
	if msg.Request != nil {
		msg.Request.SuggestedRules = config.GenerateSuggestions(msg.Request.ToolName, m.buildPermissionArgs(msg.Request), 5)
	}

	m.approval.Show(msg.Request, m.width, m.height)

	// Fire Notification hook when permission prompt is shown
	if m.hookEngine != nil {
		m.hookEngine.ExecuteAsync(hooks.Notification, hooks.HookInput{
			Message:          "Permission required for " + msg.Request.ToolName,
			NotificationType: "permission_prompt",
		})
	}

	return nil
}

func (m *model) abortToolWithError(errorMsg string) tea.Cmd {
	if m.tool.PendingCalls != nil && m.tool.CurrentIdx < len(m.tool.PendingCalls) {
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
	}
	m.tool.Reset()
	m.conv.Stream.Active = false
	return tea.Batch(m.commitMessages()...)
}

// checkPermissionHook runs PermissionRequest hooks and returns:
//   - blocked: hook explicitly denied the tool
//   - allowed: hook explicitly approved the tool (still needs safety invariant check)
//   - reason: block reason or hook source
func (m *model) checkPermissionHook(req *permission.PermissionRequest) (blocked, allowed bool, reason string) {
	if m.hookEngine == nil || req == nil {
		return false, false, ""
	}

	hookInput := hooks.HookInput{
		ToolName:  req.ToolName,
		ToolInput: m.fullToolInputForHook(req),
	}
	hookInput.PermissionSuggestions = m.buildPermissionSuggestions(req)

	outcome := m.hookEngine.Execute(context.Background(), hooks.PermissionRequest, hookInput)

	if outcome.ShouldBlock {
		return true, false, outcome.BlockReason
	}

	if outcome.PermissionAllow {
		// Apply structured permission updates from hook
		m.applyPermissionUpdates(outcome.UpdatedPermissions)
		// Propagate updated input back to the pending tool call
		if outcome.UpdatedInput != nil {
			m.applyUpdatedToolInput(outcome.UpdatedInput)
		}
		return false, true, outcome.HookSource
	}

	return false, false, ""
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
		config.Reload()
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
		if m.hookEngine != nil && msg.Request != nil {
			m.hookEngine.ExecuteAsync(hooks.PermissionDenied, hooks.HookInput{
				ToolName:  msg.Request.ToolName,
				ToolInput: m.buildPermissionArgs(msg.Request),
			})
		}
		return m.abortToolWithError("User denied permission")
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
			apptool.ExecuteApproved(m.tool.Ctx, m.output.ProgressHub, m.tool.PendingCalls, m.tool.CurrentIdx, m.cwd),
			m.output.HandleProgressTick(true),
		)
	}

	return apptool.ExecuteApproved(m.tool.Ctx, m.output.ProgressHub, m.tool.PendingCalls, m.tool.CurrentIdx, m.cwd)
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
	config.Reload()
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
