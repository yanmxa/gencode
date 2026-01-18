# Skills System - Manual Testing Guide

This guide provides step-by-step instructions to manually test the Skills System implementation.

## Prerequisites

```bash
# Build the project
npm run build

# Or run in dev mode
npm run dev
```

## Test Scenarios

### Scenario 1: Verify Skill Discovery

**Objective**: Confirm that skills are discovered from all 4 hierarchical directories.

**Steps**:

1. **Create test skills at all levels**:

```bash
# User Claude level (lowest priority)
mkdir -p ~/.claude/skills/discovery-test
cat > ~/.claude/skills/discovery-test/SKILL.md <<'EOF'
---
name: discovery-test
description: Test skill from user claude (should be overridden)
tags: [test, user, claude]
---

# User Claude Discovery Test

This skill is loaded from `~/.claude/skills/discovery-test/`.

**Priority**: Lowest (should be overridden by user gen)
EOF

# User Gen level (overrides user claude)
mkdir -p ~/.gen/skills/discovery-test
cat > ~/.gen/skills/discovery-test/SKILL.md <<'EOF'
---
name: discovery-test
description: Test skill from user gen (should win)
tags: [test, user, gen]
version: 2.0.0
---

# User Gen Discovery Test

This skill is loaded from `~/.gen/skills/discovery-test/`.

**Priority**: User level, highest priority
**Expected**: This version should be used when invoked
EOF

# Project Claude level
mkdir -p .claude/skills/project-test
cat > .claude/skills/project-test/SKILL.md <<'EOF'
---
name: project-test
description: Test skill from project claude
tags: [test, project, claude]
---

# Project Claude Test

This skill is loaded from `.claude/skills/project-test/`.

This is a project-specific skill that doesn't conflict with user skills.
EOF

# Project Gen level (highest priority for project skills)
mkdir -p .gen/skills/project-test
cat > .gen/skills/project-test/SKILL.md <<'EOF'
---
name: project-test
description: Test skill from project gen (should override project claude)
tags: [test, project, gen]
version: 3.0.0
---

# Project Gen Test

This skill is loaded from `.gen/skills/project-test/`.

**Priority**: Highest priority for project-level skills
**Expected**: This version should be used when invoked
EOF
```

2. **Start gencode and check Skill tool description**:

```bash
npm start
```

In the gencode session, ask:
```
Please show me all available skills from the Skill tool
```

**Expected Output**:
- You should see both `discovery-test` and `project-test` listed
- `discovery-test` should show description from user gen ("Test skill from user gen (should win)")
- `project-test` should show description from project gen ("Test skill from project gen (should override project claude)")

3. **Invoke the skills to verify content**:

```
Please use the Skill tool to activate discovery-test
```

**Expected**: The activated skill content should be from `~/.gen/skills/discovery-test/` (version 2.0.0)

```
Please use the Skill tool to activate project-test
```

**Expected**: The activated skill content should be from `.gen/skills/project-test/` (version 3.0.0)

---

### Scenario 2: Test Merge Priority

**Objective**: Verify that the merge strategy correctly prioritizes: project gen > project claude > user gen > user claude

**Steps**:

1. **Create same-name skill at all 4 levels**:

```bash
# User Claude (priority 1 - lowest)
mkdir -p ~/.claude/skills/priority-test
cat > ~/.claude/skills/priority-test/SKILL.md <<'EOF'
---
name: priority-test
description: Priority 1 - User Claude
version: 1.0.0
---
# Priority Test - User Claude
Source: ~/.claude/skills/priority-test/
Priority: 1 (Lowest)
EOF

# User Gen (priority 2)
mkdir -p ~/.gen/skills/priority-test
cat > ~/.gen/skills/priority-test/SKILL.md <<'EOF'
---
name: priority-test
description: Priority 2 - User Gen
version: 2.0.0
---
# Priority Test - User Gen
Source: ~/.gen/skills/priority-test/
Priority: 2 (Overrides user claude)
EOF

# Project Claude (priority 3)
mkdir -p .claude/skills/priority-test
cat > .claude/skills/priority-test/SKILL.md <<'EOF'
---
name: priority-test
description: Priority 3 - Project Claude
version: 3.0.0
---
# Priority Test - Project Claude
Source: .claude/skills/priority-test/
Priority: 3 (Overrides user gen)
EOF

# Project Gen (priority 4 - highest)
mkdir -p .gen/skills/priority-test
cat > .gen/skills/priority-test/SKILL.md <<'EOF'
---
name: priority-test
description: Priority 4 - Project Gen (WINNER)
version: 4.0.0
---
# Priority Test - Project Gen
Source: .gen/skills/priority-test/
Priority: 4 (Highest - Should WIN)
EOF
```

