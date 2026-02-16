# MCP Servers

MCP (Model Context Protocol) servers extend GenCode with external tools, resources, and prompts. GenCode connects to MCP servers via STDIO, HTTP, or SSE transports, giving the LLM access to databases, APIs, issue trackers, and more during conversations.

## Configuration Files

MCP server configurations are stored in JSON files at three scope levels, loaded in priority order (later overrides earlier for same-named servers):

| Scope | File Path | Git | Purpose |
|-------|-----------|-----|---------|
| User | `~/.gen/mcp.json` | N/A | Personal servers shared across all projects |
| Project | `.gen/mcp.json` | Committed | Team-shared servers for this project |
| Local | `.gen/mcp.local.json` | Ignored | Personal servers with credentials for this project |

All files share the same format:

```json
{
  "mcpServers": {
    "server-name": {
      "type": "http",
      "url": "https://example.com/mcp"
    },
    "another-server": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "."],
      "env": {
        "API_KEY": "your-key"
      }
    }
  }
}
```

### Scope Selection Guide

- **User** (`--scope user`): Servers you use across all projects (e.g., GitHub, Sentry). Stored in your home directory.
- **Project** (`--scope project`): Servers the whole team needs. Committed to git so collaborators get them automatically.
- **Local** (`--scope local`, default): Servers with personal credentials or experimental configs. Never committed to git.

### Precedence

When the same server name exists in multiple scopes, local > project > user. This lets you override a team-shared server with personal settings.

## Adding Servers

### CLI

```bash
# STDIO server (default transport)
gen mcp add <name> -- <command> [args...]

# HTTP server
gen mcp add --transport http <name> <url>

# SSE server
gen mcp add --transport sse <name> <url>

# With scope
gen mcp add --transport http --scope project <name> <url>

# With environment variables (STDIO)
gen mcp add --env API_KEY=xxx <name> -- npx -y some-server

# With HTTP headers
gen mcp add --transport http --header "Authorization:Bearer tok" <name> <url>

# From JSON
gen mcp add-json <name> '{"type":"http","url":"https://example.com/mcp"}'
```

### TUI (`/mcp add`)

Same syntax as CLI, within the TUI:

```
/mcp add myserver -- npx -y @modelcontextprotocol/server-filesystem .
/mcp add --transport http pubmed https://pubmed.mcp.claude.com/mcp
/mcp add --transport http --scope project myapi https://api.example.com/mcp
/mcp add --env API_KEY=xxx myserver -- npx -y some-mcp-server
```

### Manual Edit

Create or edit the JSON file directly at the appropriate scope path.

### Options Reference

| Option | Short | Description | Applies To |
|--------|-------|-------------|------------|
| `--transport <type>` | `-t` | Transport: `stdio` (default), `http`, `sse` | All |
| `--scope <scope>` | `-s` | Scope: `local` (default), `project`, `user` | All |
| `--env KEY=value` | `-e` | Environment variable (repeatable) | STDIO |
| `--header Key:Value` | `-H` | HTTP header (repeatable) | HTTP/SSE |

For STDIO servers, use `--` to separate GenCode flags from the server command:

```
gen mcp add --env KEY=val myserver -- npx server --port 8080
                                   ^^
                        separator: everything after this is the server command
```

## Managing Servers

### CLI

```bash
gen mcp list              # List all configured servers
gen mcp get <name>        # Show server config details
gen mcp remove <name>     # Remove from all scopes
```

### TUI — Interactive Selector (`/mcp`)

The `/mcp` command opens a two-level management UI:

**List View** — shows all servers with status, type, and tool count.

- Type to fuzzy-search servers by name or type
- `Enter` / `Right` / `l` — open server detail view
- `Ctrl+N` — add a new server (pre-fills `/mcp add `)
- `Esc` — clear search, or close selector
- `j` / `k` — vim-style navigation (when not searching)

**Detail View** — shows full server info (status, type, scope, URL/command, tools, errors) and context-sensitive actions:

| Server Status | Available Actions |
|---|---|
| Connected | Disable, Reconnect, Remove |
| Connecting | Disable, Remove |
| Error | Connect, Remove |
| Disabled | Connect, Remove |

