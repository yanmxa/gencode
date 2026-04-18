package app

import (
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/subagent"
	"github.com/yanmxa/gencode/internal/task/tracker"
)

// Services holds references to domain service singletons, injected into
// model at construction time.  Model methods access services through this
// struct instead of calling Default() package-level accessors directly.
type Services struct {
	Setting       setting.Service
	LLM           llm.Service
	Hook          hook.Service
	Session       session.Service
	Skill         skill.Service
	Subagent      subagent.Service
	Tracker       tracker.Service
	Orchestration orchestration.Service
}

func newServices() Services {
	return Services{
		Setting:       setting.Default(),
		LLM:           llm.Default(),
		Hook:          hook.DefaultIfInit(),
		Session:       session.Default(),
		Skill:         skill.Default(),
		Subagent:      subagent.Default(),
		Tracker:       tracker.Default(),
		Orchestration: orchestration.Default(),
	}
}
