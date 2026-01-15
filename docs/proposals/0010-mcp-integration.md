# Proposal: MCP Integration (Model Context Protocol)

- **Proposal ID**: 0010
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement support for the Model Context Protocol (MCP), enabling mycode to connect to external tools and data sources through a standardized protocol. MCP servers can provide additional tools, resources, and prompts that extend the agent's capabilities.

## Motivation

Currently, mycode has a fixed set of built-in tools. This limits:

1. **Integrations**: Can't connect to external services (Jira, GitHub, Slack)
2. **Custom tools**: Can't add domain-specific tools
3. **Data access**: Can't access databases, APIs, or file systems beyond local
4. **Extensibility**: Adding new capabilities requires code changes
5. **Ecosystem**: Can't benefit from community-built tools

MCP provides a standard way to extend the agent with external capabilities.

## Claude Code Reference

Claude Code supports MCP as both client and server:

### Installation
```bash
claude mcp add filesystem  # Add built-in server
claude mcp add github --env GITHUB_TOKEN=xxx  # With config
```

### Configuration
```json
// .mcp.json (project-level)
{
  "servers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-filesystem", "/path/to/dir"]
    },
    "github": {
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-github"],
      "env": {
        "GITHUB_TOKEN": "${GITHUB_TOKEN}"
      }
    }
  }
}
```

### MCP Capabilities
1. **Tools**: Additional tools the agent can use
2. **Resources**: Data sources (files, databases, APIs)
3. **Prompts**: Predefined prompt templates

### Tool Invocation
MCP tools appear as regular tools with prefix:
```
/mcp__github__create_issue title="Bug fix" body="..."
```

### Example Workflow
```
User: Create a Jira ticket for this bug

Agent: I'll use the Jira MCP server to create a ticket.
[mcp__jira__create_issue:
  project: "MYPROJ"
  summary: "Fix login validation"
  description: "..."
]

Created issue MYPROJ-123: https://jira.example.com/browse/MYPROJ-123
```

## Detailed Design

### API Design

```typescript
// src/mcp/types.ts
interface MCPServerConfig {
  command: string;
  args?: string[];
  env?: Record<string, string>;
  cwd?: string;
}

interface MCPTool {
  name: string;
  description: string;
  inputSchema: JSONSchema;
  serverName: string;
}

interface MCPResource {
  uri: string;
  name: string;
  mimeType?: string;
  description?: string;
}

interface MCPPrompt {
  name: string;
  description?: string;
  arguments?: Array<{
    name: string;
    description?: string;
    required?: boolean;
  }>;
}

interface MCPServer {
  name: string;
  config: MCPServerConfig;
  tools: MCPTool[];
  resources: MCPResource[];
  prompts: MCPPrompt[];
  status: 'connected' | 'disconnected' | 'error';
}
```

```typescript
// src/mcp/mcp-manager.ts
class MCPManager {
  private servers: Map<string, MCPServer>;

  constructor();

  // Load servers from config files
  async loadConfig(configPath: string): Promise<void>;

  // Start an MCP server
  async startServer(name: string, config: MCPServerConfig): Promise<MCPServer>;

  // Stop a server
  async stopServer(name: string): Promise<void>;

  // Get all available tools from all servers
  getTools(): MCPTool[];

  // Execute an MCP tool
  async executeTool(serverName: string, toolName: string, input: any): Promise<any>;

  // Read a resource
  async readResource(uri: string): Promise<any>;

  // Get prompts
  getPrompts(): MCPPrompt[];

  // List connected servers
  listServers(): MCPServer[];
}
```

### MCP Protocol Implementation

```typescript
// src/mcp/protocol.ts
interface MCPMessage {
  jsonrpc: '2.0';
  id?: string | number;
  method?: string;
  params?: any;
  result?: any;
  error?: { code: number; message: string; data?: any };
}

class MCPConnection {
  private process: ChildProcess;
  private pending: Map<string, { resolve: Function; reject: Function }>;

  constructor(config: MCPServerConfig);

  // Initialize connection
  async initialize(): Promise<{ capabilities: any; serverInfo: any }>;

  // List available tools
  async listTools(): Promise<MCPTool[]>;

  // Call a tool
  async callTool(name: string, arguments: any): Promise<any>;

  // List resources
  async listResources(): Promise<MCPResource[]>;

  // Read a resource
  async readResource(uri: string): Promise<any>;

  // List prompts
  async listPrompts(): Promise<MCPPrompt[]>;

  // Get a prompt
  async getPrompt(name: string, arguments?: any): Promise<any>;

  // Close connection
  async close(): Promise<void>;
}
```

### Integration with Tool Registry

