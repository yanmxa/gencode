package agent

import (
	"fmt"

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

	PlanEnabled         bool
	UserInstructions    string
	ProjectInstructions string
	SkillsPrompt        string
	AgentsPrompt        string
	DeferredToolsPrompt string
	Extra               []system.ExtraLayer

	DisabledTools map[string]bool
	MCPTools      []core.Tool

	PermissionDecider PermDecisionFunc
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
		PlanMode:            p.PlanEnabled,
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
		PlanMode: p.PlanEnabled,
	}).Tools()
	tools := tool.AdaptToolRegistry(schemas, cwdFunc)
	for _, t := range p.MCPTools {
		tools.Add(t)
	}

	pb := NewPermissionBridge(p.PermissionDecider)

	ag := core.NewAgent(core.Config{
		ID:    "main",
		LLM:   client,
		System: sys,
		Tools:  tool.WithPermission(tools, pb.PermissionFunc()),
		CWD:   p.CWD,
	})

	return ag, pb, nil
}
