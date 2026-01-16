# GenCode Prompt System Optimization Proposal

- **Proposal ID**: 0042
- **Author**: GenCode Team
- **Status**: Implemented
- **Created**: 2026-01-16
- **Updated**: 2026-01-16
- **Implemented**: 2026-01-16

---

## Table of Contents

1. [Overview](#1-overview)
2. [Current State Analysis](#2-current-state-analysis)
3. [Research Findings](#3-research-findings)
4. [Architecture Design](#4-architecture-design)
5. [Implementation Details](#5-implementation-details)
6. [Prompt Loading Timing](#6-prompt-loading-timing)
7. [Provider Comparison](#7-provider-comparison)
8. [Verification Plan](#8-verification-plan)
9. [Future Enhancements](#9-future-enhancements)
10. [Implementation Notes](#10-implementation-notes)
11. [References](#11-references)

---

## 1. Overview

This proposal outlines a comprehensive refactoring of GenCode's prompt system based on research of production AI coding assistants including Claude Code, OpenCode, Gemini CLI, and Cursor.

### 1.1 Goals

1. Improve prompt effectiveness and AI response quality
2. Establish a maintainable prompt architecture
3. Support provider-based and model-based prompt loading
4. Add task management capabilities

### 1.2 Scope

- System prompt optimization
- Enhanced tool descriptions
- Provider-specific and model-specific prompt adaptations
- TodoWrite tool implementation

---

## 2. Current State Analysis

### 2.1 Current GenCode Prompt System

#### 2.1.1 System Prompt (`src/agent/agent.ts`)

Currently only ~10 lines with limited guidance:

```typescript
const DEFAULT_SYSTEM_PROMPT = `You are a helpful AI assistant...
When using tools:
- Use Read to view file contents before editing
...
Be concise and focus on completing the user's task.`;
```

#### 2.1.2 Tool Descriptions

Inline in TypeScript files, only 1-2 lines:

```typescript
description: 'Read the contents of a file. Returns the file content with line numbers.',
```

### 2.2 Comparison Analysis

| # | Aspect | GenCode | Claude Code | OpenCode |
|---|--------|---------|-------------|----------|
| 1 | System prompt | ~10 lines | ~192 lines | ~100+ lines |
| 2 | Tool descriptions | Inline, 1-2 lines | 20-50 lines/tool | Separate `.txt` files |
| 3 | Provider adaptation | None | N/A | 5 variants |
| 4 | Examples | None | Extensive | Extensive |
| 5 | Maintainability | Low | Medium | High |

### 2.3 Problem Summary

1. **Insufficient Guidance**: System prompt too brief, lacking critical guidance
2. **Sparse Tool Descriptions**: Missing usage instructions and examples
3. **No Provider Adaptation**: All providers use the same prompt
4. **Hard to Maintain**: Prompts inline in code, requiring recompilation
5. **No Task Management**: Unable to track complex task progress

---

## 3. Research Findings

### 3.1 Content Patterns (What to Say)

#### 3.1.1 Tone & Style

All production tools emphasize:

1. Conciseness (fewer than 4 lines unless detailed response requested)
2. Direct communication (no preamble/postamble)
3. Minimal emojis (only when requested)
4. Professional objectivity over validation

#### 3.1.2 Proactiveness Boundaries

1. Do the right thing when asked
2. Don't surprise users with unsolicited actions
3. Ask for clarification rather than guessing

#### 3.1.3 Code Quality

1. Follow existing conventions in the codebase
2. Never assume library availability
3. No comments unless asked
4. Security best practices (no secrets exposure)

#### 3.1.4 Task Management

1. Use todo tools frequently
2. Break complex tasks into steps
3. Mark tasks complete immediately
4. Track only ONE task as in_progress at a time

### 3.2 Organization Patterns (How to Structure)

#### 3.2.1 OpenCode Pattern (Recommended)

```
src/
├── prompts/
│   ├── system/
│   │   ├── base.txt          # Common system prompt
│   │   ├── anthropic.txt     # Claude-specific
│   │   ├── openai.txt        # GPT-specific
│   │   └── gemini.txt        # Gemini-specific
│   └── tools/
│       ├── read.txt
│       ├── bash.txt
│       └── ...
└── tools/
    └── builtin/
        └── read.ts           # Imports from prompts/tools/read.txt
```

#### 3.2.2 Pattern Benefits

1. Prompts editable without code changes
2. Easy to compare/diff prompt changes
3. Version control friendly
4. Supports prompt engineering iteration

### 3.3 Tool Description Best Practices

#### 3.3.1 Effective Structure

```
1. Core function (1 line)
2. Assumptions/guarantees (2-3 lines)
3. Usage notes (bullet points)
4. Examples for complex tools
```

#### 3.3.2 Example Transformation

| # | Aspect | Before | After |
|---|--------|--------|-------|
| 1 | Line count | 1 line | 30-50 lines |
| 2 | Usage notes | None | Detailed bullets |
| 3 | Examples | None | Multiple examples |
| 4 | Edge cases | None | Explicitly documented |

---

## 4. Architecture Design

### 4.1 Directory Structure

```
src/prompts/
├── index.ts                 # Prompt loader module
├── system/
│   ├── base.txt            # Common system prompt (~100 lines)
│   ├── anthropic.txt       # Claude-specific adjustments
│   ├── openai.txt          # GPT-specific adjustments
│   ├── gemini.txt          # Gemini-specific adjustments
│   └── generic.txt         # Fallback for unknown providers
└── tools/
    ├── read.txt            # ~35 lines
    ├── write.txt           # ~25 lines
    ├── edit.txt            # ~40 lines
    ├── bash.txt            # ~80 lines (includes git workflow)
    ├── glob.txt            # ~20 lines
    ├── grep.txt            # ~25 lines
    ├── webfetch.txt        # ~30 lines
    ├── websearch.txt       # ~35 lines
    └── todowrite.txt       # ~50 lines
```

### 4.2 Prompt Loading Strategy

The prompt loading system uses a **model → provider → prompt** flow, leveraging the `~/.gencode/providers.json` configuration to automatically determine which prompt to use.

#### 4.2.1 Loading Flow

```
model ID (e.g., "claude-sonnet-4-5@20250929")
    │
    ▼
┌─────────────────────────────────────┐
│ Look up provider in providers.json  │
│ (search models.{provider}.list)     │
└──────────────┬──────────────────────┘
               │
    ┌──────────┴──────────┐
    │ Found?              │
    ▼                     ▼
┌────────┐          ┌─────────────┐
│ Yes    │          │ No          │
│ ↓      │          │ ↓           │
│ Use    │          │ Use fallback│
│ provider│         │ provider or │
│        │          │ 'generic'   │
└────┬───┘          └──────┬──────┘
     │                     │
     └──────────┬──────────┘
                ▼
┌─────────────────────────────────────┐
│ Map provider → prompt type          │
│ anthropic → anthropic.txt           │
│ openai    → openai.txt              │
│ gemini    → gemini.txt              │
│ (other)   → generic.txt             │
└──────────────┬──────────────────────┘
               ▼
┌─────────────────────────────────────┐
│ Load base.txt + {provider}.txt      │
│ Inject environment info             │
│ Add memory context if available     │
└─────────────────────────────────────┘
```

#### 4.2.2 Provider Lookup from providers.json

The `~/.gencode/providers.json` file stores cached models for each connected provider:

```json
{
  "models": {
    "anthropic": {
      "list": [
        { "id": "claude-sonnet-4-5@20250929", "name": "Claude Sonnet 4.5" },
        { "id": "claude-opus-4-1@20250805", "name": "Claude Opus 4.1" }
      ]
    },
    "gemini": {
      "list": [
        { "id": "gemini-2.5-pro", "name": "Gemini 2.5 Pro" }
      ]
    }
  }
}
```

Given a model ID, the system searches through each provider's model list to find the owning provider.

#### 4.2.3 Provider to Prompt Type Mapping

| # | Provider | Prompt Type | Description |
|---|----------|-------------|-------------|
| 1 | anthropic | anthropic | Claude series |
| 2 | openai | openai | GPT series |
| 3 | gemini | gemini | Gemini series |
| 4 | (unknown) | generic | Fallback for unknown providers |

#### 4.2.4 Fallback Strategy

If model lookup fails (model not in providers.json):
1. Use the fallback provider from agent config (if provided)
2. Otherwise, use `generic` prompt

This ensures the system always has a valid prompt to use.

---

## 5. Implementation Details

### 5.1 Prompt Loader Module

**File**: `src/prompts/index.ts`

```typescript
// 5.1.1 Provider Type Definition
export type ProviderType = 'anthropic' | 'openai' | 'gemini' | 'generic';

// 5.1.2 Look up provider for a model from ~/.gencode/providers.json
export function getProviderForModel(model: string): string | null {
  const config = loadProvidersConfig();
  if (!config?.models) return null;

  for (const [provider, cache] of Object.entries(config.models)) {
    if (cache.list?.some((m) => m.id === model)) {
      return provider;
    }
  }
  return null;
}

// 5.1.3 Map provider to prompt type
export function mapProviderToPromptType(provider: string): ProviderType {
  switch (provider) {
    case 'anthropic': return 'anthropic';
    case 'openai': return 'openai';
    case 'gemini': return 'gemini';
    default: return 'generic';
  }
}

// 5.1.4 Get prompt type for a model (model → provider → prompt)
export function getPromptTypeForModel(model: string, fallbackProvider?: string): ProviderType {
  const provider = getProviderForModel(model);
  if (provider) return mapProviderToPromptType(provider);
  if (fallbackProvider) return mapProviderToPromptType(fallbackProvider);
  return 'generic';
}

// 5.1.5 Build system prompt for a model (recommended API)
export function buildSystemPromptForModel(
  model: string,
  cwd: string,
  isGitRepo: boolean = false,
  memoryContext?: string,
  fallbackProvider?: string
): string {
  const promptType = getPromptTypeForModel(model, fallbackProvider);
  return buildSystemPromptWithMemory(promptType, cwd, isGitRepo, memoryContext);
}
```

### 5.2 Agent Integration

```typescript
// In Agent.run(), build system prompt using model → provider → prompt flow
const systemPrompt = this.config.systemPrompt ??
  buildSystemPromptForModel(
    this.config.model,                    // e.g., "claude-sonnet-4-5@20250929"
    this.config.cwd ?? process.cwd(),
    true,
    this.loadedMemory?.context,
    this.config.provider                  // Fallback if model lookup fails
  );
```

### 5.3 System Prompt Structure

**File**: `src/prompts/system/base.txt`

| # | Section | Lines | Content |
|---|---------|-------|---------|
| 1 | Identity & Purpose | ~5 | Define AI role |
| 2 | Tone & Style | ~15 | Conciseness, no emojis, markdown |
| 3 | Proactiveness | ~10 | Balance action vs asking |
| 4 | Following Conventions | ~15 | Code style, existing patterns |
| 5 | Task Management | ~30 | TodoWrite examples |
| 6 | Tool Usage Policy | ~20 | When to use which tool |
| 7 | Code References | ~5 | file:line format |
| 8 | Environment Info | Dynamic | Working dir, platform, date |

### 5.4 Provider-Specific Adjustments

#### 5.4.1 anthropic.txt - Claude-Specific

1. Extended thinking usage
2. Parallel tool call optimization
3. Professional objectivity emphasis
4. Claude documentation references

#### 5.4.2 openai.txt - GPT-Specific

1. Structured response preferences
2. Function calling best practices
3. Token efficiency guidelines

#### 5.4.3 gemini.txt - Gemini-Specific

1. Multimodal capability notes
2. Streaming response handling
3. Context window optimization

### 5.5 TodoWrite Tool

**File**: `src/tools/builtin/todowrite.ts`

#### 5.5.1 Input Schema

```typescript
export const TodoItemSchema = z.object({
  content: z.string().min(1).describe('Task description (imperative form)'),
  status: z.enum(['pending', 'in_progress', 'completed']),
  activeForm: z.string().min(1).describe('Present continuous form'),
});

export const TodoWriteInputSchema = z.object({
  todos: z.array(TodoItemSchema),
});
```

#### 5.5.2 Features

| # | Feature | Description |
|---|---------|-------------|
| 1 | State Management | pending/in_progress/completed |
| 2 | Single Task Limit | Only one in_progress at a time |
| 3 | Progress Display | Formatted output of current state |
| 4 | Validation Rules | Ensures todo list is valid |

### 5.6 File Changes Summary

| # | File | Type | Changes |
|---|------|------|---------|
| 1 | `src/prompts/index.ts` | New | Prompt loader module |
| 2 | `src/prompts/system/base.txt` | New | Common system prompt |
| 3 | `src/prompts/system/anthropic.txt` | New | Claude-specific |
| 4 | `src/prompts/system/openai.txt` | New | GPT-specific |
| 5 | `src/prompts/system/gemini.txt` | New | Gemini-specific |
| 6 | `src/prompts/system/generic.txt` | New | Fallback for unknown providers |
| 7 | `src/prompts/tools/*.txt` | New | 9 tool descriptions |
| 8 | `src/tools/builtin/todowrite.ts` | New | TodoWrite tool |
| 9 | `src/tools/index.ts` | Modified | Export TodoWrite |
| 10 | `src/tools/types.ts` | Modified | TodoWrite schema |
| 11 | `src/tools/builtin/*.ts` | Modified | Load descriptions from .txt |
| 12 | `src/agent/agent.ts` | Modified | Use provider-specific prompts |

---

## 6. Prompt Loading Timing

This section documents when and how prompts are loaded during the GenCode lifecycle.

### 6.1 Loading Sequence Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        GenCode Startup & Prompt Loading                      │
└─────────────────────────────────────────────────────────────────────────────┘

User starts GenCode CLI
         │
         ▼
┌─────────────────────┐
│ 1. CLI Initialization│
│    - Parse args      │
│    - Load config     │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 2. Agent Creation    │
│    - Get model ID    │
│    - Get provider    │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐     ┌─────────────────────────────┐
│ 3. Memory Loading    │────▶│ Load ~/.gencode/GENCODE.md  │
│    (if exists)       │     │ Load ./CLAUDE.md (fallback) │
└──────────┬──────────┘     └─────────────────────────────┘
           │
           ▼
┌─────────────────────┐     ┌─────────────────────────────┐
│ 4. First User Input  │────▶│ Triggers Agent.run()        │
└──────────┬──────────┘     └─────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────┐
│ 5. System Prompt Construction (LAZY - on first run)     │
│                                                         │
│    buildSystemPromptForModel(model, cwd, isGitRepo,     │
│                              memoryContext, provider)    │
│                                                         │
│    ┌─────────────────────────────────────────────────┐  │
│    │ a. getPromptTypeForModel(model, fallback)       │  │
│    │    - Look up provider in ~/.gencode/providers.json│
│    │    - Map provider → prompt type                  │  │
│    │    - Fallback to 'generic' if not found         │  │
│    └─────────────────────────────────────────────────┘  │
│                           │                             │
│                           ▼                             │
│    ┌─────────────────────────────────────────────────┐  │
│    │ b. loadSystemPrompt(promptType)                 │  │
│    │    - Read src/prompts/system/base.txt           │  │
│    │    - Read src/prompts/system/{provider}.txt     │  │
│    │    - Concatenate: base + provider               │  │
│    └─────────────────────────────────────────────────┘  │
│                           │                             │
│                           ▼                             │
│    ┌─────────────────────────────────────────────────┐  │
│    │ c. Inject Environment Info                      │  │
│    │    - Replace {{ENVIRONMENT}} placeholder        │  │
│    │    - Add cwd, platform, date, git status        │  │
│    └─────────────────────────────────────────────────┘  │
│                           │                             │
│                           ▼                             │
│    ┌─────────────────────────────────────────────────┐  │
│    │ d. Append Memory Context (if loaded)            │  │
│    │    - Wrap in <claudeMd> tags                    │  │
│    │    - Add importance instructions                │  │
│    └─────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────┐
│ 6. Tool Description Loading (LAZY - per tool)           │
│                                                         │
│    When tool is registered:                             │
│    loadToolDescription('read')                          │
│      → Read src/prompts/tools/read.txt                  │
│      → Return description string                        │
│                                                         │
│    Note: Tool descriptions are loaded at import time,   │
│    not at runtime. They are static after module load.   │
└─────────────────────────────────────────────────────────┘
           │
           ▼
┌─────────────────────┐
│ 7. LLM API Call      │
│    - System prompt   │
│    - User message    │
│    - Tool definitions│
└─────────────────────┘
```

### 6.2 Loading Timing Summary

| # | Component | When Loaded | Cached | Notes |
|---|-----------|-------------|--------|-------|
| 1 | Tool descriptions | Module import time | Yes (static) | Loaded once when tool file is imported |
| 2 | providers.json | First prompt build | No (re-read each time) | Could be cached for performance |
| 3 | base.txt | First Agent.run() | No (re-read each time) | Allows hot-reloading during development |
| 4 | {provider}.txt | First Agent.run() | No (re-read each time) | Same as base.txt |
| 5 | Memory context | Agent creation | Yes (per session) | Loaded once at session start |
| 6 | Environment info | Each prompt build | No (dynamic) | Reflects current state |

### 6.3 File Resolution

```
Tool descriptions resolution:
─────────────────────────────

Running from:                   Prompts loaded from:
dist/prompts/index.js    ───▶   src/prompts/system/*.txt
                                src/prompts/tools/*.txt

This allows editing .txt files without recompiling TypeScript.
The loader checks if running from dist/ and redirects to src/.
```

---

## 7. Provider Comparison

### 7.1 Prompt Line Count Comparison

| Provider | GenCode | OpenCode | Claude Code | Notes |
|----------|---------|----------|-------------|-------|
| Base | 108 | N/A | N/A | GenCode uses base + provider pattern |
| Anthropic | 29 | 105 | 192 | OpenCode/Claude are standalone |
| OpenAI | 29 | 143 | N/A | OpenCode uses copilot-gpt-5.txt |
| Gemini | 35 | 155 | N/A | OpenCode's most detailed prompt |
| Generic | 127 | N/A | N/A | GenCode fallback for unknown providers |
| **Total (Anthropic)** | **137** | **105** | **192** | base.txt + anthropic.txt |

### 7.2 Feature Coverage Comparison

| # | Feature | GenCode | OpenCode | Claude Code |
|---|---------|---------|----------|-------------|
| 1 | Tone & Style | ✅ | ✅ | ✅ |
| 2 | Conciseness guidance | ✅ | ✅ | ✅ |
| 3 | No emoji policy | ✅ | ✅ | ✅ |
| 4 | Proactiveness boundaries | ✅ | ✅ | ✅ |
| 5 | Code conventions | ✅ | ✅ | ✅ |
| 6 | Library verification | ✅ | ✅ | ✅ |
| 7 | Task management (TodoWrite) | ✅ | ✅ | ✅ |
| 8 | Tool usage policy | ✅ | ✅ | ✅ |
| 9 | Git safety protocol | ✅ | ✅ | ✅ |
| 10 | Code references (file:line) | ✅ | ✅ | ✅ |
| 11 | Security guidance | ✅ | ✅ | ✅ |
| 12 | New application workflow | ❌ | ✅ | ❌ |
| 13 | Extended thinking guidance | ✅ | ❌ | ❌ |
| 14 | Parallel tool optimization | ✅ | ✅ | ✅ |

### 7.3 Provider-Specific Adaptations

When adapting prompts for different providers, consider these adjustments:

#### 7.3.1 Anthropic (Claude)

```
Strengths to leverage:
- Extended thinking / chain-of-thought
- Parallel tool calling
- Professional objectivity
- Long context handling

Prompt adjustments:
- Emphasize thinking through complex problems
- Encourage parallel tool batching
- Reference Claude documentation for self-help
```

#### 7.3.2 OpenAI (GPT)

```
Strengths to leverage:
- Structured output generation
- Function calling reliability
- Code generation quality

Prompt adjustments:
- Prefer structured responses
- Use code blocks with language identifiers
- TypeScript over JavaScript when applicable
```

#### 7.3.3 Gemini

```
Strengths to leverage:
- Multimodal capabilities
- Large context windows
- Fast streaming responses

Prompt adjustments:
- Note image processing capability
- Emphasize structured formats (lists, tables)
- Context window optimization
```

#### 7.3.4 Generic (Unknown Providers)

```
Design principles:
- Comprehensive standalone guidance (no assumptions)
- All best practices included inline
- Extensive examples for clarity
- No provider-specific features assumed

The generic.txt is intentionally verbose (126 lines) to ensure
good performance with any LLM without prior knowledge of its capabilities.
```

### 7.4 Prompt Adaptation Notes

When adding support for new providers:

1. **Research the model**: Understand its strengths, weaknesses, and unique features
2. **Start from generic**: Copy generic.txt as a baseline
3. **Add provider strengths**: Include guidance that leverages unique capabilities
4. **Remove irrelevant sections**: Remove guidance for features the model lacks
5. **Test extensively**: Verify with real tasks before deployment
6. **Document differences**: Note what changed and why in this section

---

## 8. Verification Plan

### 8.1 Test Matrix

| # | Test Type | Content | Status |
|---|-----------|---------|--------|
| 1 | Unit Tests | Prompt loading correctness | ⏳ |
| 2 | Integration Tests | Agent tool selection | ⏳ |
| 3 | Comparison Tests | Response quality before/after | ⏳ |
| 4 | Provider Tests | Anthropic/OpenAI/Gemini | ⏳ |
| 5 | Build Tests | TypeScript compiles without errors | ✅ |

### 8.2 Success Metrics

| # | Metric | Target | Status |
|---|--------|--------|--------|
| 1 | Prompt file separation | `.txt` files independent | ✅ |
| 2 | System prompt coverage | All production patterns | ✅ |
| 3 | Tool description lines | 30-50 lines/tool | ✅ |
| 4 | Provider prompts | 3 variants | ✅ |
| 5 | TodoWrite tool | With detailed examples | ✅ |
| 6 | Behavior regression | No regression | ⏳ |

---

## 9. Future Enhancements

### 9.1 Short-Term

| # | Task | Priority | Description |
|---|------|----------|-------------|
| 1 | Model-specific prompts | High | Add o1, Gemini 2.0 specific prompts |
| 2 | More tool examples | High | Enhance tool descriptions |
| 3 | Git workflow refinement | High | Commit/PR guidance |
| 4 | Error handling guidance | Medium | Error recovery strategies |

### 9.2 Mid-Term

| # | Task | Priority | Description |
|---|------|----------|-------------|
| 1 | Agent subtype prompts | High | explore/plan/code agents |
| 2 | Prompt versioning | Medium | Version tracking |
| 3 | A/B testing framework | Medium | Effectiveness evaluation |

### 9.3 Long-Term

| # | Task | Priority | Description |
|---|------|----------|-------------|
| 1 | User feedback optimization | High | Based on usage data |
| 2 | Effectiveness metrics | Medium | Quantitative evaluation |
| 3 | Automated evaluation | Low | Quality auto-detection |

---

## 10. Implementation Notes

### 10.1 Files Created/Modified

| # | File | Type | Description |
|---|------|------|-------------|
| 1 | `src/prompts/index.ts` | Created | Prompt loader module with provider detection |
| 2 | `src/prompts/system/base.txt` | Created | Common system prompt (108 lines) |
| 3 | `src/prompts/system/anthropic.txt` | Created | Claude-specific adjustments (29 lines) |
| 4 | `src/prompts/system/openai.txt` | Created | GPT-specific adjustments (29 lines) |
| 5 | `src/prompts/system/gemini.txt` | Created | Gemini-specific adjustments (35 lines) |
| 6 | `src/prompts/system/generic.txt` | Created | Comprehensive fallback (126 lines) |
| 7 | `src/prompts/tools/read.txt` | Created | Read tool description |
| 8 | `src/prompts/tools/write.txt` | Created | Write tool description |
| 9 | `src/prompts/tools/edit.txt` | Created | Edit tool description |
| 10 | `src/prompts/tools/bash.txt` | Created | Bash tool description (61 lines) |
| 11 | `src/prompts/tools/glob.txt` | Created | Glob tool description |
| 12 | `src/prompts/tools/grep.txt` | Created | Grep tool description |
| 13 | `src/prompts/tools/webfetch.txt` | Created | WebFetch tool description |
| 14 | `src/prompts/tools/websearch.txt` | Created | WebSearch tool description |
| 15 | `src/prompts/tools/todowrite.txt` | Created | TodoWrite tool description (72 lines) |
| 16 | `src/tools/builtin/todowrite.ts` | Created | TodoWrite tool implementation |
| 17 | `src/tools/builtin/*.ts` | Modified | Load descriptions from .txt files |
| 18 | `src/agent/agent.ts` | Modified | Use buildSystemPromptForModel() |

### 10.2 Key Implementation Decisions

#### 10.2.1 Base + Provider Pattern

GenCode uses a **base + provider** pattern instead of standalone provider prompts:

```
Final Prompt = base.txt + {provider}.txt + environment + memory
```

**Rationale**:
- Avoids duplication of common guidance across providers
- Easier to maintain consistency
- Provider files focus only on unique characteristics

#### 10.2.2 Generic Prompt Design

The `generic.txt` is intentionally verbose (126 lines) because:

1. Unknown providers may have varying capabilities
2. Cannot assume any specific features are available
3. Must provide complete guidance standalone
4. Includes extensive examples for clarity

#### 10.2.3 Lazy Loading

Prompts are loaded lazily (on first use) rather than at startup:

- Allows hot-reloading during development
- Reduces startup time
- Memory efficient (only loads what's needed)

#### 10.2.4 File Resolution for Development

The loader detects when running from `dist/` and redirects to `src/`:

```typescript
if (__dirname.includes('/dist/')) {
  const srcPath = __dirname.replace('/dist/', '/src/');
  if (existsSync(srcPath)) {
    return srcPath;
  }
}
```

This allows editing `.txt` files without recompiling TypeScript.

### 10.3 Testing Results

```bash
# Build test
npm run build  # ✅ Passes

# Prompt loading test
node -e "import('./dist/prompts/index.js').then(m => console.log(m.buildSystemPrompt('anthropic', '/tmp', true).substring(0, 100)))"
# ✅ Returns system prompt correctly
```

### 10.4 Claude Code Essence Integration (Phase 2)

Added Claude Code's key guidance patterns to both `base.txt` and `generic.txt`:

| # | Guidance | Description |
|---|----------|-------------|
| 1 | Token minimization | "Minimize output tokens as much as possible" |
| 2 | CommonMark rendering | "Rendered in monospace font using CommonMark specification" |
| 3 | No preamble/postamble | Explicit list of forbidden phrases |
| 4 | Non-trivial command explanation | "Explain what the command does and why" |
| 5 | No preaching on refusals | "Don't say why, offer alternatives" |
| 6 | Additional examples | Added 4 more examples (2+2, golf balls, watch command, multi-turn) |

**Files modified**:
- `src/prompts/system/base.txt`: 108 → ~125 lines
- `src/prompts/system/generic.txt`: 127 → ~155 lines

### 10.5 Known Limitations

1. **No caching**: providers.json is re-read on each prompt build
2. **No model-specific prompts**: Only provider-level differentiation
3. **Static tool descriptions**: Cannot be changed at runtime
4. **No prompt versioning**: Changes are not tracked

---

## 11. References

### 11.1 Research Sources

| # | Source | Path | Key Insights |
|---|--------|------|--------------|
| 1 | Claude Code | `system-prompts-and-models-of-ai-tools/Anthropic/Claude Code/` | Tone, task management, examples |
| 2 | OpenCode | `opencode/packages/opencode/src/session/prompt/` | File organization, provider variants |
| 3 | Cursor | `system-prompts-and-models-of-ai-tools/Cursor Prompts/` | Tool definitions, agent patterns |

### 11.2 Key Files

| # | Project | File | Lines | Purpose |
|---|---------|------|-------|---------|
| 1 | Claude Code | `Prompt.txt` | 192 | System prompt |
| 2 | OpenCode | `anthropic.txt` | 106 | Claude prompt |
| 3 | OpenCode | `bash.txt` | 116 | Bash tool description |
| 4 | OpenCode | `todowrite.txt` | 89 | Task management description |

---

## Appendix

### A. Glossary

| Term | Definition |
|------|------------|
| System Prompt | Base instructions defining AI behavior |
| Tool Description | Instructions telling AI how to use a tool |
| Provider | LLM provider (Anthropic/OpenAI/Google) |
| Provider-Based Loading | Loading prompts based on the LLM provider |
| Model-Based Loading | Loading prompts based on the specific model |
| TodoWrite | Task management tool for tracking work progress |

### B. Related Documentation

1. [GenCode README](../../README.md)
2. [Provider Documentation](../providers.md)
3. [Tool Development Guide](../tools.md)
