# Proposal: Keyboard Shortcuts

- **Proposal ID**: 0024
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement a keyboard shortcuts system for common CLI operations, providing power users with efficient navigation and control. Shortcuts enable quick access to commands, history, and UI controls without typing full commands.

## Motivation

Currently, mycode requires typing full commands for all operations:

1. **Slow navigation**: Must type /sessions, /resume, etc.
2. **No quick actions**: Can't quickly cancel, retry, or navigate
3. **Limited editing**: Basic readline only
4. **No vim/emacs mode**: Power users miss modal editing
5. **Inefficient workflow**: Many keystrokes for common operations

Keyboard shortcuts enable faster, more efficient interaction.

## Claude Code Reference

Claude Code provides several keyboard shortcuts:

### Known Shortcuts
| Shortcut | Action |
|----------|--------|
| Ctrl+C | Cancel current operation |
| Ctrl+D | Exit (EOF) |
| Ctrl+L | Clear screen |
| Up/Down | Navigate history |
| Tab | Autocomplete |
| Escape | Cancel input / Exit mode |

### Expected Features
- Command history navigation
- Input line editing
- Quick command access
- Mode switching
- Context-sensitive shortcuts

## Detailed Design

### API Design

```typescript
// src/cli/shortcuts/types.ts
interface KeyBinding {
  key: string;              // e.g., "ctrl+r", "meta+enter"
  action: string;           // Action identifier
  context?: ShortcutContext;
  description: string;
}

type ShortcutContext =
  | 'input'           // During user input
  | 'running'         // While agent is running
  | 'permission'      // During permission prompt
  | 'menu'            // In menu/selection mode
  | 'global';         // Always active

interface ShortcutConfig {
  bindings: KeyBinding[];
  enableVimMode: boolean;
  enableEmacsMode: boolean;
}

interface ShortcutAction {
  id: string;
  name: string;
  handler: (context: ActionContext) => Promise<void>;
}

interface ActionContext {
  input: string;
  cursorPosition: number;
  session: Session;
  isRunning: boolean;
}
```

### Shortcut Manager

```typescript
// src/cli/shortcuts/manager.ts
class ShortcutManager {
  private bindings: Map<string, KeyBinding[]> = new Map();
  private actions: Map<string, ShortcutAction> = new Map();
  private currentContext: ShortcutContext = 'input';
  private vimMode: VimMode | null = null;

  constructor(config?: Partial<ShortcutConfig>) {
    this.loadDefaultBindings();
    if (config?.enableVimMode) {
      this.vimMode = new VimMode();
    }
  }

  private loadDefaultBindings(): void {
    const defaults: KeyBinding[] = [
      // Global
      { key: 'ctrl+c', action: 'cancel', context: 'global', description: 'Cancel operation' },
      { key: 'ctrl+d', action: 'exit', context: 'global', description: 'Exit mycode' },
      { key: 'ctrl+l', action: 'clear', context: 'global', description: 'Clear screen' },

      // Input mode
      { key: 'up', action: 'history_prev', context: 'input', description: 'Previous history' },
      { key: 'down', action: 'history_next', context: 'input', description: 'Next history' },
      { key: 'ctrl+r', action: 'history_search', context: 'input', description: 'Search history' },
      { key: 'tab', action: 'autocomplete', context: 'input', description: 'Autocomplete' },
      { key: 'ctrl+a', action: 'line_start', context: 'input', description: 'Go to line start' },
      { key: 'ctrl+e', action: 'line_end', context: 'input', description: 'Go to line end' },
      { key: 'ctrl+w', action: 'delete_word', context: 'input', description: 'Delete word' },
      { key: 'ctrl+u', action: 'delete_line', context: 'input', description: 'Delete to start' },
      { key: 'meta+enter', action: 'submit_multiline', context: 'input', description: 'Submit multiline' },

      // Running mode
      { key: 'ctrl+c', action: 'abort', context: 'running', description: 'Abort agent' },
      { key: 'escape', action: 'interrupt', context: 'running', description: 'Interrupt gracefully' },

      // Permission mode
      { key: '1', action: 'allow_once', context: 'permission', description: 'Allow once' },
      { key: '2', action: 'allow_session', context: 'permission', description: 'Allow for session' },
      { key: '3', action: 'allow_always', context: 'permission', description: 'Always allow' },
      { key: '4', action: 'deny', context: 'permission', description: 'Deny' },
      { key: 'y', action: 'allow_once', context: 'permission', description: 'Yes (allow once)' },
      { key: 'n', action: 'deny', context: 'permission', description: 'No (deny)' },

      // Quick commands
      { key: 'ctrl+s', action: 'save_session', context: 'input', description: 'Save session' },
      { key: 'ctrl+n', action: 'new_session', context: 'input', description: 'New session' },
      { key: 'ctrl+o', action: 'open_session', context: 'input', description: 'Open session picker' },
      { key: 'f1', action: 'help', context: 'global', description: 'Show help' },
    ];

    for (const binding of defaults) {
      this.addBinding(binding);
    }
  }

  registerAction(action: ShortcutAction): void {
    this.actions.set(action.id, action);
  }

  addBinding(binding: KeyBinding): void {
    const existing = this.bindings.get(binding.key) || [];
    existing.push(binding);
    this.bindings.set(binding.key, existing);
  }

  setContext(context: ShortcutContext): void {
    this.currentContext = context;
  }

  async handleKeypress(key: KeypressEvent): Promise<boolean> {
    // Normalize key
    const keyStr = this.normalizeKey(key);

    // Check vim mode first
    if (this.vimMode) {
      const handled = await this.vimMode.handleKey(key);
      if (handled) return true;
    }

    // Find matching bindings
    const bindings = this.bindings.get(keyStr) || [];
    const matching = bindings.filter(b =>
      b.context === this.currentContext || b.context === 'global'
    );

    if (matching.length === 0) return false;

    // Execute first matching action
    const binding = matching[0];
    const action = this.actions.get(binding.action);
    if (action) {
      await action.handler(this.getContext());
      return true;
    }

    return false;
  }

  private normalizeKey(event: KeypressEvent): string {
    const parts: string[] = [];
    if (event.ctrl) parts.push('ctrl');
    if (event.meta) parts.push('meta');
    if (event.shift && event.name.length > 1) parts.push('shift');
    parts.push(event.name.toLowerCase());
    return parts.join('+');
  }

  listBindings(context?: ShortcutContext): KeyBinding[] {
    const all = Array.from(this.bindings.values()).flat();
    return context
      ? all.filter(b => b.context === context || b.context === 'global')
      : all;
  }
}
```

