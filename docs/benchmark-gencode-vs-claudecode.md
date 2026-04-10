# GenCode vs Claude Code: Performance Benchmark

Comparison between **GenCode v1.12.0** (Go) and **Claude Code v2.1.96** (Node.js/TypeScript).

**Environment**: macOS Darwin 25.4.0, Apple Silicon (arm64)
**Model**: Both use `claude-sonnet-4-20250514` via Anthropic API
**Date**: 2026-04-10

---

## 1. Distribution Size

| Metric | GenCode | Claude Code | Ratio |
|--------|---------|-------------|-------|
| Download size | **12 MB** (.tar.gz) | 62 MB (npm) | **5x smaller** |
| Binary / Package (on disk) | 38 MB | 62 MB | 0.6x |
| Runtime dependency | None (static binary) | Node.js v24 (~112 MB) | - |
| Total disk footprint | **38 MB** | **~174 MB** (62 + 112) | **4.6x smaller** |
| File count | 1 | ~30 + node_modules | - |

GenCode ships as a single static binary with zero runtime dependencies. Claude Code requires Node.js and installs ~62 MB of npm packages (16 MB node_modules + 34 MB vendor).

---

## 2. Startup Time (`--version`)

| Run | GenCode | Claude Code |
|-----|---------|-------------|
| 1 | 0.05s | 0.19s |
| 2 | 0.01s | 0.18s |
| 3 | 0.01s | 0.18s |
| 4 | 0.01s | 0.18s |
| 5 | 0.01s | 0.19s |
| **Avg** | **~0.02s** | **~0.18s** |

GenCode starts **~9x faster**. The first run is slightly slower due to OS page cache; subsequent runs complete in ~10ms.

---

## 3. Startup Memory (`--version`)

| Run | GenCode | Claude Code |
|-----|---------|-------------|
| 1 | 32.8 MB | 185.0 MB |
| 2 | 32.8 MB | 185.8 MB |
| 3 | 33.0 MB | 185.1 MB |
| 4 | 32.9 MB | 185.0 MB |
| 5 | 33.1 MB | 185.4 MB |
| **Avg** | **~33 MB** | **~185 MB** |

GenCode uses **~5.6x less memory** at startup. The Node.js runtime alone accounts for a large portion of Claude Code's baseline.

---

## 4. Simple Task: "What is 2+2?"

Non-interactive print mode (`-p`), measuring total wall time and peak RSS.

| Run | GenCode (time / RSS) | Claude Code (time / RSS) |
|-----|----------------------|--------------------------|
| 1 | 4.68s / 38.8 MB | 13.67s / 275.6 MB |
| 2 | 6.90s / 38.5 MB | 11.10s / 275.6 MB |
| 3 | 3.72s / 40.2 MB | 13.34s / 284.1 MB |
| 4 | 3.99s / 39.0 MB | 11.59s / 285.7 MB |
| 5 | 6.83s / 39.6 MB | 9.92s / 287.4 MB |
| **Avg** | **5.22s / 39.2 MB** | **11.92s / 281.7 MB** |

- Response time: GenCode **~2.3x faster**
- Memory: GenCode **~7.2x less**

Note: Both tools use the same Anthropic API and model. The time difference reflects client-side overhead (startup, system prompt construction, session management, hooks, etc.), not LLM inference time.

---

## 5. File Read Task: "Read main.go and count lines"

Requires tool use (Read tool) + LLM response.

| Run | GenCode (time / RSS) | Claude Code (time / RSS) |
|-----|----------------------|--------------------------|
| 1 | 3.19s / 39.0 MB | 32.92s / 284.2 MB |
| 2 | 2.95s / 39.7 MB | 14.18s / 285.4 MB |
| 3 | 2.80s / 38.8 MB | 16.71s / 288.6 MB |
| **Avg** | **2.98s / 39.2 MB** | **21.27s / 286.1 MB** |

- Response time: GenCode **~7.1x faster**
- Memory: GenCode **~7.3x less**

Claude Code's longer times include CLAUDE.md loading, hook processing, LSP initialization, and other features GenCode doesn't implement yet.

---

## 6. Tool-Use Task: "Count .go files in internal/tool"

Requires Glob/Bash tool call + counting + response.

| Run | GenCode (time / RSS) | Claude Code (time / RSS) |
|-----|----------------------|--------------------------|
| 1 | 3.21s / 39.6 MB | 14.46s / 281.4 MB |
| 2 | 2.89s / 39.2 MB | 15.31s / 284.2 MB |
| 3 | 4.60s / 39.7 MB | 13.89s / 277.5 MB |
| **Avg** | **3.57s / 39.5 MB** | **14.55s / 281.0 MB** |

- Response time: GenCode **~4.1x faster**
- Memory: GenCode **~7.1x less**

---

## Summary

| Metric | GenCode | Claude Code | GenCode Advantage |
|--------|---------|-------------|-------------------|
| Download size | 12 MB | 62 MB (+ Node.js) | **5x smaller** |
| Disk footprint | 38 MB | 174 MB | **4.6x smaller** |
| Startup time | ~0.02s | ~0.18s | **9x faster** |
| Startup memory | ~33 MB | ~185 MB | **5.6x less** |
| Simple task time | ~5.2s | ~11.9s | **2.3x faster** |
| Simple task memory | ~39 MB | ~282 MB | **7.2x less** |
| Tool-use task time | ~3.6s | ~14.6s | **4.1x faster** |
| Tool-use task memory | ~40 MB | ~281 MB | **7.1x less** |

### Why the difference?

- **Language runtime**: Go compiles to native code with a lightweight runtime (~10 MB baseline). Node.js has a heavier runtime with JIT compilation, garbage collector, and V8 engine overhead (~185 MB baseline).
- **Startup path**: GenCode initializes a Bubble Tea TUI model. Claude Code loads hooks, LSP, plugin sync, CLAUDE.md discovery, OAuth/keychain, memory system, and more.
- **Architecture**: GenCode is a single static binary. Claude Code is a bundled TypeScript application running on Node.js with npm dependencies.
- **Feature scope**: Claude Code has significantly more features (IDE integration, OAuth, Chrome integration, Teams, prompt caching, auto-memory, etc.) which add overhead. GenCode is leaner but covers core CLI agentic coding functionality.

### Caveats

- LLM inference time is identical (same API, same model) — the difference is purely client-side overhead.
- Claude Code's additional startup work (hooks, LSP, etc.) provides features GenCode doesn't have yet.
- Network latency variance affects individual runs; averages across 3-5 runs are more reliable.
- Memory is measured as peak RSS; actual working set may differ.
