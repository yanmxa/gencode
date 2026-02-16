package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/mcp"
)

// MCPSelectorLevel represents the navigation level in the MCP selector
type MCPSelectorLevel int

const (
	MCPLevelList   MCPSelectorLevel = iota // Server list view
	MCPLevelDetail                         // Server detail + actions view
)

// MCPAction represents an action available for a server in detail view
type MCPAction struct {
	Label  string
	Action string // "connect", "disconnect", "reconnect", "remove"
}

// MCPServerItem represents an MCP server in the selector
type MCPServerItem struct {
	Name      string
	Type      string // stdio, http, sse
	Status    mcp.ServerStatus
	ToolCount int
	Error     string
	Scope     string   // user, project, local
	URL       string   // for http/sse
	Command   string   // for stdio
	Args      []string // for stdio
}

// MCPSelectorState holds state for the MCP server selector
type MCPSelectorState struct {
	active       bool
	servers      []MCPServerItem
	selectedIdx  int
	width        int
	height       int
	scrollOffset int
	maxVisible   int
	connecting   bool   // True when a connection is in progress
	lastError    string // Last connection error to display

	// Fuzzy search
	searchQuery     string
	filteredServers []MCPServerItem

	// Two-level navigation
	level        MCPSelectorLevel
	parentIdx    int            // selected index when entering detail
	detailServer *MCPServerItem // server shown in detail view
	actions      []MCPAction    // context-sensitive action menu
	actionIdx    int            // selected action
}

// MCPConnectMsg is sent when connecting to a server
type MCPConnectMsg struct {
	ServerName string
}

// MCPConnectResultMsg is sent when connection completes
type MCPConnectResultMsg struct {
	ServerName string
	Success    bool
	ToolCount  int
	Error      error
}

// MCPDisconnectMsg is sent when disconnecting from a server
type MCPDisconnectMsg struct {
	ServerName string
}

// MCPReconnectMsg is sent when reconnecting to a server
type MCPReconnectMsg struct {
	ServerName string
}

// MCPRemoveMsg is sent when removing a server
type MCPRemoveMsg struct {
	ServerName string
}

// MCPAddRequestMsg is sent when the user presses "n" to add a new server
type MCPAddRequestMsg struct{}

// MCPSelectorCancelledMsg is sent when the selector is cancelled
type MCPSelectorCancelledMsg struct{}

// NewMCPSelectorState creates a new MCPSelectorState
func NewMCPSelectorState() MCPSelectorState {
	return MCPSelectorState{
		active:     false,
		servers:    []MCPServerItem{},
		maxVisible: 10,
	}
}

