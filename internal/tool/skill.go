package tool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

const (
	IconSkill = "⚡"
)

// SkillTool allows the LLM to invoke skills programmatically
type SkillTool struct{}

func (t *SkillTool) Name() string { return "Skill" }

func (t *SkillTool) Description() string {
	return "Execute a skill within the main conversation"
}

func (t *SkillTool) Icon() string { return IconSkill }

func (t *SkillTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
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

	return ui.ToolResult{
		Success: true,
		Output:  sb.String(),
		Metadata: ui.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: fmt.Sprintf("Loaded: %s", sk.FullName()),
			Duration: duration,
		},
	}
}

func init() {
	Register(&SkillTool{})
}
