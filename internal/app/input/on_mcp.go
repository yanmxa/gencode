// MCP server selector: model, state, runtime, and keyboard handling.
package input

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/core"
	coremcp "github.com/yanmxa/gencode/internal/mcp"
)

// ── State ───────────────────────────────────────────────────────────

// MCPState holds all MCP-related state for the TUI model.
type MCPState struct {
	Selector      MCPSelector
	EditingFile   string        // temp file being edited
	EditingServer string        // server name being edited
	EditingScope  coremcp.Scope // scope of the server being edited
}

// MCPEditorFinishedMsg is sent when the external MCP config editor closes.
type MCPEditorFinishedMsg struct {
	Err error
}

// ── Private types ───────────────────────────────────────────────────

// mcpSelectorLevel represents the navigation level in the MCP selector
type mcpSelectorLevel int

const (
	mcpLevelList   mcpSelectorLevel = iota // Server list view
	mcpLevelDetail                         // Server detail + actions view
)

// mcpAction represents an action available for a server in detail view.
type mcpAction struct {
	Label  string
	Action string // "edit", "connect", "disconnect", "reconnect", "remove"
}

// mcpServerItem represents an MCP server in the selector
type mcpServerItem struct {
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

// ── Model ───────────────────────────────────────────────────────────

// MCPSelector holds state for the MCP server selector
type MCPSelector struct {
	registry *coremcp.Registry

	active          bool
	servers         []mcpServerItem
	filteredServers []mcpServerItem
	nav             kit.ListNav
	width           int
	height          int
	connecting      bool   // True when a connection is in progress
	lastError       string // Last connection error to display

	// Two-level navigation
	level        mcpSelectorLevel
	parentIdx    int            // selected index when entering detail
	detailServer *mcpServerItem // server shown in detail view
	actions      []mcpAction    // context-sensitive action menu
	actionIdx    int            // selected action
}

// ── Message types ───────────────────────────────────────────────────

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

// MCPAddServerMsg is sent when the user presses "n" to add a new server
type MCPAddServerMsg struct{}

// MCPEditServerMsg is sent when the user chooses to edit a server's config
type MCPEditServerMsg struct {
	ServerName string
	Scope      string
}

// ── Constructor ─────────────────────────────────────────────────────

// NewMCPSelector creates a new MCPSelector with the given MCP registry.
func NewMCPSelector(reg *coremcp.Registry) MCPSelector {
	return MCPSelector{
		registry: reg,
		active:   false,
		servers:  []mcpServerItem{},
		nav:      kit.ListNav{MaxVisible: 10},
	}
}

// ── Model methods ───────────────────────────────────────────────────

// EnterSelect enters MCP server selection mode
func (s *MCPSelector) EnterSelect(width, height int) error {
	if s.registry == nil {
		return fmt.Errorf("MCP is not initialized")
	}

	s.refreshServers()
	s.active = true
	s.width = width
	s.height = height
	s.connecting = false
	s.lastError = ""
	s.nav.Reset()
	s.nav.Total = len(s.filteredServers)
	s.level = mcpLevelList
	s.parentIdx = 0
	s.detailServer = nil
	s.actions = nil
	s.actionIdx = 0

	return nil
}

// AutoReconnect returns a batch command to reconnect servers in error state.
// Disconnected servers are left as-is since the user intentionally disconnected them.
func (s *MCPSelector) AutoReconnect() tea.Cmd {
	var cmds []tea.Cmd
	for _, srv := range s.servers {
		if srv.Status == coremcp.StatusError {
			s.registry.SetConnecting(srv.Name, true)
			cmds = append(cmds, mcpStartConnect(s.registry, srv.Name))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// refreshServers refreshes the server list from registry
func (s *MCPSelector) refreshServers() {
	servers := s.registry.List()
	s.servers = make([]mcpServerItem, 0, len(servers))

	for _, srv := range servers {
		item := mcpServerItem{
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
func (s *MCPSelector) IsActive() bool {
	return s.active
}

// Cancel cancels the selector
func (s *MCPSelector) Cancel() {
	s.active = false
	s.servers = []mcpServerItem{}
	s.filteredServers = nil
	s.nav.Reset()
	s.nav.Total = 0
	s.connecting = false
	s.level = mcpLevelList
	s.detailServer = nil
	s.actions = nil
	s.actionIdx = 0
}

// updateFilter filters servers based on search query (fuzzy match)
func (s *MCPSelector) updateFilter() {
	if s.nav.Search == "" {
		s.filteredServers = s.servers
	} else {
		query := strings.ToLower(s.nav.Search)
		s.filteredServers = make([]mcpServerItem, 0)
		for _, srv := range s.servers {
			if kit.FuzzyMatch(strings.ToLower(srv.Name), query) ||
				kit.FuzzyMatch(strings.ToLower(srv.Type), query) {
				s.filteredServers = append(s.filteredServers, srv)
			}
		}
	}
	s.nav.ResetCursor()
	s.nav.Total = len(s.filteredServers)
}

// MoveUp moves the selection up (level-aware)
func (s *MCPSelector) MoveUp() {
	if s.level == mcpLevelDetail {
		if s.actionIdx > 0 {
			s.actionIdx--
		}
		return
	}
	s.nav.MoveUp()
}

// MoveDown moves the selection down (level-aware)
func (s *MCPSelector) MoveDown() {
	if s.level == mcpLevelDetail {
		if s.actionIdx < len(s.actions)-1 {
			s.actionIdx++
		}
		return
	}
	s.nav.MoveDown()
}

// enterDetail enters the detail view for the selected server
func (s *MCPSelector) enterDetail() {
	if len(s.filteredServers) == 0 || s.nav.Selected >= len(s.filteredServers) {
		return
	}
	s.parentIdx = s.nav.Selected
	srv := s.filteredServers[s.nav.Selected]
	s.detailServer = &srv
	s.actions = s.buildActions(srv)
	s.actionIdx = 0
	s.level = mcpLevelDetail
}

// goBack returns to the list view from detail view
func (s *MCPSelector) goBack() bool {
	if s.level == mcpLevelDetail {
		s.level = mcpLevelList
		s.nav.Selected = s.parentIdx
		s.detailServer = nil
		s.actions = nil
		s.actionIdx = 0
		s.lastError = ""
		return true
	}
	return false
}

// buildActions returns context-sensitive actions for a server
func (s *MCPSelector) buildActions(srv mcpServerItem) []mcpAction {
	edit := mcpAction{Label: "Edit", Action: "edit"}
	switch srv.Status {
	case coremcp.StatusConnected:
		return []mcpAction{
			edit,
			{Label: "Disable", Action: "disconnect"},
			{Label: "Reconnect", Action: "reconnect"},
			{Label: "Remove", Action: "remove"},
		}
	case coremcp.StatusConnecting:
		return []mcpAction{
			edit,
			{Label: "Disable", Action: "disconnect"},
			{Label: "Remove", Action: "remove"},
		}
	default: // Error or Disconnected
		return []mcpAction{
			edit,
			{Label: "Connect", Action: "connect"},
			{Label: "Remove", Action: "remove"},
		}
	}
}

// executeAction executes the currently selected action in detail view
func (s *MCPSelector) executeAction() tea.Cmd {
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
			return MCPEditServerMsg{ServerName: name, Scope: scope}
		}
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
func (s *MCPSelector) HandleConnectResult(msg MCPConnectResultMsg) {
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
func (s *MCPSelector) HandleDisconnect(name string) {
	if s.registry != nil {
		_ = s.registry.Disconnect(name)
		s.registry.SetDisabled(name, true)
	}
	s.refreshAndUpdateView()
}

// HandleReconnect handles a reconnect request.
func (s *MCPSelector) HandleReconnect(name string) {
	if s.registry != nil {
		s.registry.Disconnect(name)
	}
	s.refreshAndUpdateView()
}

// HandleRemove handles a remove request
func (s *MCPSelector) HandleRemove(name string) {
	if s.registry != nil {
		s.registry.SetDisabled(name, false)
		s.registry.RemoveServer(name)
	}
	s.refreshServers()
	s.goBack()
	s.clampSelectedIdx()
}

// refreshAndUpdateView refreshes servers and updates the detail view if active
func (s *MCPSelector) refreshAndUpdateView() {
	s.refreshServers()
	if s.level == mcpLevelDetail && s.detailServer != nil {
		s.refreshDetailView()
	}
}

// clampSelectedIdx ensures selectedIdx is within valid bounds
func (s *MCPSelector) clampSelectedIdx() {
	if s.nav.Selected >= len(s.filteredServers) && len(s.filteredServers) > 0 {
		s.nav.Selected = len(s.filteredServers) - 1
	}
}

// refreshDetailView updates the detail server and actions after a state change
func (s *MCPSelector) refreshDetailView() {
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
func (s *MCPSelector) clampActionIdx() {
	if s.actionIdx >= len(s.actions) {
		s.actionIdx = len(s.actions) - 1
	}
	if s.actionIdx < 0 {
		s.actionIdx = 0
	}
}

// AutoConnect returns a batch of commands to connect all configured MCP servers,
// skipping servers that the user has explicitly disabled.
func (s *MCPSelector) AutoConnect() tea.Cmd {
	if s.registry == nil {
		return nil
	}
	var cmds []tea.Cmd
	for _, srv := range s.registry.List() {
		name := srv.Config.Name
		if !s.registry.IsDisabled(name) {
			s.registry.SetConnecting(name, true)
			cmds = append(cmds, mcpStartConnect(s.registry, name))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// mcpStartConnect returns a tea.Cmd that connects to an MCP server.
func mcpStartConnect(reg *coremcp.Registry, name string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if reg == nil {
			return MCPConnectResultMsg{
				ServerName: name,
				Success:    false,
				Error:      fmt.Errorf("MCP not initialized"),
			}
		}

		if err := reg.Connect(ctx, name); err != nil {
			return MCPConnectResultMsg{
				ServerName: name,
				Success:    false,
				Error:      err,
			}
		}

		toolCount := 0
		if client, ok := reg.GetClient(name); ok {
			toolCount = len(client.GetCachedTools())
		}

		return MCPConnectResultMsg{
			ServerName: name,
			Success:    true,
			ToolCount:  toolCount,
		}
	}
}

// ── Runtime interface ───────────────────────────────────────────────

// UpdateMCP routes MCP server management messages.
func UpdateMCP(deps OverlayDeps, state *MCPState, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case MCPConnectMsg:
		if state.Selector.registry != nil {
			state.Selector.registry.SetDisabled(msg.ServerName, false)
			state.Selector.registry.SetConnecting(msg.ServerName, true)
		}
		return mcpStartConnect(state.Selector.registry, msg.ServerName), true

	case MCPConnectResultMsg:
		if state.Selector.registry != nil {
			state.Selector.registry.SetConnecting(msg.ServerName, false)
			if !msg.Success && msg.Error != nil {
				state.Selector.registry.SetConnectError(msg.ServerName, msg.Error.Error())
			} else {
				state.Selector.registry.SetConnectError(msg.ServerName, "")
			}
		}
		state.Selector.HandleConnectResult(msg)
		if !state.Selector.IsActive() && !msg.Success {
			content := fmt.Sprintf("Failed to connect to '%s': %v", msg.ServerName, msg.Error)
			deps.Conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: content})
			return tea.Batch(deps.CommitMessages()...), true
		}
		return nil, true

	case MCPDisconnectMsg:
		state.Selector.HandleDisconnect(msg.ServerName)
		return nil, true

	case MCPReconnectMsg:
		state.Selector.HandleReconnect(msg.ServerName)
		if state.Selector.registry != nil {
			state.Selector.registry.SetConnecting(msg.ServerName, true)
		}
		return mcpStartConnect(state.Selector.registry, msg.ServerName), true

	case MCPRemoveMsg:
		state.Selector.HandleRemove(msg.ServerName)
		return nil, true

	case MCPAddServerMsg:
		deps.State.Textarea.SetValue("/mcp add ")
		return nil, true

	case MCPEditServerMsg:
		info, err := coremcp.PrepareServerEdit(state.Selector.registry, msg.ServerName)
		if err != nil {
			deps.Conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: fmt.Sprintf("Error: %v", err)})
			return tea.Batch(deps.CommitMessages()...), true
		}
		state.EditingFile = info.TempFile
		state.EditingServer = info.ServerName
		state.EditingScope = info.Scope
		return StartMCPEditor(info.TempFile), true

	case MCPEditorFinishedMsg:
		info := &coremcp.EditInfo{
			TempFile:   state.EditingFile,
			ServerName: state.EditingServer,
			Scope:      state.EditingScope,
		}
		state.EditingFile, state.EditingServer, state.EditingScope = "", "", ""

		if msg.Err != nil {
			os.Remove(info.TempFile)
			deps.Conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: fmt.Sprintf("Editor error: %v", msg.Err)})
			return tea.Batch(deps.CommitMessages()...), true
		}

		if err := coremcp.ApplyServerEdit(state.Selector.registry, info); err != nil {
			deps.Conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: fmt.Sprintf("Failed to apply edit: %v", err)})
			return tea.Batch(deps.CommitMessages()...), true
		}

		deps.Conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: fmt.Sprintf("Updated MCP server '%s'", info.ServerName)})
		return tea.Batch(deps.CommitMessages()...), true
	}
	return nil, false
}

