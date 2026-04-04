# Feature 11: MCP System

## Overview

MCP (Model Context Protocol) connects gencode to external tool servers over STDIO, HTTP, or SSE transports. MCP tools appear alongside built-in tools in the LLM's tool list.

**Transport types:**

| Type | Config |
|------|--------|
| STDIO | Local subprocess |
| HTTP | REST endpoint |
| SSE | Server-Sent Events |

**Config scopes:**
- `~/.gen/mcp.json` — user-level
- `./.gen/mcp.json` — project-level
- `./.gen/mcp.local.json` — local (git-ignored)

**CLI commands:**

```bash
gen mcp add <name> -- <command>              # STDIO
gen mcp add --transport http <name> <url>    # HTTP
gen mcp list
gen mcp get <name>
gen mcp edit <name>
gen mcp remove <name>
```

## UI Interactions

- **`/mcp`**: opens the MCP management panel; shows connected servers and their tools.
- **Tool calls**: MCP tools appear in the same permission dialog as built-in tools.
- **Connection errors**: shown inline when a server fails to connect at startup.

## Automated Tests

```bash
go test ./internal/mcp/... -v
go test ./tests/integration/mcp/... -v
```

Covered:

```
TestMCP_ConfigLoad
TestMCP_ScopeMerge
TestMCP_Registry_Connect
TestMCP_Registry_ListTools
TestMCP_STDIO_Transport
TestMCP_STDIO_JsonRPC
TestMCP_Integration_STDIO_Server
TestMCP_Integration_ToolExecution
```

Cases to add:

```go
func TestMCP_HTTP_Transport_Connect(t *testing.T) {
    // HTTP transport must connect and list tools correctly
}

func TestMCP_ResourceListing(t *testing.T) {
    // ListMcpResourcesTool must return resources from connected server
}
```

## Interactive Tests (tmux)

```bash
# Requires Node.js
tmux new-session -d -s t_mcp -x 220 -y 60

# Add STDIO server
tmux send-keys -t t_mcp 'gen mcp add filesystem -- npx -y @modelcontextprotocol/server-filesystem /tmp' Enter
sleep 5
tmux capture-pane -t t_mcp -p
# Expected: "filesystem" server added

# List servers
tmux send-keys -t t_mcp 'gen mcp list' Enter
sleep 2
tmux capture-pane -t t_mcp -p
# Expected: "filesystem" listed with STDIO transport

# Use MCP from TUI
tmux send-keys -t t_mcp 'gen' Enter
sleep 2
tmux send-keys -t t_mcp 'list files in /tmp using the filesystem MCP tool' Enter
sleep 12
tmux capture-pane -t t_mcp -p
# Expected: /tmp listing via MCP server

# /mcp command
tmux send-keys -t t_mcp '/mcp' Enter
sleep 2
tmux capture-pane -t t_mcp -p
# Expected: MCP management UI with configured servers

# Cleanup
tmux send-keys -t t_mcp 'q' Enter
tmux send-keys -t t_mcp 'gen mcp remove filesystem' Enter
sleep 2

tmux kill-session -t t_mcp
```
