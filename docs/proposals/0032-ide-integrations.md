# Proposal: IDE Integrations

- **Proposal ID**: 0032
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement IDE integrations for VS Code, JetBrains IDEs, and other editors, allowing users to interact with mycode from within their development environment.

## Motivation

CLI-only usage has limitations:

1. **Context switching**: Switch between editor and terminal
2. **No visual integration**: Can't see code in context
3. **Limited selection**: Can't select code to discuss
4. **No inline suggestions**: Suggestions not in editor
5. **Workflow friction**: Extra steps for common tasks

IDE integrations enable seamless development workflows.

## Claude Code Reference

Claude Code offers VS Code integration:

### Features
- Sidebar panel for chat
- Code selection context
- Inline completions
- Terminal integration
- File context awareness

## Detailed Design

### API Design

```typescript
// src/ide/types.ts
interface IDEConnection {
  id: string;
  type: 'vscode' | 'jetbrains' | 'vim' | 'emacs';
  version: string;
  capabilities: string[];
}

interface IDEMessage {
  type: 'request' | 'response' | 'event';
  id: string;
  method: string;
  params?: unknown;
}

interface CodeContext {
  filePath: string;
  language: string;
  selection?: {
    start: { line: number; character: number };
    end: { line: number; character: number };
    text: string;
  };
  visibleRange?: {
    start: number;
    end: number;
  };
  diagnostics?: Diagnostic[];
}
```

### IDE Server

```typescript
// src/ide/server.ts
class IDEServer {
  private connections: Map<string, IDEConnection> = new Map();
  private wss: WebSocketServer;

  constructor(port: number = 9876) {
    this.wss = new WebSocketServer({ port });
    this.setupHandlers();
  }

  private setupHandlers(): void {
    this.wss.on('connection', (ws) => {
      const id = generateId();

      ws.on('message', async (data) => {
        const message = JSON.parse(data.toString()) as IDEMessage;
        const response = await this.handleMessage(id, message);
        ws.send(JSON.stringify(response));
      });

      ws.on('close', () => {
        this.connections.delete(id);
      });
    });
  }

  private async handleMessage(
    connectionId: string,
    message: IDEMessage
  ): Promise<IDEMessage> {
    switch (message.method) {
      case 'initialize':
        return this.handleInitialize(connectionId, message);
      case 'chat':
        return this.handleChat(connectionId, message);
      case 'complete':
        return this.handleComplete(connectionId, message);
      case 'explain':
        return this.handleExplain(connectionId, message);
      case 'fix':
        return this.handleFix(connectionId, message);
      default:
        return {
          type: 'response',
          id: message.id,
          method: 'error',
          params: { error: 'Unknown method' }
        };
    }
  }

  private async handleChat(
    connectionId: string,
    message: IDEMessage
  ): Promise<IDEMessage> {
    const { prompt, context } = message.params as {
      prompt: string;
      context?: CodeContext;
    };

    // Build context-aware prompt
    let fullPrompt = prompt;
    if (context?.selection?.text) {
      fullPrompt = `Regarding this code:\n\`\`\`${context.language}\n${context.selection.text}\n\`\`\`\n\n${prompt}`;
    }

    // Run agent
    const response = await this.runAgent(fullPrompt, context);

    return {
      type: 'response',
      id: message.id,
      method: 'chat',
      params: { response }
    };
  }
}
```

### VS Code Extension

```typescript
// vscode-extension/src/extension.ts
import * as vscode from 'vscode';

let connection: WebSocket | null = null;

export function activate(context: vscode.ExtensionContext) {
  // Register chat panel
  const provider = new MycodeViewProvider(context.extensionUri);
  context.subscriptions.push(
    vscode.window.registerWebviewViewProvider('mycode.chatView', provider)
  );

  // Register commands
  context.subscriptions.push(
    vscode.commands.registerCommand('mycode.explain', explainSelection),
    vscode.commands.registerCommand('mycode.fix', fixSelection),
    vscode.commands.registerCommand('mycode.chat', openChat),
    vscode.commands.registerCommand('mycode.terminal', openTerminal)
  );

  // Connect to mycode server
  connectToServer();
}

