#!/bin/bash
# Manual test script for hooks fallback mechanism
# Tests that .gen takes priority over .claude, and fallback works correctly

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
TEST_DIR="/tmp/gencode-hooks-test-$$"

echo "🧪 Testing Hooks Fallback Mechanism"
echo "===================================="
echo ""

# Cleanup function
cleanup() {
  echo ""
  echo "🧹 Cleaning up test directory: $TEST_DIR"
  rm -rf "$TEST_DIR"
}
trap cleanup EXIT

# Create test directory structure
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

# Test 1: Only .gen has hooks
echo "📝 Test 1: Only .gen has hooks"
echo "-------------------------------"
mkdir -p .gen
cat > .gen/settings.json <<EOF
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "echo 'FROM .GEN ONLY' > /tmp/hook-test-1.txt"
          }
        ]
      }
    ]
  }
}
EOF

echo "Created .gen/settings.json with SessionStart hook"
echo "Expected: Hook from .gen should execute"
echo ""

# Test 2: Only .claude has hooks (fallback scenario)
echo "📝 Test 2: Only .claude has hooks (fallback)"
echo "--------------------------------------------"
rm -rf .gen
mkdir -p .claude
cat > .claude/settings.json <<EOF
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "echo 'FROM .CLAUDE FALLBACK' > /tmp/hook-test-2.txt"
          }
        ]
      }
    ]
  }
}
EOF

echo "Created .claude/settings.json with SessionStart hook"
echo "Expected: Hook from .claude should execute (fallback)"
echo ""

# Test 3: Both exist, .gen has no hooks (fallback to .claude)
echo "📝 Test 3: Both exist, .gen has no hooks (fallback)"
echo "---------------------------------------------------"
mkdir -p .gen
cat > .gen/settings.json <<EOF
{
  "provider": "google"
}
EOF

cat > .claude/settings.json <<EOF
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "echo 'FROM .CLAUDE (gen has no hooks)' > /tmp/hook-test-3.txt"
          }
        ]
      }
    ]
  }
}
EOF

echo "Created .gen/settings.json (no hooks) and .claude/settings.json (has hooks)"
echo "Expected: Hook from .claude should execute (fallback)"
echo ""

# Test 4: Both have hooks for different events (merge)
echo "📝 Test 4: Both have hooks for different events (merge)"
echo "-------------------------------------------------------"
cat > .gen/settings.json <<EOF
{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "echo 'STOP FROM .GEN' > /tmp/hook-test-4-gen.txt"
          }
        ]
      }
    ]
  }
}
EOF

cat > .claude/settings.json <<EOF
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "echo 'START FROM .CLAUDE' > /tmp/hook-test-4-claude.txt"
          }
        ]
      }
    ]
  }
}
EOF

echo "Created .gen/settings.json (Stop hook) and .claude/settings.json (SessionStart hook)"
echo "Expected: Both hooks should be active (merged)"
echo ""

# Test 5: Both have hooks for SAME event (merge into array)
echo "📝 Test 5: Both have hooks for same event (merge)"
echo "-------------------------------------------------"
cat > .gen/settings.json <<EOF
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write",
        "hooks": [
          {
            "type": "command",
            "command": "echo 'Write hook from .GEN' > /tmp/hook-test-5-gen.txt"
          }
        ]
      }
    ]
  }
}
EOF

cat > .claude/settings.json <<EOF
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit",
        "hooks": [
          {
            "type": "command",
            "command": "echo 'Edit hook from .CLAUDE' > /tmp/hook-test-5-claude.txt"
          }
        ]
      }
    ]
  }
}
EOF

echo "Created .gen/settings.json (Write hook) and .claude/settings.json (Edit hook)"
echo "Expected: Both PostToolUse hooks should be active (merged array)"
echo ""

# Summary
echo "✅ Test scenarios created in: $TEST_DIR"
echo ""
echo "📋 How to test:"
echo "1. cd $TEST_DIR"
echo "2. Run 'gencode' to start a session"
echo "3. Check hook execution in session output"
echo "4. Verify hook files created in /tmp/"
echo ""
echo "💡 To test programmatically, use Node.js:"
echo ""
cat > "$TEST_DIR/verify-config.js" <<'NODESCRIPT'
const { ConfigManager } = require('../../dist/config/manager.js');

(async () => {
  const manager = new ConfigManager({ cwd: process.cwd() });
  const config = await manager.load();
  const settings = config.settings;

  console.log('\n📊 Loaded Configuration:');
  console.log('======================');
  console.log(JSON.stringify(settings.hooks, null, 2));

  console.log('\n📝 Configuration Sources:');
  config.sources.forEach(src => {
    console.log(`  - ${src.level}:${src.namespace} from ${src.path}`);
  });
})();
NODESCRIPT

echo "node verify-config.js"
echo ""
echo "Test directory will be cleaned up on exit."
