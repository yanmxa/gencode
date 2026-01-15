# Proposal: LSP Tool

- **Proposal ID**: 0014
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement an LSP (Language Server Protocol) tool that provides code intelligence features like go-to-definition, find-references, hover documentation, and symbol search. This enables the agent to navigate codebases with the same precision as modern IDEs.

## Motivation

Currently, mycode relies on text-based search (Grep, Glob) for code navigation. This leads to:

1. **Imprecise results**: Text search finds string matches, not semantic references
2. **No type information**: Can't determine variable types or function signatures
3. **No call hierarchy**: Can't trace function call relationships
4. **Limited understanding**: Agent must infer code structure from text patterns
5. **Slower exploration**: Multiple searches needed to understand relationships

An LSP tool provides IDE-quality code intelligence for precise navigation.

## Claude Code Reference

Claude Code's LSP tool provides comprehensive code intelligence:

### Tool Definition
```typescript
LSP({
  operation: "goToDefinition",
  filePath: "/path/to/file.ts",
  line: 42,
  character: 15
})
```

### Supported Operations
| Operation | Description |
|-----------|-------------|
| `goToDefinition` | Find where a symbol is defined |
| `findReferences` | Find all references to a symbol |
| `hover` | Get documentation and type info |
| `documentSymbol` | Get all symbols in a document |
| `workspaceSymbol` | Search symbols across workspace |
| `goToImplementation` | Find interface implementations |
| `prepareCallHierarchy` | Get call hierarchy item at position |
| `incomingCalls` | Find callers of a function |
| `outgoingCalls` | Find callees from a function |

### Key Characteristics
- Position-based (1-indexed line and character)
- Requires configured LSP server for file type
- Graceful fallback when no server available
- Returns structured location data

### Example Usage
```
User: Where is the AuthService class defined?

Agent: Let me find the definition.
[LSP: goToDefinition on AuthService at line 15, char 10]

AuthService is defined at:
src/services/auth.ts:25

The class has the following methods:
- login(email: string, password: string): Promise<User>
- logout(): void
- refreshToken(): Promise<string>
```

## Detailed Design

### API Design

```typescript
// src/tools/lsp/types.ts
type LSPOperation =
  | 'goToDefinition'
  | 'findReferences'
  | 'hover'
  | 'documentSymbol'
  | 'workspaceSymbol'
  | 'goToImplementation'
  | 'prepareCallHierarchy'
  | 'incomingCalls'
  | 'outgoingCalls';

interface LSPInput {
  operation: LSPOperation;
  filePath: string;
  line: number;        // 1-based line number
  character: number;   // 1-based character offset
  query?: string;      // For workspaceSymbol search
}

interface Location {
  uri: string;
  range: {
    start: { line: number; character: number };
    end: { line: number; character: number };
  };
}

interface Symbol {
  name: string;
  kind: SymbolKind;
  location: Location;
  containerName?: string;
}

interface HoverInfo {
  contents: string;      // Markdown formatted
  range?: Location['range'];
}

interface CallHierarchyItem {
  name: string;
  kind: SymbolKind;
  uri: string;
  range: Location['range'];
  selectionRange: Location['range'];
}

interface LSPOutput {
  success: boolean;
  operation: LSPOperation;
  result?: {
    definitions?: Location[];
    references?: Location[];
    hover?: HoverInfo;
    symbols?: Symbol[];
    callItems?: CallHierarchyItem[];
    incomingCalls?: { from: CallHierarchyItem; fromRanges: Location['range'][] }[];
    outgoingCalls?: { to: CallHierarchyItem; fromRanges: Location['range'][] }[];
  };
  error?: string;
}

type SymbolKind =
  | 'File' | 'Module' | 'Namespace' | 'Package'
  | 'Class' | 'Method' | 'Property' | 'Field'
  | 'Constructor' | 'Enum' | 'Interface' | 'Function'
  | 'Variable' | 'Constant' | 'String' | 'Number'
  | 'Boolean' | 'Array' | 'Object' | 'Key'
  | 'Null' | 'EnumMember' | 'Struct' | 'Event'
  | 'Operator' | 'TypeParameter';
```

```typescript
// src/tools/lsp/lsp-tool.ts
const lspTool: Tool<LSPInput> = {
  name: 'LSP',
  description: `Interact with Language Server Protocol for code intelligence.

