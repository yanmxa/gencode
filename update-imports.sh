#!/bin/bash
# Comprehensive import path update script for directory reorganization

set -e

echo "Starting import path updates..."

# Pass 1: Update common/utils imports
echo "Pass 1: Updating common/utils imports..."
find src -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../common/logger'|from '../common/utils/logger'|g" \
    -e "s|from '../common/validation'|from '../common/utils/validation'|g" \
    -e "s|from '../common/debug'|from '../common/utils/debug'|g" \
    -e "s|from '../common/path-utils'|from '../common/utils/path-utils'|g" \
    -e "s|from '../common/format-utils'|from '../common/utils/format-utils'|g" \
    -e "s|from '../common/stream-utils'|from '../common/utils/stream-utils'|g" \
    -e "s|from '../common/config-validator'|from '../common/utils/config-validator'|g" \
    -e "s|from '../common/loading-reporter'|from '../common/utils/loading-reporter'|g" \
    -e "s|from '../../common/logger'|from '../../common/utils/logger'|g" \
    -e "s|from '../../common/validation'|from '../../common/utils/validation'|g" \
    -e "s|from '../../common/debug'|from '../../common/utils/debug'|g" \
    -e "s|from '../../common/path-utils'|from '../../common/utils/path-utils'|g" \
    -e "s|from '../../common/format-utils'|from '../../common/utils/format-utils'|g" \
    -e "s|from '../../common/stream-utils'|from '../../common/utils/stream-utils'|g" \
    -e "s|from '../../common/config-validator'|from '../../common/utils/config-validator'|g" \
    -e "s|from '../../common/loading-reporter'|from '../../common/utils/loading-reporter'|g" \
    "$file"
done

# Pass 2: Update core module imports (single ../)
echo "Pass 2: Updating core module imports (single ../)..."
find src -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../agent/|from '../core/agent/|g" \
    -e "s|from '../providers/|from '../core/providers/|g" \
    -e "s|from '../tools/|from '../core/tools/|g" \
    -e "s|from '../session/|from '../core/session/|g" \
    "$file"
done

# Pass 3: Update common module imports (single ../)
echo "Pass 3: Updating common module imports (single ../)..."
find src -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../config/|from '../common/config/|g" \
    -e "s|from '../memory/|from '../common/memory/|g" \
    -e "s|from '../permissions/|from '../common/permissions/|g" \
    -e "s|from '../hooks/|from '../common/hooks/|g" \
    -e "s|from '../pricing/|from '../common/pricing/|g" \
    -e "s|from '../discovery/|from '../common/discovery/|g" \
    "$file"
done

# Pass 4: Update extensions module imports (single ../)
echo "Pass 4: Updating extensions module imports (single ../)..."
find src -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../skills/|from '../extensions/skills/|g" \
    -e "s|from '../subagents/|from '../extensions/subagents/|g" \
    -e "s|from '../commands/|from '../extensions/commands/|g" \
    -e "s|from '../mcp/|from '../extensions/mcp/|g" \
    "$file"
done

# Pass 5: Update CLI module imports (single ../)
echo "Pass 5: Updating CLI module imports (single ../)..."
find src -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../prompts/|from '../cli/prompts/|g" \
    -e "s|from '../planning/|from '../cli/planning/|g" \
    "$file"
done

# Pass 6: Update double ../ imports for core modules
echo "Pass 6: Updating double ../ imports for core modules..."
find src -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../../agent/|from '../../core/agent/|g" \
    -e "s|from '../../providers/|from '../../core/providers/|g" \
    -e "s|from '../../tools/|from '../../core/tools/|g" \
    -e "s|from '../../session/|from '../../core/session/|g" \
    "$file"
done

# Pass 7: Update double ../ imports for common modules
echo "Pass 7: Updating double ../ imports for common modules..."
find src -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../../config/|from '../../common/config/|g" \
    -e "s|from '../../memory/|from '../../common/memory/|g" \
    -e "s|from '../../permissions/|from '../../common/permissions/|g" \
    -e "s|from '../../hooks/|from '../../common/hooks/|g" \
    -e "s|from '../../pricing/|from '../../common/pricing/|g" \
    -e "s|from '../../discovery/|from '../../common/discovery/|g" \
    "$file"
done

# Pass 8: Update double ../ imports for extensions modules
echo "Pass 8: Updating double ../ imports for extensions modules..."
find src -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../../skills/|from '../../extensions/skills/|g" \
    -e "s|from '../../subagents/|from '../../extensions/subagents/|g" \
    -e "s|from '../../commands/|from '../../extensions/commands/|g" \
    -e "s|from '../../mcp/|from '../../extensions/mcp/|g" \
    "$file"
done

# Pass 9: Update double ../ imports for CLI modules
echo "Pass 9: Updating double ../ imports for CLI modules..."
find src -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../../prompts/|from '../../cli/prompts/|g" \
    -e "s|from '../../planning/|from '../../cli/planning/|g" \
    "$file"
done

# Pass 10: Update triple ../ imports
echo "Pass 10: Updating triple ../ imports..."
find src -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../../../agent/|from '../../../core/agent/|g" \
    -e "s|from '../../../providers/|from '../../../core/providers/|g" \
    -e "s|from '../../../tools/|from '../../../core/tools/|g" \
    -e "s|from '../../../session/|from '../../../core/session/|g" \
    -e "s|from '../../../config/|from '../../../common/config/|g" \
    -e "s|from '../../../memory/|from '../../../common/memory/|g" \
    -e "s|from '../../../permissions/|from '../../../common/permissions/|g" \
    -e "s|from '../../../hooks/|from '../../../common/hooks/|g" \
    -e "s|from '../../../pricing/|from '../../../common/pricing/|g" \
    -e "s|from '../../../discovery/|from '../../../common/discovery/|g" \
    -e "s|from '../../../skills/|from '../../../extensions/skills/|g" \
    -e "s|from '../../../subagents/|from '../../../extensions/subagents/|g" \
    -e "s|from '../../../commands/|from '../../../extensions/commands/|g" \
    -e "s|from '../../../mcp/|from '../../../extensions/mcp/|g" \
    -e "s|from '../../../prompts/|from '../../../cli/prompts/|g" \
    -e "s|from '../../../planning/|from '../../../cli/planning/|g" \
    "$file"
done

# Special fixes for files that moved
echo "Pass 11: Fixing imports in moved factory files..."
# These were already updated manually, so skip

# Pass 12: Fix imports within same module (remove duplicated paths)
echo "Pass 12: Fixing over-corrected imports..."
find src -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../core/core/|from '../core/|g" \
    -e "s|from '../../core/core/|from '../../core/|g" \
    -e "s|from '../../../core/core/|from '../../../core/|g" \
    -e "s|from '../common/common/|from '../common/|g" \
    -e "s|from '../../common/common/|from '../../common/|g" \
    -e "s|from '../../../common/common/|from '../../../common/|g" \
    -e "s|from '../extensions/extensions/|from '../extensions/|g" \
    -e "s|from '../../extensions/extensions/|from '../../extensions/|g" \
    -e "s|from '../../../extensions/extensions/|from '../../../extensions/|g" \
    -e "s|from '../cli/cli/|from '../cli/|g" \
    -e "s|from '../../cli/cli/|from '../../cli/|g" \
    -e "s|from '../../../cli/cli/|from '../../../cli/|g" \
    "$file"
done

echo "Import path updates complete!"
echo "Run 'npm run build' to verify changes."
