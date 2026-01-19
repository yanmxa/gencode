#!/bin/bash
# Fix imports within the same module

set -e

echo "Fixing internal imports..."

# Fix common/ internal imports (remove redundant ../common/)
echo "Pass 1: Fixing common/ internal imports..."
find src/common -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../common/utils/|from '../utils/|g" \
    -e "s|from '../common/config/|from '../config/|g" \
    -e "s|from '../common/discovery/|from '../discovery/|g" \
    -e "s|from '../common/memory/|from '../memory/|g" \
    -e "s|from '../common/permissions/|from '../permissions/|g" \
    -e "s|from '../common/hooks/|from '../hooks/|g" \
    -e "s|from '../common/pricing/|from '../pricing/|g" \
    -e "s|from '../common/logger.js'|from '../utils/logger.js'|g" \
    -e "s|from '../common/debug.js'|from '../utils/debug.js'|g" \
    -e "s|from '../common/validation.js'|from '../utils/validation.js'|g" \
    -e "s|from '../common/path-utils.js'|from '../utils/path-utils.js'|g" \
    -e "s|from '../common/format-utils.js'|from '../utils/format-utils.js'|g" \
    -e "s|from '../common/stream-utils.js'|from '../utils/stream-utils.js'|g" \
    -e "s|from '../common/config-validator.js'|from '../utils/config-validator.js'|g" \
    -e "s|from '../common/loading-reporter.js'|from '../utils/loading-reporter.js'|g" \
    "$file"
done

# Fix core/ internal imports (remove redundant ../core/)
echo "Pass 2: Fixing core/ internal imports..."
find src/core -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../core/agent/|from '../agent/|g" \
    -e "s|from '../core/providers/|from '../providers/|g" \
    -e "s|from '../core/tools/|from '../tools/|g" \
    -e "s|from '../core/session/|from '../session/|g" \
    -e "s|from '../../core/agent/|from '../../agent/|g" \
    -e "s|from '../../core/providers/|from '../../providers/|g" \
    -e "s|from '../../core/tools/|from '../../tools/|g" \
    -e "s|from '../../core/session/|from '../../session/|g" \
    "$file"
done

# Fix extensions/ internal imports (remove redundant ../extensions/)
echo "Pass 3: Fixing extensions/ internal imports..."
find src/extensions -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../extensions/skills/|from '../skills/|g" \
    -e "s|from '../extensions/subagents/|from '../subagents/|g" \
    -e "s|from '../extensions/commands/|from '../commands/|g" \
    -e "s|from '../extensions/mcp/|from '../mcp/|g" \
    "$file"
done

# Fix cli/ internal imports (remove redundant ../cli/)
echo "Pass 4: Fixing cli/ internal imports..."
find src/cli -name "*.ts" -o -name "*.tsx" | while read file; do
  sed -i '' \
    -e "s|from '../cli/prompts/|from '../prompts/|g" \
    -e "s|from '../cli/planning/|from '../planning/|g" \
    -e "s|from '../../cli/prompts/|from '../../prompts/|g" \
    -e "s|from '../../cli/planning/|from '../../planning/|g" \
    "$file"
done

# Fix specific known issues
echo "Pass 5: Fixing specific known import issues..."

# cli/components/App.tsx
sed -i '' \
  -e "s|from '../../checkpointing/index.js'|from '../../core/session/checkpointing/index.js'|g" \
  -e "s|from '../../input/index.js'|from '../../core/session/input/index.js'|g" \
  -e "s|from '../../tasks/task-manager.js'|from '../../core/session/tasks/task-manager.js'|g" \
  -e "s|from '../../commands/index.js'|from '../../extensions/commands/index.js'|g" \
  src/cli/components/App.tsx

# cli/index.tsx
sed -i '' \
  -e "s|from '../agent/agent.js'|from '../core/agent/agent.js'|g" \
  src/cli/index.tsx

