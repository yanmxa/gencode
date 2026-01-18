# MCP Integration

GenCode supports the Model Context Protocol (MCP), allowing you to extend the agent with external tools and data sources through MCP servers. The configuration format is compatible with Claude Code.

## What is MCP?

Model Context Protocol (MCP) is an open standard for connecting AI agents to external tools, data sources, and services. MCP servers can provide:

- **Tools**: Functions the agent can call (e.g., GitHub API, database queries)
- **Resources**: Data sources the agent can read (e.g., files, web content)
- **Prompts**: Pre-defined prompt templates

GenCode currently supports **Tools** with Resources and Prompts planned for future releases.

## Quick Start

### Local MCP Server (Stdio)

Create `.gen/.mcp.json` in your project:

```json
{
  "mcpServers": {
    "filesystem": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/dir"]
    }
  }
}
```

Start GenCode and the filesystem server will automatically connect:

```bash
gencode
```

The agent now has access to MCP tools like `mcp_filesystem_read_file`, `mcp_filesystem_list_directory`, etc.

### Remote MCP Server (HTTP)

Create `~/.gen/.mcp.json` for user-wide access:

```json
{
  "mcpServers": {
    "github": {
      "type": "http",
      "url": "https://mcp.github.com",
      "headers": {
        "Authorization": "Bearer ${GITHUB_TOKEN}"
      }
    }
  }
}
```

Set your token:

```bash
export GITHUB_TOKEN="ghp_your_token_here"
gencode
```

## Configuration

### Configuration Scopes

MCP servers can be configured at multiple levels with fallback:

| Priority | Scope | Location | Purpose |
|----------|-------|----------|---------|
| 1 | Managed (gen) | `/Library/Application Support/GenCode/managed-mcp.json` | System-wide enforced |
| 2 | Managed (claude) | `/Library/Application Support/ClaudeCode/managed-mcp.json` | Claude Code managed |
| 3 | Local (gen) | `~/.gen.json` (under project) | Personal project config |
| 4 | Local (claude) | `~/.claude.json` (under project) | Claude Code local config |
| 5 | Project (gen) | `.gen/.mcp.json` | Shared with team (version control) |
| 6 | Project (claude) | `.claude/.mcp.json` | Claude Code project config |
| 7 | User (gen) | `~/.gen/.mcp.json` | Available across all projects |
| 8 | User (claude) | `~/.claude/.mcp.json` | Claude Code user config |

**Note**: First match wins. If a server is defined in multiple locations, the highest priority configuration is used.

### Server Types

#### Stdio (Local Processes)

Run a local MCP server as a subprocess:

```json
{
  "type": "stdio",
  "command": "/path/to/server",
  "args": ["--config", "config.json"],
  "env": {
    "API_KEY": "${API_KEY}"
  }
}
```

**Fields**:
- `command`: Executable path or command name
- `args`: Command arguments (optional)
- `env`: Environment variables (optional)
- `enabled`: Set to `false` to disable (optional)

#### HTTP (Remote Servers - Recommended)

Connect to a remote MCP server via HTTP:

```json
{
  "type": "http",
  "url": "https://api.example.com/mcp",
  "headers": {
    "Authorization": "Bearer ${API_TOKEN}",
    "X-Custom-Header": "value"
  }
}
```

**Fields**:
- `url`: Server URL
- `headers`: HTTP headers (optional)
- `enabled`: Set to `false` to disable (optional)

**Transport Fallback**: GenCode tries StreamableHTTP first, then falls back to SSE if the server doesn't support it.

#### SSE (Server-Sent Events - Legacy)

Connect via Server-Sent Events:

```json
{
  "type": "sse",
  "url": "https://mcp.example.com/sse",
  "headers": {
    "Authorization": "Bearer ${API_TOKEN}"
  }
}
```

**Fields**: Same as HTTP.

### Environment Variables

Use `${VAR}` or `${VAR:-default}` syntax for environment variable expansion:

```json
{
  "mcpServers": {
    "api": {
      "type": "http",
      "url": "${API_URL:-https://api.example.com}/mcp",
      "headers": {
        "Authorization": "Bearer ${API_KEY}"
      }
    }
  }
}
```

**Supported Syntax**:
- `${VAR}` - Replace with environment variable value, empty string if not set
- `${VAR:-default}` - Replace with environment variable value, or default if not set

**Expansion Locations**:
- `command`, `args`, `env` (stdio)
- `url`, `headers` (HTTP/SSE)

## Tool Namespacing

MCP tools are namespaced to avoid conflicts:

```
Original MCP tool: "create_issue"
From server: "github"
GenCode tool name: "mcp_github_create_issue"
```

The agent sees MCP tools alongside built-in tools and can call them naturally:

```
> Create an issue in the GitHub repo titled "Bug fix"
```

The agent will use `mcp_github_create_issue` if the GitHub MCP server is connected.

## Authentication

### OAuth 2.0 (Remote Servers)

Some remote MCP servers require OAuth authentication:

1. Configure server with OAuth endpoint
2. Run OAuth flow (opens browser)
3. Tokens are stored securely in `~/.gen/mcp-auth.json` (0600 permissions)
4. Tokens are automatically refreshed when expired

**OAuth Flow Example**:

```bash
# Start gencode (will prompt for auth if needed)
gencode

# Or trigger auth manually
# TODO: Add CLI commands for auth
```

### API Tokens (Simple Auth)

For simpler authentication, use environment variables:

```json
{
  "mcpServers": {
    "api": {
      "type": "http",
      "url": "https://api.example.com/mcp",
      "headers": {
        "Authorization": "Bearer ${API_TOKEN}"
      }
    }
  }
}
```

```bash
export API_TOKEN="your_token_here"
gencode
```

## Available MCP Servers

