# Feature 9: Skills System

## Overview

Skills are reusable prompt workflows stored as Markdown files with YAML frontmatter. They can be invoked as slash commands or activated to be visible in the LLM's system prompt.

**States:**

| State | Behavior |
|-------|----------|
| `Disable` | Hidden from user and model |
| `Enable` | Available as `/command`; model is unaware |
| `Active` | Included in system prompt; model is aware |

**Load scopes** (lowest → highest priority):

1. `~/.claude/skills/` (Claude user)
2. `~/.gen/plugins/*/skills/` (User plugins)
3. `~/.gen/skills/` (GenCode user)
4. `./.claude/skills/` (Claude project)
5. `./.gen/plugins/*/skills/` (Project plugins)
6. `./.gen/skills/` (GenCode project)

**Frontmatter fields:**

```yaml
---
name: review
namespace: git           # invoked as /git:review
description: Review the last commit
allowed-tools: [Bash, Read]
argument-hint: <pr-number>
---
```

## UI Interactions

- **`/skills`**: opens a picker showing all skills with their current state; toggle with Enter.
- **Invoke**: type `/skillname` or `/namespace:skillname` to run the skill's prompt.
- **Argument hint**: shown in the input box after the command if `argument-hint` is set.

## Automated Tests

```bash
go test ./internal/skill/... -v
go test ./tests/integration/skill/... -v
```

Covered:

```
# Internal skill tests
TestSkillDiskReading                        — reading skills from disk
TestSkillStateNextState                     — state transition logic
TestSkillStateIcon                          — state icon rendering
TestSkillFullName                           — full name with namespace
TestLoadSkillFile                           — single skill file loading
TestLoadAllSkills                           — loading all skills from directory
TestLoadSkillWithNamespace                  — skill with namespace loaded
TestSkillRegistry                           — skill registry operations
TestLoadPluginSkills                        — skills loaded from plugins
TestPluginSkillExplicitNamespaceOverride    — plugin skill namespace override

# Integration tests
TestSkill_StateTransitions                  — state cycle: disable → enable → active
TestSkill_StateIcons                        — icon for each state
TestSkill_BooleanHelpers                    — IsEnabled, IsActive helpers
TestSkill_RegistryLookup                    — lookup by name and namespace
TestSkill_AvailablePrompt                   — available prompt generation
TestSkill_InvocationPrompt                  — invocation prompt generation
TestSkill_ScopePriority                     — scope priority order
TestSkill_Persistence                       — state persistence across restarts
TestSkill_FullName                          — namespace:name formatting
TestSkill_ScopePriority_ProjectOverridesUser — project skill shadows user skill
TestSkill_Active_AppearsInSystemPrompt       — active skill content in system prompt
```

Cases to add:

```go
func TestSkill_ArgumentHint_DisplayedInInput(t *testing.T) {
    // argument-hint must be shown in the input box after the command
}

func TestSkill_AllowedTools_EnforcedDuringInvocation(t *testing.T) {
    // Tools outside allowed-tools must be blocked during skill execution
}

func TestSkill_DisabledState_HiddenFromModel(t *testing.T) {
    // Disabled skills must not appear in system prompt or slash commands
}

func TestSkill_EnabledState_AvailableAsCommand(t *testing.T) {
    // Enabled skills must be invocable as /command but not in system prompt
}
```

## Interactive Tests (tmux)

```bash
mkdir -p /tmp/skill_test/.gen/skills/greet

cat > /tmp/skill_test/.gen/skills/greet/SKILL.md << 'EOF'
---
name: greet
description: Greet the user warmly
allowed-tools: []
---

Say "Hello! Hope you're having a great day!" and nothing else.
EOF

tmux new-session -d -s t_skills -x 220 -y 60
tmux send-keys -t t_skills 'cd /tmp/skill_test && gen' Enter
sleep 2

# Test 1: View skills — /skills picker
tmux send-keys -t t_skills '/skills' Enter
sleep 1
tmux capture-pane -t t_skills -p
# Expected: selector titled "Manage Skills" with "greet" listed

# Test 2: Invoke the skill
tmux send-keys -t t_skills Escape
tmux send-keys -t t_skills '/greet' Enter
sleep 5
tmux capture-pane -t t_skills -p
# Expected: "Hello! Hope you're having a great day!"

# Test 3: Namespaced skill invocation
mkdir -p /tmp/skill_test/.gen/skills/git-review
cat > /tmp/skill_test/.gen/skills/git-review/SKILL.md << 'EOF'
---
name: review
namespace: git
description: Summarize the last git commit
allowed-tools: [Bash]
---

Run `git log -1 --stat` and summarize the most recent commit.
EOF

tmux send-keys -t t_skills '/git:review' Enter
sleep 8
tmux capture-pane -t t_skills -p
# Expected: Bash runs git log; commit summary shown

# Test 4: Skill state toggle via /skills
tmux send-keys -t t_skills '/skills' Enter
sleep 1
# Navigate to "greet" and toggle state
tmux send-keys -t t_skills Enter
sleep 1
tmux capture-pane -t t_skills -p
# Expected: "greet" state changed (enable ↔ active ↔ disable)

# Test 5: Argument hint display
mkdir -p /tmp/skill_test/.gen/skills/search
cat > /tmp/skill_test/.gen/skills/search/SKILL.md << 'EOF'
---
name: search
description: Search files
allowed-tools: [Glob, Grep]
argument-hint: <pattern>
---

Search for the given pattern in the project files.
EOF
tmux send-keys -t t_skills Escape
tmux send-keys -t t_skills '/search'
sleep 1
tmux capture-pane -t t_skills -p
# Expected: argument hint "<pattern>" shown after /search

# Test 6: Allowed-tools enforcement
tmux send-keys -t t_skills ' *.go' Enter
sleep 5
tmux capture-pane -t t_skills -p
# Expected: only Glob and Grep tools used (no Write, Edit, Bash)

tmux kill-session -t t_skills
rm -rf /tmp/skill_test
```
