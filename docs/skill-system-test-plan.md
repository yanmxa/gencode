# GenCode Skill System 测试计划

## 测试环境准备

### 1. 创建测试 Skill

```bash
# 创建测试目录
mkdir -p ~/.gen/skills/test-skill/{scripts,references,assets}

# 创建 SKILL.md
cat > ~/.gen/skills/test-skill/SKILL.md << 'EOF'
---
name: test-skill
description: A test skill for verification. Use this when testing skill functionality.
allowed-tools:
  - Bash
  - Read
argument-hint: "[--verbose]"
---

# Test Skill

This is a test skill for verifying the skill system.

## Instructions

1. Say "Hello from test-skill!"
2. If --verbose flag is provided, also show current directory
3. List any available scripts

## Scripts

Run scripts from the scripts/ directory using Bash.
EOF

# 创建测试脚本
cat > ~/.gen/skills/test-skill/scripts/hello.sh << 'EOF'
#!/bin/bash
echo "Hello from test-skill script!"
echo "Current time: $(date)"
EOF
chmod +x ~/.gen/skills/test-skill/scripts/hello.sh

# 创建测试 Python 脚本
cat > ~/.gen/skills/test-skill/scripts/info.py << 'EOF'
#!/usr/bin/env python3
import os
import sys

print("Test Skill Info Script")
print(f"Python version: {sys.version}")
print(f"Working directory: {os.getcwd()}")
print(f"Arguments: {sys.argv[1:]}")
EOF
chmod +x ~/.gen/skills/test-skill/scripts/info.py

# 创建参考文档
cat > ~/.gen/skills/test-skill/references/README.md << 'EOF'
# Test Skill Reference

This is a reference document for the test skill.

## Usage

- Use `hello.sh` for basic greeting
- Use `info.py` for system information
EOF

# 创建资源文件
echo "Test asset content" > ~/.gen/skills/test-skill/assets/sample.txt
```

### 2. 创建带命名空间的 Skill

```bash
# 创建带命名空间的 skill
mkdir -p ~/.gen/skills/namespaced-skill/scripts

cat > ~/.gen/skills/namespaced-skill/SKILL.md << 'EOF'
---
name: demo
namespace: myns
description: A namespaced skill for testing namespace:name format.
---

# Namespaced Demo Skill

This skill tests the namespace functionality.

## Usage

Invoke with `/myns:demo` or through Skill tool.
EOF
```

---

## 测试用例

### Phase 1: Skill 工具测试

#### TC-1.1: 基本 Skill 工具调用

**前提**: test-skill 已创建且启用

**步骤**:
1. 启动 `./gen`
2. 输入: `use the Skill tool to invoke test-skill`

**预期结果**:
```
⚡Skill(test-skill)
  ⎿  Skill → Loaded: test-skill

◆ [LLM 执行 skill 指令...]
```

**验证点**:
- [ ] Skill 工具被调用
- [ ] 返回 `<skill-invocation>` 内容
- [ ] LLM 按照指令执行

---

#### TC-1.2: 带参数的 Skill 调用

**步骤**:
1. 输入: `invoke test-skill with --verbose flag`

**预期结果**:
```
⚡Skill(test-skill, --verbose)
  ⎿  Skill → Loaded: test-skill
```

**验证点**:
- [ ] 参数正确传递
- [ ] `User arguments: --verbose` 出现在输出中

---

#### TC-1.3: 禁用 Skill 调用失败

**前提**: 创建一个禁用的 skill

**步骤**:
1. 禁用 test-skill: `/skill` → 选择 test-skill → 切换为 disable
2. 输入: `use Skill tool to invoke test-skill`

**预期结果**:
```
⚡Skill(test-skill)
  ✗  Skill → 1 lines
    Error: skill is disabled: test-skill
```

**验证点**:
- [ ] 返回错误信息
- [ ] 明确指出 skill 被禁用

---

#### TC-1.4: 不存在的 Skill

**步骤**:
1. 输入: `use Skill tool to invoke nonexistent-skill`

**预期结果**:
```
⚡Skill(nonexistent-skill)
  ✗  Skill → 1 lines
    Error: skill not found: nonexistent-skill
```

**验证点**:
- [ ] 返回 "skill not found" 错误

---

#### TC-1.5: 命名空间 Skill 调用

**前提**: myns:demo skill 已创建且启用

**步骤**:
1. 输入: `invoke the myns:demo skill`

**预期结果**:
```
⚡Skill(myns:demo)
  ⎿  Skill → Loaded: myns:demo
```

**验证点**:
- [ ] 正确识别 namespace:name 格式

---

#### TC-1.6: 部分名称匹配

**步骤**:
1. 输入: `invoke the demo skill` (不带 namespace)

**预期结果**:
```
⚡Skill(demo)
  ⎿  Skill → Loaded: myns:demo
```

**验证点**:
- [ ] FindByPartialName 正确工作
- [ ] demo → myns:demo

---

### Phase 2: 脚本支持测试

#### TC-2.1: 脚本路径列出

**步骤**:
1. 启用 test-skill 为 active 状态
2. 输入: `invoke test-skill`

**预期结果**:
Skill 输出中包含:
```
Available scripts (use Bash to execute):
  - /path/to/test-skill/scripts/hello.sh
  - /path/to/test-skill/scripts/info.py
```

**验证点**:
- [ ] 脚本路径正确列出
- [ ] 多个脚本都显示

---

#### TC-2.2: 脚本执行

**步骤**:
1. 输入: `run the hello.sh script from test-skill`

**预期结果**:
```
⚡Skill(test-skill)
⚡Bash(~/.gen/skills/test-skill/scripts/hello.sh)
  ⎿  Bash → ...
    Hello from test-skill script!
    Current time: ...
```

