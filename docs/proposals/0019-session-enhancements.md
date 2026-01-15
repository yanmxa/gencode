# Proposal: Session Enhancements

- **Proposal ID**: 0019
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Enhance the session management system with advanced features including branching, tagging, search, compression, and improved persistence. This provides users with powerful conversation history management comparable to git workflows.

## Motivation

The current session system provides basic CRUD operations but lacks:

1. **No branching**: Can't explore alternative paths
2. **Limited search**: Can't find specific conversations
3. **No compression**: Old sessions waste disk space
4. **No tagging**: Can't categorize or label sessions
5. **Basic metadata**: Limited context about session content
6. **No merge**: Can't combine sessions

Enhanced sessions enable sophisticated conversation management.

## Claude Code Reference

Claude Code provides several session-related features:

### Session Resume
```bash
claude --resume <session-id>
claude --continue  # Resume most recent
```

### Session Commands
```
/sessions [--all | -a]  # List sessions
/resume [index|id]      # Resume session
/new [title]           # New session
/fork [title]          # Fork current
/clear                 # Clear history
```

### Key Features
- Project-scoped sessions (by working directory)
- Automatic summarization for long conversations
- Fork capability for exploration
- Session persistence across restarts

## Detailed Design

### Enhanced Session Types

```typescript
// src/session/types.ts
interface Session {
  metadata: SessionMetadata;
  messages: Message[];
  branches?: Branch[];
  tags?: string[];
  summary?: SessionSummary;
  compressed?: boolean;
}

interface SessionMetadata {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
  provider: string;
  model: string;
  cwd: string;
  messageCount: number;
  tokenUsage?: TokenUsage;
  parentId?: string;           // For forked sessions
  branchPoint?: number;        // Message index where branched
  description?: string;        // User-provided description
  archived?: boolean;          // Archived sessions
}

interface TokenUsage {
  input: number;
  output: number;
  total: number;
  estimatedCost?: number;
}

interface Branch {
  id: string;
  name: string;
  messageIndex: number;        // Where branch diverges
  sessionId: string;           // Reference to branch session
  createdAt: string;
}

interface SessionSummary {
  topics: string[];
  keyActions: string[];
  filesModified: string[];
  generatedAt: string;
}

interface SessionSearchResult {
  session: SessionMetadata;
  matches: SearchMatch[];
  score: number;
}

interface SearchMatch {
  messageIndex: number;
  role: 'user' | 'assistant';
  snippet: string;
  highlight: [number, number][];
}
```

### Enhanced Session Manager

```typescript
// src/session/manager.ts
class SessionManager {
  // Existing methods...

  // === New: Branching ===
  async branch(
    sessionId: string,
    messageIndex: number,
    branchName?: string
  ): Promise<Session> {
    const session = await this.load(sessionId);
    if (!session) throw new Error('Session not found');

    // Create new session with messages up to branch point
    const branchSession = await this.create({
      provider: session.metadata.provider,
      model: session.metadata.model,
      cwd: session.metadata.cwd,
      title: branchName || `Branch of ${session.metadata.title}`,
      parentId: sessionId
    });

    // Copy messages up to branch point
    branchSession.messages = session.messages.slice(0, messageIndex + 1);
    branchSession.metadata.branchPoint = messageIndex;

    // Record branch in parent
    session.branches = session.branches || [];
    session.branches.push({
      id: generateId(),
      name: branchName || `branch-${session.branches.length + 1}`,
      messageIndex,
      sessionId: branchSession.metadata.id,
      createdAt: new Date().toISOString()
    });

    await this.save(session);
    await this.save(branchSession);

    return branchSession;
  }

  async listBranches(sessionId: string): Promise<Branch[]> {
    const session = await this.load(sessionId);
    return session?.branches || [];
  }

  // === New: Tagging ===
  async addTag(sessionId: string, tag: string): Promise<void> {
    const session = await this.load(sessionId);
    if (!session) throw new Error('Session not found');

    session.tags = session.tags || [];
    if (!session.tags.includes(tag)) {
      session.tags.push(tag);
      await this.save(session);
    }
  }

  async removeTag(sessionId: string, tag: string): Promise<void> {
    const session = await this.load(sessionId);
    if (!session) throw new Error('Session not found');

    session.tags = (session.tags || []).filter(t => t !== tag);
    await this.save(session);
  }

  async findByTag(tag: string): Promise<SessionMetadata[]> {
    const all = await this.listAll();
    return all.filter(s => s.tags?.includes(tag));
  }

  // === New: Search ===
  async search(
    query: string,
    options?: { cwd?: string; limit?: number }
  ): Promise<SessionSearchResult[]> {
    const sessions = options?.cwd
      ? await this.list({ cwd: options.cwd })
      : await this.listAll();

    const results: SessionSearchResult[] = [];
    const queryLower = query.toLowerCase();

    for (const metadata of sessions) {
      const session = await this.load(metadata.id);
      if (!session) continue;

      const matches: SearchMatch[] = [];

      session.messages.forEach((msg, idx) => {
        const content = typeof msg.content === 'string'
          ? msg.content
          : JSON.stringify(msg.content);

        const contentLower = content.toLowerCase();
        const matchIndex = contentLower.indexOf(queryLower);

        if (matchIndex !== -1) {
          const start = Math.max(0, matchIndex - 50);
          const end = Math.min(content.length, matchIndex + query.length + 50);
          matches.push({
            messageIndex: idx,
            role: msg.role as 'user' | 'assistant',
            snippet: content.slice(start, end),
            highlight: [[matchIndex - start, matchIndex - start + query.length]]
          });
        }
      });

      if (matches.length > 0) {
        results.push({
          session: metadata,
          matches,
          score: matches.length
        });
      }
    }

    // Sort by relevance
    results.sort((a, b) => b.score - a.score);

    return options?.limit ? results.slice(0, options.limit) : results;
  }

  // === New: Compression ===
  async compress(sessionId: string): Promise<void> {
    const session = await this.load(sessionId);
    if (!session) throw new Error('Session not found');

    // Generate summary before compression
    session.summary = await this.generateSummary(session);

    // Compress messages (keep first and last N)
    const KEEP_RECENT = 10;
    if (session.messages.length > KEEP_RECENT * 2) {
      const first = session.messages.slice(0, 5);
      const last = session.messages.slice(-KEEP_RECENT);
      session.messages = [
        ...first,
        {
          role: 'system',
          content: `[${session.messages.length - first.length - last.length} messages compressed]`
        },
        ...last
      ];
      session.compressed = true;
    }

    await this.save(session);
  }

  async archive(sessionId: string): Promise<void> {
    const session = await this.load(sessionId);
    if (!session) throw new Error('Session not found');

    session.metadata.archived = true;
    await this.compress(sessionId);
    await this.save(session);

    // Move to archive directory
    const archivePath = path.join(this.archiveDir, `${sessionId}.json.gz`);
    await this.compressAndMove(session, archivePath);
  }

  private async generateSummary(session: Session): Promise<SessionSummary> {
    // Extract topics from messages
    const topics: Set<string> = new Set();
    const actions: string[] = [];
    const files: Set<string> = new Set();

    for (const msg of session.messages) {
      // Extract file paths mentioned
      const filePaths = extractFilePaths(msg.content);
      filePaths.forEach(f => files.add(f));

      // Extract tool actions
      if (msg.role === 'assistant' && typeof msg.content !== 'string') {
        for (const block of msg.content) {
          if (block.type === 'tool_use') {
            actions.push(`${block.name}: ${summarizeInput(block.input)}`);
          }
        }
      }
    }

    return {
      topics: Array.from(topics),
      keyActions: actions.slice(0, 10),
      filesModified: Array.from(files),
      generatedAt: new Date().toISOString()
    };
  }
}
```

