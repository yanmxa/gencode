# Proposal: AskUserQuestion Tool

- **Proposal ID**: 0012
- **Author**: gencode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-16

## Summary

Implement an AskUserQuestion tool that allows the agent to pause execution and present structured questions to the user with predefined options. This enables gathering user preferences, clarifying ambiguous instructions, and making decisions during task execution.

## Problem Analysis

### Current Limitations

Without a structured questioning mechanism, the agent faces several challenges:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         PROBLEM SCENARIOS                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Scenario 1: Ambiguous Requirements                                          │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ User: "Add authentication to my app"                                   │  │
│  │                                                                        │  │
│  │ Agent's dilemma:                                                       │  │
│  │   ├── OAuth 2.0? JWT? Session-based?                                   │  │
│  │   ├── Which providers? Google? GitHub? Email/Password?                 │  │
│  │   └── Store in database? External service?                             │  │
│  │                                                                        │  │
│  │ Current behavior: Agent GUESSES → Wrong choice → Rework required       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  Scenario 2: Multiple Valid Approaches                                       │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ User: "Optimize database queries"                                      │  │
│  │                                                                        │  │
│  │ Options available:                                                     │  │
│  │   ├── Add indexes (fastest, minimal code change)                       │  │
│  │   ├── Denormalize tables (faster reads, more storage)                  │  │
│  │   ├── Add caching layer (best for hot data)                            │  │
│  │   └── Query restructuring (most maintainable)                          │  │
│  │                                                                        │  │
│  │ Current behavior: Agent picks one → User wanted different approach     │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  Scenario 3: User Preference Required                                        │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ User: "Set up testing framework"                                       │  │
│  │                                                                        │  │
│  │ User preferences matter:                                               │  │
│  │   ├── Jest vs Vitest vs Mocha                                          │  │
│  │   ├── Component testing? E2E testing?                                  │  │
│  │   └── Coverage thresholds?                                             │  │
│  │                                                                        │  │
│  │ Current behavior: Unstructured text questions lost in output           │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Root Causes

1. **No Structured Input Mechanism**: Agent can only receive plain text, no way to present choices
2. **Execution Cannot Pause**: Once started, agent runs to completion without checkpoints
3. **No UI for Multi-Select**: Cannot gather multiple preferences efficiently
4. **Questions Mixed with Output**: Important questions get lost in long responses

## Value Proposition

### Quantified Benefits

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           VALUE METRICS                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────┐    ┌─────────────────────┐                         │
│  │   WITHOUT TOOL      │    │    WITH TOOL        │                         │
│  ├─────────────────────┤    ├─────────────────────┤                         │
│  │ Rework Rate: ~40%   │ →  │ Rework Rate: ~5%    │  ↓ 87% reduction        │
│  │ Avg Iterations: 3-4 │ →  │ Avg Iterations: 1-2 │  ↓ 50% faster           │
│  │ User Satisfaction:  │ →  │ User Satisfaction:  │                         │
│  │   Medium            │    │   High              │  ↑ Clear expectations   │
│  └─────────────────────┘    └─────────────────────┘                         │
│                                                                              │
│  Key Improvements:                                                           │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ 1. CLARITY          │ Structured options eliminate ambiguity          │  │
│  │ 2. EFFICIENCY       │ Get preferences upfront, not after failure      │  │
│  │ 3. USER CONTROL     │ Users drive decisions, not agent assumptions    │  │
│  │ 4. TRANSPARENCY     │ Clear what agent is asking and why              │  │
│  │ 5. MULTI-SELECT     │ Gather multiple preferences in one interaction  │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Use Case Coverage

| Use Case | Without Tool | With Tool |
|----------|-------------|-----------|
| Technology choices | Guess → Fix | Ask → Correct first time |
| Configuration options | Default → Override | Present options → Match preference |
| Approach selection | Pick one → Iterate | Present trade-offs → User decides |
| Multi-feature enablement | Ask one-by-one | Multi-select in one prompt |

## Architecture Flow

### Execution Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     ASKUSERQUESTION EXECUTION FLOW                           │
└─────────────────────────────────────────────────────────────────────────────┘

