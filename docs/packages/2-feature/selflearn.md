---
package: github.com/genai-io/san/internal/selflearn
layer: feature
---

# selflearn

Runs bounded background reviews of completed foreground turns and writes
approved memory or skill updates through restricted tools.

## Entrypoints

- `New` creates the per-session reviewer and cadence state.
- `Reviewer.Observe` receives completed agent results.
- `RunReview` creates a headless `internal/agent/runtime` fork with a bounded
  deadline and restricted write tools.

## Flow

`internal/app` owns reviewer lifecycle and cancellation. A review receives a
snapshot of canonical `core.Message` values, inherits the parent model/system
prompt, trims pending user messages, and executes without a TUI outbox.

## Configuration and tests

Settings select the memory/skill review arms and cadence. Tests under
`internal/selflearn/` cover review parsing, memory and skill writes, and fork
message ordering. Review goroutines must remain session-scoped; clear, cwd
switch, and shutdown cancel them.
