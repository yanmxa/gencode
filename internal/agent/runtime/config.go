// Package agentruntime implements the core.Agent contract. Keeping the engine in a
// feature-owned package leaves internal/core as stable messages and interfaces.
package agentruntime

import (
	"context"
	"time"

	. "github.com/genai-io/san/internal/core"
)

const (
	defaultMaxTurnRetries    = 2
	defaultFirstChunkTimeout = 5 * time.Minute
	defaultStreamIdleTimeout = 60 * time.Second
)

type Config struct {
	ID                      string
	LLM                     LLM
	System                  System
	Tools                   Tools
	AgentType               string
	CompactFunc             func(ctx context.Context, msgs []Message) (string, error)
	CWD                     string
	MaxSteps                int
	MaxOutputRecovery       int
	MaxTurnRetries          int
	StreamFirstChunkTimeout time.Duration
	StreamIdleTimeout       time.Duration
	InboxBuf                int
	OutboxBuf               int
	OnEvent                 func(Event)
}

func New(cfg Config) Agent {
	if cfg.LLM == nil {
		panic("agent/runtime.New: LLM is required")
	}
	if cfg.System == nil {
		panic("agent/runtime.New: System is required")
	}
	if cfg.Tools == nil {
		panic("agent/runtime.New: Tools is required")
	}
	if cfg.InboxBuf <= 0 {
		cfg.InboxBuf = 16
	}
	if cfg.OutboxBuf == 0 {
		cfg.OutboxBuf = 64
	}
	if cfg.MaxTurnRetries <= 0 {
		cfg.MaxTurnRetries = defaultMaxTurnRetries
	}
	if cfg.StreamFirstChunkTimeout <= 0 {
		cfg.StreamFirstChunkTimeout = defaultFirstChunkTimeout
	}
	if cfg.StreamIdleTimeout <= 0 {
		cfg.StreamIdleTimeout = defaultStreamIdleTimeout
	}

	var outbox chan Event
	if cfg.OutboxBuf > 0 {
		outbox = make(chan Event, cfg.OutboxBuf)
	}

	a := &agent{
		id:                cfg.ID,
		agentType:         cfg.AgentType,
		system:            cfg.System,
		tools:             cfg.Tools,
		compactFunc:       cfg.CompactFunc,
		llm:               cfg.LLM,
		cwd:               cfg.CWD,
		maxSteps:          cfg.MaxSteps,
		maxOutputRecovery: cfg.MaxOutputRecovery,
		maxTurnRetries:    cfg.MaxTurnRetries,
		firstChunkTimeout: cfg.StreamFirstChunkTimeout,
		idleTimeout:       cfg.StreamIdleTimeout,
		inbox:             make(chan Message, cfg.InboxBuf),
		outbox:            outbox,
		onEvent:           cfg.OnEvent,
	}
	cfg.System.SetObserver(func(c SystemChange) {
		a.emitTelemetry(SystemChangeEvent(a.id, c))
	})
	cfg.Tools.SetObserver(func(c ToolsChange) {
		a.emitTelemetry(ToolsChangeEvent(a.id, c))
	})
	return a
}
