# Proposal: Git Integration

- **Proposal ID**: 0026
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement native Git integration with specialized tools and workflows for version control operations. This provides the agent with git-aware context and safe commit/PR creation capabilities.

## Motivation

Currently, mycode uses Bash for all git operations:

1. **No git awareness**: Agent doesn't know repo state
2. **Unsafe operations**: Easy to run destructive commands
3. **Manual workflows**: PR creation is multi-step
4. **No diff context**: Agent doesn't see changes
5. **Repetitive prompts**: Same git patterns repeated

Native git integration enables safe, efficient version control.

## Claude Code Reference

Claude Code provides extensive git integration:

### Git Safety Protocol
```
- NEVER update the git config
- NEVER run destructive/irreversible commands (force push, hard reset)
- NEVER skip hooks (--no-verify)
- NEVER force push to main/master
- Avoid git commit --amend unless conditions are met
- NEVER commit unless explicitly asked
```

### Commit Workflow
```
1. Run git status (never use -uall flag)
2. Run git diff for staged/unstaged changes
3. Run git log for recent commit style
4. Analyze changes, draft commit message
5. Add files, create commit with:
   Co-Authored-By: Claude <noreply@anthropic.com>
6. Run git status to verify
```

### PR Creation Workflow
```
1. git status, git diff, check remote tracking
2. git log and git diff [base]...HEAD for commit history
3. Create branch if needed
4. Push with -u flag
5. Create PR with gh pr create
```

## Detailed Design

### API Design

```typescript
// src/git/types.ts
interface GitContext {
  isRepo: boolean;
  branch: string;
  remoteBranch?: string;
  defaultBranch: string;
  hasUncommittedChanges: boolean;
  hasUntrackedFiles: boolean;
  aheadBehind?: { ahead: number; behind: number };
  lastCommit?: {
    hash: string;
    message: string;
    author: string;
    date: Date;
  };
}

interface GitStatus {
  staged: FileChange[];
  unstaged: FileChange[];
  untracked: string[];
}

interface FileChange {
  path: string;
  status: 'added' | 'modified' | 'deleted' | 'renamed';
  oldPath?: string;  // For renames
}

interface CommitOptions {
  message: string;
  files?: string[];      // Specific files (default: staged)
  coAuthor?: string;     // Co-author for attribution
  signOff?: boolean;     // DCO sign-off
  allowEmpty?: boolean;
}

interface PROptions {
  title: string;
  body: string;
  base?: string;         // Base branch
  draft?: boolean;
  labels?: string[];
  reviewers?: string[];
}

// Safety constraints
interface GitSafetyConfig {
  blockDestructive: boolean;     // Block force push, hard reset
  requireConfirmation: string[]; // Commands needing confirmation
  protectedBranches: string[];   // Branches that can't be modified
  requireSignOff: boolean;
}
```

### Git Tool

```typescript
// src/tools/git/git-tool.ts
const gitTool: Tool<GitInput> = {
  name: 'Git',
  description: `Perform git operations safely.

Operations:
- status: Get repository status
- diff: Show changes (staged, unstaged, or between refs)
- log: Show commit history
- add: Stage files for commit
- commit: Create a commit
- branch: List, create, or switch branches
- push: Push commits to remote
- pull: Pull changes from remote
- stash: Stash/unstash changes
- pr: Create a pull request (requires gh)

Safety features:
- Blocks destructive operations (force push, hard reset)
- Requires confirmation for commits
- Never modifies git config
- Protects main/master branches

Use this tool instead of Bash for git operations.
`,
  parameters: z.object({
    operation: z.enum([
      'status', 'diff', 'log', 'add', 'commit',
      'branch', 'push', 'pull', 'stash', 'pr'
    ]),
    args: z.record(z.unknown()).optional()
  }),
  execute: async (input, context) => {
    return await gitService.execute(input.operation, input.args, context);
  }
};
```

### Git Service

