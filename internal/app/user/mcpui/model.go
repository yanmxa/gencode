// Package mcpui provides the MCP server selector feature.
package mcpui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	coremcp "github.com/yanmxa/gencode/internal/extension/mcp"
	"github.com/yanmxa/gencode/internal/app/kit"
)

// selectorLevel represents the navigation level in the MCP selector
type selectorLevel int

const (
	levelList   selectorLevel = iota // Server list view
	levelDetail                      // Server detail + actions view
)

// action represents an action available for a server in detail view.
type action struct {
	Label  string
	Action string // "edit", "connect", "disconnect", "reconnect", "remove"
}

// serverItem represents an MCP server in the selector
type serverItem struct {
	Name      string
	Type      string // stdio, http, sse
	Status    coremcp.ServerStatus
	ToolCount int
	Error     string
	Scope     string   // user, project, local
	URL       string   // for http/sse
	Command   string   // for stdio
	Args      []string // for stdio
}

// Model holds state for the MCP server selector
type Model struct {
	registry *coremcp.Registry

	active       bool
	servers      []serverItem
	selectedIdx  int
	width        int
	height       int
	scrollOffset int
	maxVisible   int
	connecting   bool   // True when a connection is in progress
	lastError    string // Last connection error to display

	// Fuzzy search
	searchQuery     string
	filteredServers []serverItem

	// Two-level navigation
	level        selectorLevel
	parentIdx    int         // selected index when entering detail
	detailServer *serverItem // server shown in detail view
	actions      []action    // context-sensitive action menu
	actionIdx    int         // selected action
}

// ConnectMsg is sent when connecting to a server
type ConnectMsg struct {
	ServerName string
}

// ConnectResultMsg is sent when connection completes
type ConnectResultMsg struct {
	ServerName string
	Success    bool
	ToolCount  int
	Error      error
}

// DisconnectMsg is sent when disconnecting from a server
type DisconnectMsg struct {
	ServerName string
}

// ReconnectMsg is sent when reconnecting to a server
type ReconnectMsg struct {
	ServerName string
}

// RemoveMsg is sent when removing a server
type RemoveMsg struct {
	ServerName string
}

// AddServerMsg is sent when the user presses "n" to add a new server
type AddServerMsg struct{}

// EditServerMsg is sent when the user chooses to edit a server's config
type EditServerMsg struct {
	ServerName string
	Scope      string
}

// New creates a new Model with the given MCP registry.
func New(reg *coremcp.Registry) Model {
	return Model{
		registry:   reg,
		active:     false,
		servers:    []serverItem{},
		maxVisible: 10,
	}
}

// EnterSelect enters MCP server selection mode
func (s *Model) EnterSelect(width, height int) error {
	if s.registry == nil {
		return fmt.Errorf("MCP is not initialized")
	}

	s.refreshServers()
	s.active = true
	s.selectedIdx = 0
	s.scrollOffset = 0
	s.width = width
	s.height = height
	s.connecting = false
	s.lastError = ""
	s.searchQuery = ""
	s.filteredServers = s.servers
	s.level = levelList
	s.parentIdx = 0
	s.detailServer = nil
	s.actions = nil
	s.actionIdx = 0

	return nil
}

