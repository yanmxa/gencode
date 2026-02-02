# GenCode Skill System Design

## 目标

将 GenCode 的 Skill 系统与 [Agent Skills 规范](https://agentskills.io) 完全对齐，实现与 Claude Code 兼容的 Skill 功能。

## Agent Skills 规范概述

### Skill 目录结构

```
skill-name/
├── SKILL.md (必需)
│   ├── YAML frontmatter (必需)
│   │   ├── name: skill-name
│   │   └── description: 描述和触发条件
│   └── Markdown 指令 (必需)
└── 可选资源
    ├── scripts/      - 可执行脚本 (Python/Bash)
    ├── references/   - 参考文档 (按需加载)
    └── assets/       - 输出资源 (模板、图片等)
```

### 核心原则

1. **渐进式加载**: 只在需要时加载内容
   - Level 1: name + description (~100 words) - 始终在上下文中
   - Level 2: SKILL.md body (<5k words) - skill 触发时加载
   - Level 3: 资源文件 - Claude 按需读取

2. **脚本执行**: Scripts 可以直接执行，无需加载到上下文

3. **简洁优先**: 上下文窗口是公共资源，只添加 Claude 不知道的内容

---

## 当前 GenCode 实现 vs Agent Skills 规范

| 功能 | Agent Skills | GenCode 当前 | 差距 |
|------|-------------|--------------|------|
| SKILL.md 格式 | ✓ | ✓ | 完全兼容 |
| name/description | ✓ | ✓ | 完全兼容 |
| scripts/ 目录 | ✓ | ✗ | 需实现脚本执行 |
| references/ 目录 | ✓ | ✗ | 需实现按需加载 |
| assets/ 目录 | ✓ | ✗ | 需实现资源引用 |
| Skill 工具调用 | ✓ | ✗ | 需实现 Skill 工具 |
| 渐进式加载 | ✓ | 部分 | 需优化 |
| .skill 打包 | ✓ | ✗ | 可选实现 |

---

## 实现方案

### Phase 1: Skill 工具 (高优先级)

让 LLM 能够主动调用 Skill。

#### 1.1 Skill Tool Schema

```go
// internal/tool/skill.go

type SkillTool struct{}

func (t *SkillTool) Name() string { return "Skill" }

func (t *SkillTool) Description() string {
    return `Execute a skill within the main conversation.

When users ask to perform tasks, check if available skills can help.
Skills provide specialized capabilities and domain knowledge.

When users reference "/<skill-name>", use this tool to invoke it.

Example:
  User: "run /commit"
  Assistant: [Calls Skill tool with skill: "commit"]

How to invoke:
- skill: "pdf" - invoke the pdf skill
- skill: "commit", args: "-m 'Fix bug'" - invoke with arguments
- skill: "git:pr" - invoke using namespace:name

Important:
- Invoke this tool IMMEDIATELY when a skill is relevant
- Skills listed in available_skills are available for invocation
- Do not invoke a skill that is already running`
}

// Schema
var SkillToolSchema = provider.Tool{
    Name:        "Skill",
    Description: SkillTool{}.Description(),
    Parameters: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "skill": map[string]any{
                "type":        "string",
                "description": "The skill name (e.g., 'commit', 'git:pr', 'pdf')",
            },
            "args": map[string]any{
                "type":        "string",
                "description": "Optional arguments for the skill",
            },
        },
        "required": []string{"skill"},
    },
}
```

#### 1.2 Skill Tool Execution

```go
func (t *SkillTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
    skillName, _ := params["skill"].(string)
    args, _ := params["args"].(string)

    // 1. 查找 skill
    sk, ok := skill.DefaultRegistry.Get(skillName)
    if !ok {
        return ui.NewErrorResult(t.Name(), fmt.Sprintf("Skill not found: %s", skillName))
    }

    // 2. 加载完整指令
    instructions := sk.GetInstructions()

    // 3. 构建执行上下文
    var sb strings.Builder
    sb.WriteString(fmt.Sprintf("<skill-invocation name=\"%s\">\n", sk.FullName()))
    if args != "" {
        sb.WriteString(fmt.Sprintf("Arguments: %s\n\n", args))
    }
    sb.WriteString(instructions)
    sb.WriteString("\n</skill-invocation>")

    // 4. 返回结果 - 注入到下一轮对话
    return ui.ToolResult{
        Success: true,
        Output:  sb.String(),
        Metadata: ui.ResultMetadata{
            Title:    t.Name(),
            Icon:     "⚡",
            Subtitle: fmt.Sprintf("Loaded skill: %s", sk.FullName()),
        },
        // 特殊标记：指令需要注入到系统提示
        SkillInstructions: sb.String(),
    }
}
```

#### 1.3 System Prompt 中的 Available Skills

修改 `GetAvailableSkillsPrompt()` 以匹配 Claude Code 格式：

```go
func (r *Registry) GetAvailableSkillsPrompt() string {
    active := r.GetActive()
    if len(active) == 0 {
        return ""
    }

    var sb strings.Builder
    sb.WriteString("Available skills:\n")

    for _, skill := range active {
        sb.WriteString(fmt.Sprintf("- %s: %s\n", skill.FullName(), skill.Description))
    }

    sb.WriteString("\nWhen a skill is relevant, invoke it using the Skill tool.")
    return sb.String()
}
```

---

### Phase 2: 脚本执行支持

让 Skill 目录中的脚本可以被 Claude 执行。

#### 2.1 Skill 结构扩展

```go
// internal/skill/types.go

type Skill struct {
    // 现有字段...

    // 新增：资源路径
    SkillDir    string   // Skill 目录路径
    Scripts     []string // scripts/ 目录中的脚本列表
    References  []string // references/ 目录中的文件列表
    Assets      []string // assets/ 目录中的文件列表
}

// GetScriptPath 返回脚本的完整路径
func (s *Skill) GetScriptPath(name string) string {
    return filepath.Join(s.SkillDir, "scripts", name)
}

// GetReferencePath 返回参考文件的完整路径
func (s *Skill) GetReferencePath(name string) string {
    return filepath.Join(s.SkillDir, "references", name)
}
```

#### 2.2 加载器扩展

```go
// internal/skill/loader.go

func (l *Loader) loadSkillFile(path string, scope SkillScope, defaultNamespace string) (*Skill, error) {
    // 现有代码...

    // 扫描资源目录
    skillDir := filepath.Dir(path)
    skill.SkillDir = skillDir

    // 扫描 scripts/
    scriptsDir := filepath.Join(skillDir, "scripts")
    if entries, err := os.ReadDir(scriptsDir); err == nil {
        for _, e := range entries {
            if !e.IsDir() {
                skill.Scripts = append(skill.Scripts, e.Name())
            }
        }
    }

    // 扫描 references/
    refsDir := filepath.Join(skillDir, "references")
    if entries, err := os.ReadDir(refsDir); err == nil {
        for _, e := range entries {
            if !e.IsDir() {
                skill.References = append(skill.References, e.Name())
            }
        }
    }

    // 扫描 assets/
    assetsDir := filepath.Join(skillDir, "assets")
    if entries, err := os.ReadDir(assetsDir); err == nil {
        for _, e := range entries {
            skill.Assets = append(skill.Assets, e.Name())
        }
    }

    return skill, nil
}
```

#### 2.3 脚本执行权限

在 SKILL.md frontmatter 中添加 `allowed-tools`：

```yaml
---
name: pdf-editor
description: Edit and manipulate PDF files
allowed-tools:
  - Bash
  - Read
  - Write
---
```

当 Skill 激活时，这些工具自动获得权限。

---

### Phase 3: 渐进式加载优化

#### 3.1 Skill Metadata in System Prompt

只包含 name + description，不加载完整内容：

```go
// 系统提示中只显示元数据
func (r *Registry) GetSkillMetadataPrompt() string {
    active := r.GetActive()
    if len(active) == 0 {
        return ""
    }

    var sb strings.Builder
    sb.WriteString("# Available Skills\n\n")
    sb.WriteString("Use the Skill tool to invoke these capabilities:\n\n")

    for _, s := range active {
        // 只包含 name 和 description，不加载 instructions
        sb.WriteString(fmt.Sprintf("- **%s**: %s\n", s.FullName(), s.Description))
    }

    return sb.String()
}
```

#### 3.2 按需加载 References

当 Skill 被调用时，Claude 可以使用 Read 工具读取 references/ 文件：

```markdown
# PDF Editor Skill

## Quick Start
Use `scripts/rotate_pdf.py` to rotate PDFs.

## Advanced Features
- **Form filling**: See [references/FORMS.md](references/FORMS.md)
- **API reference**: See [references/API.md](references/API.md)
```

Claude 会根据需要使用 Read 工具加载这些文件。

---

### Phase 4: UI/UX 改进

#### 4.1 Skill 调用静默化

类似 Task 工具，Skill 调用也应该静默：

```go
// internal/tui/render.go

var silentTools = map[string]bool{
    "TaskCreate": true,
    "TaskUpdate": true,
    "TaskGet":    true,
    "Skill":      true,  // 添加 Skill
}
```

#### 4.2 Skill 执行指示器

显示简洁的 Skill 激活信息：

```
❯ help me create a document

  ⚡ Skill: docx

◆ [LLM 使用 docx skill 的指令执行任务...]
```

---

## 实现计划

### Phase 1: Skill 工具 (1-2 天)
1. 创建 `internal/tool/skill.go`
2. 添加 SkillToolSchema 到 schema.go
3. 实现 Execute 方法
4. 更新 system prompt 格式
5. 测试基本调用

### Phase 2: 脚本支持 (1 天)
1. 扩展 Skill 结构体
2. 扫描 scripts/references/assets 目录
3. 更新加载器
4. 测试脚本执行

### Phase 3: 渐进式加载 (0.5 天)
1. 优化 system prompt 格式
2. 只加载元数据
3. 测试按需加载

### Phase 4: UI 优化 (0.5 天)
1. Skill 调用静默化
2. 添加执行指示器
3. 测试完整流程

---

## 兼容性

### Claude Code 兼容

- 支持 `~/.claude/skills/` 目录
- 支持 `.claude/skills/` 目录
- 支持 `installed_plugins.json` 格式
- SKILL.md 格式完全兼容

### Agent Skills 规范兼容

- 支持标准目录结构
- 支持 scripts/references/assets
- 支持渐进式加载
- 支持 .skill 包格式（可选）

---

## 参考资料

- [Agent Skills Specification](https://agentskills.io/specification)
- [Anthropic Skills Repository](https://github.com/anthropics/skills)
- [Claude Code Skills Documentation](https://code.claude.com/docs/en/skills)
- [How to Create Custom Skills](https://support.claude.com/en/articles/12512198-how-to-create-custom-skills)
