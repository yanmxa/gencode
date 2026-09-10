---
package: github.com/genai-io/san/internal/todo
layer: feature
---

# todo

Owns the agent-visible plan: ordered work items, dependency/blocker state, and
per-session persistence. This is deliberately separate from `internal/task`,
which tracks running background processes and subagents.

## Entrypoints

- `NewStore()` creates the concrete store.
- `Default()` exposes the compatibility singleton used by tool adapters.
- `SetDefaultStore` and `ResetDefaultStore` are test-only default overrides.
- `Create`, `Get`, `Update`, `Delete`, and `List` mutate/query plan items.
- `TrackWorker` and `CompleteWorker` project background job state into plan
  items through a narrow local interface.

The former package-wide `Service` interface was removed: it combined CRUD,
queries, persistence, and lifecycle into seventeen methods. App composition
uses `*Store`; consumers that need abstraction define a smaller role locally.

## Flow and persistence

`TaskCreate`/`TaskUpdate` tool adapters call the store, the TUI renders its
items, and session snapshots persist/export them. `SetStorageDir` scopes the
store to the active session.

## Tests and pitfalls

Tests live under `internal/todo/*_test.go` and `internal/tool/tasktools/`.
Do not use “task” to mean both a plan item and a running job in new Go APIs;
prefer “plan item” here and “background task/job” for `internal/task`.
