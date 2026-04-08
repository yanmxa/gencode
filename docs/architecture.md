# Architecture

This repository is organized around a small number of clear layers.
The goal is to keep feature work easy to place, easy to review, and hard to spread across unrelated packages.

## Design Goals

- Keep domain capabilities independent from Bubble Tea and terminal UI concerns.
- Keep interactive app orchestration in one place instead of scattering it across many root-level packages.
- Prefer feature-oriented packages over generic "utils" or catch-all folders.
- Make it obvious where new code belongs before writing it.

## Layers

### 1. Entrypoints

Use `cmd/` for binaries and CLI bootstrap only.

- `cmd/gen` wires flags, env loading, logging, and starts the app.
- `cmd/rendercheck` is a standalone utility binary.

Entrypoints should stay thin. They should parse input, assemble options, and hand off to the app/runtime layer.

### 2. App Shell

Use `internal/app` for the interactive application shell.
This is the only layer that should know about Bubble Tea top-level model wiring, routing, view composition, and interactive session flow.

This layer is responsible for:

- top-level model assembly
- message routing
- command dispatch
- view composition
- startup and shutdown flow
- coordination across features during an interactive session

Subpackages under `internal/app/*` should be feature-specific UI/application modules such as provider selection, plugin management, session selection, memory editing, and tool execution state.

This layer may depend on domain packages such as `internal/provider`, `internal/tool`, `internal/plugin`, `internal/mcp`, and `internal/core`.

### 3. Domain Capabilities

Use root `internal/*` packages for reusable capabilities that are not specific to the interactive shell.

Examples:

- `internal/core`: reusable agent loop/runtime
- `internal/provider`: provider implementations and provider registry
- `internal/tool`: built-in tool definitions and execution registry
- `internal/plugin`: plugin loading and integration
- `internal/skill`: skill loading and registry
- `internal/mcp`: MCP protocol integration and registry
- `internal/task`: task runtime and state
- `internal/config`: settings, permissions, and config loading
- `internal/transcriptstore`: persisted transcript/session storage

These packages should remain usable from multiple entrypoints, including TUI, headless agent execution, tests, and future non-TUI interfaces.

### 4. Shared UI Building Blocks

Use `internal/ui` only for reusable presentational pieces and shared UI helpers.

Examples:

- styles
- themes
- shared UI messages
- reusable selector/history/progress widgets

Do not put business rules, provider logic, or persistence logic in `internal/ui`.

### 5. Tests and Docs

- `tests/integration` is for cross-package behavioral verification.
- `docs/features` is for user-facing or feature-focused documentation.
- top-level docs in `docs/` are for architecture and subsystem design.

## Dependency Direction

Keep dependencies pointing inward toward reusable capabilities.

Allowed direction:

`cmd -> internal/app -> internal/{core,provider,tool,plugin,skill,mcp,...}`

Also allowed:

`internal/app/<feature> -> internal/app/<shared feature helpers>`
`internal/ui -> internal/ui/*`
`internal/core -> internal/{message,permission,system,tool,...}`

Avoid the reverse direction:

- domain packages should not import `internal/app`
- domain packages should not depend on Bubble Tea types
- `internal/ui` should not become a back door for domain behavior

If a package must understand `tea.Msg`, `tea.Cmd`, textarea, selector focus, or terminal rendering, it belongs in `internal/app` or `internal/ui`, not in domain packages.

## Placement Rules

When adding code, use these rules.

### Put code in `internal/app` when it:

- manages interactive state
- handles slash commands or key events
- renders terminal views
- coordinates multiple domain packages during one UI flow
- depends on Bubble Tea model/update/view semantics

### Put code in a root `internal/*` package when it:

- can run without Bubble Tea
- represents a reusable subsystem or registry
- loads/saves data
- talks to providers, plugins, MCP, tools, or the filesystem as a reusable capability
- should be testable outside the interactive shell

### Put code in `internal/ui` when it:

- is presentational
- is reusable across multiple app features
- does not own business state or persistence

## Package Shape Guidelines

Prefer packages shaped by feature or subsystem responsibility.

Good:

- `internal/app/provider`
- `internal/app/session`
- `internal/provider/openai`
- `internal/plugin`

Avoid:

- `internal/helpers`
- `internal/utils`
- `internal/misc`
- large grab-bag packages with unrelated behaviors

Within a package:

- keep one package focused on one responsibility
- split by behavior only when files become hard to navigate
- avoid duplicating metadata in one package and execution logic in another when both describe the same concept

## Current Structure Guidance

The current repository already has the right broad split:

- `cmd/` for entrypoints
- `internal/app/*` for interactive orchestration
- root `internal/*` packages for reusable capabilities
- `internal/ui/*` for shared UI pieces

The main thing to preserve going forward is this rule:
do not let top-level interactive orchestration leak into domain packages, and do not let new features bypass the existing feature-oriented `internal/app/*` modules by adding unrelated root-level files.

## Refactoring Direction

When restructuring, prefer incremental moves that improve boundaries without churning the whole repository.

Recommended order:

1. Consolidate structure documentation before moving code.
2. Keep command metadata and command execution registered from a single source.
3. Continue pushing feature-specific interactive state into `internal/app/<feature>`.
4. Keep `internal/core` and other reusable packages free of TUI-specific concerns.
5. Correct docs whenever package names or boundaries change.

## Review Checklist

Before merging structural changes, verify:

- Can a new contributor tell where a new file should go?
- Does the new package have one clear responsibility?
- Does any domain package now depend on `internal/app` or Bubble Tea?
- Did we introduce duplicate registries or duplicate sources of truth?
- Did we update docs to match the new structure?
