# Proposal: Plan Mode

- **Proposal ID**: 0004
- **Author**: mycode team
- **Status**: Implemented
- **Created**: 2025-01-15
- **Updated**: 2026-01-19
- **Implemented**: 2026-01-19

## Summary

Implement a Plan Mode that enables the agent to thoroughly explore and design implementation approaches before writing code. In plan mode, the agent can only read and explore (no edits), creates a structured plan file, and requires user approval before proceeding with implementation.

## Motivation

Currently, mycode jumps directly into implementation. This leads to:

1. **Wasted effort**: Implementing wrong approach requires redoing work
2. **Misalignment**: Agent's assumptions may not match user intent
3. **Poor architecture**: No time for design consideration
4. **Unexpected changes**: Users surprised by what gets modified
5. **No preview**: Can't review approach before execution

Plan mode enables careful design and user approval before changes.

## Claude Code Reference

Claude Code's Plan Mode provides structured planning workflow:

### Activation
```
User: /plan  (or use EnterPlanMode tool)
```

### Key Features
- Agent restricted to read-only tools (no Edit, Write, Bash writes)
- Structured plan written to plan file
- 5-phase workflow: Understanding → Design → Review → Final Plan → Exit
- User questions via AskUserQuestion tool
- Plan approval via ExitPlanMode tool
- Permission pre-requests for implementation

### Plan Mode Phases
1. **Initial Understanding**: Explore codebase, understand request
2. **Design**: Create implementation approach
3. **Review**: Verify alignment with user intent
4. **Final Plan**: Write concise plan to file
5. **Exit**: Call ExitPlanMode for approval

### Example Usage
```
User: /plan Add user authentication to the app

Agent: [Enters plan mode]
I'll explore the codebase to understand the current structure.
[Read, Glob, Grep - exploration]

Based on my exploration, I have a question:
[AskUserQuestion: Which auth method? OAuth/JWT/Session]

User: JWT

Agent: [Writes plan to .claude/plans/xxx.md]

## Authentication Implementation Plan

### Approach
- Add JWT-based authentication
- Create auth middleware
- Add login/logout endpoints

### Files to Modify
- src/middleware/auth.ts (create)
- src/routes/auth.ts (create)
- src/models/user.ts (modify)

### Verification
- Run test suite
- Manual login/logout test

[ExitPlanMode]

User: [Reviews and approves plan]

Agent: [Exits plan mode, begins implementation]
```

## Detailed Design

### API Design

```typescript
// src/planning/types.ts
interface PlanModeState {
  active: boolean;
  planFile: string;
  phase: 'understanding' | 'design' | 'review' | 'final' | 'approval';
  explorationResults: ExplorationResult[];
  userAnswers: Record<string, string>;
}

interface PlanFile {
  id: string;
  title: string;
  createdAt: Date;
  approach: string;
  files: FileChange[];
  verification: string[];
  permissions: PermissionRequest[];
}

interface FileChange {
  path: string;
  action: 'create' | 'modify' | 'delete';
  description: string;
}

interface PermissionRequest {
  tool: 'Bash';
  prompt: string;  // e.g., "run tests", "install dependencies"
}
```

```typescript
// src/planning/enter-plan-mode-tool.ts
const enterPlanModeTool: Tool<{}> = {
  name: 'EnterPlanMode',
  description: `Enter plan mode for non-trivial implementation tasks.

Use plan mode when:
- New feature implementation
- Multiple valid approaches exist
- Code modifications affect existing behavior
- Architectural decisions required
- Multi-file changes expected
- Requirements are unclear

Skip plan mode for:
- Single-line fixes
- Trivial changes
- Specific, detailed instructions given
`,
  parameters: z.object({}),
  execute: async (input, context) => { ... }
};
```

```typescript
// src/planning/exit-plan-mode-tool.ts
const exitPlanModeTool: Tool<ExitPlanModeInput> = {
  name: 'ExitPlanMode',
  description: `Exit plan mode and request user approval.

Call this when:
- Plan is complete and written to plan file
- Ready for user to review and approve

Include allowedPrompts to request permissions needed for implementation.
`,
  parameters: z.object({
    allowedPrompts: z.array(z.object({
      tool: z.literal('Bash'),
      prompt: z.string()
    })).optional()
  }),
  execute: async (input, context) => { ... }
};
```

### Implementation Approach

1. **Mode Management**: Track plan mode state in session
2. **Tool Filtering**: In plan mode, only allow read tools + plan file write
3. **Plan File**: Create structured plan at `.mycode/plans/<id>.md`
4. **Phase Tracking**: Guide agent through planning phases
5. **Approval Flow**: Present plan for user approval
6. **Permission Pre-auth**: Allow requesting bash permissions upfront

```typescript
// Plan mode tool filtering
const PLAN_MODE_ALLOWED_TOOLS = [
  'Read',
  'Glob',
  'Grep',
  'LS',
  'WebFetch',
  'WebSearch',
  'Task',           // For Explore/Plan subagents
  'AskUserQuestion',
  'TodoWrite',
  'EnterPlanMode',
  'ExitPlanMode',
  'Write'           // Only for plan file
];

function filterToolsForPlanMode(tools: Tool[], planFile: string): Tool[] {
  return tools.filter(tool => {
    if (PLAN_MODE_ALLOWED_TOOLS.includes(tool.name)) {
      if (tool.name === 'Write') {
        // Only allow writing to plan file
        return (input) => input.file_path === planFile;
      }
      return true;
    }
    return false;
  });
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/planning/types.ts` | Create | Plan mode type definitions |
| `src/planning/plan-manager.ts` | Create | Plan mode state management |
| `src/planning/enter-plan-mode-tool.ts` | Create | EnterPlanMode tool |
| `src/planning/exit-plan-mode-tool.ts` | Create | ExitPlanMode tool |
| `src/planning/plan-file.ts` | Create | Plan file read/write |
| `src/planning/index.ts` | Create | Module exports |
| `src/agent/agent.ts` | Modify | Plan mode integration |
| `src/tools/index.ts` | Modify | Register plan tools |