### Vim Mode

```typescript
// src/cli/shortcuts/vim-mode.ts
type VimState = 'normal' | 'insert' | 'visual' | 'command';

class VimMode {
  private state: VimState = 'insert';
  private commandBuffer: string = '';
  private registerContent: string = '';

  handleKey(event: KeypressEvent): boolean {
    switch (this.state) {
      case 'insert':
        return this.handleInsertMode(event);
      case 'normal':
        return this.handleNormalMode(event);
      case 'visual':
        return this.handleVisualMode(event);
      case 'command':
        return this.handleCommandMode(event);
    }
  }

  private handleInsertMode(event: KeypressEvent): boolean {
    if (event.name === 'escape') {
      this.state = 'normal';
      return true;
    }
    return false;  // Let normal input handling take over
  }

  private handleNormalMode(event: KeypressEvent): boolean {
    const key = event.name;

    // Mode transitions
    if (key === 'i') { this.state = 'insert'; return true; }
    if (key === 'a') { this.state = 'insert'; /* move cursor right */ return true; }
    if (key === 'v') { this.state = 'visual'; return true; }
    if (key === ':') { this.state = 'command'; this.commandBuffer = ''; return true; }

    // Navigation
    if (key === 'h') { /* move left */ return true; }
    if (key === 'j') { /* history next */ return true; }
    if (key === 'k') { /* history prev */ return true; }
    if (key === 'l') { /* move right */ return true; }
    if (key === 'w') { /* word forward */ return true; }
    if (key === 'b') { /* word back */ return true; }
    if (key === '0') { /* line start */ return true; }
    if (key === '$') { /* line end */ return true; }

    // Editing
    if (key === 'x') { /* delete char */ return true; }
    if (key === 'd') { /* start delete */ return true; }
    if (key === 'y') { /* start yank */ return true; }
    if (key === 'p') { /* paste */ return true; }
    if (key === 'u') { /* undo */ return true; }

    return false;
  }

  private handleCommandMode(event: KeypressEvent): boolean {
    if (event.name === 'escape') {
      this.state = 'normal';
      this.commandBuffer = '';
      return true;
    }

    if (event.name === 'return') {
      this.executeCommand(this.commandBuffer);
      this.state = 'normal';
      this.commandBuffer = '';
      return true;
    }

    this.commandBuffer += event.sequence;
    return true;
  }

  private executeCommand(cmd: string): void {
    switch (cmd) {
      case 'w': /* save */ break;
      case 'q': /* quit */ break;
      case 'wq': /* save and quit */ break;
      case 'help': /* show help */ break;
    }
  }

  getState(): VimState {
    return this.state;
  }

  getModeIndicator(): string {
    const indicators = {
      insert: '-- INSERT --',
      normal: '-- NORMAL --',
      visual: '-- VISUAL --',
      command: `:${this.commandBuffer}`
    };
    return indicators[this.state];
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/cli/shortcuts/types.ts` | Create | Type definitions |
| `src/cli/shortcuts/manager.ts` | Create | Shortcut management |
| `src/cli/shortcuts/vim-mode.ts` | Create | Vim mode support |
| `src/cli/shortcuts/actions.ts` | Create | Built-in actions |
| `src/cli/shortcuts/index.ts` | Create | Module exports |
| `src/cli/index.ts` | Modify | Integrate shortcuts |
| `src/cli/input.ts` | Modify | Key handling |

