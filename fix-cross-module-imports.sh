#!/bin/bash
# Fix cross-module imports (imports between core, common, extensions, cli)

set -e

echo "Fixing cross-module imports..."

# Files in core/ importing from common/ (need ../)
echo "Pass 1: core/ → common/ imports..."
find src/core -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../common/|from '../../common/|g" \
    -e "s|from '../../common/common/|from '../../common/|g" \
    "$file"
done

# Files in core/ importing from extensions/ (need ../)
echo "Pass 2: core/ → extensions/ imports..."
find src/core -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../extensions/|from '../../extensions/|g" \
    "$file"
done

# Files in core/ importing from cli/ (need ../)
echo "Pass 3: core/ → cli/ imports..."
find src/core -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../cli/|from '../../cli/|g" \
    "$file"
done

# Files in common/ importing from core/ (need ../)
echo "Pass 4: common/ → core/ imports..."
find src/common -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../core/|from '../../core/|g" \
    "$file"
done

# Files in cli/components importing from other cli/ modules
echo "Pass 5: cli/components/ imports..."
sed -i '' \
  -e "s|from '../../commands/index.js'|from '../../../extensions/commands/index.js'|g" \
  -e "s|from '../../tasks/task-manager.js'|from '../../../core/session/tasks/task-manager.js'|g" \
  src/cli/components/App.tsx

# cli/index.tsx
echo "Pass 6: cli/index.tsx imports..."
sed -i '' \
  -e "s|from '../agent/agent.js'|from '../core/agent/agent.js'|g" \
  src/cli/index.tsx

# core/agent/agent.ts - fix remaining planning import
echo "Pass 7: core/agent/agent.ts planning import..."
sed -i '' \
  -e "s|from '../planning/index.js'|from '../../cli/planning/index.js'|g" \
  src/core/agent/agent.ts

# core/session/tasks/output-streamer.ts - fix agent import
echo "Pass 8: core/session/tasks/output-streamer.ts import..."
sed -i '' \
  -e "s|from '../agent/types.js'|from '../../agent/types.js'|g" \
  src/core/session/tasks/output-streamer.ts

# core/session/types.ts - fix extensions import
echo "Pass 9: core/session/types.ts import..."
sed -i '' \
  -e "s|from '../extensions/subagents/types.js'|from '../../../extensions/subagents/types.js'|g" \
  src/core/session/types.ts

echo "Cross-module import fixes complete!"