Official MCP servers from Anthropic and community:

### Official Servers

- **@modelcontextprotocol/server-filesystem** - Local file operations
- **@modelcontextprotocol/server-github** - GitHub API integration
- **@modelcontextprotocol/server-gitlab** - GitLab API integration
- **@modelcontextprotocol/server-google-drive** - Google Drive access
- **@modelcontextprotocol/server-postgres** - PostgreSQL database queries
- **@modelcontextprotocol/server-sqlite** - SQLite database queries

### Community Servers

See the [MCP Registry](https://github.com/modelcontextprotocol/servers) for a full list of community MCP servers.

## Examples

### Filesystem Server

```json
{
  "mcpServers": {
    "filesystem": {
      "type": "stdio",
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-filesystem",
        "/Users/you/projects"
      ]
    }
  }
}
```

**Tools provided**:
- `mcp_filesystem_read_file` - Read file contents
- `mcp_filesystem_write_file` - Write file contents
- `mcp_filesystem_list_directory` - List directory contents

### GitHub Server

```json
{
  "mcpServers": {
    "github": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_TOKEN": "${GITHUB_TOKEN}"
      }
    }
  }
}
```

**Tools provided**:
- `mcp_github_create_issue` - Create GitHub issue
- `mcp_github_create_pull_request` - Create pull request
- `mcp_github_search_repositories` - Search repositories
- And more...

### PostgreSQL Server

```json
{
  "mcpServers": {
    "postgres": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-postgres"],
      "env": {
        "POSTGRES_CONNECTION_STRING": "${DATABASE_URL}"
      }
    }
  }
}
```

**Tools provided**:
- `mcp_postgres_query` - Execute SQL queries
- `mcp_postgres_list_tables` - List database tables
- `mcp_postgres_describe_table` - Describe table schema

## Permission System

MCP tools go through the same permission system as built-in tools. You can allow/deny MCP tools by pattern:

```json
{
  "permissions": {
    "allow": [
      "mcp_github_*",        // Allow all GitHub MCP tools
      "mcp_*_read_*"         // Allow all read operations
    ],
    "deny": [
      "mcp_*_delete_*"       // Deny all delete operations
    ]
  }
}
```

## Troubleshooting

### Server Won't Connect

1. **Check configuration syntax** - Ensure JSON is valid
2. **Verify command exists** - For stdio, ensure command is in PATH
3. **Check environment variables** - Ensure all required env vars are set
4. **Review logs** - Look for error messages in debug output

Enable debug logging:

```bash
DEBUG=mcp:* gencode
```

### Tools Not Appearing

1. **Server connected?** - Check server status
2. **Enabled?** - Ensure `enabled` is not `false` in config
3. **Permissions?** - Check if tools are blocked by permission rules

### OAuth Issues

1. **Callback server blocked?** - Ensure port 19876 is available
2. **Browser didn't open?** - Manually visit the URL shown
3. **Tokens expired?** - Re-authenticate to refresh tokens

### Performance Issues

1. **Too many servers?** - Disable unused servers with `"enabled": false`
2. **Slow remote server?** - Increase timeout or use local server
3. **Large responses?** - MCP tools may return large data

## Security Considerations

### Credential Storage

- OAuth tokens stored in `~/.gen/mcp-auth.json` with 0600 permissions
- Never commit `.mcp.json` files with hardcoded secrets
- Always use environment variables for sensitive data

### CSRF Protection

- OAuth flow uses state parameter for CSRF protection
- Callback server validates state before accepting authorization code

### Command Injection

- Stdio commands are executed directly (no shell)
- Arguments are passed as array (no shell expansion)

### SSRF Protection

- Remote URLs must be explicitly configured by user
- No automatic URL following or redirects

### Network Security

- HTTP connections should use HTTPS in production
- Validate server certificates
- Use secure headers for authentication

## Advanced Usage

### Multiple Environments

Use environment-specific configs:

```bash
# Development
export API_URL="https://dev-api.example.com"

# Production
export API_URL="https://api.example.com"
```

Config:

```json
{
  "mcpServers": {
    "api": {
      "type": "http",
      "url": "${API_URL}/mcp"
    }
  }
}
```

### Conditional Server Loading

Disable servers programmatically:

```json
{
  "mcpServers": {
    "dev-db": {
      "type": "stdio",
      "command": "mcp-postgres",
      "enabled": false  // Disabled by default
    }
  }
}
```

Enable via environment:

```bash
# Enable dev-db server
# TODO: Add support for env-based enabled flag
```

### Custom Timeout

Configure per-server timeout (planned):

```json
{
  "mcpServers": {
    "slow-server": {
      "type": "http",
      "url": "https://slow.example.com/mcp",
      "timeout": 120000  // 2 minutes (planned feature)
    }
  }
}
```

## Limitations

Current limitations (planned improvements):

1. **Resources not supported** - Only Tools are currently supported
2. **Prompts not supported** - Prompt templates not yet implemented
3. **No CLI commands** - Server management via config files only
4. **No server status UI** - No visual indication of server health
5. **Limited error recovery** - Failed servers don't auto-reconnect

## Future Enhancements

Planned features:

- [ ] Resources support (read data sources)
- [ ] Prompts support (template system)
- [ ] CLI commands (`/mcp list`, `/mcp add`, etc.)
- [ ] Server health monitoring
- [ ] Auto-reconnect on failure
- [ ] Streaming tool execution
- [ ] Tool caching
- [ ] Server discovery

## See Also

- [MCP Specification](https://modelcontextprotocol.io/)
- [MCP SDK Documentation](https://github.com/modelcontextprotocol/typescript-sdk)
- [Official MCP Servers](https://github.com/modelcontextprotocol/servers)
- [Permission System](./permissions.md)
- [Hooks System](./hooks.md)
