#!/bin/bash
# Final comprehensive import path updates for new architecture

set -e

echo "Updating imports for new architecture..."

# Phase 1: common/ → infrastructure/
echo "Phase 1: Updating common/ → infrastructure/..."
find src -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../common/|from '../infrastructure/|g" \
    -e "s|from '../../common/|from '../../infrastructure/|g" \
    -e "s|from '../../../common/|from '../../../infrastructure/|g" \
    -e "s|from '../../../../common/|from '../../../../infrastructure/|g" \
    "$file"
done

# Phase 2: common/permissions/ → core/permissions/
echo "Phase 2: Updating permissions imports..."
find src -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../infrastructure/permissions/|from '../core/permissions/|g" \
    -e "s|from '../../infrastructure/permissions/|from '../../core/permissions/|g" \
    -e "s|from '../../../infrastructure/permissions/|from '../../../core/permissions/|g" \
    "$file"
done

# Phase 3: common/memory/ → core/memory/
echo "Phase 3: Updating memory imports..."
find src -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../infrastructure/memory/|from '../core/memory/|g" \
    -e "s|from '../../infrastructure/memory/|from '../../core/memory/|g" \
    -e "s|from '../../../infrastructure/memory/|from '../../../core/memory/|g" \
    "$file"
done

# Phase 4: common/pricing/ → core/pricing/
echo "Phase 4: Updating pricing imports..."
find src -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../infrastructure/pricing/|from '../core/pricing/|g" \
    -e "s|from '../../infrastructure/pricing/|from '../../core/pricing/|g" \
    -e "s|from '../../../infrastructure/pricing/|from '../../../core/pricing/|g" \
    "$file"
done

# Phase 5: common/hooks/ → extensions/hooks/
echo "Phase 5: Updating hooks imports..."
find src -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../infrastructure/hooks/|from '../extensions/hooks/|g" \
    -e "s|from '../../infrastructure/hooks/|from '../../extensions/hooks/|g" \
    -e "s|from '../../../infrastructure/hooks/|from '../../../extensions/hooks/|g" \
    "$file"
done

# Phase 6: Fix infrastructure/ internal imports (remove redundant infrastructure/)
echo "Phase 6: Fixing infrastructure/ internal imports..."
find src/infrastructure -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../infrastructure/|from '../|g" \
    "$file"
done

# Phase 7: Fix core/ internal imports for moved modules
echo "Phase 7: Fixing core/ internal imports..."
find src/core -name "*.ts" | while read file; do
  # Fix permissions/memory/pricing imports within core/
  sed -i '' \
    -e "s|from '../../core/permissions/|from '../../permissions/|g" \
    -e "s|from '../../core/memory/|from '../../memory/|g" \
    -e "s|from '../../core/pricing/|from '../../pricing/|g" \
    "$file"
done

# Phase 8: Fix extensions/ internal imports for hooks
echo "Phase 8: Fixing extensions/ internal imports..."
find src/extensions -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../extensions/hooks/|from '../hooks/|g" \
    "$file"
done

# Phase 9: Update src/index.ts (main entry point)
echo "Phase 9: Updating src/index.ts..."
sed -i '' \
  -e "s|from './providers/|from './core/providers/|g" \
  -e "s|from './tools/|from './core/tools/|g" \
  -e "s|from './permissions/|from './core/permissions/|g" \
  -e "s|from './agent/|from './core/agent/|g" \
  -e "s|from './session/|from './core/session/|g" \
  -e "s|from './checkpointing/|from './core/session/checkpointing/|g" \
  -e "s|from './config/|from './infrastructure/config/|g" \
  -e "s|from './memory/|from './core/memory/|g" \
  -e "s|from './discovery/|from './infrastructure/discovery/|g" \
  -e "s|from './skills/|from './extensions/skills/|g" \
  -e "s|from './subagents/|from './extensions/subagents/|g" \
  -e "s|from './commands/|from './extensions/commands/|g" \
  -e "s|from './mcp/|from './extensions/mcp/|g" \
  src/index.ts

# Phase 10: Update test files in scripts/
echo "Phase 10: Updating test files..."
find scripts -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../src/common/|from '../src/infrastructure/|g" \
    -e "s|from '../src/infrastructure/permissions/|from '../src/core/permissions/|g" \
    -e "s|from '../src/infrastructure/memory/|from '../src/core/memory/|g" \
    -e "s|from '../src/infrastructure/pricing/|from '../src/core/pricing/|g" \
    -e "s|from '../src/infrastructure/hooks/|from '../src/extensions/hooks/|g" \
    "$file"
done

# Phase 11: Fix over-corrections (infrastructure/infrastructure/, core/core/, etc.)
echo "Phase 11: Fixing over-corrected imports..."
find src -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../infrastructure/infrastructure/|from '../infrastructure/|g" \
    -e "s|from '../../infrastructure/infrastructure/|from '../../infrastructure/|g" \
    -e "s|from '../core/core/|from '../core/|g" \
    -e "s|from '../../core/core/|from '../../core/|g" \
    -e "s|from '../extensions/extensions/|from '../extensions/|g" \
    -e "s|from '../../extensions/extensions/|from '../../extensions/|g" \
    "$file"
done

echo "✓ Import path updates complete!"
echo "Run 'npm run build' to verify changes."
