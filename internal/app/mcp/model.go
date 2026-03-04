// Package mcp provides the MCP server selector feature.
package mcp

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	coremcp "github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/ui/shared"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

// SelectorLevel represents the navigation level in the MCP selector
type SelectorLevel int

const (
	LevelList   SelectorLevel = iota // Server list view
	LevelDetail                      // Server detail + actions view
)

// Action represents an action available for a server in detail view
type Action struct {
	Label  string
	Action string // "connect", "disconnect", "reconnect", "remove"
}

// ServerItem represents an MCP server in the selector
type ServerItem struct {
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
	active       bool
	servers      []ServerItem
	selectedIdx  int
	width        int
	height       int
	scrollOffset int
	maxVisible   int
	connecting   bool   // True when a connection is in progress
	lastError    string // Last connection error to display

	// Fuzzy search
	searchQuery     string
	filteredServers []ServerItem

	// Two-level navigation
	level        SelectorLevel
	parentIdx    int         // selected index when entering detail
	detailServer *ServerItem // server shown in detail view
	actions      []Action    // context-sensitive action menu
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


// New creates a new Model
func New() Model {
	return Model{
		active:     false,
		servers:    []ServerItem{},
		maxVisible: 10,
	}
}

// EnterSelect enters MCP server selection mode
func (s *Model) EnterSelect(width, height int) error {
	if coremcp.DefaultRegistry == nil {
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
	s.level = LevelList
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
			coremcp.DefaultRegistry.SetConnecting(srv.Name, true)
			cmds = append(cmds, StartConnect(srv.Name))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// refreshServers refreshes the server list from registry
func (s *Model) refreshServers() {
	servers := coremcp.DefaultRegistry.List()
	s.servers = make([]ServerItem, 0, len(servers))

	for _, srv := range servers {
		item := ServerItem{
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
	s.servers = []ServerItem{}
	s.filteredServers = nil
	s.selectedIdx = 0
	s.scrollOffset = 0
	s.connecting = false
	s.searchQuery = ""
	s.level = LevelList
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
		s.filteredServers = make([]ServerItem, 0)
		for _, srv := range s.servers {
			if shared.FuzzyMatch(strings.ToLower(srv.Name), query) ||
				shared.FuzzyMatch(strings.ToLower(srv.Type), query) {
				s.filteredServers = append(s.filteredServers, srv)
			}
		}
	}
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// MoveUp moves the selection up (level-aware)
func (s *Model) MoveUp() {
	if s.level == LevelDetail {
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
	if s.level == LevelDetail {
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
	s.level = LevelDetail
}

// goBack returns to the list view from detail view
func (s *Model) goBack() bool {
	if s.level == LevelDetail {
		s.level = LevelList
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
func (s *Model) buildActions(srv ServerItem) []Action {
	switch srv.Status {
	case coremcp.StatusConnected:
		return []Action{
			{Label: "Disable", Action: "disconnect"},
			{Label: "Reconnect", Action: "reconnect"},
			{Label: "Remove", Action: "remove"},
		}
	case coremcp.StatusConnecting:
		return []Action{
			{Label: "Disable", Action: "disconnect"},
			{Label: "Remove", Action: "remove"},
		}
	default: // Error or Disconnected
		return []Action{
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
	if coremcp.DefaultRegistry != nil {
		coremcp.DefaultRegistry.Disconnect(name)
		coremcp.DefaultRegistry.SetDisabled(name, true)
	}
	s.refreshAndUpdateView()
}

// HandleReconnect handles a reconnect request.
// Unlike HandleDisconnect, this does NOT mark the server as disabled,
// since the user intends to reconnect immediately.
func (s *Model) HandleReconnect(name string) {
	if coremcp.DefaultRegistry != nil {
		coremcp.DefaultRegistry.Disconnect(name)
	}
	s.refreshAndUpdateView()
}

// HandleRemove handles a remove request
func (s *Model) HandleRemove(name string) {
	if coremcp.DefaultRegistry != nil {
		coremcp.DefaultRegistry.SetDisabled(name, false)
		coremcp.DefaultRegistry.RemoveServer(name)
	}
	s.refreshServers()
	s.goBack()
	s.clampSelectedIdx()
}

// refreshAndUpdateView refreshes servers and updates the detail view if active
func (s *Model) refreshAndUpdateView() {
	s.refreshServers()
	if s.level == LevelDetail && s.detailServer != nil {
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

// statusIconAndStyle returns the status icon and style for an MCP server status
func statusIconAndStyle(status coremcp.ServerStatus) (string, lipgloss.Style) {
	icon, _ := shared.MCPStatusDisplay(status)
	switch status {
	case coremcp.StatusConnected:
		return icon, shared.SelectorStatusConnected
	case coremcp.StatusConnecting:
		return icon, shared.SelectorStatusReady
	case coremcp.StatusError:
		return icon, shared.SelectorStatusError
	default:
		return icon, shared.SelectorStatusNone
	}
}

// HandleKeypress handles a keypress and returns a command if needed
func (s *Model) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	// Only allow escape while connecting
	if s.connecting {
		if key.Type == tea.KeyEsc {
			s.Cancel()
			return func() tea.Msg { return shared.DismissedMsg{} }
		}
		return nil
	}

	// Detail view keypress handling
	if s.level == LevelDetail {
		return s.handleDetailKeypress(key)
	}

	// List view keypress handling
	return s.handleListKeypress(key)
}

// handleDetailKeypress handles keypresses in the detail view
func (s *Model) handleDetailKeypress(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return nil
	case tea.KeyDown, tea.KeyCtrlN:
		s.MoveDown()
		return nil
	case tea.KeyEnter:
		return s.executeAction()
	case tea.KeyEsc, tea.KeyLeft:
		s.goBack()
		return nil
	case tea.KeyRunes:
		switch key.String() {
		case "k":
			s.MoveUp()
		case "j":
			s.MoveDown()
		case "h":
			s.goBack()
		}
		return nil
	}
	return nil
}

// handleListKeypress handles keypresses in the list view
func (s *Model) handleListKeypress(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return nil
	case tea.KeyDown, tea.KeyCtrlJ:
		s.MoveDown()
		return nil
	case tea.KeyCtrlN:
		s.Cancel()
		return func() tea.Msg { return AddServerMsg{} }
	case tea.KeyEnter, tea.KeyRight:
		s.enterDetail()
		return nil
	case tea.KeyEsc:
		// First clear search if active
		if s.searchQuery != "" {
			s.searchQuery = ""
			s.updateFilter()
			return nil
		}
		// Then close the selector
		s.Cancel()
		return func() tea.Msg { return shared.DismissedMsg{} }
	case tea.KeyBackspace:
		if len(s.searchQuery) > 0 {
			s.searchQuery = s.searchQuery[:len(s.searchQuery)-1]
			s.updateFilter()
		}
		return nil
	case tea.KeyRunes:
		r := key.String()
		// vim navigation when not searching
		if s.searchQuery == "" {
			switch r {
			case "j":
				s.MoveDown()
				return nil
			case "k":
				s.MoveUp()
				return nil
			case "l":
				s.enterDetail()
				return nil
			}
		}
		// Append to search query
		s.searchQuery += r
		s.updateFilter()
		return nil
	}
	return nil
}

// Render renders the MCP selector
func (s *Model) Render() string {
	if !s.active {
		return ""
	}

	if s.level == LevelDetail {
		return s.renderDetail()
	}
	return s.renderList()
}

// renderErrorAndFooter appends the error message (if any) and footer hint to the builder
func (s *Model) renderErrorAndFooter(sb *strings.Builder, hint string) {
	if s.lastError != "" {
		sb.WriteString(shared.SelectorStatusError.Render("    ! " + s.lastError + "\n"))
	}
	sb.WriteString("\n")
	if s.connecting {
		sb.WriteString(shared.SelectorHintStyle.Render("Connecting... (Esc to cancel)"))
	} else {
		sb.WriteString(shared.SelectorHintStyle.Render(hint))
	}
}

// renderBox wraps content in a centered bordered box
func (s *Model) renderBox(content string) string {
	boxWidth := shared.CalculateToolBoxWidth(s.width)
	box := shared.SelectorBorderStyle.Width(boxWidth).Render(content)
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

// renderList renders the list view
func (s *Model) renderList() string {
	var sb strings.Builder
	descStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)

	// Title with filtered/total count
	title := fmt.Sprintf("MCP Servers (%d/%d)", len(s.filteredServers), len(s.servers))
	sb.WriteString(shared.SelectorTitleStyle.Render(title))
	sb.WriteString("\n")

	// Search input
	searchPrompt := ">> "
	if s.searchQuery == "" {
		sb.WriteString(shared.SelectorHintStyle.Render(searchPrompt + "Type to filter..."))
	} else {
		sb.WriteString(shared.SelectorBreadcrumbStyle.Render(searchPrompt + s.searchQuery + "|"))
	}
	sb.WriteString("\n\n")

	if len(s.filteredServers) == 0 {
		if len(s.servers) == 0 {
			sb.WriteString(shared.SelectorHintStyle.Render("  No MCP servers configured\n\n"))
			sb.WriteString(shared.SelectorHintStyle.Render("  Add servers with:\n"))
			sb.WriteString(shared.SelectorHintStyle.Render("    gen mcp add <name> -- <command>\n"))
		} else {
			sb.WriteString(shared.SelectorHintStyle.Render("  No servers match the filter"))
			sb.WriteString("\n")
		}
	} else {
		endIdx := min(s.scrollOffset+s.maxVisible, len(s.filteredServers))

		if s.scrollOffset > 0 {
			sb.WriteString(shared.SelectorHintStyle.Render("  ^ more above"))
			sb.WriteString("\n")
		}

		for i := s.scrollOffset; i < endIdx; i++ {
			srv := s.filteredServers[i]
			icon, statusStyle := statusIconAndStyle(srv.Status)

			// Name uses status color for connected, muted for others
			nameStyle := descStyle
			if srv.Status == coremcp.StatusConnected {
				nameStyle = statusStyle
			}

			details := s.serverDetails(srv)
			line := fmt.Sprintf("%s %-20s %s  %s",
				statusStyle.Render(icon),
				nameStyle.Render(srv.Name),
				descStyle.Render(fmt.Sprintf("[%s]", srv.Type)),
				descStyle.Render(details),
			)

			if i == s.selectedIdx {
				sb.WriteString(shared.SelectorSelectedStyle.Render("> " + line))
			} else {
				sb.WriteString(shared.SelectorItemStyle.Render("  " + line))
			}
			sb.WriteString("\n")
		}

		if endIdx < len(s.filteredServers) {
			sb.WriteString(shared.SelectorHintStyle.Render("  v more below"))
			sb.WriteString("\n")
		}
	}

	s.renderErrorAndFooter(&sb, "up/down navigate . Enter/right details . ^N add . Esc close")
	return s.renderBox(sb.String())
}

// renderDetail renders the detail view for a selected server
func (s *Model) renderDetail() string {
	if s.detailServer == nil {
		return s.renderList()
	}

	var sb strings.Builder
	boxWidth := shared.CalculateToolBoxWidth(s.width)
	srv := s.detailServer
	maxValueLen := boxWidth - 20

	labelStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	valueStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)

	// Title
	sb.WriteString(shared.SelectorTitleStyle.Render("MCP Server"))
	sb.WriteString("\n")

	// Server name breadcrumb
	sb.WriteString(shared.SelectorBreadcrumbStyle.Render("> " + srv.Name))
	sb.WriteString("\n\n")

	// Status
	icon, statusStyle := statusIconAndStyle(srv.Status)
	_, statusLabel := shared.MCPStatusDisplay(srv.Status)
	fmt.Fprintf(&sb, "  %s  %s\n",
		labelStyle.Render("Status:"),
		statusStyle.Render(icon+" "+statusLabel),
	)

	// Type
	fmt.Fprintf(&sb, "  %s  %s\n",
		labelStyle.Render("Type:  "),
		valueStyle.Render(srv.Type),
	)

	// Scope
	if srv.Scope != "" {
		fmt.Fprintf(&sb, "  %s  %s\n",
			labelStyle.Render("Scope: "),
			valueStyle.Render(srv.Scope),
		)
	}

	// URL or Command
	if srv.URL != "" {
		fmt.Fprintf(&sb, "  %s  %s\n",
			labelStyle.Render("URL:   "),
			valueStyle.Render(shared.TruncateText(srv.URL, maxValueLen)),
		)
	}
	if srv.Command != "" {
		cmd := srv.Command
		if len(srv.Args) > 0 {
			cmd += " " + strings.Join(srv.Args, " ")
		}
		fmt.Fprintf(&sb, "  %s  %s\n",
			labelStyle.Render("Cmd:   "),
			valueStyle.Render(shared.TruncateText(cmd, maxValueLen)),
		)
	}

	// Tool count
	if srv.Status == coremcp.StatusConnected {
		fmt.Fprintf(&sb, "  %s  %s\n",
			labelStyle.Render("Tools: "),
			valueStyle.Render(fmt.Sprintf("%d", srv.ToolCount)),
		)
	}

	// Error
	if srv.Error != "" {
		fmt.Fprintf(&sb, "  %s  %s\n",
			labelStyle.Render("Error: "),
			shared.SelectorStatusError.Render(srv.Error),
		)
	}

	sb.WriteString("\n")

	// Actions
	sb.WriteString(labelStyle.Render("  Actions:"))
	sb.WriteString("\n")
	for i, action := range s.actions {
		if i == s.actionIdx {
			sb.WriteString(shared.SelectorSelectedStyle.Render("> " + action.Label))
		} else {
			sb.WriteString(shared.SelectorItemStyle.Render("  " + action.Label))
		}
		sb.WriteString("\n")
	}

	s.renderErrorAndFooter(&sb, "up/down navigate . Enter execute . left/Esc back")
	return s.renderBox(sb.String())
}

// serverDetails returns the details string for a server item
func (s *Model) serverDetails(srv ServerItem) string {
	if srv.Status == coremcp.StatusConnected {
		return fmt.Sprintf("Tools: %d", srv.ToolCount)
	}
	if srv.Error != "" {
		if len(srv.Error) > 30 {
			return srv.Error[:27] + "..."
		}
		return srv.Error
	}
	return ""
}

// AutoConnect returns a batch of commands to connect all configured MCP servers,
// skipping servers that the user has explicitly disabled.
func AutoConnect() tea.Cmd {
	if coremcp.DefaultRegistry == nil {
		return nil
	}
	var cmds []tea.Cmd
	for _, s := range coremcp.DefaultRegistry.List() {
		name := s.Config.Name
		if !coremcp.DefaultRegistry.IsDisabled(name) {
			coremcp.DefaultRegistry.SetConnecting(name, true)
			cmds = append(cmds, StartConnect(name))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// StartConnect returns a tea.Cmd that connects to an MCP server
func StartConnect(name string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if coremcp.DefaultRegistry == nil {
			return ConnectResultMsg{
				ServerName: name,
				Success:    false,
				Error:      fmt.Errorf("MCP not initialized"),
			}
		}

		if err := coremcp.DefaultRegistry.Connect(ctx, name); err != nil {
			return ConnectResultMsg{
				ServerName: name,
				Success:    false,
				Error:      err,
			}
		}

		toolCount := 0
		if client, ok := coremcp.DefaultRegistry.GetClient(name); ok {
			toolCount = len(client.GetCachedTools())
		}

		return ConnectResultMsg{
			ServerName: name,
			Success:    true,
			ToolCount:  toolCount,
		}
	}
}