```typescript
// Bridge MCP tools to native tool format
function mcpToolToNativeTool(mcpTool: MCPTool, mcpManager: MCPManager): Tool<any> {
  return {
    name: `mcp__${mcpTool.serverName}__${mcpTool.name}`,
    description: mcpTool.description,
    parameters: jsonSchemaToZod(mcpTool.inputSchema),
    execute: async (input, context) => {
      const result = await mcpManager.executeTool(
        mcpTool.serverName,
        mcpTool.name,
        input
      );
      return { success: true, output: result };
    }
  };
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/mcp/types.ts` | Create | MCP type definitions |
| `src/mcp/mcp-manager.ts` | Create | MCP server management |
| `src/mcp/protocol.ts` | Create | MCP protocol implementation |
| `src/mcp/connection.ts` | Create | Server connection handling |
| `src/mcp/tool-bridge.ts` | Create | MCP to native tool bridge |
| `src/mcp/index.ts` | Create | Module exports |
| `src/tools/registry.ts` | Modify | Register MCP tools |
| `src/agent/agent.ts` | Modify | Initialize MCP on startup |
| `src/cli/commands/mcp.ts` | Create | /mcp command |

## User Experience

### Adding MCP Server
```
> /mcp add github

Enter GitHub token: ********

✓ Added MCP server: github
Available tools:
  • mcp__github__create_issue
  • mcp__github__list_repos
  • mcp__github__get_file
```

### Listing Servers
```
> /mcp list

MCP Servers:
┌────────────┬───────────┬─────────────────────────┐
│ Server     │ Status    │ Tools                   │
├────────────┼───────────┼─────────────────────────┤
│ github     │ connected │ 5 tools                 │
│ jira       │ connected │ 8 tools                 │
│ filesystem │ error     │ Connection failed       │
└────────────┴───────────┴─────────────────────────┘
```

### Using MCP Tools
```
User: Create a GitHub issue for this bug

Agent: I'll create a GitHub issue using the MCP server.
[mcp__github__create_issue]

✓ Created issue: https://github.com/user/repo/issues/42
```

### Configuration File
`.mycode/mcp.json`:
```json
{
  "servers": {
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_TOKEN": "${GITHUB_TOKEN}"
      }
    },
    "slack": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-slack"],
      "env": {
        "SLACK_TOKEN": "${SLACK_TOKEN}"
      }
    }
  }
}
```

## Alternatives Considered

### Alternative 1: Custom Plugin Format
Define our own plugin/extension format.

**Pros**: Full control, optimized for our use case
**Cons**: No ecosystem, incompatible with Claude Code
**Decision**: Rejected - MCP provides ecosystem benefits

### Alternative 2: HTTP-Only Integrations
Only support HTTP-based tool servers.

**Pros**: Simpler, no process management
**Cons**: Missing local tools, extra latency
**Decision**: Rejected - Process-based is more flexible

### Alternative 3: Built-in Integrations
Hardcode popular integrations (GitHub, Jira).

**Pros**: Better integration, no setup
**Cons**: Limited extensibility, maintenance burden
**Decision**: Rejected - MCP is more scalable

## Security Considerations

1. **Process Isolation**: MCP servers run as separate processes
2. **Credential Security**: Secure handling of API keys/tokens
3. **Permission Control**: Control which MCP tools are allowed
4. **Environment Variables**: Safe expansion of env vars
5. **Sandboxing**: Consider sandboxing MCP server processes
6. **Audit Logging**: Log MCP tool invocations

```typescript
// Secure environment variable expansion
function expandEnvVars(env: Record<string, string>): Record<string, string> {
  const result: Record<string, string> = {};
  for (const [key, value] of Object.entries(env)) {
    if (value.startsWith('${') && value.endsWith('}')) {
      const envName = value.slice(2, -1);
      result[key] = process.env[envName] || '';
    } else {
      result[key] = value;
    }
  }
  return result;
}
```

## Testing Strategy

1. **Unit Tests**:
   - Protocol message handling
   - Tool bridging
   - Configuration parsing

2. **Integration Tests**:
   - Server lifecycle (start/stop)
   - Tool execution
   - Error handling

3. **Manual Testing**:
   - Real MCP servers
   - Various server types
   - Network/process failures

## Migration Path

1. **Phase 1**: Core MCP protocol implementation
2. **Phase 2**: Tool bridging and registration
3. **Phase 3**: /mcp command for server management
4. **Phase 4**: Resource and prompt support
5. **Phase 5**: Permission controls and UI improvements

No breaking changes to existing functionality.

## References

- [Model Context Protocol Specification](https://spec.modelcontextprotocol.io/)
- [MCP GitHub Repository](https://github.com/modelcontextprotocol)
- [Claude Code MCP Documentation](https://code.claude.com/docs/en/mcp)
- [Building MCP Servers](https://modelcontextprotocol.io/quickstart)