Operations:
- goToDefinition: Find where a symbol is defined
- findReferences: Find all references to a symbol
- hover: Get documentation and type info for a symbol
- documentSymbol: Get all symbols in a document
- workspaceSymbol: Search for symbols across workspace
- goToImplementation: Find implementations of interface/abstract
- prepareCallHierarchy: Get call hierarchy item at position
- incomingCalls: Find all callers of a function
- outgoingCalls: Find all functions called by a function

Parameters:
- operation: The LSP operation to perform
- filePath: Path to the file (absolute or relative)
- line: Line number (1-based, as shown in editors)
- character: Character offset (1-based, as shown in editors)

Note: LSP servers must be configured for the file type.
Returns error if no server is available.
`,
  parameters: z.object({
    operation: z.enum([
      'goToDefinition', 'findReferences', 'hover',
      'documentSymbol', 'workspaceSymbol', 'goToImplementation',
      'prepareCallHierarchy', 'incomingCalls', 'outgoingCalls'
    ]),
    filePath: z.string(),
    line: z.number().int().positive(),
    character: z.number().int().positive(),
    query: z.string().optional()
  }),
  execute: async (input, context) => { ... }
};
```

### Implementation Approach

1. **Server Management**: Maintain pool of LSP servers by language
2. **Protocol Communication**: JSON-RPC over stdio/socket
3. **Document Sync**: Keep servers informed of file changes
4. **Result Mapping**: Convert LSP results to our format
5. **Graceful Degradation**: Return helpful error when no server

```typescript
// src/tools/lsp/server-manager.ts
interface LSPServerConfig {
  language: string;
  command: string;
  args: string[];
  rootPath?: string;
}

const DEFAULT_SERVERS: Record<string, LSPServerConfig> = {
  typescript: {
    language: 'typescript',
    command: 'typescript-language-server',
    args: ['--stdio']
  },
  javascript: {
    language: 'javascript',
    command: 'typescript-language-server',
    args: ['--stdio']
  },
  python: {
    language: 'python',
    command: 'pylsp',
    args: []
  },
  go: {
    language: 'go',
    command: 'gopls',
    args: []
  },
  rust: {
    language: 'rust',
    command: 'rust-analyzer',
    args: []
  }
};

class LSPServerManager {
  private servers: Map<string, LanguageServer> = new Map();

  async getServer(filePath: string): Promise<LanguageServer | null> {
    const language = detectLanguage(filePath);

    if (this.servers.has(language)) {
      return this.servers.get(language)!;
    }

    const config = DEFAULT_SERVERS[language];
    if (!config) return null;

    try {
      const server = await this.startServer(config);
      this.servers.set(language, server);
      return server;
    } catch (error) {
      console.warn(`Failed to start LSP server for ${language}:`, error);
      return null;
    }
  }

  private async startServer(config: LSPServerConfig): Promise<LanguageServer> {
    const process = spawn(config.command, config.args, { stdio: 'pipe' });
    const server = new LanguageServer(process);
    await server.initialize({ rootPath: config.rootPath || process.cwd() });
    return server;
  }

  async shutdown(): Promise<void> {
    for (const server of this.servers.values()) {
      await server.shutdown();
    }
    this.servers.clear();
  }
}
```

```typescript
// Core LSP client implementation
class LanguageServer {
  private process: ChildProcess;
  private requestId: number = 0;
  private pendingRequests: Map<number, { resolve: Function; reject: Function }>;

  async goToDefinition(uri: string, line: number, char: number): Promise<Location[]> {
    return this.request('textDocument/definition', {
      textDocument: { uri },
      position: { line: line - 1, character: char - 1 }  // Convert to 0-based
    });
  }

  async findReferences(uri: string, line: number, char: number): Promise<Location[]> {
    return this.request('textDocument/references', {
      textDocument: { uri },
      position: { line: line - 1, character: char - 1 },
      context: { includeDeclaration: true }
    });
  }

  async hover(uri: string, line: number, char: number): Promise<HoverInfo | null> {
    const result = await this.request('textDocument/hover', {
      textDocument: { uri },
      position: { line: line - 1, character: char - 1 }
    });
    if (!result) return null;
    return {
      contents: formatHoverContents(result.contents),
      range: result.range
    };
  }

  private async request(method: string, params: unknown): Promise<unknown> {
    const id = this.requestId++;
    return new Promise((resolve, reject) => {
      this.pendingRequests.set(id, { resolve, reject });
      this.send({ jsonrpc: '2.0', id, method, params });
    });
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/tools/lsp/types.ts` | Create | LSP type definitions |
| `src/tools/lsp/lsp-tool.ts` | Create | Tool implementation |
| `src/tools/lsp/server-manager.ts` | Create | LSP server lifecycle |
| `src/tools/lsp/language-server.ts` | Create | LSP protocol client |
| `src/tools/lsp/utils.ts` | Create | Helper functions |
| `src/tools/lsp/index.ts` | Create | Module exports |
| `src/tools/index.ts` | Modify | Register LSP tool |
| `package.json` | Modify | Add vscode-languageserver-protocol |