### New CLI Commands

```typescript
// Additional CLI commands
const sessionCommands = {
  '/branch': 'Create a branch at current or specified message',
  '/tag': 'Add or remove tags from current session',
  '/search': 'Search across all sessions',
  '/archive': 'Archive and compress old sessions',
  '/branches': 'List all branches of current session',
  '/describe': 'Add description to current session'
};

// Examples:
// /branch "experiment-1"
// /branch 5 "try-different-approach"
// /tag add important
// /tag remove wip
// /search "authentication error"
// /archive
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/session/types.ts` | Modify | Add new types |
| `src/session/manager.ts` | Modify | Add new methods |
| `src/session/search.ts` | Create | Search implementation |
| `src/session/compression.ts` | Create | Compression utilities |
| `src/cli/commands/session.ts` | Modify | Add new commands |

## User Experience

### Branching
```
User: /branch "try-redis-cache"

Created branch 'try-redis-cache' at message 15.
Now working on branch. Original session preserved.

Switch back with: /resume original-session-id
```

### Tagging
```
User: /tag add important
Added tag 'important' to current session.

User: /sessions --tag important
Sessions tagged 'important':
  1. [abc123] Fix authentication bug (2 hours ago)
  2. [def456] Implement caching layer (yesterday)
```

### Search
```
User: /search "database migration"

Found 3 matches:

1. [abc123] Database refactoring (2 days ago)
   Message 12: "...let me write the database migration script..."

2. [def456] Schema update (1 week ago)
   Message 5: "...running database migration for users table..."

3. [ghi789] Production fix (2 weeks ago)
   Message 23: "...database migration failed, rolling back..."
```

### Archive
```
User: /archive

Archived session: abc123
  - Generated summary
  - Compressed 156 messages → 15 messages
  - Saved 2.3 MB

Archived sessions available in ~/.mycode/sessions/archive/
```

## Alternatives Considered

### Alternative 1: Git-based Storage
Store sessions in git repository.

**Pros**: Built-in branching, versioning
**Cons**: Heavy, complex for simple use
**Decision**: Deferred - Consider for enterprise

### Alternative 2: SQLite Backend
Use SQLite instead of JSON files.

**Pros**: Better search, indexing
**Cons**: Added dependency, complexity
**Decision**: Deferred - Phase 2 enhancement

### Alternative 3: Cloud Sync
Sync sessions across devices.

**Pros**: Mobility, backup
**Cons**: Privacy concerns, complexity
**Decision**: Deferred - Future feature

## Security Considerations

1. **Sensitive Data**: Search may expose secrets in old sessions
2. **Compression Safety**: Ensure compressed data is recoverable
3. **Archive Encryption**: Consider encrypting archived sessions
4. **Access Control**: Prevent cross-user session access
5. **Cleanup**: Properly delete archived sessions on request

## Testing Strategy

1. **Unit Tests**:
   - Branching logic
   - Search matching
   - Compression/decompression
   - Tag operations

2. **Integration Tests**:
   - Full branch workflow
   - Search across many sessions
   - Archive and restore

3. **Manual Testing**:
   - Large session handling
   - Edge cases in search

## Migration Path

1. **Phase 1**: Tagging and search
2. **Phase 2**: Branching support
3. **Phase 3**: Compression and archival
4. **Phase 4**: Summary generation
5. **Phase 5**: Export/import improvements

Backward compatible - existing sessions work unchanged.

## References

- [Existing Session Implementation](../../../src/session/manager.ts)
- [Claude Code Session Commands](https://code.claude.com/docs/en/cli)
- [Git Branching Model](https://git-scm.com/book/en/v2/Git-Branching-Branches-in-a-Nutshell)