```typescript
// src/git/service.ts
class GitService {
  private safety: GitSafetyConfig;
  private gitContext: GitContext | null = null;

  constructor(safety?: Partial<GitSafetyConfig>) {
    this.safety = {
      blockDestructive: true,
      requireConfirmation: ['commit', 'push', 'reset'],
      protectedBranches: ['main', 'master'],
      requireSignOff: false,
      ...safety
    };
  }

  async execute(
    operation: string,
    args: Record<string, unknown> = {},
    context: ToolContext
  ): Promise<ToolResult> {
    // Refresh context
    this.gitContext = await this.getContext(context.cwd);

    if (!this.gitContext.isRepo) {
      return { success: false, error: 'Not a git repository' };
    }

    switch (operation) {
      case 'status':
        return this.status(context.cwd);
      case 'diff':
        return this.diff(args, context.cwd);
      case 'log':
        return this.log(args, context.cwd);
      case 'add':
        return this.add(args, context.cwd);
      case 'commit':
        return this.commit(args as CommitOptions, context.cwd);
      case 'branch':
        return this.branch(args, context.cwd);
      case 'push':
        return this.push(args, context.cwd);
      case 'pull':
        return this.pull(context.cwd);
      case 'stash':
        return this.stash(args, context.cwd);
      case 'pr':
        return this.createPR(args as PROptions, context.cwd);
      default:
        return { success: false, error: `Unknown operation: ${operation}` };
    }
  }

  private async status(cwd: string): Promise<ToolResult> {
    const result = await this.runGit(['status', '--porcelain=v2', '--branch'], cwd);

    if (!result.success) return result;

    const status = this.parseStatus(result.output);

    return {
      success: true,
      output: this.formatStatus(status)
    };
  }

  private async diff(args: Record<string, unknown>, cwd: string): Promise<ToolResult> {
    const gitArgs = ['diff'];

    if (args.staged) gitArgs.push('--staged');
    if (args.ref) gitArgs.push(args.ref as string);
    if (args.stat) gitArgs.push('--stat');

    return this.runGit(gitArgs, cwd);
  }

  private async commit(options: CommitOptions, cwd: string): Promise<ToolResult> {
    // Safety checks
    if (!options.message) {
      return { success: false, error: 'Commit message required' };
    }

    // Build commit message with co-author
    let message = options.message;
    if (options.coAuthor) {
      message += `\n\nCo-Authored-By: ${options.coAuthor}`;
    }
    if (options.signOff || this.safety.requireSignOff) {
      // Add DCO sign-off
      const user = await this.getGitConfig('user.name', cwd);
      const email = await this.getGitConfig('user.email', cwd);
      message += `\n\nSigned-off-by: ${user} <${email}>`;
    }

    // Add specific files if provided
    if (options.files?.length) {
      const addResult = await this.runGit(['add', ...options.files], cwd);
      if (!addResult.success) return addResult;
    }

    // Create commit using heredoc for message
    const commitArgs = ['commit', '-m', message];
    if (options.allowEmpty) commitArgs.push('--allow-empty');

    return this.runGit(commitArgs, cwd);
  }

  private async createPR(options: PROptions, cwd: string): Promise<ToolResult> {
    // Check for gh CLI
    const ghCheck = await this.runCommand('which', ['gh'], cwd);
    if (!ghCheck.success) {
      return { success: false, error: 'GitHub CLI (gh) not installed' };
    }

    const args = [
      'pr', 'create',
      '--title', options.title,
      '--body', options.body
    ];

    if (options.base) args.push('--base', options.base);
    if (options.draft) args.push('--draft');
    if (options.labels?.length) args.push('--label', options.labels.join(','));
    if (options.reviewers?.length) args.push('--reviewer', options.reviewers.join(','));

    return this.runCommand('gh', args, cwd);
  }

  private async push(args: Record<string, unknown>, cwd: string): Promise<ToolResult> {
    // Safety: block force push
    if (args.force && this.safety.blockDestructive) {
      return { success: false, error: 'Force push is blocked for safety. Use --force explicitly in Bash if needed.' };
    }

    // Safety: protect main branches
    const branch = this.gitContext?.branch;
    if (branch && this.safety.protectedBranches.includes(branch) && args.force) {
      return { success: false, error: `Force push to ${branch} is not allowed` };
    }

    const gitArgs = ['push'];
    if (args.upstream) gitArgs.push('-u', 'origin', branch || 'HEAD');
    if (args.tags) gitArgs.push('--tags');

    return this.runGit(gitArgs, cwd);
  }

  async getContext(cwd: string): Promise<GitContext> {
    // ... implementation
  }

  private async runGit(args: string[], cwd: string): Promise<ToolResult> {
    return this.runCommand('git', args, cwd);
  }
}

export const gitService = new GitService();
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/git/types.ts` | Create | Type definitions |
| `src/git/service.ts` | Create | Git operations |
| `src/git/safety.ts` | Create | Safety checks |
| `src/git/parsers.ts` | Create | Output parsing |
| `src/git/index.ts` | Create | Module exports |
| `src/tools/git/git-tool.ts` | Create | Git tool |
| `src/agent/agent.ts` | Modify | Inject git context |

