package mcp

import (
	"context"
	"sync"

	"github.com/yanmxa/gencode/internal/core"
)

// Service is the public contract for the mcp module.
type Service interface {
	// connection
	ListServers() []Server                          // all configured servers with status
	Connect(ctx context.Context, name string) error // connect to a named server
	ConnectAll(ctx context.Context) []error          // connect to all configured servers
	Disconnect(name string) error                   // disconnect from a named server
	Reconnect(ctx context.Context, name string) error // disconnect then reconnect

	// tools
	ListTools() []core.ToolSchema // tool schemas from all connected servers
	NewCaller() *Caller           // create execution caller

	// config
	EditConfig(name string) (*EditInfo, error) // prepare config for editing
	SaveConfig(info *EditInfo) error           // save edited config

	// registry access (backward compat)
	Registry() *Registry // underlying registry for callers that need it directly
}

// Compile-time check: *service implements Service.
var _ Service = (*service)(nil)

// ── singleton ──────────────────────────────────────────────

var (
	svcMu      sync.RWMutex
	svcInstance Service
)

// Default returns the singleton Service instance.
// Panics if Initialize has not been called.
func Default() Service {
	svcMu.RLock()
	s := svcInstance
	svcMu.RUnlock()
	if s == nil {
		panic("mcp: not initialized")
	}
	return s
}

// SetDefault replaces the singleton instance. Intended for tests.
func SetDefault(s Service) {
	svcMu.Lock()
	svcInstance = s
	svcMu.Unlock()
}

// ResetService clears the singleton instance. Intended for tests.
func ResetService() {
	svcMu.Lock()
	svcInstance = nil
	svcMu.Unlock()
}

// ── implementation ─────────────────────────────────────────

// service wraps the legacy Registry to satisfy the Service interface.
type service struct {
	reg *Registry
}

func (s *service) ListServers() []Server {
	return s.reg.List()
}

func (s *service) Connect(ctx context.Context, name string) error {
	return s.reg.Connect(ctx, name)
}

func (s *service) ConnectAll(ctx context.Context) []error {
	return s.reg.ConnectAll(ctx)
}

func (s *service) Disconnect(name string) error {
	return s.reg.Disconnect(name)
}

func (s *service) Reconnect(ctx context.Context, name string) error {
	if err := s.reg.Disconnect(name); err != nil {
		return err
	}
	return s.reg.Connect(ctx, name)
}

func (s *service) ListTools() []core.ToolSchema {
	return s.reg.GetToolSchemas()
}

func (s *service) NewCaller() *Caller {
	return NewCaller(s.reg)
}

func (s *service) EditConfig(name string) (*EditInfo, error) {
	return PrepareServerEdit(s.reg, name)
}

func (s *service) SaveConfig(info *EditInfo) error {
	return ApplyServerEdit(s.reg, info)
}

func (s *service) Registry() *Registry {
	return s.reg
}