┌────────────┐     ┌────────────┐     ┌────────────┐     ┌────────────┐
│   Agent    │────▶│  Detects   │────▶│   Calls    │────▶│   Tool     │
│   Running  │     │ Ambiguity  │     │ AskUser    │     │ Execution  │
└────────────┘     └────────────┘     └────────────┘     └────────────┘
                                                               │
                                                               ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           TOOL EXECUTION                                     │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ 1. Validate input (1-4 questions, 2-4 options each)                  │    │
│  │ 2. Emit special event: { type: 'ask_user', questions: [...] }        │    │
│  │ 3. Block execution - wait for user response                          │    │
│  │ 4. Return structured answers to agent                                │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
                                                               │
                                                               ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                             CLI LAYER                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                        QuestionPrompt UI                             │    │
│  │                                                                      │    │
│  │  ┌─ Database ─────────────────────────────────┐                     │    │
│  │  │ Which database should we use?              │                     │    │
│  │  │                                            │                     │    │
│  │  │ > ○ PostgreSQL (Recommended)               │  ← Arrow keys       │    │
│  │  │     Relational DB with rich features       │    to navigate      │    │
│  │  │                                            │                     │    │
│  │  │   ○ MongoDB                                │                     │    │
│  │  │     Document-based NoSQL database          │                     │    │
│  │  │                                            │                     │    │
│  │  │   ○ SQLite                                 │                     │    │
│  │  │     Lightweight embedded database          │                     │    │
│  │  │                                            │                     │    │
│  │  │   ○ Other                                  │  ← Always available │    │
│  │  │     Provide custom input                   │                     │    │
│  │  └────────────────────────────────────────────┘                     │    │
│  │                                                                      │    │
│  │  [Enter] Select  [↑↓] Navigate  [Space] Toggle (multi-select)       │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
                                                               │
                                                               ▼
┌────────────┐     ┌────────────┐     ┌────────────┐     ┌────────────┐
│   User     │────▶│  Answer    │────▶│  Tool      │────▶│   Agent    │
│  Selects   │     │ Collected  │     │  Returns   │     │  Continues │
└────────────┘     └────────────┘     └────────────┘     └────────────┘
```

### Component Interaction

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        COMPONENT ARCHITECTURE                                │
└─────────────────────────────────────────────────────────────────────────────┘

┌──────────────────┐        ┌──────────────────┐        ┌──────────────────┐
│   Agent Loop     │        │   AskUserQuestion│        │   CLI App        │
│  (agent.ts)      │        │   Tool           │        │  (App.tsx)       │
├──────────────────┤        ├──────────────────┤        ├──────────────────┤
│                  │        │                  │        │                  │
│  run() {         │        │  execute() {     │        │  [QuestionState] │
│    for event     │───────▶│    validate()    │───────▶│       ↓          │
│    of stream:    │        │    return {      │        │  <QuestionPrompt>│
│      ...         │        │      type: 'ask' │        │       ↓          │
│  }               │◀───────│      promise     │◀───────│  onAnswer()      │
│                  │        │    }             │        │                  │
└──────────────────┘        └──────────────────┘        └──────────────────┘
         │                           │                           │
         │                           │                           │
         ▼                           ▼                           ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                              EVENT FLOW                                       │
│                                                                               │
│  Agent                    Tool                       CLI                      │
│    │                        │                         │                       │
│    │──── tool_use ─────────▶│                         │                       │
│    │                        │──── ask_user ──────────▶│                       │
│    │                        │     (questions)         │                       │
│    │                        │                         │── Display UI          │
│    │                        │                         │                       │
│    │                        │◀─── answers ────────────│                       │
│    │◀─── tool_result ───────│                         │                       │
│    │     (formatted)        │                         │                       │
│    │                        │                         │                       │
│    ▼                        ▼                         ▼                       │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Usage Examples

### Example 1: Technology Choice

```typescript
// Agent detects need to set up database
// Instead of guessing, asks user:

AskUserQuestion({
  questions: [{
    question: "Which database should we use for this project?",
    header: "Database",
    options: [
      { label: "PostgreSQL (Recommended)", description: "Robust relational DB, great for complex queries" },
      { label: "MongoDB", description: "Document DB, flexible schema for rapid development" },
      { label: "SQLite", description: "Embedded DB, zero configuration, good for small apps" }
    ],
    multiSelect: false
  }]
})

