// handler_command_mode.go contains mode-related command handlers:
// /plan, /think, /compact, and /tokenlimit.
package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/ui/providerui"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/llm"
)

func handlePlanCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if args == "" {
		return "Usage: /plan <task description>\n\nEnter plan mode to explore the codebase and create an implementation plan before making changes.", nil, nil
	}

	m.mode.Operation = config.ModePlan
	m.mode.Enabled = true
	m.mode.Task = args

	m.mode.SessionPermissions.AllowAllEdits = false
	m.mode.SessionPermissions.AllowAllWrites = false
	m.mode.SessionPermissions.AllowAllBash = false
	m.mode.SessionPermissions.AllowAllSkills = false

	store, err := plan.NewStore()
	if err != nil {
		return "", nil, fmt.Errorf("failed to initialize plan store: %w", err)
	}
	m.mode.Store = store

	return fmt.Sprintf("Entering plan mode for: %s\n\nI will explore the codebase and create an implementation plan. Only read-only tools are available until the plan is approved.", args), nil, nil
}

func handleThinkCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	args = strings.TrimSpace(strings.ToLower(args))

	switch args {
	case "off", "0":
		m.provider.ThinkingLevel = llm.ThinkingOff
	case "", "toggle":
		// Cycle to next level
		m.provider.ThinkingLevel = m.provider.ThinkingLevel.Next()
	case "think", "normal", "1":
		m.provider.ThinkingLevel = llm.ThinkingNormal
	case "think+", "high", "2":
		m.provider.ThinkingLevel = llm.ThinkingHigh
	case "ultra", "ultrathink", "max", "3":
		m.provider.ThinkingLevel = llm.ThinkingUltra
	default:
		return "Usage: /think [off|think|think+|ultra]\n\nLevels:\n  off        — No extended thinking\n  think      — Moderate thinking budget\n  think+     — Extended thinking budget\n  ultra      — Maximum thinking budget\n\nWithout arguments, cycles to the next level.", nil, nil
	}

	m.provider.StatusMessage = fmt.Sprintf("thinking: %s", m.provider.ThinkingLevel.String())
	return "", providerui.StatusTimer(3 * time.Second), nil
}