# core/agent/agent.ts - fix checkpointing path
sed -i '' \
  -e "s|from '../checkpointing/index.js'|from '../session/checkpointing/index.js'|g" \
  -e "s|from '../planning/index.js'|from '../../cli/planning/index.js'|g" \
  src/core/agent/agent.ts

# core/session/manager.ts - fix checkpointing path
sed -i '' \
  -e "s|from '../checkpointing/index.js'|from './checkpointing/index.js'|g" \
  src/core/session/manager.ts

# core/session/types.ts - fix checkpointing path
sed -i '' \
  -e "s|from '../checkpointing/types.js'|from './checkpointing/types.js'|g" \
  src/core/session/types.ts

# core/tools/builtin - fix prompts and tasks imports
find src/core/tools/builtin -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../../cli/prompts/index.js'|from '../../../cli/prompts/index.js'|g" \
    -e "s|from '../../tasks/task-manager.js'|from '../../session/tasks/task-manager.js'|g" \
    -e "s|from '../../tasks/types.js'|from '../../session/tasks/types.js'|g" \
    -e "s|from '../../core/providers/search/index.js'|from '../../providers/search/index.js'|g" \
    "$file"
done

# core/tools/index.ts
sed -i '' \
  -e "s|from '../common/logger.js'|from '../../common/utils/logger.js'|g" \
  -e "s|from '../common/debug.js'|from '../../common/utils/debug.js'|g" \
  -e "s|from '../cli/planning/index.js'|from '../../cli/planning/index.js'|g" \
  -e "s|from '../extensions/subagents/index.js'|from '../../extensions/subagents/index.js'|g" \
  src/core/tools/index.ts

# core/session/tasks/output-streamer.ts
sed -i '' \
  -e "s|from '../core/agent/types.js'|from '../../agent/types.js'|g" \
  src/core/session/tasks/output-streamer.ts

# core/session/tasks/task-manager.ts
sed -i '' \
  -e "s|from '../extensions/subagents/subagent.js'|from '../../../extensions/subagents/subagent.js'|g" \
  -e "s|from '../extensions/subagents/types.js'|from '../../../extensions/subagents/types.js'|g" \
  src/core/session/tasks/task-manager.ts

# core/session/tasks/types.ts
sed -i '' \
  -e "s|from '../extensions/subagents/types.js'|from '../../../extensions/subagents/types.js'|g" \
  src/core/session/tasks/types.ts

# cli/planning/tools
find src/cli/planning/tools -name "*.ts" | while read file; do
  sed -i '' \
    -e "s|from '../../core/tools/types.js'|from '../../../core/tools/types.js'|g" \
    "$file"
done

# extensions/skills/manager.ts (was discovery.ts)
if [ -f "src/extensions/skills/manager.ts" ]; then
  sed -i '' \
    -e "s|from '../discovery/index.js'|from '../../common/discovery/index.js'|g" \
    -e "s|from './discovery.js'|from './manager.js'|g" \
    src/extensions/skills/manager.ts
fi

# Update skills/index.ts to export from manager instead of discovery
if [ -f "src/extensions/skills/index.ts" ]; then
  sed -i '' \
    -e "s|from './discovery.js'|from './manager.js'|g" \
    -e "s|export { discoverSkills }|export { discoverSkills, SkillDiscovery }|g" \
    src/extensions/skills/index.ts
fi

# Update subagents/index.ts to export from manager instead of custom-agent-loader
if [ -f "src/extensions/subagents/index.ts" ]; then
  sed -i '' \
    -e "s|from './custom-agent-loader.js'|from './manager.js'|g" \
    src/extensions/subagents/index.ts
fi

# core/tools/factories/skill-tool-factory.ts - fix discovery import
sed -i '' \
  -e "s|from '../../../extensions/skills/discovery.js'|from '../../../extensions/skills/manager.js'|g" \
  src/core/tools/factories/skill-tool-factory.ts

echo "Internal import fixes complete!"