## User Experience

### Entering Plan Mode
```
User: Help me implement user authentication

Agent: This is a non-trivial task. I'll enter plan mode to design the approach.

┌─ Plan Mode Active ────────────────────────────┐
│ Read-only exploration enabled                 │
│ Plan file: .mycode/plans/auth-impl.md         │
│                                               │
│ Phase 1/5: Understanding                      │
└───────────────────────────────────────────────┘

Let me explore the current codebase structure...
```

### Plan Display
```
┌─ Implementation Plan ─────────────────────────┐
│                                               │
│ ## JWT Authentication                         │
│                                               │
│ ### Approach                                  │
│ - Add JWT token generation and validation    │
│ - Create auth middleware for protected routes│
│ - Add user registration and login endpoints  │
│                                               │
│ ### Files to Change                           │
│ + src/middleware/auth.ts (create)            │
│ + src/routes/auth.ts (create)                │
│ ~ src/models/user.ts (modify)                │
│ ~ src/app.ts (modify)                        │
│                                               │
│ ### Verification                              │
│ - npm test                                    │
│ - Manual login flow test                     │
│                                               │
└───────────────────────────────────────────────┘

Ready to proceed? [Approve] [Modify] [Cancel]
```

### Plan Approval
```
User: [Approves plan]

Agent: Plan approved. Exiting plan mode and beginning implementation.

┌─ Implementation Started ──────────────────────┐
│ Following plan: .mycode/plans/auth-impl.md    │
│ Pre-approved: npm test, npm install           │
└───────────────────────────────────────────────┘
```

## Alternatives Considered

### Alternative 1: Inline Planning
Plan in regular output without special mode.

**Pros**: Simpler
**Cons**: No enforcement, easy to skip, no approval gate
**Decision**: Rejected - Mode provides structure and safety

### Alternative 2: Mandatory Planning
Always require plan mode for changes.

**Pros**: Maximum safety
**Cons**: Overhead for simple tasks
**Decision**: Rejected - Optional with agent judgment is better

### Alternative 3: External Plan Files
Store plans outside project.

**Pros**: No project pollution
**Cons**: Harder to track, no version control
**Decision**: Rejected - Project plans are versioned with project

## Security Considerations

1. **Tool Restrictions**: Strictly enforce read-only in plan mode
2. **Plan File Location**: Only allow writing to designated plan directory
3. **Permission Scope**: Pre-approved permissions are scoped and temporary
4. **Plan Review**: User must explicitly approve before implementation

## Testing Strategy

1. **Unit Tests**:
   - Mode state management
   - Tool filtering
   - Plan file formatting

2. **Integration Tests**:
   - Enter/exit flow
   - Tool restriction enforcement
   - Approval workflow

3. **Manual Testing**:
   - Various planning scenarios
   - Phase transitions
   - Plan file quality

## Migration Path

1. **Phase 1**: Basic plan mode with tool restrictions
2. **Phase 2**: Plan file structure and writing
3. **Phase 3**: User approval flow
4. **Phase 4**: Permission pre-authorization
5. **Phase 5**: Phase guidance and agent prompting

No breaking changes to existing functionality.

## References

- [Claude Code Plan Mode Documentation](https://code.claude.com/docs/en/planning)
- [Claude Code Best Practices - Explore, Plan, Code](https://www.anthropic.com/engineering/claude-code-best-practices)

---

## Implementation Notes

### Implementation Date
**2026-01-19** - Plan Mode fully implemented

### Implemented Features

✅ **Core Plan Mode**:
- EnterPlanMode tool (`src/cli/planning/tools/enter-plan-mode.ts`)
- ExitPlanMode tool (`src/cli/planning/tools/exit-plan-mode.ts`)
- Plan mode state management (`src/cli/planning/state.ts`)
- Plan file generation (`src/cli/planning/plan-file.ts`)

✅ **Tool Restrictions**:
- Read-only mode enforced (Read, Glob, Grep, WebFetch allowed)
- Write/Edit/Bash blocked during planning
- Tool filtering via PlanModeManager

✅ **User Interaction**:
- AskUserQuestion tool for clarifications
- Plan approval workflow
- Permission pre-authorization via allowedPrompts

✅ **Plan File Structure**:
- Markdown-based plan files
- Stored in `.claude/plans/` directory
- Includes approach, files to modify, verification steps

### Files Created/Modified

| File | Action | Description |
|------|--------|-------------|
| `src/cli/planning/index.ts` | Created | Plan mode exports |
| `src/cli/planning/state.ts` | Created | PlanModeManager singleton |
| `src/cli/planning/types.ts` | Created | Plan mode types |
| `src/cli/planning/plan-file.ts` | Created | Plan file operations |
| `src/cli/planning/tools/enter-plan-mode.ts` | Created | EnterPlanMode tool |
| `src/cli/planning/tools/exit-plan-mode.ts` | Created | ExitPlanMode tool |
| `src/core/tools/index.ts` | Modified | Registered plan mode tools |

### Usage

Users can activate plan mode via:
1. Tool call: `EnterPlanMode()`
2. System prompt guides agent through planning phases
3. Agent explores, asks questions, creates plan
4. Agent calls `ExitPlanMode()` for approval
5. User reviews plan and approves/rejects

### Status

Fully functional and integrated into GenCode CLI.