// User selects: PostgreSQL
// Agent proceeds with PostgreSQL setup
```

**CLI Display:**
```
┌─ Database ─────────────────────────────────────────────────────────────────┐
│ Which database should we use for this project?                              │
│                                                                             │
│ > ○ PostgreSQL (Recommended)                                                │
│     Robust relational DB, great for complex queries                         │
│                                                                             │
│   ○ MongoDB                                                                 │
│     Document DB, flexible schema for rapid development                      │
│                                                                             │
│   ○ SQLite                                                                  │
│     Embedded DB, zero configuration, good for small apps                    │
│                                                                             │
│   ○ Other                                                                   │
│     Provide custom input                                                    │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Example 2: Multi-Select Features

```typescript
// Agent setting up a new React project
// Asks about features to enable:

AskUserQuestion({
  questions: [{
    question: "Which features should we enable?",
    header: "Features",
    options: [
      { label: "TypeScript", description: "Type safety and better IDE support" },
      { label: "ESLint + Prettier", description: "Code linting and formatting" },
      { label: "Testing (Vitest)", description: "Unit and component testing" },
      { label: "Tailwind CSS", description: "Utility-first CSS framework" }
    ],
    multiSelect: true
  }]
})

// User selects: TypeScript, ESLint + Prettier, Tailwind CSS
// Agent sets up project with selected features
```

**CLI Display:**
```
┌─ Features ─────────────────────────────────────────────────────────────────┐
│ Which features should we enable? (Select multiple with Space)              │
│                                                                             │
│   ☑ TypeScript                                                              │
│     Type safety and better IDE support                                      │
│                                                                             │
│   ☑ ESLint + Prettier                                                       │
│     Code linting and formatting                                             │
│                                                                             │
│   ☐ Testing (Vitest)                                                        │
│     Unit and component testing                                              │
│                                                                             │
│ > ☑ Tailwind CSS                                                            │
│     Utility-first CSS framework                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Example 3: Multiple Questions

```typescript
// Agent implementing authentication system
// Needs multiple decisions:

AskUserQuestion({
  questions: [
    {
      question: "Which authentication method should we use?",
      header: "Auth Method",
      options: [
        { label: "OAuth 2.0 (Recommended)", description: "Industry standard, supports social login" },
        { label: "JWT", description: "Stateless tokens, good for APIs" },
        { label: "Session-based", description: "Traditional cookie sessions" }
      ],
      multiSelect: false
    },
    {
      question: "Which OAuth providers should we support?",
      header: "Providers",
      options: [
        { label: "Google", description: "Most widely used" },
        { label: "GitHub", description: "Popular for developer tools" },
        { label: "Microsoft", description: "Enterprise integration" },
        { label: "Apple", description: "Required for iOS apps" }
      ],
      multiSelect: true
    }
  ]
})

// User selects: OAuth 2.0, Google + GitHub
// Agent implements OAuth with Google and GitHub providers
```

### Example 4: Custom "Other" Input

```typescript
AskUserQuestion({
  questions: [{
    question: "Which package manager do you prefer?",
    header: "Package Mgr",
    options: [
      { label: "npm", description: "Default Node.js package manager" },
      { label: "pnpm (Recommended)", description: "Fast, efficient disk space usage" },
      { label: "yarn", description: "Alternative with workspaces support" }
    ],
    multiSelect: false
  }]
})

// User selects: Other
// CLI prompts: "Please specify:"
// User types: "bun"
// Agent proceeds with bun as package manager
```

## Claude Code Reference

Claude Code's AskUserQuestion tool provides rich interactive questioning with these specifications:

### Parameter Schema (from Claude Code)

```typescript
interface AskUserQuestionTool {
  questions: Question[];              // Required: 1-4 questions
  answers?: Record<string, string>;   // User responses collected
  metadata?: {                        // Optional analytics
    source?: string;                  // e.g., "remember" for /remember command
  };
}

interface Question {
  question: string;      // Complete question text (required)
  header: string;        // Very short label, max 12 chars (required)
  multiSelect: boolean;  // Allow multiple selections (required)
  options: Option[];     // 2-4 choices (required)
}

