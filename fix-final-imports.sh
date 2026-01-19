#!/bin/bash
# Fix remaining import issues after architecture reorganization

set -e

echo "Fixing remaining import issues..."

# Fix 1: core/ internal - access to permissions/memory/pricing
echo "Fix 1: core/ internal imports..."
find src/core/agent -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../../permissions/|from '../permissions/|g" \
    -e "s|from '../../memory/|from '../memory/|g" \
    -e "s|from '../../pricing/|from '../pricing/|g" \
    "$file"
done

find src/core/providers -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../../pricing/|from '../pricing/|g" \
    "$file"
done

find src/core/session -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../../pricing/|from '../pricing/|g" \
    "$file"
done

# Fix 2: core/memory accessing infrastructure/config
echo "Fix 2: core/memory → infrastructure/config..."
sed -i '' \
  -e "s|from '../config/types.js'|from '../../infrastructure/config/types.js'|g" \
  src/core/memory/memory-manager.ts

# Fix 3: core/agent accessing cli/planning
echo "Fix 3: core/agent → cli/planning..."
sed -i '' \
  -e "s|from '../planning/index.js'|from '../../cli/planning/index.js'|g" \
  src/core/agent/agent.ts

# Fix 4: core/tools accessing extensions/mcp
echo "Fix 4: core/tools → extensions/mcp..."
sed -i '' \
  -e "s|from '../mcp/index.js'|from '../../extensions/mcp/index.js'|g" \
  src/core/tools/index.ts

# Fix 5: cli/components imports
echo "Fix 5: cli/components imports..."
sed -i '' \
  -e "s|from '../../tasks/task-manager.js'|from '../../../core/session/tasks/task-manager.js'|g" \
  -e "s|from '../../commands/index.js'|from '../../../extensions/commands/index.js'|g" \
  src/cli/components/App.tsx

# Fix 6: cli/index.tsx
echo "Fix 6: cli/index.tsx..."
sed -i '' \
  -e "s|from '../agent/agent.js'|from '../core/agent/agent.js'|g" \
  src/cli/index.tsx

# Fix 7: extensions/ accessing infrastructure/utils
echo "Fix 7: extensions/ → infrastructure/utils..."
find src/extensions/commands -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../../infrastructure/debug.js'|from '../../infrastructure/utils/debug.js'|g" \
    -e "s|from '../../infrastructure/logger.js'|from '../../infrastructure/utils/logger.js'|g" \
    -e "s|from '../../infrastructure/validation.js'|from '../../infrastructure/utils/validation.js'|g" \
    -e "s|from '../../infrastructure/config-validator.js'|from '../../infrastructure/utils/config-validator.js'|g" \
    "$file"
done

find src/extensions/skills -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../infrastructure/|from '../../infrastructure/|g" \
    -e "s|from '../../infrastructure/validation.js'|from '../../infrastructure/utils/validation.js'|g" \
    -e "s|from '../../infrastructure/config-validator.js'|from '../../infrastructure/utils/config-validator.js'|g" \
    "$file"
done

find src/extensions/mcp -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../../infrastructure/path-utils.js'|from '../../infrastructure/utils/path-utils.js'|g" \
    -e "s|from '../../infrastructure/logger.js'|from '../../infrastructure/utils/logger.js'|g" \
    -e "s|from '../../infrastructure/debug.js'|from '../../infrastructure/utils/debug.js'|g" \
    -e "s|from '../core/tools/types.js'|from '../../core/tools/types.js'|g" \
    "$file"
done

find src/extensions/subagents -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../../infrastructure/logger.js'|from '../../infrastructure/utils/logger.js'|g" \
    -e "s|from '../core/|from '../../core/|g" \
    -e "s|from './custom-agent-loader.js'|from './manager.js'|g" \
    "$file"
done

# Fix 8: extensions/hooks accessing infrastructure/utils
echo "Fix 8: extensions/hooks → infrastructure/utils..."
find src/extensions/hooks -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../utils/debug.js'|from '../../infrastructure/utils/debug.js'|g" \
    -e "s|from '../utils/logger.js'|from '../../infrastructure/utils/logger.js'|g" \
    -e "s|from '../utils/format-utils.js'|from '../../infrastructure/utils/format-utils.js'|g" \
    "$file"
done

echo "✓ Final import fixes complete!"