// EnterMCPSelect enters MCP server selection mode
func (s *MCPSelectorState) EnterMCPSelect(width, height int) error {
	if mcp.DefaultRegistry == nil {
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
	s.level = MCPLevelList
	s.parentIdx = 0
	s.detailServer = nil
	s.actions = nil
	s.actionIdx = 0

	return nil
}

// autoReconnect returns a batch command to reconnect servers in error state.
// Disconnected servers are left as-is since the user intentionally disconnected them.
func (s *MCPSelectorState) autoReconnect() tea.Cmd {
	var cmds []tea.Cmd
	for _, srv := range s.servers {
		if srv.Status == mcp.StatusError {
			mcp.DefaultRegistry.SetConnecting(srv.Name, true)
			cmds = append(cmds, startMCPConnect(srv.Name))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// refreshServers refreshes the server list from registry
func (s *MCPSelectorState) refreshServers() {
	servers := mcp.DefaultRegistry.List()
	s.servers = make([]MCPServerItem, 0, len(servers))

	for _, srv := range servers {
		item := MCPServerItem{
			Name:    srv.Config.Name,
			Type:    string(srv.Config.GetType()),
			Status:  srv.Status,
			Error:   srv.Error,
			Scope:   string(srv.Config.Scope),
			URL:     srv.Config.URL,
			Command: srv.Config.Command,
			Args:    srv.Config.Args,
		}
		if srv.Status == mcp.StatusConnected {
			item.ToolCount = len(srv.Tools)
		}
		s.servers = append(s.servers, item)
	}
	s.updateFilter()
}

// IsActive returns whether the selector is active
func (s *MCPSelectorState) IsActive() bool {
	return s.active
}

// Cancel cancels the selector
func (s *MCPSelectorState) Cancel() {
	s.active = false
	s.servers = []MCPServerItem{}
	s.filteredServers = nil
	s.selectedIdx = 0
	s.scrollOffset = 0
	s.connecting = false
	s.searchQuery = ""
	s.level = MCPLevelList
	s.detailServer = nil
	s.actions = nil
	s.actionIdx = 0
}

// updateFilter filters servers based on search query (fuzzy match)
func (s *MCPSelectorState) updateFilter() {
	if s.searchQuery == "" {
		s.filteredServers = s.servers
	} else {
		query := strings.ToLower(s.searchQuery)
		s.filteredServers = make([]MCPServerItem, 0)
		for _, srv := range s.servers {
			if fuzzyMatch(strings.ToLower(srv.Name), query) ||
				fuzzyMatch(strings.ToLower(srv.Type), query) {
				s.filteredServers = append(s.filteredServers, srv)
			}
		}
	}
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// MoveUp moves the selection up (level-aware)
func (s *MCPSelectorState) MoveUp() {
	if s.level == MCPLevelDetail {
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
func (s *MCPSelectorState) MoveDown() {
	if s.level == MCPLevelDetail {
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
func (s *MCPSelectorState) ensureVisible() {
	if s.selectedIdx < s.scrollOffset {
		s.scrollOffset = s.selectedIdx
	}
	if s.selectedIdx >= s.scrollOffset+s.maxVisible {
		s.scrollOffset = s.selectedIdx - s.maxVisible + 1
	}
}

// enterDetail enters the detail view for the selected server
func (s *MCPSelectorState) enterDetail() {
	if len(s.filteredServers) == 0 || s.selectedIdx >= len(s.filteredServers) {
		return
	}
	s.parentIdx = s.selectedIdx
	srv := s.filteredServers[s.selectedIdx]
	s.detailServer = &srv
	s.actions = s.buildActions(srv)
	s.actionIdx = 0
	s.level = MCPLevelDetail
}

// goBack returns to the list view from detail view
func (s *MCPSelectorState) goBack() bool {
	if s.level == MCPLevelDetail {
		s.level = MCPLevelList
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
func (s *MCPSelectorState) buildActions(srv MCPServerItem) []MCPAction {
	switch srv.Status {
	case mcp.StatusConnected:
		return []MCPAction{
			{Label: "Disable", Action: "disconnect"},
			{Label: "Reconnect", Action: "reconnect"},
			{Label: "Remove", Action: "remove"},
		}
	case mcp.StatusConnecting:
		return []MCPAction{
			{Label: "Disable", Action: "disconnect"},
			{Label: "Remove", Action: "remove"},
		}
	default: // Error or Disconnected
		return []MCPAction{
			{Label: "Connect", Action: "connect"},
			{Label: "Remove", Action: "remove"},
		}
	}
}

// executeAction executes the currently selected action in detail view
func (s *MCPSelectorState) executeAction() tea.Cmd {
	if s.detailServer == nil || s.actionIdx >= len(s.actions) || s.connecting {
		return nil
	}

	action := s.actions[s.actionIdx]
	name := s.detailServer.Name

	switch action.Action {
	case "connect":
		s.connecting = true
		return func() tea.Msg { return MCPConnectMsg{ServerName: name} }
	case "disconnect":
		return func() tea.Msg { return MCPDisconnectMsg{ServerName: name} }
	case "reconnect":
		s.connecting = true
		return func() tea.Msg { return MCPReconnectMsg{ServerName: name} }
	case "remove":
		return func() tea.Msg { return MCPRemoveMsg{ServerName: name} }
	}
	return nil
}

// HandleConnectResult handles the result of a connection attempt
func (s *MCPSelectorState) HandleConnectResult(msg MCPConnectResultMsg) {
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
func (s *MCPSelectorState) HandleDisconnect(name string) {
	if mcp.DefaultRegistry != nil {
		mcp.DefaultRegistry.Disconnect(name)
		mcp.DefaultRegistry.SetDisabled(name, true)
	}
	s.refreshAndUpdateView()
}

// HandleReconnect handles a reconnect request.
// Unlike HandleDisconnect, this does NOT mark the server as disabled,
// since the user intends to reconnect immediately.
func (s *MCPSelectorState) HandleReconnect(name string) {
	if mcp.DefaultRegistry != nil {
		mcp.DefaultRegistry.Disconnect(name)
	}
	s.refreshAndUpdateView()
}

// HandleRemove handles a remove request
func (s *MCPSelectorState) HandleRemove(name string) {
	if mcp.DefaultRegistry != nil {
		mcp.DefaultRegistry.SetDisabled(name, false)
		mcp.DefaultRegistry.RemoveServer(name)
	}
	s.refreshServers()
	s.goBack()
	s.clampSelectedIdx()
}

// refreshAndUpdateView refreshes servers and updates the detail view if active
func (s *MCPSelectorState) refreshAndUpdateView() {
	s.refreshServers()
	if s.level == MCPLevelDetail && s.detailServer != nil {
		s.refreshDetailView()
	}
}

// clampSelectedIdx ensures selectedIdx is within valid bounds
func (s *MCPSelectorState) clampSelectedIdx() {
	if s.selectedIdx >= len(s.filteredServers) && len(s.filteredServers) > 0 {
		s.selectedIdx = len(s.filteredServers) - 1
	}
}

// refreshDetailView updates the detail server and actions after a state change
func (s *MCPSelectorState) refreshDetailView() {
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
func (s *MCPSelectorState) clampActionIdx() {
	if s.actionIdx >= len(s.actions) {
		s.actionIdx = len(s.actions) - 1
	}
	if s.actionIdx < 0 {
		s.actionIdx = 0
	}
}

// mcpStatusIconAndStyle returns the status icon and style for an MCP server status
func mcpStatusIconAndStyle(status mcp.ServerStatus) (string, lipgloss.Style) {
	icon, _ := mcpStatusDisplay(status)
	switch status {
	case mcp.StatusConnected:
		return icon, selectorStatusConnected
	case mcp.StatusConnecting:
		return icon, selectorStatusReady
	case mcp.StatusError:
		return icon, selectorStatusError
	default:
		return icon, selectorStatusNone
	}
}

// mcpStatusDisplay returns icon and label for an MCP server status
// Used by both the interactive selector and command output
func mcpStatusDisplay(status mcp.ServerStatus) (icon, label string) {
	switch status {
	case mcp.StatusConnected:
		return "●", "connected"
	case mcp.StatusConnecting:
		return "◌", "connecting"
	case mcp.StatusError:
		return "✗", "error"
	default:
		return "○", "disconnected"
	}
}

// HandleKeypress handles a keypress and returns a command if needed
func (s *MCPSelectorState) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	// Only allow escape while connecting
	if s.connecting {
		if key.Type == tea.KeyEsc {
			s.Cancel()
			return func() tea.Msg { return MCPSelectorCancelledMsg{} }
		}
		return nil
	}

	// Detail view keypress handling
	if s.level == MCPLevelDetail {
		return s.handleDetailKeypress(key)
	}

	// List view keypress handling
	return s.handleListKeypress(key)
}

// handleDetailKeypress handles keypresses in the detail view
func (s *MCPSelectorState) handleDetailKeypress(key tea.KeyMsg) tea.Cmd {
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
func (s *MCPSelectorState) handleListKeypress(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return nil
	case tea.KeyDown, tea.KeyCtrlJ:
		s.MoveDown()
		return nil
	case tea.KeyCtrlN:
		s.Cancel()
		return func() tea.Msg { return MCPAddRequestMsg{} }
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
		return func() tea.Msg { return MCPSelectorCancelledMsg{} }
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
func (s *MCPSelectorState) Render() string {
	if !s.active {
		return ""
	}

	if s.level == MCPLevelDetail {
		return s.renderDetail()
	}
	return s.renderList()
}

// renderErrorAndFooter appends the error message (if any) and footer hint to the builder
func (s *MCPSelectorState) renderErrorAndFooter(sb *strings.Builder, hint string) {
	if s.lastError != "" {
		sb.WriteString(selectorStatusError.Render("    ! " + s.lastError + "\n"))
	}
	sb.WriteString("\n")
	if s.connecting {
		sb.WriteString(selectorHintStyle.Render("Connecting... (Esc to cancel)"))
	} else {
		sb.WriteString(selectorHintStyle.Render(hint))
	}
}

// renderBox wraps content in a centered bordered box
func (s *MCPSelectorState) renderBox(content string) string {
	boxWidth := calculateToolBoxWidth(s.width)
	box := selectorBorderStyle.Width(boxWidth).Render(content)
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

// truncateText truncates text to maxLen, adding ellipsis if needed
func truncateText(text string, maxLen int) string {
	if maxLen > 0 && len(text) > maxLen {
		return text[:maxLen-3] + "..."
	}
	return text
}

// renderList renders the list view
func (s *MCPSelectorState) renderList() string {
	var sb strings.Builder
	descStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Muted)

	// Title with filtered/total count
	title := fmt.Sprintf("MCP Servers (%d/%d)", len(s.filteredServers), len(s.servers))
	sb.WriteString(selectorTitleStyle.Render(title))
	sb.WriteString("\n")

	// Search input
	searchPrompt := ">> "
	if s.searchQuery == "" {
		sb.WriteString(selectorHintStyle.Render(searchPrompt + "Type to filter..."))
	} else {
		sb.WriteString(selectorBreadcrumbStyle.Render(searchPrompt + s.searchQuery + "|"))
	}
	sb.WriteString("\n\n")

	if len(s.filteredServers) == 0 {
		if len(s.servers) == 0 {
			sb.WriteString(selectorHintStyle.Render("  No MCP servers configured\n\n"))
			sb.WriteString(selectorHintStyle.Render("  Add servers with:\n"))
			sb.WriteString(selectorHintStyle.Render("    gen mcp add <name> -- <command>\n"))
		} else {
			sb.WriteString(selectorHintStyle.Render("  No servers match the filter"))
			sb.WriteString("\n")
		}
	} else {
		endIdx := min(s.scrollOffset+s.maxVisible, len(s.filteredServers))

		if s.scrollOffset > 0 {
			sb.WriteString(selectorHintStyle.Render("  ^ more above"))
			sb.WriteString("\n")
		}

		for i := s.scrollOffset; i < endIdx; i++ {
			srv := s.filteredServers[i]
			icon, statusStyle := mcpStatusIconAndStyle(srv.Status)

			// Name uses status color for connected, muted for others
			nameStyle := descStyle
			if srv.Status == mcp.StatusConnected {
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
				sb.WriteString(selectorSelectedStyle.Render("> " + line))
			} else {
				sb.WriteString(selectorItemStyle.Render("  " + line))
			}
			sb.WriteString("\n")
		}

		if endIdx < len(s.filteredServers) {
			sb.WriteString(selectorHintStyle.Render("  v more below"))
			sb.WriteString("\n")
		}
	}

	s.renderErrorAndFooter(&sb, "up/down navigate . Enter/right details . ^N add . Esc close")
	return s.renderBox(sb.String())
}

// renderDetail renders the detail view for a selected server
func (s *MCPSelectorState) renderDetail() string {
	if s.detailServer == nil {
		return s.renderList()
	}

	var sb strings.Builder
	boxWidth := calculateToolBoxWidth(s.width)
	srv := s.detailServer
	maxValueLen := boxWidth - 20

	labelStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Muted)
	valueStyle := lipgloss.NewStyle().Foreground(CurrentTheme.TextBright)

	// Title
	sb.WriteString(selectorTitleStyle.Render("MCP Server"))
	sb.WriteString("\n")

	// Server name breadcrumb
	sb.WriteString(selectorBreadcrumbStyle.Render("> " + srv.Name))
	sb.WriteString("\n\n")

	// Status
	icon, statusStyle := mcpStatusIconAndStyle(srv.Status)
	_, statusLabel := mcpStatusDisplay(srv.Status)
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
			valueStyle.Render(truncateText(srv.URL, maxValueLen)),
		)
	}
	if srv.Command != "" {
		cmd := srv.Command
		if len(srv.Args) > 0 {
			cmd += " " + strings.Join(srv.Args, " ")
		}
		fmt.Fprintf(&sb, "  %s  %s\n",
			labelStyle.Render("Cmd:   "),
			valueStyle.Render(truncateText(cmd, maxValueLen)),
		)
	}

	// Tool count
	if srv.Status == mcp.StatusConnected {
		fmt.Fprintf(&sb, "  %s  %s\n",
			labelStyle.Render("Tools: "),
			valueStyle.Render(fmt.Sprintf("%d", srv.ToolCount)),
		)
	}

	// Error
	if srv.Error != "" {
		fmt.Fprintf(&sb, "  %s  %s\n",
			labelStyle.Render("Error: "),
			selectorStatusError.Render(srv.Error),
		)
	}

	sb.WriteString("\n")

	// Actions
	sb.WriteString(labelStyle.Render("  Actions:"))
	sb.WriteString("\n")
	for i, action := range s.actions {
		if i == s.actionIdx {
			sb.WriteString(selectorSelectedStyle.Render("> " + action.Label))
		} else {
			sb.WriteString(selectorItemStyle.Render("  " + action.Label))
		}
		sb.WriteString("\n")
	}

	s.renderErrorAndFooter(&sb, "up/down navigate . Enter execute . left/Esc back")
	return s.renderBox(sb.String())
}

// serverDetails returns the details string for a server item
func (s *MCPSelectorState) serverDetails(srv MCPServerItem) string {
	if srv.Status == mcp.StatusConnected {
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

// autoConnectMCPServers returns a batch of commands to connect all configured MCP servers,
// skipping servers that the user has explicitly disabled.
func autoConnectMCPServers() tea.Cmd {
	if mcp.DefaultRegistry == nil {
		return nil
	}
	var cmds []tea.Cmd
	for _, s := range mcp.DefaultRegistry.List() {
		name := s.Config.Name
		if !mcp.DefaultRegistry.IsDisabled(name) {
			mcp.DefaultRegistry.SetConnecting(name, true)
			cmds = append(cmds, startMCPConnect(name))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// startMCPConnect returns a tea.Cmd that connects to an MCP server
func startMCPConnect(name string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if mcp.DefaultRegistry == nil {
			return MCPConnectResultMsg{
				ServerName: name,
				Success:    false,
				Error:      fmt.Errorf("MCP not initialized"),
			}
		}

		if err := mcp.DefaultRegistry.Connect(ctx, name); err != nil {
			return MCPConnectResultMsg{
				ServerName: name,
				Success:    false,
				Error:      err,
			}
		}

		toolCount := 0
		if client, ok := mcp.DefaultRegistry.GetClient(name); ok {
			toolCount = len(client.GetCachedTools())
		}

		return MCPConnectResultMsg{
			ServerName: name,
			Success:    true,
			ToolCount:  toolCount,
		}
	}
}