## User Experience

### Git Status in Prompt
```
┌─ mycode [main ↑2] ────────────────────────────────────────┐
│ > help me commit these changes                            │
└───────────────────────────────────────────────────────────┘
```

### Safe Commit Workflow
```
User: Commit these changes

Agent: Let me check the current repository status.

[Git: status]

Current branch: feature/auth
Changes to commit:
  M  src/auth.ts
  M  src/auth.test.ts
  A  src/types/auth.ts

[Git: diff staged]

I can see you've:
- Added JWT token validation
- Created new auth types
- Updated tests

Suggested commit message:
"feat: add JWT token validation to auth module"

[Git: commit message="feat: add JWT token validation..." coAuthor="Claude..."]

✓ Commit created: abc1234

[Git: status]

Branch is ahead of origin/feature/auth by 1 commit.
Push to remote?
```

### PR Creation
```
User: Create a PR for this

Agent: I'll create a pull request for your changes.

[Git: log base=main]
[Git: diff main...HEAD stat=true]

This PR includes 3 commits:
- feat: add JWT validation
- test: add auth tests
- docs: update auth README

[Git: pr
  title="feat: JWT token validation"
  body="## Summary\n- Added JWT validation..."
  base="main"
]

✓ Pull request created: https://github.com/org/repo/pull/123
```

### Safety Block
```
Agent: [Git: push force=true]

⚠️ Operation Blocked

Force push is not allowed for safety.
If you really need to force push, use Bash:
  bash: git push --force

This requires explicit user confirmation.
```

## Alternatives Considered

### Alternative 1: Bash Only
Continue using Bash for all git operations.

**Pros**: Simpler, full git access
**Cons**: No safety, no context awareness
**Decision**: Rejected - Safety is important

### Alternative 2: Read-only Git
Only provide git status/diff/log.

**Pros**: Maximum safety
**Cons**: Too limiting for workflows
**Decision**: Rejected - Need commit/push

### Alternative 3: External Git UI
Integrate with GitKraken/Tower.

**Pros**: Rich UI
**Cons**: Heavy dependency
**Decision**: Rejected - CLI-first approach

## Security Considerations

1. **Credential Handling**: Use git credential helpers
2. **Remote Validation**: Verify remote URLs
3. **Branch Protection**: Honor branch rules
4. **Commit Signing**: Support GPG signing
5. **Token Exposure**: Don't log auth tokens

## Testing Strategy

1. **Unit Tests**:
   - Status parsing
   - Safety checks
   - Command building

2. **Integration Tests**:
   - Full commit workflow
   - PR creation
   - Branch operations

3. **Safety Tests**:
   - Destructive command blocking
   - Protected branch enforcement

## Migration Path

1. **Phase 1**: Basic status/diff/log
2. **Phase 2**: Add/commit with safety
3. **Phase 3**: Push/pull operations
4. **Phase 4**: PR creation (gh integration)
5. **Phase 5**: Advanced workflows

Bash git commands remain available.

## References

- [Git Documentation](https://git-scm.com/doc)
- [GitHub CLI](https://cli.github.com/)
- [Conventional Commits](https://www.conventionalcommits.org/)
- [DCO Sign-off](https://developercertificate.org/)