// AutoReconnect returns a batch command to reconnect servers in error state.
// Disconnected servers are left as-is since the user intentionally disconnected them.
func (s *Model) AutoReconnect() tea.Cmd {
	var cmds []tea.Cmd
	for _, srv := range s.servers {
		if srv.Status == coremcp.StatusError {
			s.registry.SetConnecting(srv.Name, true)
			cmds = append(cmds, startConnect(s.registry, srv.Name))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// refreshServers refreshes the server list from registry
func (s *Model) refreshServers() {
	servers := s.registry.List()
	s.servers = make([]serverItem, 0, len(servers))

	for _, srv := range servers {
		item := serverItem{
			Name:    srv.Config.Name,
			Type:    string(srv.Config.GetType()),
			Status:  srv.Status,
			Error:   srv.Error,
			Scope:   string(srv.Config.Scope),
			URL:     srv.Config.URL,
			Command: srv.Config.Command,
			Args:    srv.Config.Args,
		}
		if srv.Status == coremcp.StatusConnected {
			item.ToolCount = len(srv.Tools)
		}
		s.servers = append(s.servers, item)
	}
	s.updateFilter()
}

// IsActive returns whether the selector is active
func (s *Model) IsActive() bool {
	return s.active
}

// Cancel cancels the selector
func (s *Model) Cancel() {
	s.active = false
	s.servers = []serverItem{}
	s.filteredServers = nil
	s.selectedIdx = 0
	s.scrollOffset = 0
	s.connecting = false
	s.searchQuery = ""
	s.level = levelList
	s.detailServer = nil
	s.actions = nil
	s.actionIdx = 0
}

// updateFilter filters servers based on search query (fuzzy match)
func (s *Model) updateFilter() {
	if s.searchQuery == "" {
		s.filteredServers = s.servers
	} else {
		query := strings.ToLower(s.searchQuery)
		s.filteredServers = make([]serverItem, 0)
		for _, srv := range s.servers {
			if kit.FuzzyMatch(strings.ToLower(srv.Name), query) ||
				kit.FuzzyMatch(strings.ToLower(srv.Type), query) {
				s.filteredServers = append(s.filteredServers, srv)
			}
		}
	}
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// MoveUp moves the selection up (level-aware)
func (s *Model) MoveUp() {
	if s.level == levelDetail {
		if s.actionIdx > 0 {
			s.actionIdx--
		}
		return
	}
	if s.selectedIdx > 0 {
		s.selectedIdx--
		s.ensureVisible()
	}
}

// MoveDown moves the selection down (level-aware)
func (s *Model) MoveDown() {
	if s.level == levelDetail {
		if s.actionIdx < len(s.actions)-1 {
			s.actionIdx++
		}
		return
	}
	if s.selectedIdx < len(s.filteredServers)-1 {
		s.selectedIdx++
		s.ensureVisible()
	}
}

// ensureVisible adjusts scrollOffset to keep selectedIdx visible
func (s *Model) ensureVisible() {
	if s.selectedIdx < s.scrollOffset {
		s.scrollOffset = s.selectedIdx
	}
	if s.selectedIdx >= s.scrollOffset+s.maxVisible {
		s.scrollOffset = s.selectedIdx - s.maxVisible + 1
	}
}

// enterDetail enters the detail view for the selected server
func (s *Model) enterDetail() {
	if len(s.filteredServers) == 0 || s.selectedIdx >= len(s.filteredServers) {
		return
	}
	s.parentIdx = s.selectedIdx
	srv := s.filteredServers[s.selectedIdx]
	s.detailServer = &srv
	s.actions = s.buildActions(srv)
	s.actionIdx = 0
	s.level = levelDetail
}

// goBack returns to the list view from detail view
func (s *Model) goBack() bool {
	if s.level == levelDetail {
		s.level = levelList
		s.selectedIdx = s.parentIdx
		s.detailServer = nil
		s.actions = nil
		s.actionIdx = 0
		s.lastError = ""
		return true
	}
	return false
}

// buildActions returns context-sensitive actions for a server
func (s *Model) buildActions(srv serverItem) []action {
	edit := action{Label: "Edit", Action: "edit"}
	switch srv.Status {
	case coremcp.StatusConnected:
		return []action{
			edit,
			{Label: "Disable", Action: "disconnect"},
			{Label: "Reconnect", Action: "reconnect"},
			{Label: "Remove", Action: "remove"},
		}
	case coremcp.StatusConnecting:
		return []action{
			edit,
			{Label: "Disable", Action: "disconnect"},
			{Label: "Remove", Action: "remove"},
		}
	default: // Error or Disconnected
		return []action{
			edit,
			{Label: "Connect", Action: "connect"},
			{Label: "Remove", Action: "remove"},
		}
	}
}

// executeAction executes the currently selected action in detail view
func (s *Model) executeAction() tea.Cmd {
	if s.detailServer == nil || s.actionIdx >= len(s.actions) || s.connecting {
		return nil
	}

	action := s.actions[s.actionIdx]
	name := s.detailServer.Name

	switch action.Action {
	case "edit":
		scope := s.detailServer.Scope
		s.Cancel()
		return func() tea.Msg {
			return EditServerMsg{ServerName: name, Scope: scope}
		}
	case "connect":
		s.connecting = true
		return func() tea.Msg { return ConnectMsg{ServerName: name} }
	case "disconnect":
		return func() tea.Msg { return DisconnectMsg{ServerName: name} }
	case "reconnect":
		s.connecting = true
		return func() tea.Msg { return ReconnectMsg{ServerName: name} }
	case "remove":
		return func() tea.Msg { return RemoveMsg{ServerName: name} }
	}
	return nil
}

// HandleConnectResult handles the result of a connection attempt
func (s *Model) HandleConnectResult(msg ConnectResultMsg) {
	s.connecting = false
	if msg.Success {
		s.lastError = ""
	} else if msg.Error != nil {
		s.lastError = fmt.Sprintf("Failed to connect: %v", msg.Error)
	}
	s.refreshAndUpdateView()
}

// HandleDisconnect handles a disconnect (disable) request.
// Marks the server as disabled so it won't auto-connect on restart.
func (s *Model) HandleDisconnect(name string) {
	if s.registry != nil {
		_ = s.registry.Disconnect(name)
		s.registry.SetDisabled(name, true)
	}
	s.refreshAndUpdateView()
}

// HandleReconnect handles a reconnect request.
// Unlike HandleDisconnect, this does NOT mark the server as disabled,
// since the user intends to reconnect immediately.
func (s *Model) HandleReconnect(name string) {
	if s.registry != nil {
		s.registry.Disconnect(name)
	}
	s.refreshAndUpdateView()
}

// HandleRemove handles a remove request
func (s *Model) HandleRemove(name string) {
	if s.registry != nil {
		s.registry.SetDisabled(name, false)
		s.registry.RemoveServer(name)
	}
	s.refreshServers()
	s.goBack()
	s.clampSelectedIdx()
}

// refreshAndUpdateView refreshes servers and updates the detail view if active
func (s *Model) refreshAndUpdateView() {
	s.refreshServers()
	if s.level == levelDetail && s.detailServer != nil {
		s.refreshDetailView()
	}
}

// clampSelectedIdx ensures selectedIdx is within valid bounds
func (s *Model) clampSelectedIdx() {
	if s.selectedIdx >= len(s.filteredServers) && len(s.filteredServers) > 0 {
		s.selectedIdx = len(s.filteredServers) - 1
	}
}

// refreshDetailView updates the detail server and actions after a state change
func (s *Model) refreshDetailView() {
	if s.detailServer == nil {
		return
	}
	name := s.detailServer.Name
	for _, srv := range s.filteredServers {
		if srv.Name == name {
			s.detailServer = &srv
			s.actions = s.buildActions(srv)
			s.clampActionIdx()
			return
		}
	}
	// Server no longer in list (removed or filtered out) - go back
	s.goBack()
}

// clampActionIdx ensures actionIdx is within valid bounds
func (s *Model) clampActionIdx() {
	if s.actionIdx >= len(s.actions) {
		s.actionIdx = len(s.actions) - 1
	}
	if s.actionIdx < 0 {
		s.actionIdx = 0
	}
}

// AutoConnect returns a batch of commands to connect all configured MCP servers,
// skipping servers that the user has explicitly disabled.
func (s *Model) AutoConnect() tea.Cmd {
	if s.registry == nil {
		return nil
	}
	var cmds []tea.Cmd
	for _, srv := range s.registry.List() {
		name := srv.Config.Name
		if !s.registry.IsDisabled(name) {
			s.registry.SetConnecting(name, true)
			cmds = append(cmds, startConnect(s.registry, name))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// startConnect returns a tea.Cmd that connects to an MCP server.
func startConnect(reg *coremcp.Registry, name string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if reg == nil {
			return ConnectResultMsg{
				ServerName: name,
				Success:    false,
				Error:      fmt.Errorf("MCP not initialized"),
			}
		}

		if err := reg.Connect(ctx, name); err != nil {
			return ConnectResultMsg{
				ServerName: name,
				Success:    false,
				Error:      err,
			}
		}

		toolCount := 0
		if client, ok := reg.GetClient(name); ok {
			toolCount = len(client.GetCachedTools())
		}

		return ConnectResultMsg{
			ServerName: name,
			Success:    true,
			ToolCount:  toolCount,
		}
	}
}