async function explainSelection() {
  const editor = vscode.window.activeTextEditor;
  if (!editor) return;

  const selection = editor.document.getText(editor.selection);
  const language = editor.document.languageId;

  const response = await sendMessage('explain', {
    code: selection,
    language,
    filePath: editor.document.uri.fsPath
  });

  showResponse(response);
}

async function fixSelection() {
  const editor = vscode.window.activeTextEditor;
  if (!editor) return;

  const diagnostics = vscode.languages.getDiagnostics(editor.document.uri);
  const selection = editor.document.getText(editor.selection);

  const response = await sendMessage('fix', {
    code: selection,
    language: editor.document.languageId,
    diagnostics: diagnostics.map(d => ({
      message: d.message,
      severity: d.severity,
      range: d.range
    }))
  });

  if (response.fix) {
    await editor.edit(builder => {
      builder.replace(editor.selection, response.fix);
    });
  }
}

class MycodeViewProvider implements vscode.WebviewViewProvider {
  resolveWebviewView(webviewView: vscode.WebviewView) {
    webviewView.webview.html = this.getHtml();

    webviewView.webview.onDidReceiveMessage(async (message) => {
      if (message.type === 'chat') {
        const context = this.getEditorContext();
        const response = await sendMessage('chat', {
          prompt: message.prompt,
          context
        });
        webviewView.webview.postMessage({ type: 'response', content: response });
      }
    });
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/ide/types.ts` | Create | Type definitions |
| `src/ide/server.ts` | Create | WebSocket server |
| `src/ide/handlers/*.ts` | Create | Request handlers |
| `src/ide/index.ts` | Create | Module exports |
| `vscode-extension/` | Create | VS Code extension |
| `jetbrains-plugin/` | Create | JetBrains plugin |

## User Experience

### VS Code Sidebar
```
┌─ mycode ───────────────────────────────────┐
│                                            │
│ [Select code and ask questions]            │
│                                            │
│ ┌────────────────────────────────────────┐ │
│ │ Type your question...                  │ │
│ └────────────────────────────────────────┘ │
│                                            │
│ Recent:                                    │
│ • Explain this function                   │
│ • Fix the type error                      │
│ • Add error handling                      │
│                                            │
└────────────────────────────────────────────┘
```

### Context Menu
```
Right-click on selected code:
  ├─ mycode: Explain Selection
  ├─ mycode: Fix Issues
  ├─ mycode: Add Tests
  ├─ mycode: Refactor
  └─ mycode: Ask...
```

### Keyboard Shortcuts
```
Ctrl+Shift+M  Open mycode chat
Ctrl+Shift+E  Explain selection
Ctrl+Shift+F  Fix selection
```

## Alternatives Considered

### Alternative 1: Language Server
Implement as LSP server.

**Pros**: Standard protocol
**Cons**: Limited to LSP capabilities
**Decision**: WebSocket for flexibility

### Alternative 2: Editor Plugins Only
No server, direct API calls.

**Pros**: Simpler architecture
**Cons**: No session sharing
**Decision**: Server enables session persistence

## Security Considerations

1. Local-only connections
2. Authentication tokens
3. No remote code execution
4. Sandboxed extension

## Testing Strategy

1. Unit tests for server
2. Extension integration tests
3. Manual UX testing

## Migration Path

1. **Phase 1**: VS Code extension
2. **Phase 2**: JetBrains plugin
3. **Phase 3**: Vim/Neovim plugin
4. **Phase 4**: Emacs package
5. **Phase 5**: LSP server mode

## References

- [VS Code Extension API](https://code.visualstudio.com/api)
- [IntelliJ Plugin Development](https://plugins.jetbrains.com/docs/intellij/)
- [Language Server Protocol](https://microsoft.github.io/language-server-protocol/)