## User Experience

### Go to Definition
```
Agent: Let me find where AuthService is defined.

[LSP: goToDefinition at src/api/users.ts:15:10]

┌─ Definition ──────────────────────────────────────┐
│ Symbol: AuthService                               │
│ Location: src/services/auth.ts:25:1              │
│ Kind: Class                                       │
└───────────────────────────────────────────────────┘
```

### Find References
```
Agent: I'll find all usages of the getUserById function.

[LSP: findReferences at src/services/users.ts:42:15]

┌─ References (8 found) ────────────────────────────┐
│ src/api/users.ts:15                              │
│ src/api/admin.ts:78                              │
│ src/services/auth.ts:92                          │
│ src/hooks/useUser.ts:23                          │
│ src/tests/users.test.ts:45, 67, 89, 102          │
└───────────────────────────────────────────────────┘
```

### Hover Information
```
Agent: Let me check the type of this variable.

[LSP: hover at src/components/UserCard.tsx:18:12]

┌─ Hover Info ──────────────────────────────────────┐
│ (parameter) user: User                           │
│                                                   │
│ Represents a user in the system.                 │
│                                                   │
│ @interface User                                  │
│ @property {string} id - Unique identifier        │
│ @property {string} email - User email            │
│ @property {string} name - Display name           │
└───────────────────────────────────────────────────┘
```

### Call Hierarchy
```
Agent: Let me trace who calls this function.

[LSP: incomingCalls at src/services/api.ts:156:10]

┌─ Incoming Calls to fetchData() ───────────────────┐
│ ← useQuery (src/hooks/useQuery.ts:34)            │
│ ← loadInitialData (src/app/init.ts:78)           │
│ ← refreshCache (src/services/cache.ts:45)        │
└───────────────────────────────────────────────────┘
```

### No Server Available
```
Agent: [LSP: goToDefinition on file.xyz:10:5]

No LSP server configured for .xyz files.
Falling back to text-based search.

[Grep: searching for definition...]
```

## Alternatives Considered

### Alternative 1: Built-in Parser
Parse code directly without external servers.

**Pros**: No external dependencies, always available
**Cons**: Complex to support multiple languages accurately
**Decision**: Rejected - LSP provides better accuracy

### Alternative 2: Tree-sitter Integration
Use tree-sitter for parsing.

**Pros**: Fast, incremental parsing
**Cons**: Query language learning curve, less semantic info
**Decision**: Considered for fallback

### Alternative 3: IDE Extension Only
Require VS Code or similar IDE.

**Pros**: Guaranteed LSP availability
**Cons**: Limits CLI-only usage
**Decision**: Rejected - CLI independence is important

## Security Considerations

1. **Process Isolation**: LSP servers run in separate processes
2. **Resource Limits**: Limit memory and CPU per server
3. **Timeout**: Kill unresponsive servers
4. **Path Validation**: Only access files in workspace
5. **No Arbitrary Code**: LSP servers don't execute user code

```typescript
const SERVER_LIMITS = {
  maxMemoryMB: 512,
  startupTimeoutMs: 30000,
  requestTimeoutMs: 10000,
  maxServers: 5
};
```

## Testing Strategy

1. **Unit Tests**:
   - Protocol message formatting
   - Result parsing
   - Server lifecycle

2. **Integration Tests**:
   - TypeScript server operations
   - Python server operations
   - Multi-language projects

3. **Manual Testing**:
   - Large codebases
   - Cross-file references
   - Complex type hierarchies

## Migration Path

1. **Phase 1**: TypeScript/JavaScript server
2. **Phase 2**: Python, Go, Rust servers
3. **Phase 3**: Custom server configuration
4. **Phase 4**: Auto-install missing servers
5. **Phase 5**: IDE integration for shared servers

No breaking changes to existing tools.

## References

- [Language Server Protocol Specification](https://microsoft.github.io/language-server-protocol/)
- [VSCode Language Server Node](https://github.com/microsoft/vscode-languageserver-node)
- [typescript-language-server](https://github.com/typescript-language-server/typescript-language-server)
- [gopls - Go Language Server](https://pkg.go.dev/golang.org/x/tools/gopls)
- [rust-analyzer](https://rust-analyzer.github.io/)