- `Enter` — execute selected action
- `Left` / `Esc` / `h` — back to list

### TUI — Commands

```
/mcp                      # Interactive server selector
/mcp list                 # List servers with scope and status
/mcp get <name>           # Show config, status, tool count
/mcp connect <name>       # Connect to a server
/mcp disconnect <name>    # Disconnect from a server
/mcp reconnect <name>     # Disconnect then reconnect
/mcp remove <name>        # Remove from all scopes and disconnect
```

`/mcp add` auto-connects after adding. `/mcp remove` auto-disconnects before removing.

### Connection Lifecycle

- **Startup**: All configured servers auto-connect, except those explicitly disabled by the user.
- **Error recovery**: Servers in `error` state auto-reconnect when the `/mcp` selector is opened.
- **Disable**: Marking a server as "Disable" persists across restarts — the server won't auto-connect until explicitly re-enabled via "Connect".
- **State persistence**: Disabled server state is stored in `.gen/mcp-state.json`.

## Transport Types

### STDIO

Runs the server as a local child process. Communication over stdin/stdout.

```json
{
  "command": "npx",
  "args": ["-y", "@modelcontextprotocol/server-filesystem", "."],
  "env": { "HOME": "/workspace" }
}
```

Best for: local tools, file system access, custom scripts.

### HTTP (Streamable HTTP)

Connects to a remote server over HTTP. Recommended for cloud services.

```json
{
  "type": "http",
  "url": "https://pubmed.mcp.claude.com/mcp",
  "headers": { "Authorization": "Bearer token" }
}
```

Best for: cloud APIs, shared services, remote tools.

### SSE (Server-Sent Events)

Connects to a remote server using SSE transport. Considered legacy; prefer HTTP where available.

```json
{
  "type": "sse",
  "url": "https://mcp.example.com/sse"
}
```

## Server Config Schema

```json
{
  "type": "stdio | http | sse",

  "command": "string",
  "args": ["string"],
  "env": { "KEY": "value" },

  "url": "string",
  "headers": { "Key": "Value" }
}
```

- `type`: Transport type. Omit for auto-detection (`url` present → `http`, otherwise → `stdio`).
- `command` + `args`: STDIO server executable and arguments.
- `env`: Environment variables passed to the STDIO process.
- `url`: HTTP/SSE server endpoint.
- `headers`: HTTP headers sent with every request.

## Examples

### PubMed (HTTP, project scope)

```bash
gen mcp add --transport http --scope project pubmed https://pubmed.mcp.claude.com/mcp
```

Team members clone the repo and get PubMed tools automatically.

### GitHub (HTTP, user scope)

```bash
gen mcp add --transport http --scope user github https://api.githubcopilot.com/mcp/
```

Available across all your projects. Authenticate via `/mcp` in TUI.

### Filesystem (STDIO, local scope)

```bash
gen mcp add filesystem -- npx -y @modelcontextprotocol/server-filesystem /path/to/dir
```

Local-only, gives the LLM read/write access to a directory.

### Database with credentials (STDIO, local scope)

```bash
gen mcp add --env DB_URL=postgresql://user:pass@host:5432/db dbtools -- npx -y @bytebase/dbhub
```

Credentials stay in `.gen/mcp.local.json`, never committed.

## Architecture

```
internal/mcp/
├── types.go       # ServerConfig, Server, Scope, TransportType, etc.
├── config.go      # ConfigLoader: load/save/remove from JSON files
├── registry.go    # Registry: manages configs, live clients, disabled state
├── client.go      # Client: single MCP server connection lifecycle
└── transport/     # STDIO, HTTP, SSE transport implementations

cmd/gen/mcp.go     # CLI subcommands (add, add-json, list, get, remove)
internal/tui/
├── commands.go    # TUI /mcp command handlers
└── mcpselector.go # Interactive two-level MCP server selector UI
```

The `Registry` holds configs (from disk), live clients (in memory), and disabled state (persisted to `.gen/mcp-state.json`). Adding a server writes to disk and updates the in-memory config. Connecting creates a `Client` that establishes a transport, initializes the MCP protocol, and caches the server's tool/resource/prompt schemas. Disabling a server persists across restarts, preventing auto-connect until the user explicitly re-enables it.
