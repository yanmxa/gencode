package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/core/system"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/tool"
)

// BuildParams contains all values needed to construct a core.Agent.
// The app layer assembles this from env, services, and workspace state.
type BuildParams struct {
	Provider      llm.Provider
	ModelID       string
	MaxTokens     int
	ThinkingLevel llm.ThinkingLevel

	CWD     string
	CWDFunc func() string // dynamic CWD for tool execution; falls back to CWD if nil
	IsGit   bool

	UserInstructions    string
	ProjectInstructions string
	SkillsPrompt        string
	AgentsPrompt        string
	DeferredToolsPrompt string
	Extra               []system.ExtraLayer

	DisabledTools map[string]bool
	MCPTools      []core.Tool

	PermissionDecider PermDecisionFunc
	InteractionFunc   tool.InteractionFunc
}

func buildAgent(p BuildParams) (core.Agent, *PermissionBridge, error) {
	if p.Provider == nil {
		return nil, nil, fmt.Errorf("no LLM provider configured")
	}

	client := llm.NewClient(p.Provider, p.ModelID, p.MaxTokens)
	client.SetThinking(p.ThinkingLevel)

	sys := system.Build(system.Config{
		ProviderName:        client.Name(),
		ModelID:             client.ModelID(),
		Cwd:                 p.CWD,
		IsGit:               p.IsGit,
		UserInstructions:    p.UserInstructions,
		ProjectInstructions: p.ProjectInstructions,
		Skills:              p.SkillsPrompt,
		Agents:              p.AgentsPrompt,
		DeferredTools:       p.DeferredToolsPrompt,
		Extra:               p.Extra,
	})

	cwdFunc := p.CWDFunc
	if cwdFunc == nil {
		cwd := p.CWD
		cwdFunc = func() string { return cwd }
	}

	schemas := (&tool.Set{
		Disabled: p.DisabledTools,
	}).Tools()
	var adaptOpts []tool.AdaptOption
	if p.InteractionFunc != nil {
		adaptOpts = append(adaptOpts, tool.WithInteraction(p.InteractionFunc))
	}
	tools := tool.AdaptToolRegistry(schemas, cwdFunc, adaptOpts...)
	for _, t := range p.MCPTools {
		tools.Add(t)
	}

	pb := NewPermissionBridge(p.PermissionDecider)

	compactClient := client
	compactFunc := func(ctx context.Context, msgs []core.Message) (string, error) {
		text := core.BuildConversationText(msgs)
		resp, err := compactClient.Complete(ctx, system.CompactPrompt(), []core.Message{core.UserMessage(text, nil)}, core.CompactMaxTokens)
		if err != nil {
			return "", err
		}
		summary := strings.TrimSpace(resp.Content)
		if summary == "" {
			return "", fmt.Errorf("compaction produced empty summary")
		}
		return summary, nil
	}

	ag := core.NewAgent(core.Config{
		ID:          "main",
		LLM:         client,
		System:      sys,
		Tools:       tool.WithPermission(tools, pb.PermissionFunc()),
		CompactFunc: compactFunc,
		CWD:         p.CWD,
	})

	return ag, pb, nil
}