interface Option {
  label: string;         // Display text, 1-5 words (required)
  description: string;   // Choice explanation (required)
}
```

### Constraints

- Header text: maximum 12 characters
- Option label: 1-5 words maximum
- Options per question: minimum 2, maximum 4
- Questions per call: minimum 1, maximum 4
- `multiSelect` must be explicitly specified (not optional)
- "Other" option is auto-added, don't include manually
- Recommended option should be first with "(Recommended)" suffix

### Usage Guidelines

From Claude Code's system prompt:
- Use when gathering preferences or clarifying ambiguous instructions
- Use `multiSelect: true` for non-mutually-exclusive options
- Put recommended option first with "(Recommended)" suffix
- In Plan Mode: use to clarify requirements BEFORE finalizing plan
- NOT for asking "Is my plan ready?" (use ExitPlanMode instead)

## Detailed Design

### API Design

```typescript
// src/tools/builtin/ask-user.ts

import { z } from 'zod';
import type { Tool, ToolResult, ToolContext } from '../types.js';

// Zod Schemas
export const QuestionOptionSchema = z.object({
  label: z.string().min(1).max(50).describe('Display text (1-5 words)'),
  description: z.string().min(1).max(200).describe('Explanation of the option'),
});

export const QuestionSchema = z.object({
  question: z.string().min(1).describe('The complete question to ask'),
  header: z.string().min(1).max(12).describe('Short label (max 12 chars)'),
  options: z.array(QuestionOptionSchema).min(2).max(4).describe('2-4 options'),
  multiSelect: z.boolean().describe('Allow multiple selections'),
});

export const AskUserQuestionInputSchema = z.object({
  questions: z.array(QuestionSchema).min(1).max(4).describe('1-4 questions'),
});

export type QuestionOption = z.infer<typeof QuestionOptionSchema>;
export type Question = z.infer<typeof QuestionSchema>;
export type AskUserQuestionInput = z.infer<typeof AskUserQuestionInputSchema>;

// Answer types
export interface QuestionAnswer {
  question: string;
  header: string;
  selectedOptions: string[];   // Labels of selected options
  customInput?: string;        // If "Other" was selected
}

export interface AskUserQuestionResult {
  answers: QuestionAnswer[];
}
```

### Tool Implementation

```typescript
// src/tools/builtin/ask-user.ts

export const askUserQuestionTool: Tool<AskUserQuestionInput> = {
  name: 'AskUserQuestion',
  description: loadToolDescription('ask-user'),
  parameters: AskUserQuestionInputSchema,

  async execute(input, context): Promise<ToolResult> {
    // Validation is handled by Zod schema

    // The actual questioning is handled by the CLI layer
    // Tool returns a special result that signals the agent loop
    // to pause and wait for user input

    // This is a placeholder - actual implementation requires
    // integration with the agent loop and CLI
    return {
      success: true,
      output: JSON.stringify({
        type: 'ask_user_question',
        questions: input.questions,
      }),
      metadata: {
        title: 'AskUserQuestion',
        subtitle: `${input.questions.length} question(s)`,
      },
    };
  },
};
```

### Extended ToolContext

```typescript
// src/tools/types.ts - Extended

export interface ToolContext {
  cwd: string;
  abortSignal?: AbortSignal;

  // New: User interaction callbacks
  askUser?: (questions: Question[]) => Promise<QuestionAnswer[]>;
}
```

### Agent Loop Integration

```typescript
// src/agent/agent.ts - Modified tool execution

// In the tool execution section:
if (call.name === 'AskUserQuestion') {
  // Special handling for AskUserQuestion
  const input = call.input as AskUserQuestionInput;

  // Emit special event for CLI to handle
  yield {
    type: 'ask_user',
    id: call.id,
    questions: input.questions
  };

  // Wait for response (this will be set by the CLI)
  const answers = await this.waitForUserAnswers(call.id);

  // Continue with answers as tool result
  toolResults.push({
    type: 'tool_result',
    toolUseId: call.id,
    content: formatAnswers(answers),
    isError: false,
  });
}
```

### New Agent Event Type

```typescript
// src/agent/types.ts

