package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/mcp"
)

// MCPServerItem represents an MCP server in the selector
type MCPServerItem struct {
	Name      string
	Type      string // stdio, http, sse
	Status    mcp.ServerStatus
	ToolCount int
	Error     string
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

	return nil
}

// refreshServers refreshes the server list from registry
func (s *MCPSelectorState) refreshServers() {
	servers := mcp.DefaultRegistry.List()
	s.servers = make([]MCPServerItem, 0, len(servers))

	for _, srv := range servers {
		item := MCPServerItem{
			Name:   srv.Config.Name,
			Type:   string(srv.Config.GetType()),
			Status: srv.Status,
			Error:  srv.Error,
		}
		if srv.Status == mcp.StatusConnected {
			item.ToolCount = len(srv.Tools)
		}
		s.servers = append(s.servers, item)
	}
}

// IsActive returns whether the selector is active
func (s *MCPSelectorState) IsActive() bool {
	return s.active
}

// Cancel cancels the selector
func (s *MCPSelectorState) Cancel() {
	s.active = false
	s.servers = []MCPServerItem{}
	s.selectedIdx = 0
	s.scrollOffset = 0
	s.connecting = false
}

// MoveUp moves the selection up
func (s *MCPSelectorState) MoveUp() {
	if s.selectedIdx > 0 {
		s.selectedIdx--
		s.ensureVisible()
	}
}

// MoveDown moves the selection down
func (s *MCPSelectorState) MoveDown() {
	if s.selectedIdx < len(s.servers)-1 {
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

// Toggle toggles the connection state of the currently selected server
func (s *MCPSelectorState) Toggle() tea.Cmd {
	if len(s.servers) == 0 || s.selectedIdx >= len(s.servers) || s.connecting {
		return nil
	}

	selected := s.servers[s.selectedIdx]

	if selected.Status == mcp.StatusConnected {
		// Disconnect
		return func() tea.Msg {
			return MCPDisconnectMsg{ServerName: selected.Name}
		}
	}

	// Connect
	s.connecting = true
	return func() tea.Msg {
		return MCPConnectMsg{ServerName: selected.Name}
	}
}

// HandleConnectResult handles the result of a connection attempt
func (s *MCPSelectorState) HandleConnectResult(msg MCPConnectResultMsg) {
	s.connecting = false
	if msg.Success {
		s.lastError = ""
	} else if msg.Error != nil {
		s.lastError = fmt.Sprintf("Failed to connect: %v", msg.Error)
	}
	s.refreshServers()
}

// mcpStatusIconAndStyle returns the status icon and style for an MCP server status
func mcpStatusIconAndStyle(status mcp.ServerStatus) (string, lipgloss.Style) {
	icon, _ := mcpStatusDisplay(status)
	switch status {
	case mcp.StatusConnected:
		return icon, selectorStatusConnected
	case mcp.StatusConnecting:
		return icon, selectorStatusReady
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

// HandleDisconnect handles a disconnect request
func (s *MCPSelectorState) HandleDisconnect(name string) {
	if mcp.DefaultRegistry != nil {
		mcp.DefaultRegistry.Disconnect(name)
	}
	s.refreshServers()
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

	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
	case tea.KeyDown, tea.KeyCtrlN:
		s.MoveDown()
	case tea.KeyEnter, tea.KeySpace:
		return s.Toggle()
	case tea.KeyEsc:
		s.Cancel()
		return func() tea.Msg { return MCPSelectorCancelledMsg{} }
	case tea.KeyRunes:
		// vim-style navigation
		switch key.String() {
		case "j":
			s.MoveDown()
		case "k":
			s.MoveUp()
		}
	}
	return nil
}

// Render renders the MCP selector
func (s *MCPSelectorState) Render() string {
	if !s.active {
		return ""
	}

	var sb strings.Builder
	boxWidth := calculateToolBoxWidth(s.width)
	descStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Muted)

	sb.WriteString(selectorTitleStyle.Render(fmt.Sprintf("MCP Servers (%d)", len(s.servers))))
	sb.WriteString("\n\n")

	if len(s.servers) == 0 {
		sb.WriteString(selectorHintStyle.Render("  No MCP servers configured\n\n"))
		sb.WriteString(selectorHintStyle.Render("  Add servers with:\n"))
		sb.WriteString(selectorHintStyle.Render("    gen mcp add <name> -- <command>\n"))
	} else {
		endIdx := min(s.scrollOffset+s.maxVisible, len(s.servers))

		if s.scrollOffset > 0 {
			sb.WriteString(selectorHintStyle.Render("  ↑ more above\n"))
		}

		for i := s.scrollOffset; i < endIdx; i++ {
			srv := s.servers[i]
			icon, style := mcpStatusIconAndStyle(srv.Status)

			details := s.serverDetails(srv)
			line := fmt.Sprintf("%s %-20s %s  %s",
				style.Render(icon),
				srv.Name,
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

		if endIdx < len(s.servers) {
			sb.WriteString(selectorHintStyle.Render("  ↓ more below\n"))
		}
	}

	if s.lastError != "" {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		sb.WriteString(errorStyle.Render("    ✗ " + s.lastError + "\n"))
	}

	sb.WriteString("\n")
	if s.connecting {
		sb.WriteString(selectorHintStyle.Render("Connecting... (Esc to cancel)"))
	} else {
		sb.WriteString(selectorHintStyle.Render("↑/↓ navigate · Enter connect/disconnect · Esc close"))
	}

	box := selectorBorderStyle.Width(boxWidth).Render(sb.String())
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
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