// StartMCPEditor launches the external editor for an MCP config file.
// Exported for use by command handlers in the parent app package.
func StartMCPEditor(filePath string) tea.Cmd {
	return kit.StartExternalEditor(filePath, func(err error) tea.Msg {
		return MCPEditorFinishedMsg{Err: err}
	})
}

// ── Keyboard handling ───────────────────────────────────────────────

// HandleKeypress handles a keypress and returns a command if needed
func (s *MCPSelector) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	// Only allow escape while connecting
	if s.connecting {
		if key.Type == tea.KeyEsc {
			s.Cancel()
			return func() tea.Msg { return kit.DismissedMsg{} }
		}
		return nil
	}

	// Detail view keypress handling
	if s.level == mcpLevelDetail {
		return s.handleDetailKeypress(key)
	}

	// List view keypress handling
	return s.handleListKeypress(key)
}

// handleDetailKeypress handles keypresses in the detail view
func (s *MCPSelector) handleDetailKeypress(key tea.KeyMsg) tea.Cmd {
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
func (s *MCPSelector) handleListKeypress(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return nil
	case tea.KeyDown, tea.KeyCtrlJ:
		s.MoveDown()
		return nil
	case tea.KeyCtrlN:
		s.Cancel()
		return func() tea.Msg { return MCPAddServerMsg{} }
	case tea.KeyCtrlD:
		if len(s.filteredServers) > 0 && s.nav.Selected < len(s.filteredServers) {
			name := s.filteredServers[s.nav.Selected].Name
			return func() tea.Msg { return MCPRemoveMsg{ServerName: name} }
		}
		return nil
	case tea.KeyEnter, tea.KeyRight:
		s.enterDetail()
		return nil
	case tea.KeyEsc:
		if s.nav.Search != "" {
			s.nav.Search = ""
			s.updateFilter()
			return nil
		}
		s.Cancel()
		return func() tea.Msg { return kit.DismissedMsg{} }
	case tea.KeyBackspace:
		if len(s.nav.Search) > 0 {
			s.nav.Search = s.nav.Search[:len(s.nav.Search)-1]
			s.updateFilter()
		}
		return nil
	case tea.KeyRunes:
		r := key.String()
		// vim navigation when not searching
		if s.nav.Search == "" {
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
		s.nav.Search += r
		s.updateFilter()
		return nil
	}
	return nil
}

// HandleMCPCommand dispatches /mcp subcommands.
func HandleMCPCommand(ctx context.Context, selector *MCPSelector, width, height int, args string) (string, *coremcp.EditInfo, error) {
	if selector.registry == nil {
		return "MCP is not initialized.\n\nAdd MCP servers with:\n  /mcp add <name> -- <command> [args...]", nil, nil
	}

	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	if len(parts) == 0 {
		if err := selector.EnterSelect(width, height); err != nil {
			return "", nil, err
		}
		return "", nil, nil
	}

	subCmd := strings.ToLower(parts[0])
	var serverName string
	if len(parts) > 1 {
		serverName = parts[1]
	}

	switch subCmd {
	case "add":
		r, err := handleMCPAdd(selector.registry, ctx, parts[1:])
		return r, nil, err
	case "edit":
		return handleMCPEdit(selector.registry, serverName)
	case "remove":
		r, err := handleMCPRemove(selector.registry, serverName)
		return r, nil, err
	case "get":
		r, err := handleMCPGet(selector.registry, serverName)
		return r, nil, err
	case "connect":
		r, err := handleMCPConnect(selector.registry, ctx, serverName)
		return r, nil, err
	case "disconnect":
		r, err := handleMCPDisconnect(selector.registry, serverName)
		return r, nil, err
	case "reconnect":
		r, err := handleMCPReconnect(selector.registry, ctx, serverName)
		return r, nil, err
	case "list", "status":
		r, err := handleMCPList(selector.registry)
		return r, nil, err
	default:
		r, err := handleMCPConnect(selector.registry, ctx, subCmd)
		return r, nil, err
	}
}

func handleMCPList(reg *coremcp.Registry) (string, error) {
	servers := reg.List()

	if len(servers) == 0 {
		return "No MCP servers configured.\n\nAdd servers with:\n  /mcp add <name> -- <command> [args...]\n  /mcp add --transport http <name> <url>", nil
	}

	var sb strings.Builder
	sb.WriteString("MCP Servers:\n\n")

	for _, srv := range servers {
		icon, label := mcpStatusDisplay(srv.Status)
		scope := string(srv.Config.Scope)
		if scope == "" {
			scope = "local"
		}
		fmt.Fprintf(&sb, "  %s %s [%s] (%s, %s)\n", icon, srv.Config.Name, srv.Config.GetType(), scope, label)

		if srv.Status == coremcp.StatusConnected {
			if len(srv.Tools) > 0 {
				fmt.Fprintf(&sb, "    Tools: %d\n", len(srv.Tools))
			}
			if len(srv.Resources) > 0 {
				fmt.Fprintf(&sb, "    Resources: %d\n", len(srv.Resources))
			}
			if len(srv.Prompts) > 0 {
				fmt.Fprintf(&sb, "    Prompts: %d\n", len(srv.Prompts))
			}
		}

		if srv.Error != "" {
			fmt.Fprintf(&sb, "    Error: %s\n", srv.Error)
		}
	}

	sb.WriteString("\nCommands:\n")
	sb.WriteString("  /mcp add <name> ...     Add a server\n")
	sb.WriteString("  /mcp edit <name>        Edit server config in $EDITOR\n")
	sb.WriteString("  /mcp remove <name>      Remove a server\n")
	sb.WriteString("  /mcp get <name>         Show server details\n")
	sb.WriteString("  /mcp connect <name>     Connect to server\n")
	sb.WriteString("  /mcp disconnect <name>  Disconnect from server\n")
	sb.WriteString("  /mcp reconnect <name>   Reconnect to server\n")

	return sb.String(), nil
}

func handleMCPEdit(reg *coremcp.Registry, name string) (string, *coremcp.EditInfo, error) {
	if name == "" {
		return "Usage: /mcp edit <server-name>", nil, nil
	}
	info, err := coremcp.PrepareServerEdit(reg, name)
	if err != nil {
		return err.Error(), nil, nil
	}
	return "", info, nil
}

func handleMCPConnect(reg *coremcp.Registry, ctx context.Context, name string) (string, error) {
	if name == "" {
		return "Usage: /mcp connect <server-name>", nil
	}

	if _, ok := reg.GetConfig(name); !ok {
		return fmt.Sprintf("Server not found: %s\n\nUse /mcp list to see available servers.", name), nil
	}

	if err := reg.Connect(ctx, name); err != nil {
		return fmt.Sprintf("Failed to connect to %s: %v", name, err), nil
	}

	if client, ok := reg.GetClient(name); ok {
		tools := client.GetCachedTools()
		return fmt.Sprintf("Connected to %s\nTools available: %d", name, len(tools)), nil
	}

	return fmt.Sprintf("Connected to %s", name), nil
}

func handleMCPDisconnect(reg *coremcp.Registry, name string) (string, error) {
	if name == "" {
		return "Usage: /mcp disconnect <server-name>", nil
	}

	if err := reg.Disconnect(name); err != nil {
		return fmt.Sprintf("Failed to disconnect from %s: %v", name, err), nil
	}

	return fmt.Sprintf("Disconnected from %s", name), nil
}

func handleMCPAdd(reg *coremcp.Registry, ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return mcpAddUsage(), nil
	}

	var (
		transport  = "stdio"
		scope      = "local"
		envVars    []string
		headers    []string
		name       string
		positional []string
		dashIdx    = -1
	)

	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			dashIdx = i
			break
		}
		switch args[i] {
		case "--transport", "-t":
			if i+1 < len(args) {
				i++
				transport = args[i]
			}
		case "--scope", "-s":
			if i+1 < len(args) {
				i++
				scope = args[i]
			}
		case "--env", "-e":
			if i+1 < len(args) {
				i++
				envVars = append(envVars, args[i])
			}
		case "--header", "-H":
			if i+1 < len(args) {
				i++
				headers = append(headers, args[i])
			}
		default:
			positional = append(positional, args[i])
		}
	}

	if len(positional) == 0 {
		return mcpAddUsage(), nil
	}
	name = positional[0]

	var config coremcp.ServerConfig
	config.Type = coremcp.TransportType(transport)

	switch config.Type {
	case coremcp.TransportSTDIO:
		if dashIdx == -1 || dashIdx >= len(args)-1 {
			return "STDIO transport requires: /mcp add <name> -- <command> [args...]", nil
		}
		cmdArgs := args[dashIdx+1:]
		config.Command = cmdArgs[0]
		if len(cmdArgs) > 1 {
			config.Args = cmdArgs[1:]
		}

	case coremcp.TransportHTTP, coremcp.TransportSSE:
		if len(positional) < 2 {
			return fmt.Sprintf("%s transport requires a URL: /mcp add --transport %s <name> <url>", transport, transport), nil
		}
		config.URL = positional[1]
		config.Headers = coremcp.ParseKeyValues(headers, ":")

	default:
		return fmt.Sprintf("Unsupported transport type: %s (use stdio, http, or sse)", transport), nil
	}

	config.Env = coremcp.ParseKeyValues(envVars, "=")

	mcpScope := coremcp.ParseScope(scope)
	if err := reg.AddServer(name, config, mcpScope); err != nil {
		return fmt.Sprintf("Failed to add server: %v", err), nil
	}

	if err := reg.Connect(ctx, name); err != nil {
		return fmt.Sprintf("Added '%s' to %s scope, but failed to connect: %v", name, scope, err), nil
	}

	toolCount := 0
	if client, ok := reg.GetClient(name); ok {
		toolCount = len(client.GetCachedTools())
	}

	return fmt.Sprintf("Added and connected to '%s' (%s, %s scope)\nTools available: %d", name, transport, scope, toolCount), nil
}