**验证点**:
- [ ] LLM 正确找到脚本路径
- [ ] 脚本成功执行
- [ ] 输出正确

---

#### TC-2.3: References 路径列出

**步骤**:
1. 查看 test-skill 的 Skill 调用输出

**预期结果**:
```
Reference files (use Read when needed):
  - /path/to/test-skill/references/README.md
```

**验证点**:
- [ ] References 路径正确列出

---

#### TC-2.4: Reference 文件读取

**步骤**:
1. 输入: `read the README reference from test-skill`

**预期结果**:
```
⚡Skill(test-skill)
⚡Read(~/.gen/skills/test-skill/references/README.md)
  ⎿  Read → ...
```

**验证点**:
- [ ] LLM 正确读取 reference 文件

---

### Phase 3: 渐进式加载测试

#### TC-3.1: System Prompt 只包含元数据

**步骤**:
1. 启用多个 skill 为 active 状态
2. 查看 debug log 中的 system prompt

**预期结果**:
System prompt 包含:
```
# Available Skills

Use the Skill tool to invoke these capabilities:

- **test-skill**: A test skill for verification [2 scripts, 1 refs]
- **myns:demo**: A namespaced skill for testing
```

**验证点**:
- [ ] 只包含 name 和 description
- [ ] 不包含完整 instructions
- [ ] 显示资源计数 [X scripts, Y refs]

---

#### TC-3.2: Skill 调用时加载完整内容

**步骤**:
1. 触发 Skill 工具调用
2. 检查返回内容

**预期结果**:
```
<skill-invocation name="test-skill">
[完整 SKILL.md 内容]
</skill-invocation>
```

**验证点**:
- [ ] 完整 instructions 只在调用时加载

---

### Phase 4: UI/UX 测试

#### TC-4.1: Skill 工具执行指示器

**步骤**:
1. 触发 Skill 工具调用
2. 观察 spinner 文本

**预期结果**:
```
⚡Skill
  ⠹ Loading skill...
```

**验证点**:
- [ ] 显示 "Loading skill..."

---

#### TC-4.2: Skill 调用结果显示

**步骤**:
1. 成功调用一个 skill

**预期结果**:
```
⚡Skill(test-skill)
  ⎿  Skill → Loaded: test-skill
```

**验证点**:
- [ ] 显示 skill 名称
- [ ] 显示 "Loaded: xxx"

---

### Phase 5: 兼容性测试

#### TC-5.1: Slash 命令调用 (现有功能)

**步骤**:
1. 启用 test-skill
2. 输入: `/test-skill --verbose`

**预期结果**:
- skill 指令注入到对话
- LLM 按照指令执行

**验证点**:
- [ ] Slash 命令仍然工作
- [ ] 参数正确传递

---

#### TC-5.2: Claude Code 目录兼容

**步骤**:
1. 创建 skill 在 `~/.claude/skills/claude-skill/`
2. 启动 GenCode

**预期结果**:
- claude-skill 被加载
- 可以正常调用

**验证点**:
- [ ] Claude 目录 skills 被加载

---

#### TC-5.3: Plugin Skills 加载

**步骤**:
1. 安装一个包含 skills 的 plugin
2. 启动 GenCode

**预期结果**:
- Plugin skills 被加载
- 使用 plugin 名称作为 namespace

**验证点**:
- [ ] Plugin skills 正确加载
- [ ] Namespace 正确设置

---

## 测试执行脚本

```bash
#!/bin/bash
# test-skill-system.sh

echo "=== GenCode Skill System Test ==="

# 准备测试环境
echo "1. Setting up test skills..."
./setup-test-skills.sh

# 启动 GenCode 进行交互测试
echo "2. Starting GenCode..."
echo "Please run the following tests manually:"
echo ""
echo "TC-1.1: use the Skill tool to invoke test-skill"
echo "TC-1.2: invoke test-skill with --verbose flag"
echo "TC-1.3: (disable test-skill first) use Skill tool to invoke test-skill"
echo "TC-1.4: use Skill tool to invoke nonexistent-skill"
echo "TC-2.1: invoke test-skill (check script paths)"
echo "TC-2.2: run the hello.sh script from test-skill"
echo ""

./gen
```

---

## 回归测试清单

| 测试项 | 通过 | 备注 |
|--------|------|------|
| TC-1.1 基本调用 | [ ] | |
| TC-1.2 带参数调用 | [ ] | |
| TC-1.3 禁用 skill | [ ] | |
| TC-1.4 不存在 skill | [ ] | |
| TC-1.5 命名空间调用 | [ ] | |
| TC-1.6 部分名称匹配 | [ ] | |
| TC-2.1 脚本列出 | [ ] | |
| TC-2.2 脚本执行 | [ ] | |
| TC-2.3 References 列出 | [ ] | |
| TC-2.4 Reference 读取 | [ ] | |
| TC-3.1 渐进式加载 | [ ] | |
| TC-3.2 完整内容加载 | [ ] | |
| TC-4.1 执行指示器 | [ ] | |
| TC-4.2 结果显示 | [ ] | |
| TC-5.1 Slash 命令 | [ ] | |
| TC-5.2 Claude 目录 | [ ] | |
| TC-5.3 Plugin skills | [ ] | |

---

## 自动化测试 (未来)

```go
// internal/skill/skill_test.go

func TestSkillToolExecution(t *testing.T) {
    // 创建测试 skill
    // 调用 Skill 工具
    // 验证返回结果
}

func TestSkillResourceScanning(t *testing.T) {
    // 创建带 scripts/references/assets 的 skill
    // 验证资源正确扫描
}

func TestProgressiveLoading(t *testing.T) {
    // 验证 system prompt 只包含元数据
    // 验证调用时加载完整内容
}
```