export type AgentEvent =
  | { type: 'text'; text: string }
  | { type: 'tool_start'; id: string; name: string; input: unknown }
  | { type: 'tool_result'; id: string; name: string; result: ToolResult }
  | { type: 'ask_user'; id: string; questions: Question[] }  // NEW
  | { type: 'done'; text: string }
  | { type: 'error'; error: Error };
```

### CLI Component

```tsx
// src/cli/components/QuestionPrompt.tsx

import { useState } from 'react';
import { Box, Text, useInput } from 'ink';
import { colors } from './theme.js';
import type { Question, QuestionAnswer } from '../../tools/builtin/ask-user.js';

interface QuestionPromptProps {
  questions: Question[];
  onComplete: (answers: QuestionAnswer[]) => void;
}

export function QuestionPrompt({ questions, onComplete }: QuestionPromptProps) {
  const [currentIndex, setCurrentIndex] = useState(0);
  const [answers, setAnswers] = useState<QuestionAnswer[]>([]);
  const [selectedOptions, setSelectedOptions] = useState<Set<string>>(new Set());
  const [optionIndex, setOptionIndex] = useState(0);
  const [showOtherInput, setShowOtherInput] = useState(false);
  const [otherInput, setOtherInput] = useState('');

  const currentQuestion = questions[currentIndex];
  const optionsWithOther = [...currentQuestion.options, {
    label: 'Other',
    description: 'Provide custom input'
  }];

  useInput((input, key) => {
    if (showOtherInput) {
      // Handle text input for "Other"
      if (key.return) {
        finishQuestion([...selectedOptions, 'Other'], otherInput);
      } else if (key.backspace) {
        setOtherInput(prev => prev.slice(0, -1));
      } else if (input && !key.ctrl) {
        setOtherInput(prev => prev + input);
      }
      return;
    }

    if (key.upArrow) {
      setOptionIndex(i => Math.max(0, i - 1));
    } else if (key.downArrow) {
      setOptionIndex(i => Math.min(optionsWithOther.length - 1, i + 1));
    } else if (key.return) {
      handleSelect();
    } else if (input === ' ' && currentQuestion.multiSelect) {
      toggleOption();
    }
  });

  const toggleOption = () => {
    const option = optionsWithOther[optionIndex];
    setSelectedOptions(prev => {
      const next = new Set(prev);
      if (next.has(option.label)) {
        next.delete(option.label);
      } else {
        next.add(option.label);
      }
      return next;
    });
  };

  const handleSelect = () => {
    const option = optionsWithOther[optionIndex];

    if (currentQuestion.multiSelect) {
      // Multi-select: Enter confirms all selections
      if (selectedOptions.size === 0) {
        toggleOption(); // Select current if none selected
      }
      if (selectedOptions.has('Other')) {
        setShowOtherInput(true);
      } else {
        finishQuestion([...selectedOptions]);
      }
    } else {
      // Single select
      if (option.label === 'Other') {
        setShowOtherInput(true);
      } else {
        finishQuestion([option.label]);
      }
    }
  };

  const finishQuestion = (selected: string[], customInput?: string) => {
    const answer: QuestionAnswer = {
      question: currentQuestion.question,
      header: currentQuestion.header,
      selectedOptions: selected.filter(s => s !== 'Other'),
      customInput,
    };

    const newAnswers = [...answers, answer];

    if (currentIndex < questions.length - 1) {
      setAnswers(newAnswers);
      setCurrentIndex(i => i + 1);
      setSelectedOptions(new Set());
      setOptionIndex(0);
      setShowOtherInput(false);
      setOtherInput('');
    } else {
      onComplete(newAnswers);
    }
  };

  return (
    <Box flexDirection="column" marginTop={1}>
      {/* Header */}
      <Box>
        <Text color={colors.primary}>─ {currentQuestion.header} </Text>
        <Text color={colors.textMuted}>{'─'.repeat(50)}</Text>
      </Box>

      {/* Question */}
      <Box marginTop={1}>
        <Text>{currentQuestion.question}</Text>
        {currentQuestion.multiSelect && (
          <Text color={colors.textMuted}> (Select multiple with Space)</Text>
        )}
      </Box>

      {/* Options */}
      <Box flexDirection="column" marginTop={1} paddingLeft={2}>
        {optionsWithOther.map((option, index) => {
          const isSelected = index === optionIndex;
          const isChecked = selectedOptions.has(option.label);
          const checkbox = currentQuestion.multiSelect
            ? (isChecked ? '☑' : '☐')
            : (isSelected ? '>' : ' ');

          return (
            <Box key={option.label} flexDirection="column">
              <Box>
                <Text color={isSelected ? colors.primary : colors.textMuted}>
                  {checkbox}
                </Text>
                <Text color={isSelected ? colors.text : colors.textSecondary}>
                  {currentQuestion.multiSelect ? '' : (isSelected ? '○' : '○')} {option.label}
                </Text>
              </Box>
              <Box paddingLeft={4}>
                <Text color={colors.textMuted}>{option.description}</Text>
              </Box>
            </Box>
          );
        })}
      </Box>

      {/* Other input */}
      {showOtherInput && (
        <Box marginTop={1} paddingLeft={2}>
          <Text color={colors.primary}>Please specify: </Text>
          <Text>{otherInput}</Text>
          <Text color={colors.textMuted}>_</Text>
        </Box>
      )}

      {/* Progress indicator */}
      {questions.length > 1 && (
        <Box marginTop={1}>
          <Text color={colors.textMuted}>
            Question {currentIndex + 1} of {questions.length}
          </Text>
        </Box>
      )}

      {/* Help */}
      <Box marginTop={1}>
        <Text color={colors.textMuted}>
          [Enter] Select  [↑↓] Navigate
          {currentQuestion.multiSelect && '  [Space] Toggle'}
        </Text>
      </Box>
    </Box>
  );
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/tools/builtin/ask-user.ts` | Create | AskUserQuestion tool implementation |
| `src/tools/types.ts` | Modify | Add AskUserQuestionInput schema |
| `src/tools/index.ts` | Modify | Register and export tool |
| `src/agent/types.ts` | Modify | Add `ask_user` event type |
| `src/agent/agent.ts` | Modify | Handle AskUserQuestion in tool loop |
| `src/cli/components/QuestionPrompt.tsx` | Create | Question UI component |
| `src/cli/components/App.tsx` | Modify | Integrate QuestionPrompt |
| `src/prompts/tools/ask-user.txt` | Create | Tool description for LLM |

## User Experience

### Claude Code UI Alignment

Our implementation follows Claude Code's exact visual patterns for consistency and familiarity.

#### Visual Design Reference (Claude Code Style)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        CLAUDE CODE UI PATTERNS                               │
└─────────────────────────────────────────────────────────────────────────────┘

1. HEADER AS CHIP/TAG (max 12 chars)
   ┌──────────────────────────────────────────────────────────────────────┐
   │  ╭─────────────╮                                                      │
   │  │  Database   │  ← Header displayed as colored chip/tag              │
   │  ╰─────────────╯                                                      │
   └──────────────────────────────────────────────────────────────────────┘

2. SINGLE-SELECT (Radio Buttons: ○ / ●)
   ┌──────────────────────────────────────────────────────────────────────┐
   │                                                                       │
   │  Which database should we use?                                        │
   │                                                                       │
   │  > ● PostgreSQL (Recommended)                                         │
   │      Robust relational DB, great for complex queries                  │
   │                                                                       │
   │    ○ MongoDB                                                          │
   │      Document DB, flexible schema                                     │
   │                                                                       │
   │    ○ SQLite                                                           │
   │      Lightweight embedded database                                    │
   │                                                                       │
   │    ○ Other                                                            │
   │      Type something else...                                           │
   │                                                                       │
   └──────────────────────────────────────────────────────────────────────┘

3. MULTI-SELECT (Checkboxes: ☐ / ☑)
   ┌──────────────────────────────────────────────────────────────────────┐
   │                                                                       │
   │  Which features should we enable?                                     │
   │                                                                       │
   │    ☑ TypeScript                                                       │
   │      Type safety and better IDE support                               │
   │                                                                       │
   │  > ☑ ESLint + Prettier                                                │
   │      Code linting and formatting                                      │
   │                                                                       │
   │    ☐ Testing (Vitest)                                                 │
   │      Unit and component testing                                       │
   │                                                                       │
   │    ☑ Tailwind CSS                                                     │
   │      Utility-first CSS framework                                      │
   │                                                                       │
   └──────────────────────────────────────────────────────────────────────┘

4. "OTHER" CUSTOM INPUT
   ┌──────────────────────────────────────────────────────────────────────┐
   │                                                                       │
   │  Please specify: bun█                                                 │
   │                                                                       │
   └──────────────────────────────────────────────────────────────────────┘

5. KEYBOARD HINTS (footer)
   ┌──────────────────────────────────────────────────────────────────────┐
   │  ↑↓ navigate  •  enter select  •  space toggle  •  esc cancel        │
   └──────────────────────────────────────────────────────────────────────┘
```

### Color Scheme (theme.ts aligned)

| Element | Color | Hex Value | Usage |
|---------|-------|-----------|-------|
| Header chip | `primary` | `#818CF8` | Question header tag |
| Selected option | `text` | `#F1F5F9` | Currently focused option |
| Unselected option | `textSecondary` | `#94A3B8` | Other options |
| Description | `textMuted` | `#64748B` | Option descriptions |
| Radio/Checkbox | `primary` | `#818CF8` | Selection indicators |
| Recommended | `success` | `#34D399` | "(Recommended)" suffix |
| Custom input cursor | `primary` | `#818CF8` | Text input cursor |

### Terminal Rendering

```tsx
// Exact rendering matching Claude Code patterns

// Header as chip
<Box>
  <Text color={colors.primary} bold>{'╭─'}</Text>
  <Text color={colors.primary} bold backgroundColor="#1E293B">
    {` ${header} `}
  </Text>
  <Text color={colors.primary} bold>{'─╮'}</Text>
</Box>

// Single-select option
<Box>
  <Text color={isSelected ? colors.text : colors.textMuted}>
    {isSelected ? '>' : ' '}
  </Text>
  <Text> </Text>
  <Text color={colors.primary}>
    {isChosen ? '●' : '○'}
  </Text>
  <Text color={isSelected ? colors.text : colors.textSecondary}>
    {' '}{label}
  </Text>
  {isRecommended && (
    <Text color={colors.success}> (Recommended)</Text>
  )}
</Box>

// Multi-select option
<Box>
  <Text color={isSelected ? colors.text : colors.textMuted}>
    {isSelected ? '>' : ' '}
  </Text>
  <Text> </Text>
  <Text color={colors.primary}>
    {isChecked ? '☑' : '☐'}
  </Text>
  <Text color={isSelected ? colors.text : colors.textSecondary}>
    {' '}{label}
  </Text>
</Box>

// Description (indented)
<Box paddingLeft={4}>
  <Text color={colors.textMuted}>{description}</Text>
</Box>
```

### Keyboard Navigation

| Key | Action | Context |
|-----|--------|---------|
| `↑` / `↓` | Navigate options | Always |
| `Enter` | Select current option | Single-select |
| `Enter` | Confirm all selections | Multi-select |
| `Space` | Toggle option on/off | Multi-select only |
| `Esc` | Cancel and dismiss | Optional |
| `1-4` | Quick select by number | Single-select |

### Answer Confirmation Display

After user answers, show Claude Code style confirmation:

```
╭─ Database ─╮
│ ✔ PostgreSQL
╰────────────╯

╭─ Features ─╮
│ ✔ TypeScript
│ ✔ ESLint + Prettier
│ ✔ Tailwind CSS
╰────────────╯
```

Or simplified inline format:

```
✔ Database: PostgreSQL
✔ Features: TypeScript, ESLint + Prettier, Tailwind CSS
```

### Tool Result Format

The tool returns a structured format that the agent can parse:

```
User answered the following questions:

1. Database (Which database should we use for this project?)
   Selected: PostgreSQL

2. Features (Which features should we enable?)
   Selected: TypeScript, ESLint + Prettier, Tailwind CSS

Proceeding with user selections.
```

### Progress Indicator (Multi-Question)

```
╭─ Auth Method ─╮  Question 1 of 2
│
│  Which authentication method should we use?
│
│  > ● OAuth 2.0 (Recommended)
│      Industry standard, supports social login
│
│    ○ JWT
│      Stateless tokens, good for APIs
│
│    ○ Session-based
│      Traditional cookie sessions
│
╰────────────────────────────────────────────────────────────────────╯
```

### Animation & Feedback

| State | Visual Feedback |
|-------|-----------------|
| Navigating | Cursor (`>`) moves instantly |
| Selecting | Radio/checkbox toggles with color change |
| Confirming | Brief flash of `success` color |
| Error | Red border flash if validation fails |
| Timeout | Dim with warning message (if enabled) |

## Security Considerations

1. **Input Sanitization**: Validate user custom input (max length, no control characters)
2. **Option Limits**: Enforce max questions (4) and options (4) per call
3. **Timeout**: Consider timeout for unresponsive users (configurable, default: none)
4. **No Code Execution**: Custom input is text only, never evaluated

## Testing Strategy

### Unit Tests

```typescript
// tests/tools/ask-user.test.ts

describe('AskUserQuestion', () => {
  test('validates question count (1-4)', () => {
    // Test with 0, 1, 4, 5 questions
  });

  test('validates option count (2-4)', () => {
    // Test with 1, 2, 4, 5 options
  });

  test('validates header length (max 12)', () => {
    // Test with headers of various lengths
  });

  test('requires multiSelect to be explicit', () => {
    // Ensure multiSelect is required
  });
});
```

### Integration Tests

```typescript
describe('AskUserQuestion Integration', () => {
  test('pauses agent execution until answered', async () => {
    // Verify agent waits for response
  });

  test('passes answers correctly to agent', async () => {
    // Verify answer format and content
  });

  test('handles Other option with custom input', async () => {
    // Test custom input flow
  });
});
```

### Manual Testing Checklist

- [ ] Single question, single select
- [ ] Single question, multi-select
- [ ] Multiple questions flow
- [ ] "Other" option with custom input
- [ ] Keyboard navigation (↑↓ Enter Space)
- [ ] Cancel/Escape handling
- [ ] Long option labels and descriptions
- [ ] Answer display after completion

## Migration Path

1. **Phase 1**: Core tool and basic select UI
2. **Phase 2**: Multi-select support
3. **Phase 3**: Enhanced UI with descriptions and progress
4. **Phase 4**: Keyboard shortcuts and accessibility

No breaking changes to existing functionality.

## Theme Extensions

Add the following icons to `src/cli/components/theme.ts`:

```typescript
export const icons = {
  // ... existing icons ...

  // AskUserQuestion specific
  checkbox: '☑',        // Checked checkbox
  checkboxEmpty: '☐',   // Empty checkbox
  chipLeft: '╭─',       // Chip border left
  chipRight: '─╮',      // Chip border right
  boxTop: '╭',          // Box top corner
  boxBottom: '╰',       // Box bottom corner
  boxVertical: '│',     // Box vertical line
};
```

## References

### Primary Sources
- [Claude Code AskUserQuestion Tool Guide](https://www.atcyrus.com/stories/claude-code-ask-user-question-tool-guide) - Comprehensive usage guide
- [Claude Code System Prompts - AskUserQuestion](https://github.com/Piebald-AI/claude-code-system-prompts/blob/main/system-prompts/tool-description-askuserquestion.md) - Official tool description
- [Internal Claude Code Tools Implementation](https://gist.github.com/bgauryy/0cdb9aa337d01ae5bd0c803943aa36bd) - Parameter schema reference
- [Claude Docs - Handle User Input](https://platform.claude.com/docs/en/agent-sdk/user-input) - Agent SDK integration

### UI/UX Research
- [GitHub Issue #12609 - Interactive UI for AskUserQuestion](https://github.com/anthropics/claude-code/issues/12609) - VS Code extension UI proposal
- [How Claude Code is Built - Pragmatic Engineer](https://newsletter.pragmaticengineer.com/p/how-claude-code-is-built) - Architecture insights
- [SmartScope - AskUserQuestion Guide](https://smartscope.blog/en/generative-ai/claude/claude-code-askuserquestion-tool-guide/) - Usage patterns

### Framework References
- [Ink - React for CLIs](https://github.com/vadimdemedes/ink) - Terminal UI framework
- [Inquirer.js](https://github.com/SBoudrias/Inquirer.js) - CLI prompt patterns
- [ccexp](https://github.com/nyatinte/ccexp) - Claude Code config explorer (UI reference)
