# Proposal: AskUserQuestion Tool

- **Proposal ID**: 0012
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement an AskUserQuestion tool that allows the agent to pause execution and present structured questions to the user with predefined options. This enables gathering user preferences, clarifying ambiguous instructions, and making decisions during task execution.

## Motivation

Currently, mycode has no structured way for the agent to ask clarifying questions. This leads to:

1. **Assumptions**: Agent guesses when requirements are unclear
2. **Wasted work**: Wrong assumptions lead to redoing work
3. **Poor UX**: Unstructured questions mixed with output
4. **No multi-select**: Can't gather multiple preferences at once
5. **No defaults**: Can't recommend options to users

A structured question tool enables clear, efficient user interaction.

## Claude Code Reference

Claude Code's AskUserQuestion tool provides rich interactive questioning:

### Tool Definition
```typescript
AskUserQuestion({
  questions: [
    {
      question: "Which database should we use?",
      header: "Database",      // Short label (max 12 chars)
      options: [
        { label: "PostgreSQL (Recommended)", description: "Relational DB with rich features" },
        { label: "MongoDB", description: "Document-based NoSQL database" },
        { label: "SQLite", description: "Lightweight embedded database" }
      ],
      multiSelect: false
    }
  ]
})
```

### Key Features
- 1-4 questions per invocation
- 2-4 options per question
- Automatic "Other" option for custom input
- Multi-select support
- Short headers for chips/tags
- Descriptions for each option
- Recommended options (first position + label suffix)

### Example Usage
```
Agent: Before implementing authentication, I need to clarify some details.

[AskUserQuestion:
  Q1: "Which authentication method should we use?"
      - OAuth 2.0 (Recommended) - Industry standard, supports social login
      - JWT - Stateless tokens, good for APIs
      - Session-based - Traditional cookie sessions

  Q2: "Which user storage should we use?"
      - PostgreSQL (Recommended) - Your existing database
      - Firebase Auth - Managed auth service
]

User selects: OAuth 2.0, PostgreSQL

Agent: Great! I'll implement OAuth 2.0 authentication with PostgreSQL storage.
```

## Detailed Design

### API Design

```typescript
// src/tools/ask-user/types.ts
interface QuestionOption {
  label: string;        // Display text (1-5 words)
  description: string;  // Explanation of the option
}

interface Question {
  question: string;     // The question to ask
  header: string;       // Short label (max 12 chars)
  options: QuestionOption[];  // 2-4 options
  multiSelect: boolean; // Allow multiple selections
}

interface AskUserQuestionInput {
  questions: Question[];  // 1-4 questions
}

interface QuestionAnswer {
  question: string;
  selectedOptions: string[];  // Labels of selected options
  customInput?: string;       // If "Other" was selected
}

interface AskUserQuestionOutput {
  answers: QuestionAnswer[];
}
```

```typescript
// src/tools/ask-user/ask-user-tool.ts
const askUserQuestionTool: Tool<AskUserQuestionInput> = {
  name: 'AskUserQuestion',
  description: `Ask the user structured questions during execution.

Use this tool when you need to:
1. Gather user preferences or requirements
2. Clarify ambiguous instructions
3. Get decisions on implementation choices
4. Offer choices about direction

Guidelines:
- Use multiSelect: true for non-mutually-exclusive options
- Put recommended option first with "(Recommended)" suffix
- Keep headers short (max 12 chars)
- Users can always select "Other" for custom input
`,
  parameters: z.object({
    questions: z.array(z.object({
      question: z.string(),
      header: z.string().max(12),
      options: z.array(z.object({
        label: z.string(),
        description: z.string()
      })).min(2).max(4),
      multiSelect: z.boolean()
    })).min(1).max(4)
  }),
  execute: async (input, context) => { ... }
};
```

### Implementation Approach

1. **Tool Registration**: Add AskUserQuestion to tool registry
2. **UI Integration**: Create interactive question display
3. **Blocking Execution**: Tool blocks until user responds
4. **Answer Processing**: Return structured answers to agent
5. **Other Handling**: Support custom text input

