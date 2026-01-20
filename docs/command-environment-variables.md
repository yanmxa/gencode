# Command Environment Variables

## Overview

GenCode provides environment variables for command templates to reference external resources (scripts, files, etc.) without hardcoding absolute paths. This prevents the LLM from seeing file paths and triggering unnecessary file exploration.

## Available Variables

### `$GEN_CONFIG_DIR`

The base configuration directory where the command is located.

**Examples:**
- Command from `~/.claude/commands/` → `$GEN_CONFIG_DIR` = `~/.claude`
- Command from `~/.gen/commands/` → `$GEN_CONFIG_DIR` = `~/.gen`
- Command from `./.claude/commands/` → `$GEN_CONFIG_DIR` = `{projectRoot}/.claude`
- Command from `./.gen/commands/` → `$GEN_CONFIG_DIR` = `{projectRoot}/.gen`

**Usage:**
```bash
$GEN_CONFIG_DIR/scripts/my-script.sh
$GEN_CONFIG_DIR/templates/template.md
$GEN_CONFIG_DIR/data/config.json
```

## Migration Guide

### Before (Hardcoded Paths)

```markdown
---
description: Example command
allowed-tools: [Bash]
---

Run the helper script:

```bash
~/.claude/scripts/helper.sh $ARGUMENTS
```
```

**Problem:**
- LLM sees `~/.claude/scripts/helper.sh`
- May trigger exploration of `~/.claude/scripts/` directory
- Not portable across different config directories

### After (Using Environment Variables)

```markdown
---
description: Example command
allowed-tools: [Bash]
---

Run the helper script:

```bash
$GEN_CONFIG_DIR/scripts/helper.sh $ARGUMENTS
```
```

**Benefits:**
- LLM only sees `$GEN_CONFIG_DIR/scripts/helper.sh` (no actual path)
- No file exploration triggered
- Works across `~/.claude`, `~/.gen`, project-level configs
- More maintainable

## Best Practices

### 1. Use Environment Variables for All External Resources

```markdown
# Good
$GEN_CONFIG_DIR/scripts/helper.sh

# Bad
~/.claude/scripts/helper.sh
```

### 2. Combine with `includes` for Full Content Injection

For scripts that should be completely hidden from LLM:

```yaml
---
description: Example with included script
allowed-tools: [Bash]
includes:
  - ../../scripts/helper.sh
---

The helper script has been included above.
Use it directly without reading additional files.

DO NOT explore filesystem or read other files.
```

The script content will be inlined during template expansion, and LLM won't need to read it.

### 3. Use Relative Paths with `@file` Syntax

For project files:

```markdown
The configuration is in: @config.json

This will inline the content without showing the path.
```

## Complete Example

```markdown
---
description: Process data with external script
allowed-tools: [Bash]
includes:
  - ../../scripts/process-data.sh
---

Process the data using the included script.

**Script location:** $GEN_CONFIG_DIR/scripts/process-data.sh

Run the script:
```bash
bash $GEN_CONFIG_DIR/scripts/process-data.sh $ARGUMENTS
```

Note: Script content has been included inline above.
DO NOT read additional files.
```

## Implementation Details

Environment variables are expanded during template processing:

1. Command template is loaded
2. `$ARGUMENTS`, `$1`, `$2`, etc. are expanded
3. **`$GEN_CONFIG_DIR` is expanded** to the actual config directory
4. `@file` includes are expanded
5. Final prompt is sent to LLM

The LLM never sees the original environment variable names or actual paths (except in the final expanded bash commands).

## Compatibility

- ✅ Works with all command sources: `~/.claude`, `~/.gen`, project-level
- ✅ Backward compatible: existing commands with hardcoded paths still work
- ✅ Forward compatible: new commands should use `$GEN_CONFIG_DIR`
