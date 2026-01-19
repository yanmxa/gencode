#!/bin/bash
# Fix remaining import issues

set -e

echo "Fixing remaining imports..."

# cli/components - fix planning, tasks, commands imports
echo "Pass 1: cli/components imports..."
find src/cli/components -name "*.tsx" -o -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../../planning/|from '../planning/|g" \
    -e "s|from '../../tasks/|from '../../../core/session/tasks/|g" \
    -e "s|from '../../commands/|from '../../../extensions/commands/|g" \
    "$file"
done

# cli/index.tsx - fix agent import
echo "Pass 2: cli/index.tsx import..."
sed -i '' \
  -e "s|from '../agent/agent.js'|from '../core/agent/agent.js'|g" \
  src/cli/index.tsx

# core/agent/agent.ts - fix planning import
echo "Pass 3: core/agent/agent.ts import..."
sed -i '' \
  -e "s|from '../planning/index.js'|from '../../cli/planning/index.js'|g" \
  src/core/agent/agent.ts

# core/tools/registry.ts - fix checkpointing import
echo "Pass 4: core/tools/registry.ts import..."
sed -i '' \
  -e "s|from '../checkpointing/index.js'|from '../session/checkpointing/index.js'|g" \
  src/core/tools/registry.ts

# core/tools/index.ts - fix mcp import
echo "Pass 5: core/tools/index.ts import..."
sed -i '' \
  -e "s|from '../mcp/index.js'|from '../../extensions/mcp/index.js'|g" \
  src/core/tools/index.ts

# extensions/commands - fix common imports
echo "Pass 6: extensions/commands imports..."
find src/extensions/commands -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../common/|from '../../common/|g" \
    "$file"
done

# extensions/mcp - fix common and bridge imports
echo "Pass 7: extensions/mcp imports..."
find src/extensions/mcp -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../common/|from '../../common/|g" \
    -e "s|from './bridge.js'|from '../../core/tools/factories/mcp-tool-factory.js'|g" \
    "$file"
done

# extensions/subagents - fix common imports
echo "Pass 8: extensions/subagents imports..."
find src/extensions/subagents -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../common/|from '../../common/|g" \
    "$file"
done

echo "Remaining import fixes complete!"
