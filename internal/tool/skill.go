package tool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/tool/permission"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

const (
	IconSkill = "⚡"
)

// SkillTool allows the LLM to invoke skills programmatically
// It implements PermissionAwareTool to require user confirmation before loading skills
type SkillTool struct{}

func (t *SkillTool) Name() string { return "Skill" }

func (t *SkillTool) Description() string {
	return "Execute a skill within the main conversation"
}

func (t *SkillTool) Icon() string { return IconSkill }

// RequiresPermission returns true - skills require user confirmation
func (t *SkillTool) RequiresPermission() bool {
	return true
}

// PreparePermission prepares a permission request with skill metadata
func (t *SkillTool) PreparePermission(ctx context.Context, params map[string]any, cwd string) (*permission.PermissionRequest, error) {
	// Get skill name
	skillName, ok := params["skill"].(string)
	if !ok || skillName == "" {
		return nil, fmt.Errorf("skill parameter is required")
	}

	// Get optional args
	args, _ := params["args"].(string)

	// Find skill in registry
	if skill.DefaultRegistry == nil {
		return nil, fmt.Errorf("skill registry not initialized")
	}

	sk, ok := skill.DefaultRegistry.Get(skillName)
	if !ok {
		// Try to find by partial match
		sk = skill.DefaultRegistry.FindByPartialName(skillName)
		if sk == nil {
			return nil, fmt.Errorf("skill not found: %s", skillName)
		}
	}

	// Check if skill is enabled
	if !sk.IsEnabled() {
		return nil, fmt.Errorf("skill is disabled: %s", sk.FullName())
	}

	// Build description
	desc := fmt.Sprintf("Load skill: %s", sk.FullName())
	if args != "" {
		desc = fmt.Sprintf("Load skill: %s with args: %s", sk.FullName(), args)
	}

	return &permission.PermissionRequest{
		ToolName:    t.Name(),
		Description: desc,
		SkillMeta: &permission.SkillMetadata{
			SkillName:   sk.FullName(),
			Description: sk.Description,
			Args:        args,
			ScriptCount: len(sk.Scripts),
			RefCount:    len(sk.References),
			Scripts:     sk.Scripts,
			References:  sk.References,
		},
	}, nil
}

// ExecuteApproved executes the skill after user approval
func (t *SkillTool) ExecuteApproved(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	return t.execute(ctx, params, cwd)
}

// Execute runs the tool (for direct execution when permission is pre-approved)
func (t *SkillTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	return t.execute(ctx, params, cwd)
}

// execute is the internal implementation
func (t *SkillTool) execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	start := time.Now()

	// Get skill name
	skillName, ok := params["skill"].(string)
	if !ok || skillName == "" {
		return ui.NewErrorResult(t.Name(), "skill parameter is required")
	}

	// Get optional args
	args, _ := params["args"].(string)

	// Find skill in registry
	if skill.DefaultRegistry == nil {
		return ui.NewErrorResult(t.Name(), "skill registry not initialized")
	}

	sk, ok := skill.DefaultRegistry.Get(skillName)
	if !ok {
		// Try to find by partial match
		sk = skill.DefaultRegistry.FindByPartialName(skillName)
		if sk == nil {
			return ui.NewErrorResult(t.Name(), fmt.Sprintf("skill not found: %s", skillName))
		}
	}

	// Check if skill is enabled
	if !sk.IsEnabled() {
		return ui.NewErrorResult(t.Name(), fmt.Sprintf("skill is disabled: %s", sk.FullName()))
	}

	// Load full instructions
	instructions := sk.GetInstructions()
	if instructions == "" {
		return ui.NewErrorResult(t.Name(), fmt.Sprintf("skill has no instructions: %s", sk.FullName()))
	}

	// Build skill invocation context
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<skill-invocation name=\"%s\">\n", sk.FullName()))

	// Include arguments if provided
	if args != "" {
		sb.WriteString(fmt.Sprintf("User arguments: %s\n\n", args))
	}

	// Include skill directory info for scripts/references/assets
	if sk.SkillDir != "" {
		if len(sk.Scripts) > 0 {
			sb.WriteString("Available scripts (use Bash to execute):\n")
			for _, script := range sk.Scripts {
				sb.WriteString(fmt.Sprintf("  - %s/scripts/%s\n", sk.SkillDir, script))
			}
			sb.WriteString("\n")
		}
		if len(sk.References) > 0 {
			sb.WriteString("Reference files (use Read when needed):\n")
			for _, ref := range sk.References {
				sb.WriteString(fmt.Sprintf("  - %s/references/%s\n", sk.SkillDir, ref))
			}
			sb.WriteString("\n")
		}
	}

	// Add instructions
	sb.WriteString(instructions)
	sb.WriteString("\n</skill-invocation>")

	duration := time.Since(start)

	// Count resources for display
	scriptCount := len(sk.Scripts)
	refCount := len(sk.References)

	return ui.ToolResult{
		Success: true,
		Output:  sb.String(),
		Metadata: ui.ResultMetadata{
			Title:     t.Name(),
			Icon:      t.Icon(),
			Subtitle:  sk.FullName(),
			Duration:  duration,
			ItemCount: scriptCount + refCount,
		},
		// Store skill-specific info for custom rendering
		SkillInfo: &ui.SkillResultInfo{
			SkillName:   sk.FullName(),
			ScriptCount: scriptCount,
			RefCount:    refCount,
		},
	}
}

func init() {
	Register(&SkillTool{})
}