## User Experience

### Help Display
```
User: (presses F1 or /shortcuts)

Keyboard Shortcuts:
┌────────────────────────────────────────────────────────────┐
│ Global                                                     │
│   Ctrl+C        Cancel/abort current operation            │
│   Ctrl+D        Exit mycode                               │
│   Ctrl+L        Clear screen                              │
│   F1            Show this help                            │
│                                                           │
│ Input Mode                                                 │
│   Up/Down       Navigate command history                  │
│   Ctrl+R        Search history                            │
│   Tab           Autocomplete commands                     │
│   Ctrl+A/E      Go to start/end of line                   │
│   Meta+Enter    Submit multiline input                    │
│   Ctrl+S        Save current session                      │
│   Ctrl+N        New session                               │
│   Ctrl+O        Open session picker                       │
│                                                           │
│ Permission Prompt                                          │
│   1/y           Allow once                                │
│   2             Allow for session                         │
│   3             Always allow                              │
│   4/n           Deny                                      │
└────────────────────────────────────────────────────────────┘

Vim mode: Disabled (enable in settings)
```

### Vim Mode Indicator
```
┌─ mycode ──────────────────────────────────────────────────┐
│ > type your message here_                                 │
│                                                           │
│ -- INSERT --                                              │
└───────────────────────────────────────────────────────────┘

(press Escape)

┌─ mycode ──────────────────────────────────────────────────┐
│ > type your message here                                  │
│                        ^                                  │
│ -- NORMAL --                                              │
└───────────────────────────────────────────────────────────┘
```

### History Search (Ctrl+R)
```
(reverse-i-search)`auth': /sessions --tag authentication

Press Enter to select, Ctrl+R for next match, Esc to cancel
```

## Alternatives Considered

### Alternative 1: Mouse-based UI
Add clickable UI elements.

**Pros**: Discoverable
**Cons**: Requires terminal mouse support
**Decision**: Deferred - Can add alongside shortcuts

### Alternative 2: Command Palette
VS Code-style command palette.

**Pros**: Discoverable, searchable
**Cons**: More complex UI
**Decision**: Consider for future

### Alternative 3: No Vim Mode
Skip vim/emacs modes.

**Pros**: Simpler
**Cons**: Power users miss it
**Decision**: Rejected - Vim mode is expected

## Security Considerations

1. **Input Validation**: Validate shortcut configurations
2. **No Injection**: Shortcuts can't execute arbitrary commands
3. **Confirmation**: Dangerous actions still require confirmation
4. **Escape Hatch**: Always allow Ctrl+C to abort

## Testing Strategy

1. **Unit Tests**:
   - Key normalization
   - Binding matching
   - Action execution
   - Vim mode state machine

2. **Integration Tests**:
   - Full input handling
   - Context switching
   - History navigation

3. **Manual Testing**:
   - Various terminal emulators
   - Key combinations
   - Vim workflow

## Migration Path

1. **Phase 1**: Basic shortcuts (Ctrl+C, arrows, etc.)
2. **Phase 2**: History search
3. **Phase 3**: Vim mode
4. **Phase 4**: Customization
5. **Phase 5**: Emacs mode

No breaking changes - enhances existing input.

## References

- [readline - Node.js](https://nodejs.org/api/readline.html)
- [Vim Commands Cheat Sheet](https://vim.rtorr.com/)
- [GNU Readline](https://tiswww.case.edu/php/chet/readline/readline.html)
- [Inquirer.js](https://github.com/SBoudrias/Inquirer.js)