2. **Rebuild and start gencode**:

```bash
npm run build
npm start
```

3. **Activate the skill**:

```
Please use the Skill tool to activate priority-test
```

**Expected Output**:
- Description should be "Priority 4 - Project Gen (WINNER)"
- Content should show "Source: .gen/skills/priority-test/"
- Version should be 4.0.0
- Must NOT show content from any other priority level

4. **Test removal in priority order**:

Remove project gen version and verify fallback:
```bash
rm -rf .gen/skills/priority-test
npm run build
npm start
```

Activate skill again - should now show Priority 3 (Project Claude).

Repeat for project claude and user gen to verify complete priority chain.

---

### Scenario 3: Test Skill Arguments

**Objective**: Verify that skill arguments are passed correctly.

**Steps**:

1. **Create a skill that uses arguments**:

```bash
mkdir -p ~/.gen/skills/args-test
cat > ~/.gen/skills/args-test/SKILL.md <<'EOF'
---
name: args-test
description: Test skill with argument support
---

# Arguments Test Skill

This skill demonstrates argument handling.

## Usage

When invoked with arguments, you should:
1. Parse the provided arguments
2. Execute the appropriate action based on the arguments
3. Return results

## Example Arguments

- `--mode debug` - Enable debug mode
- `--verbose` - Show verbose output
- `--config path/to/config.json` - Use custom config
EOF
```

2. **Test with and without arguments**:

```bash
npm start
```

Invoke without arguments:
```
Please use the Skill tool to activate args-test
```

**Expected**: Skill content is displayed without any argument mention.

Invoke with arguments:
```
Please use the Skill tool to activate args-test with args "--mode debug --verbose"
```

**Expected**: Skill content is displayed WITH argument line showing "Arguments: --mode debug --verbose"

---

### Scenario 4: Test Invalid Skills

**Objective**: Verify graceful handling of invalid SKILL.md files.

**Steps**:

1. **Create invalid skills**:

```bash
# Missing required 'name' field
mkdir -p ~/.gen/skills/invalid-no-name
cat > ~/.gen/skills/invalid-no-name/SKILL.md <<'EOF'
---
description: Missing name field
---
This should be skipped during discovery.
EOF

# Missing required 'description' field
mkdir -p ~/.gen/skills/invalid-no-desc
cat > ~/.gen/skills/invalid-no-desc/SKILL.md <<'EOF'
---
name: invalid-no-desc
---
This should be skipped during discovery.
EOF

# Malformed YAML
mkdir -p ~/.gen/skills/invalid-yaml
cat > ~/.gen/skills/invalid-yaml/SKILL.md <<'EOF'
---
name: invalid
description: { bad yaml here
---
Content
EOF

# Directory without SKILL.md
mkdir -p ~/.gen/skills/not-a-skill
touch ~/.gen/skills/not-a-skill/README.md
```

2. **Start gencode and list skills**:

```bash
npm start
```

```
Please show me all available skills
```

**Expected**:
- Invalid skills should NOT appear in the list
- Valid skills should still be discovered correctly
- No errors or crashes should occur

3. **Cleanup**:

```bash
rm -rf ~/.gen/skills/invalid-no-name
rm -rf ~/.gen/skills/invalid-no-desc
rm -rf ~/.gen/skills/invalid-yaml
rm -rf ~/.gen/skills/not-a-skill
```

---

### Scenario 5: Test Real-World Usage

**Objective**: Create and use a practical skill.

**Steps**:

1. **Create a code review skill**:

```bash
mkdir -p ~/.gen/skills/code-review
cat > ~/.gen/skills/code-review/SKILL.md <<'EOF'
---
name: code-review
description: Perform thorough code reviews with best practices
tags: [development, quality, review]
allowed-tools: [Read, Glob, Grep]
---

# Code Review Skill

You are an expert code reviewer. When activated, follow this systematic review process:

## Review Checklist

1. **Code Quality**
   - Check for code duplication
   - Verify naming conventions
   - Assess code readability and maintainability

2. **Potential Bugs**
   - Look for null/undefined handling
   - Check error handling
   - Verify edge cases

3. **Performance**
   - Identify inefficient algorithms
   - Check for unnecessary computations
   - Look for memory leaks

4. **Security**
   - Verify input validation
   - Check for injection vulnerabilities
   - Review authentication/authorization

5. **Testing**
   - Ensure adequate test coverage
   - Verify tests are meaningful
   - Check for missing test cases

## Output Format

Provide feedback in this format:

**Summary**: Brief overview of code quality

**Issues Found**:
- 🔴 Critical: [description]
- 🟡 Warning: [description]
- 🔵 Suggestion: [description]

**Strengths**: What's done well

**Recommendations**: Prioritized list of improvements
EOF
```

2. **Use the skill in a real code review**:

```bash
npm start
```

```
Please use the code-review skill to review the file src/skills/discovery.ts
```

**Expected**:
- Skill is activated with full content
- Claude follows the structured review process
- Output includes checklist items and formatted feedback

---

## Verification Checklist

After completing all scenarios, verify:

- [ ] Skills discovered from all 4 directories (user claude, user gen, project claude, project gen)
- [ ] Merge priority works correctly (project gen highest, user claude lowest)
- [ ] Same-name skills follow priority order
- [ ] Different-name skills are all kept (additive behavior)
- [ ] Skill arguments are passed and displayed correctly
- [ ] Invalid SKILL.md files are gracefully skipped
- [ ] Skill content is injected as tool_result
- [ ] No TypeScript compilation errors
- [ ] Tool description dynamically lists available skills

---

## Cleanup Test Skills

After testing, remove test skills:

```bash
# User level
rm -rf ~/.claude/skills/discovery-test
rm -rf ~/.gen/skills/discovery-test
rm -rf ~/.claude/skills/priority-test
rm -rf ~/.gen/skills/priority-test
rm -rf ~/.gen/skills/args-test

# Project level
rm -rf .claude/skills/project-test
rm -rf .gen/skills/project-test
rm -rf .claude/skills/priority-test
rm -rf .gen/skills/priority-test
```

---

## Troubleshooting

### Skills Not Discovered

**Problem**: Skills created but not showing in tool description

**Solutions**:
1. Verify SKILL.md has correct frontmatter with `name` and `description`
2. Rebuild the project: `npm run build`
3. Restart gencode session
4. Check file permissions on skills directories

### Wrong Skill Version Loaded

**Problem**: Lower priority skill is being used

**Solutions**:
1. Verify directory structure (`.gen/` vs `.claude/`)
2. Check skill naming is identical across directories
3. Clear any cached sessions
4. Rebuild: `npm run build`

### Skill Content Not Displaying

**Problem**: Skill activates but content is empty

**Solutions**:
1. Check SKILL.md has content after frontmatter (YAML section)
2. Verify file is saved with UTF-8 encoding
3. Check for YAML syntax errors in frontmatter

---

## Expected Behavior Summary

| Scenario | User Claude | User Gen | Project Claude | Project Gen | Winner |
|----------|-------------|----------|----------------|-------------|--------|
| Same skill all 4 levels | ❌ | ❌ | ❌ | ✅ | Project Gen |
| Same skill user levels only | ❌ | ✅ | - | - | User Gen |
| Same skill project levels only | - | - | ❌ | ✅ | Project Gen |
| Unique skill in each | ✅ | ✅ | ✅ | ✅ | All kept (merge) |

**Key Principle**: Last loaded (highest priority) overwrites earlier versions for same-name skills.
