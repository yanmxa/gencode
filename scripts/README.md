# Scripts Directory

This directory contains utility scripts for GenCode development and testing.

## Directory Structure

```
scripts/
├── README.md                    # This file
├── migrate.ts                   # Migration script (.gencode → .gen)
└── tests/                       # Test scripts
    ├── test-component-loading.ts      # Component loading tests
    ├── test-commands-functional.ts    # Commands functional tests
    ├── test-commands-loading.ts       # Commands loading tests
    ├── test-functional-all.ts         # Comprehensive functional tests
    ├── test-hooks-functional.ts       # Hooks functional tests
    ├── test-hooks-loading.ts          # Hooks loading tests
    ├── test-mcp-loading.ts            # MCP loading tests
    ├── test-skills-functional.ts      # Skills functional tests
    ├── test-skills-loading.ts         # Skills loading tests
    ├── test-subagents-functional.ts   # Subagents functional tests
    └── test-subagents-loading.ts      # Subagents loading tests
```

## Utilities

### migrate.ts
Migration script for upgrading from old GenCode configuration format:
```bash
npx tsx scripts/migrate.ts
```

This will migrate:
- `~/.gencode/` → `~/.gen/`
- `./.gencode/` → `./.gen/`
- `AGENT.md` → `GEN.md`
- `AGENT.local.md` → `GEN.local.md`
- Old provider config format → New format

## Running Tests

All tests can be run via npm scripts defined in `package.json`:

### Component Tests
```bash
npm run test:components          # Component loading tests
```

### Loading Tests
```bash
npm run test:skills:load         # Skills loading
npm run test:commands:load       # Commands loading
npm run test:subagents:load      # Subagents loading
npm run test:hooks:load          # Hooks loading
npm run test:mcp:load            # MCP loading
```

### Functional Tests
```bash
npm run test:skills:func         # Skills functional tests
npm run test:commands:func       # Commands functional tests
npm run test:subagents:func      # Subagents functional tests
npm run test:hooks:func          # Hooks functional tests
npm run test:functional          # All functional tests
```

### Run All Tests
```bash
npm run test:all                 # Component + functional tests
```

## Test Types

### Loading Tests (`test-*-loading.ts`)
Test that components (skills, commands, subagents, hooks, MCP) load correctly from configuration.

### Functional Tests (`test-*-functional.ts`)
Test the end-to-end functionality of features with actual execution and validation.

### Component Tests (`test-component-loading.ts`)
Test core component initialization and loading mechanisms.

## Debug Mode

Functional tests can be run with verbose debug output:
```bash
GEN_DEBUG=2 npm run test:hooks:func
```

This will show detailed execution logs for troubleshooting.
