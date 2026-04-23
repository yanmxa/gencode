package app

import (
	"github.com/yanmxa/gencode/internal/agent"
	"github.com/yanmxa/gencode/internal/command"
	"github.com/yanmxa/gencode/internal/cron"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/subagent"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/task/tracker"
	"github.com/yanmxa/gencode/internal/tool"
)

// services holds references to domain service singletons, injected into
// model at construction time. Model methods access services through this
// struct instead of calling Default() package-level accessors directly.
type services struct {
	Setting  setting.Service
	LLM      llm.Service
	Tool     tool.Service
	Hook     hook.Service
	Session  session.Service
	Skill    skill.Service
	Subagent subagent.Service
	Command  command.Service
	Task     task.Service
	Tracker  tracker.Service
	Cron     cron.Service
	MCP      mcp.Service
	Plugin   plugin.Service
	Agent    agent.Service
}

func newServices() services {
	return services{
		Setting:  setting.Default(),
		LLM:      llm.Default(),
		Tool:     tool.Default(),
		Hook:     hook.DefaultIfInit(),
		Session:  session.Default(),
		Skill:    skill.Default(),
		Subagent: subagent.Default(),
		Command:  command.Default(),
		Task:     task.Default(),
		Tracker:  tracker.Default(),
		Cron:     cron.Default(),
		MCP:      mcp.Default(),
		Plugin:   plugin.Default(),
		Agent:    agent.Default(),
	}
}

// refreshAfterReload re-snapshots the 5 services whose singletons are replaced
// by Initialize() calls in ReloadPluginBackedState. The remaining services
// (LLM, Hook, Session, Tool, Task, Tracker, Cron, Plugin)
// are stable — their singletons are created once at startup and never replaced.
func (s *services) refreshAfterReload() {
	s.Setting = setting.Default()
	s.Skill = skill.Default()
	s.Command = command.Default()
	s.Subagent = subagent.Default()
	s.MCP = mcp.Default()
}
