# Manual Testing Guide: Hooks Fallback Mechanism

This guide helps you manually verify that the hooks configuration fallback mechanism works correctly.

## Quick Verification

Run the verification script to see your current hooks configuration:

```bash
# In gencode project root
npm run build
node scripts/verify-hooks-config.mjs

# Or test a specific directory
node scripts/verify-hooks-config.mjs /path/to/test/dir
```

## Test Scenarios

### Scenario 1: Only `.gen` has hooks

**Setup:**
```bash
mkdir -p test-scenario-1/.gen
cd test-scenario-1

cat > .gen/settings.json <<'EOF'
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "echo 'Hook from .gen only'"
          }
        ]
      }
    ]
  }
}
EOF
```

**Verify:**
```bash
node ../scripts/verify-hooks-config.mjs .
```

**Expected Output:**
- ✅ Only .gen has hooks
- → Using .gen hooks directly
- Events with hooks: 1
- Command shows: `echo 'Hook from .gen only'`

---

### Scenario 2: Only `.claude` has hooks (Fallback)

**Setup:**
```bash
mkdir -p test-scenario-2/.claude
cd test-scenario-2

cat > .claude/settings.json <<'EOF'
{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "echo 'Fallback hook from .claude'"
          }
        ]
      }
    ]
  }
}
EOF
```

**Verify:**
```bash
node ../scripts/verify-hooks-config.mjs .
```

**Expected Output:**
- ✅ Only .claude has hooks
- → FALLBACK to .claude hooks
- Events with hooks: 1
- Command shows: `echo 'Fallback hook from .claude'`

---

### Scenario 3: `.gen` has no hooks, fallback to `.claude`

**Setup:**
```bash
mkdir -p test-scenario-3/{.gen,.claude}
cd test-scenario-3

cat > .gen/settings.json <<'EOF'
{
  "provider": "google",
  "model": "gemini-2.0-flash"
}
EOF

cat > .claude/settings.json <<'EOF'
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "npm run lint:fix $FILE_PATH"
          }
        ]
      }
    ]
  }
}
EOF
```

**Verify:**
```bash
node ../scripts/verify-hooks-config.mjs .
```

**Expected Output:**
- ✅ Only .claude has hooks
- → FALLBACK to .claude hooks
- Events with hooks: 1
- Provider: google (from .gen)
- PostToolUse hook for Write|Edit pattern

---

### Scenario 4: Both have hooks for different events (Merge)

**Setup:**
```bash
mkdir -p test-scenario-4/{.gen,.claude}
cd test-scenario-4

cat > .gen/settings.json <<'EOF'
{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "echo 'Stop hook from .gen'"
          }
        ]
      }
    ]
  }
}
EOF

cat > .claude/settings.json <<'EOF'
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "echo 'Start hook from .claude'"
          }
        ]
      }
    ]
  }
}
EOF
```

**Verify:**
```bash
node ../scripts/verify-hooks-config.mjs .
```

**Expected Output:**
- ✅ Both .gen and .claude have hooks
- → Hooks are MERGED (arrays concatenated)
- Events with hooks: 2
- SessionStart: from .claude
- Stop: from .gen

---

### Scenario 5: Both have hooks for SAME event (Merge)

**Setup:**
```bash
mkdir -p test-scenario-5/{.gen,.claude}
cd test-scenario-5

cat > .gen/settings.json <<'EOF'
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write",
        "hooks": [
          {
            "type": "command",
            "command": "prettier --write $FILE_PATH"
          }
        ]
      }
    ]
  }
}
EOF

cat > .claude/settings.json <<'EOF'
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit",
        "hooks": [
          {
            "type": "command",
            "command": "eslint --fix $FILE_PATH"
          }
        ]
      }
    ]
  }
}
EOF
```

**Verify:**
```bash
node ../scripts/verify-hooks-config.mjs .
```

**Expected Output:**
- ✅ Both .gen and .claude have hooks
- → Hooks are MERGED (arrays concatenated)
- Events with hooks: 1
- PostToolUse: 2 matchers
  - [1] Pattern: Write (prettier)
  - [2] Pattern: Edit (eslint)

---

## Testing with Real GenCode

After verifying configuration loading, test with real gencode execution:

```bash
cd test-scenario-5  # or any scenario

# Build gencode first
cd /path/to/gencode
npm run build

# Run gencode in test directory
cd -
gencode -p "Create a test file"
```

Watch for hook execution in the output.

## Configuration Priority

The configuration system follows this priority (lowest to highest):

1. `~/.claude/settings.json`
2. `~/.gen/settings.json`
3. `<project>/.claude/settings.json`
4. `<project>/.gen/settings.json`
5. `<project>/.gen/settings.local.json`

Within each level:
- `.claude` is loaded first
- `.gen` is loaded second (higher priority)
- Arrays are concatenated (hooks from both sources)
- Objects are deep-merged
- Scalars from `.gen` override `.claude`

## Troubleshooting

### Hooks not loading

1. **Check file exists:**
   ```bash
   ls -la .gen/settings.json .claude/settings.json
   ```

2. **Check JSON syntax:**
   ```bash
   jq . .gen/settings.json
   jq . .claude/settings.json
   ```

3. **Check permissions:**
   ```bash
   cat .gen/settings.json
   ```

### Unexpected hook behavior

1. **Check which sources loaded:**
   ```bash
   node scripts/verify-hooks-config.mjs .
   ```

2. **Check merge order:**
   - Look at "Configuration Sources" section
   - Sources listed in load order (lowest priority first)

3. **Check global config interference:**
   ```bash
   cat ~/.gen/settings.json
   cat ~/.claude/settings.json
   ```

## Expected Test Results Summary

| Scenario | .gen hooks | .claude hooks | Result |
|----------|-----------|---------------|--------|
| 1 | ✓ | ✗ | Use .gen only |
| 2 | ✗ | ✓ | Fallback to .claude |
| 3 | ✗ (other settings) | ✓ | Fallback to .claude + .gen settings |
| 4 | ✓ (Stop) | ✓ (SessionStart) | Merge both (2 events) |
| 5 | ✓ (PostToolUse/Write) | ✓ (PostToolUse/Edit) | Merge both (1 event, 2 matchers) |

All scenarios should pass with the verification script showing the expected configuration.