```typescript
// Question display and collection
async function execute(input: AskUserQuestionInput, context: ToolContext): Promise<ToolResult> {
  const { promptUser } = context;

  const answers: QuestionAnswer[] = [];

  for (const question of input.questions) {
    // Display question with options
    const response = await promptUser({
      type: question.multiSelect ? 'multiselect' : 'select',
      message: question.question,
      choices: [
        ...question.options.map(opt => ({
          name: opt.label,
          message: opt.label,
          hint: opt.description
        })),
        { name: 'Other', message: 'Other (custom input)' }
      ]
    });

    let customInput: string | undefined;
    if (response.includes('Other')) {
      customInput = await promptUser({
        type: 'input',
        message: 'Please specify:'
      });
    }

    answers.push({
      question: question.question,
      selectedOptions: response,
      customInput
    });
  }

  return {
    success: true,
    output: JSON.stringify({ answers })
  };
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/tools/ask-user/types.ts` | Create | Question/answer type definitions |
| `src/tools/ask-user/ask-user-tool.ts` | Create | AskUserQuestion tool implementation |
| `src/tools/ask-user/index.ts` | Create | Module exports |
| `src/tools/index.ts` | Modify | Register tool |
| `src/cli/components/QuestionPrompt.tsx` | Create | Question UI component |

## User Experience

### Single Select Display
```
┌─ Database ─────────────────────────────────┐
│ Which database should we use?              │
│                                            │
│ ○ PostgreSQL (Recommended)                 │
│   Relational DB with rich features         │
│                                            │
│ ○ MongoDB                                  │
│   Document-based NoSQL database            │
│                                            │
│ ○ SQLite                                   │
│   Lightweight embedded database            │
│                                            │
│ ○ Other                                    │
│   Provide custom input                     │
└────────────────────────────────────────────┘
```

### Multi Select Display
```
┌─ Features ─────────────────────────────────┐
│ Which features should we enable?           │
│ (Select multiple with space)               │
│                                            │
│ ☑ Dark mode                                │
│   Enable dark theme support                │
│                                            │
│ ☐ Notifications                            │
│   Push notification support                │
│                                            │
│ ☑ Offline mode                             │
│   Work without internet connection         │
└────────────────────────────────────────────┘
```

### Keyboard Navigation
- `↑/↓` - Navigate options
- `Space` - Toggle selection (multi-select)
- `Enter` - Confirm selection
- `Esc` - Cancel (if allowed)

### Answer Display
After user answers:
```
✓ Database: PostgreSQL
✓ Features: Dark mode, Offline mode

Continuing with implementation...
```

## Alternatives Considered

### Alternative 1: Plain Text Questions
Agent asks questions in regular output.

**Pros**: Simpler, no special UI
**Cons**: No structure, easy to miss, no validation
**Decision**: Rejected - Structured questions are clearer

### Alternative 2: Form-Based Input
Complex form with multiple field types.

**Pros**: More input flexibility
**Cons**: Overcomplicated for most use cases
**Decision**: Rejected - Options are sufficient for most needs

### Alternative 3: Always Allow Custom Input
Every option is editable text.

**Pros**: Maximum flexibility
**Cons**: Slower, more error-prone
**Decision**: Rejected - "Other" option provides this when needed

## Security Considerations

1. **Input Sanitization**: Validate user custom input
2. **Option Limits**: Enforce max questions and options
3. **Timeout**: Consider timeout for unresponsive users
4. **No Code Execution**: Custom input is text only

## Testing Strategy

1. **Unit Tests**:
   - Input validation
   - Option parsing
   - Answer formatting

2. **Integration Tests**:
   - Tool registration
   - Agent flow with questions
   - Answer processing

3. **Manual Testing**:
   - UI rendering
   - Keyboard navigation
   - Multi-select behavior
   - Custom input flow

## Migration Path

1. **Phase 1**: Core tool and basic select UI
2. **Phase 2**: Multi-select support
3. **Phase 3**: Enhanced UI with descriptions
4. **Phase 4**: Keyboard shortcuts and accessibility

No breaking changes to existing functionality.

## References

- [Claude Code AskUserQuestion Tool](https://code.claude.com/docs/en/tools)
- [Inquirer.js](https://github.com/SBoudrias/Inquirer.js) (inspiration for UI)