func handleMCPRemove(reg *coremcp.Registry, name string) (string, error) {
	if name == "" {
		return "Usage: /mcp remove <server-name>", nil
	}

	if _, ok := reg.GetConfig(name); !ok {
		return fmt.Sprintf("Server not found: %s\n\nUse /mcp list to see available servers.", name), nil
	}

	if err := reg.RemoveServer(name); err != nil {
		return fmt.Sprintf("Failed to remove %s: %v", name, err), nil
	}

	return fmt.Sprintf("Removed server '%s'", name), nil
}

func handleMCPGet(reg *coremcp.Registry, name string) (string, error) {
	if name == "" {
		return "Usage: /mcp get <server-name>", nil
	}

	config, ok := reg.GetConfig(name)
	if !ok {
		return fmt.Sprintf("Server not found: %s\n\nUse /mcp list to see available servers.", name), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Server: %s\n", name)

	scope := string(config.Scope)
	if scope == "" {
		scope = "local"
	}
	fmt.Fprintf(&sb, "Scope:  %s\n", scope)
	fmt.Fprintf(&sb, "Type:   %s\n", config.GetType())

	switch config.GetType() {
	case coremcp.TransportSTDIO:
		cmd := config.Command
		if len(config.Args) > 0 {
			cmd += " " + strings.Join(config.Args, " ")
		}
		fmt.Fprintf(&sb, "Command: %s\n", cmd)
	case coremcp.TransportHTTP, coremcp.TransportSSE:
		fmt.Fprintf(&sb, "URL:    %s\n", config.URL)
	}

	if len(config.Env) > 0 {
		sb.WriteString("Env:\n")
		for k, v := range config.Env {
			masked := "***"
			if v == "" {
				masked = "(empty)"
			}
			fmt.Fprintf(&sb, "  %s=%s\n", k, masked)
		}
	}

	if len(config.Headers) > 0 {
		sb.WriteString("Headers:\n")
		for k, v := range config.Headers {
			masked := "***"
			if v == "" {
				masked = "(empty)"
			}
			fmt.Fprintf(&sb, "  %s: %s\n", k, masked)
		}
	}

	icon, label := mcpStatusDisplay(coremcp.StatusDisconnected)
	toolCount := 0
	if client, ok := reg.GetClient(name); ok {
		srv := client.ToServer()
		icon, label = mcpStatusDisplay(srv.Status)
		toolCount = len(srv.Tools)

		if srv.Error != "" {
			fmt.Fprintf(&sb, "Error:  %s\n", srv.Error)
		}
	}
	fmt.Fprintf(&sb, "Status: %s %s\n", icon, label)
	if toolCount > 0 {
		fmt.Fprintf(&sb, "Tools:  %d\n", toolCount)
	}

	return sb.String(), nil
}

func handleMCPReconnect(reg *coremcp.Registry, ctx context.Context, name string) (string, error) {
	if name == "" {
		return "Usage: /mcp reconnect <server-name>", nil
	}

	if _, ok := reg.GetConfig(name); !ok {
		return fmt.Sprintf("Server not found: %s\n\nUse /mcp list to see available servers.", name), nil
	}

	_ = reg.Disconnect(name)

	if err := reg.Connect(ctx, name); err != nil {
		return fmt.Sprintf("Failed to reconnect to %s: %v", name, err), nil
	}

	toolCount := 0
	if client, ok := reg.GetClient(name); ok {
		toolCount = len(client.GetCachedTools())
	}

	return fmt.Sprintf("Reconnected to %s\nTools available: %d", name, toolCount), nil
}

func mcpAddUsage() string {
	return `Usage: /mcp add [options] <name> [-- <command> [args...]] or <url>

Options:
  --transport <type>   Transport: stdio (default), http, sse
  --scope <scope>      Scope: local (default), project, user
  --env KEY=value      Environment variable (repeatable, STDIO only)
  --header Key:Value   HTTP header (repeatable, HTTP/SSE only)

Short flags: -t, -s, -e, -H

Examples:
  /mcp add myserver -- npx -y @modelcontextprotocol/server-filesystem .
  /mcp add --transport http pubmed https://pubmed.mcp.example.com/mcp
  /mcp add --transport http --scope project myapi https://api.example.com/mcp
  /mcp add --env API_KEY=xxx myserver -- npx -y some-mcp-server`
}

// mcpStatusDisplay returns icon and label for an MCP server status.
func mcpStatusDisplay(status coremcp.ServerStatus) (icon, label string) {
	switch status {
	case coremcp.StatusConnected:
		return "●", "connected"
	case coremcp.StatusConnecting:
		return "◌", "connecting"
	case coremcp.StatusError:
		return "✗", "error"
	default:
		return "○", "disconnected"
	}
}

// mcpStatusIconAndStyle returns the status icon and style for an MCP server status
func mcpStatusIconAndStyle(status coremcp.ServerStatus) (string, lipgloss.Style) {
	icon, _ := mcpStatusDisplay(status)
	switch status {
	case coremcp.StatusConnected:
		return icon, kit.SelectorStatusConnected()
	case coremcp.StatusConnecting:
		return icon, kit.SelectorStatusReady()
	case coremcp.StatusError:
		return icon, kit.SelectorStatusError()
	default:
		return icon, kit.SelectorStatusNone()
	}
}

// Render renders the MCP selector
func (s *MCPSelector) Render() string {
	if !s.active {
		return ""
	}

	if s.level == mcpLevelDetail {
		return s.renderDetail()
	}
	return s.renderList()
}

// renderErrorAndFooter appends the error message (if any) and footer hint to the builder
func (s *MCPSelector) renderErrorAndFooter(sb *strings.Builder, hint string) {
	if s.lastError != "" {
		sb.WriteString(kit.SelectorStatusError().Render("    ! " + s.lastError + "\n"))
	}
	sb.WriteString("\n")
	if s.connecting {
		sb.WriteString(kit.SelectorHintStyle().Render("Connecting... (Esc to cancel)"))
	} else {
		sb.WriteString(kit.SelectorHintStyle().Render(hint))
	}
}

// renderBox wraps content in a centered bordered box
func (s *MCPSelector) renderBox(content string) string {
	boxWidth := kit.CalculateToolBoxWidth(s.width)
	box := kit.SelectorBorderStyle().Width(boxWidth).Render(content)
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

// renderList renders the list view
func (s *MCPSelector) renderList() string {
	var sb strings.Builder
	descStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)

	// Title with filtered/total count
	title := fmt.Sprintf("MCP Servers (%d/%d)", len(s.filteredServers), len(s.servers))
	sb.WriteString(kit.SelectorTitleStyle().Render(title))
	sb.WriteString("\n")

	// Search input
	searchPrompt := ">> "
	if s.nav.Search == "" {
		sb.WriteString(kit.SelectorHintStyle().Render(searchPrompt + "Type to filter..."))
	} else {
		sb.WriteString(kit.SelectorBreadcrumbStyle().Render(searchPrompt + s.nav.Search + "|"))
	}
	sb.WriteString("\n\n")

	if len(s.filteredServers) == 0 {
		if len(s.servers) == 0 {
			sb.WriteString(kit.SelectorHintStyle().Render("  No MCP servers configured\n\n"))
			sb.WriteString(kit.SelectorHintStyle().Render("  Add servers with:\n"))
			sb.WriteString(kit.SelectorHintStyle().Render("    gen mcp add <name> -- <command>\n"))
		} else {
			sb.WriteString(kit.SelectorHintStyle().Render("  No servers match the filter"))
			sb.WriteString("\n")
		}
	} else {
		endIdx := min(s.nav.Scroll+s.nav.MaxVisible, len(s.filteredServers))

		if s.nav.Scroll > 0 {
			sb.WriteString(kit.SelectorHintStyle().Render("  ^ more above"))
			sb.WriteString("\n")
		}

		for i := s.nav.Scroll; i < endIdx; i++ {
			srv := s.filteredServers[i]
			icon, statusStyle := mcpStatusIconAndStyle(srv.Status)

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

			if i == s.nav.Selected {
				sb.WriteString(kit.SelectorSelectedStyle().Render("> " + line))
			} else {
				sb.WriteString(kit.SelectorItemStyle().Render("  " + line))
			}
			sb.WriteString("\n")
		}

		if endIdx < len(s.filteredServers) {
			sb.WriteString(kit.SelectorHintStyle().Render("  v more below"))
			sb.WriteString("\n")
		}
	}

	s.renderErrorAndFooter(&sb, "↑↓ navigate . Enter details . ^N add . ^D remove . Esc close")
	return s.renderBox(sb.String())
}

// renderDetail renders the detail view for a selected server
func (s *MCPSelector) renderDetail() string {
	if s.detailServer == nil {
		return s.renderList()
	}

	var sb strings.Builder
	boxWidth := kit.CalculateToolBoxWidth(s.width)
	srv := s.detailServer
	maxValueLen := boxWidth - 20

	labelStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
	valueStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextBright)

	// Title
	sb.WriteString(kit.SelectorTitleStyle().Render("MCP Server"))
	sb.WriteString("\n")

	// Server name breadcrumb
	sb.WriteString(kit.SelectorBreadcrumbStyle().Render("> " + srv.Name))
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
			valueStyle.Render(kit.TruncateText(srv.URL, maxValueLen)),
		)
	}
	if srv.Command != "" {
		cmd := srv.Command
		if len(srv.Args) > 0 {
			cmd += " " + strings.Join(srv.Args, " ")
		}
		fmt.Fprintf(&sb, "  %s  %s\n",
			labelStyle.Render("Cmd:   "),
			valueStyle.Render(kit.TruncateText(cmd, maxValueLen)),
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
			kit.SelectorStatusError().Render(srv.Error),
		)
	}

	sb.WriteString("\n")

	// Actions
	sb.WriteString(labelStyle.Render("  Actions:"))
	sb.WriteString("\n")
	for i, action := range s.actions {
		if i == s.actionIdx {
			sb.WriteString(kit.SelectorSelectedStyle().Render("> " + action.Label))
		} else {
			sb.WriteString(kit.SelectorItemStyle().Render("  " + action.Label))
		}
		sb.WriteString("\n")
	}

	s.renderErrorAndFooter(&sb, "↑↓ navigate . Enter execute . Esc back")
	return s.renderBox(sb.String())
}

// serverDetails returns the details string for a server item
func (s *MCPSelector) serverDetails(srv mcpServerItem) string {
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
