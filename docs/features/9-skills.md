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
TestSkill_LoadFromDirectory
TestSkill_FrontmatterParsing
TestSkill_NamespaceResolution      — "git:review" → namespace=git name=review
TestSkill_StateToggle
TestSkill_LazyLoading              — loaded on demand
TestSkill_Integration_Invoke
TestSkill_Integration_AllowedTools — tools outside allowed-tools are blocked
```

Cases to add:

```go
func TestSkill_ScopePriority_ProjectOverridesUser(t *testing.T) {
    // Project-level skill must shadow user-level skill with the same name
}

func TestSkill_Active_AppearsInSystemPrompt(t *testing.T) {
    // An Active skill's content must appear in the system prompt
}
```

## Interactive Tests (tmux)

```bash
mkdir -p /tmp/skill_test/.gen/skills/greet

cat > /tmp/skill_test/.gen/skills/greet/skill.md << 'EOF'
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

# View skills
tmux send-keys -t t_skills '/skills' Enter
sleep 1
tmux capture-pane -t t_skills -p
# Expected: "greet" listed

# Invoke the skill
tmux send-keys -t t_skills '/greet' Enter
sleep 5
tmux capture-pane -t t_skills -p
# Expected: "Hello! Hope you're having a great day!"

# Namespaced skill
mkdir -p /tmp/skill_test/.gen/skills/git-review
cat > /tmp/skill_test/.gen/skills/git-review/skill.md << 'EOF'
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

tmux kill-session -t t_skills
rm -rf /tmp/skill_test
```
