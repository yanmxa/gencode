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
# Client tests
TestClient_ConnectAndDisconnect     — connect and disconnect lifecycle
TestClient_ConnectIdempotent        — repeated connect is safe
TestClient_DisconnectIdempotent     — repeated disconnect is safe
TestClient_ListTools                — list tools from server
TestClient_CallTool                 — call a tool successfully
TestClient_CallTool_Error           — tool call error handling
TestClient_CallTool_NotConnected    — error when not connected
TestClient_Ping                     — ping server
TestClient_Ping_NotConnected        — ping error when not connected
TestClient_ListResources            — list resources from server
TestClient_ListPrompts              — list prompts from server
TestClient_ReadResource             — read a resource
TestClient_GetPrompt                — get a prompt
TestClient_JSONRPCError             — JSON-RPC error handling
TestClient_ToServer                 — client to server config

# Registry tests
TestRegistry_ListConfigs            — list all server configs
TestRegistry_GetConfig              — get specific server config
TestRegistry_GetClient_NotConnected — error when client not connected
TestRegistry_GetToolSchemas_Empty   — empty schemas when no servers
TestRegistry_CallTool_InvalidName   — invalid tool name error
TestRegistry_CallTool_NotConnected  — error when not connected
TestRegistry_DisconnectAll_Empty    — disconnect all is safe when empty
TestRegistry_OnToolsChanged         — tool change callback fires
TestRegistry_EndToEnd_ToolSchemas   — end-to-end schema retrieval

# Config tests
TestConfigLoader_RoundTrip          — save and load config
TestConfigLoader_ScopePriority      — scope priority order
TestConfigLoader_RemoveServer       — remove server from config
TestServerConfig_GetType            — server type detection
TestParseMCPToolName                — MCP tool name parsing
TestIsMCPTool                       — MCP tool detection
TestExpandEnv                       — env var expansion
TestExpandEnvSlice                  — env var expansion in slices
TestExpandEnvMap                    — env var expansion in maps
TestBuildEnv                        — build environment

# Resource listing
TestMCP_ResourceListing             — ListMcpResourcesTool returns resources

# Real MCP integration
TestRealMCP_Everything              — end-to-end with everything server
TestRealMCP_Filesystem              — end-to-end with filesystem server
TestRealMCP_Registry_EndToEnd       — registry end-to-end
```

Cases to add:

```go
func TestMCP_HTTP_Transport_Connect(t *testing.T) {
    // HTTP transport must connect and list tools correctly
}

func TestMCP_SSE_Transport_Connect(t *testing.T) {
    // SSE transport must connect and stream events correctly
}

func TestMCP_ScopeMerge_UserProjectLocal(t *testing.T) {
    // User + project + local configs must merge with correct priority
}

func TestMCP_ServerReconnect_AfterFailure(t *testing.T) {
    // Server must reconnect after a connection failure
}

func TestMCP_ConnectionError_ShownInline(t *testing.T) {
    // Connection errors must be reported inline at startup
}
```

## Interactive Tests (tmux)

```bash
# Requires Node.js
tmux new-session -d -s t_mcp -x 220 -y 60

# Test 1: Add STDIO server
tmux send-keys -t t_mcp 'gen mcp add filesystem -- npx -y @modelcontextprotocol/server-filesystem /tmp' Enter
sleep 5
tmux capture-pane -t t_mcp -p
# Expected: "filesystem" server added

# Test 2: List servers
tmux send-keys -t t_mcp 'gen mcp list' Enter
sleep 2
tmux capture-pane -t t_mcp -p
# Expected: "filesystem" listed with STDIO transport

# Test 3: Get server details
tmux send-keys -t t_mcp 'gen mcp get filesystem' Enter
sleep 2
tmux capture-pane -t t_mcp -p
# Expected: server config details shown

# Test 4: Use MCP tool from TUI
tmux send-keys -t t_mcp 'gen' Enter
sleep 2
tmux send-keys -t t_mcp 'list files in /tmp using the filesystem MCP tool' Enter
sleep 12
tmux capture-pane -t t_mcp -p
# Expected: /tmp listing via MCP server; MCP tool appears in the conversation flow

# Test 5: /mcp command — management panel
tmux send-keys -t t_mcp '/mcp' Enter
sleep 2
tmux capture-pane -t t_mcp -p
# Expected: MCP selector titled "MCP Servers" with the configured server listed

# Test 6: Connection error handling
tmux send-keys -t t_mcp C-c
tmux send-keys -t t_mcp 'gen mcp add broken -- nonexistent-command-xyz' Enter
sleep 3
tmux send-keys -t t_mcp 'gen' Enter
sleep 3
tmux capture-pane -t t_mcp -p
# Expected: connection error shown inline at startup

# Cleanup
tmux send-keys -t t_mcp C-c
tmux send-keys -t t_mcp 'gen mcp remove filesystem' Enter
sleep 2
tmux send-keys -t t_mcp 'gen mcp remove broken' Enter
sleep 2

tmux kill-session -t t_mcp
```
