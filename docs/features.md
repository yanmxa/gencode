# GenCode 功能特性清单

GenCode 默认提供的所有功能特性列表，用于发布前回归测试。

**版本**: 0.4.1
**最后更新**: 2026-01-19

---

## 1. 多供应商支持

**参考**: [docs/providers.md](./providers.md)

- ✅ Anthropic Claude (Sonnet 3.5, Opus 3, Haiku 3)
- ✅ OpenAI GPT (GPT-4, GPT-4 Turbo, GPT-3.5 Turbo)
- ✅ Google Gemini (2.0 Flash, 1.5 Pro)
- ✅ Vertex AI Claude (通过 Google Vertex AI)
- ✅ 流式和非流式响应
- ✅ 自动供应商检测（基于 API key）
- ✅ 手动模型选择 (`--provider`, `--model`)

**测试要点**:
```bash
# 测试不同供应商
GEN_PROVIDER=anthropic gen
GEN_PROVIDER=openai gen
gen --provider google --model gemini-2.0-flash-exp
```

---

## 2. 内置工具 (11 个)

### 文件操作
- ✅ **Read** - 读取文件，支持行范围
- ✅ **Write** - 创建新文件
- ✅ **Edit** - 精确字符串替换编辑

### 搜索工具
- ✅ **Glob** - 文件模式匹配 (`**/*.ts`)
- ✅ **Grep** - 内容搜索（支持正则）

### 命令执行
- ✅ **Bash** - Shell 命令执行
  - 超时控制
  - 后台执行 (`run_in_background`)

### 网络工具
- ✅ **WebFetch** - 获取网页内容
- ✅ **WebSearch** - 网络搜索（Exa / Serper / Brave）

### 交互工具
- ✅ **TodoWrite** - 任务列表管理
- ✅ **AskUserQuestion** - 用户询问（单选/多选）
- ✅ **TaskOutput** - 后台任务查询

**测试要点**:
```
Read the README.md file
Create a file /tmp/test.txt with content "test"
Find all TypeScript files in src/
Search for "LLMProvider" in the codebase
Run: ls -la
```

---

## 3. 权限系统

**参考**: [docs/permissions.md](./permissions.md), [proposals/0023-permission-enhancements.md](./proposals/0023-permission-enhancements.md)

- ✅ 4 步权限检查流程（Claude Code 兼容）
- ✅ 规则类型: `DENY`, `ALLOW`, `ASK`
- ✅ 模式匹配（正则表达式）
- ✅ Prompt-based 权限（语义匹配）
- ✅ 默认行为：读自动通过，写需确认
- ✅ SSRF 防护
- ✅ 命令注入防护

**测试要点**:
- 读操作无需确认
- 写操作显示权限提示
- DENY 规则阻止操作
- ALLOW 规则自动通过

---

## 4. 会话管理

**参考**: [proposals/0019-session-enhancements.md](./proposals/0019-session-enhancements.md), [docs/session-compression.md](./session-compression.md)

### 持久化
- ✅ 自动保存到 `~/.gen/sessions/`
- ✅ 包含完整对话历史和 metadata

### 会话操作
- ✅ `/sessions` - 列出会话
- ✅ `/resume <id>` - 恢复会话
- ✅ `/fork <id>` - 分叉会话
- ✅ `/delete <id>` - 删除会话

### 三层压缩
- ✅ Layer 1: 工具输出剪枝 (>20k tokens)
- ✅ Layer 2: LLM 压缩 (超过上下文)
- ✅ Layer 3: 消息过滤 (会话恢复)

**测试要点**:
```bash
# 会话保存和恢复
gen
> What is 2+2?
Ctrl+C

gen
> /sessions
> /resume <id>
> What was my question?
```

---

## 5. 配置系统

**参考**: [proposals/0041-configuration-system.md](./proposals/0041-configuration-system.md)

### 6 层配置层级
1. Managed (系统) - `/Library/Application Support/GenCode/`
2. CLI 参数 - `--provider`, `--model`
3. Local (本地) - `.gen/*.local.json`
4. Project (项目) - `.gen/settings.json`
5. Extra Dirs - `GEN_CONFIG` 环境变量
6. User (用户) - `~/.gen/settings.json`

### 配置特性
- ✅ 深度配置合并
- ✅ GenCode 优先于 Claude Code (同级)
- ✅ 多格式支持 (JSON, Markdown frontmatter)
- ✅ 环境变量扩展 (`${VAR}`)

**测试要点**:
- CLI 参数覆盖配置文件
- 项目配置覆盖用户配置
- 环境变量正确扩展

---

## 6. 记忆系统

**参考**: [docs/memory-system.md](./memory-system.md), [proposals/0006-memory-system.md](./proposals/0006-memory-system.md)

- ✅ 用户全局记忆: `~/.gen/GEN.md`, `~/.claude/CLAUDE.md`
- ✅ 项目记忆: `./GEN.md`, `./CLAUDE.md`
- ✅ 多文件合并
- ✅ 规则文件解析
- ✅ Import 指令 (`@import`)

**测试要点**:
```
Do you see any memory from CLAUDE.md?
What does GEN.md tell you about this project?
```

---

## 7. MCP 集成

**参考**: [docs/mcp.md](./mcp.md), [proposals/0010-mcp-integration.md](./proposals/0010-mcp-integration.md)

### 传输方式
- ✅ Stdio - 本地子进程
- ✅ HTTP - 远程服务器
- ✅ SSE - Server-Sent Events

### MCP 功能
- ✅ 工具命名空间: `mcp_servername_toolname`
- ✅ OAuth 2.0 认证
- ✅ 安全 token 存储
- ✅ 分层配置合并 (managed > local > project > user)

**测试要点**:
- `.mcp.json` 正确加载
- MCP 服务器启动
- MCP 工具调用成功

---

## 8. 子代理系统

**参考**: [docs/custom-agents.md](./custom-agents.md), [proposals/0003-task-subagents.md](./proposals/0003-task-subagents.md)

### 内置子代理
- ✅ **Explore** - 代码探索
- ✅ **Plan** - 计划生成
- ✅ **Bash** - 命令执行专家
- ✅ **general-purpose** - 通用任务

### 自定义代理
- ✅ JSON 格式: `~/.gen/agents/*.json`
- ✅ Markdown 格式: `~/.claude/agents/*.md`
- ✅ 项目级代理
- ✅ 并行任务执行
- ✅ 后台执行 (`run_in_background`)
- ✅ 任务恢复 (`resume`)

**测试要点**:
```
Explore the project structure
Create a plan for adding a new feature
```

---

## 9. Hooks 系统

**参考**: [docs/hooks.md](./hooks.md), [proposals/0009-hooks-system.md](./proposals/0009-hooks-system.md)

### 事件类型
- ✅ PreToolUse - 工具调用前
- ✅ PostToolUse - 工具调用后
- ✅ PostToolUseFailure - 工具失败
- ✅ SessionStart - 会话开始
- ✅ Stop - 会话结束

### Hooks 功能
- ✅ Shell 命令执行
- ✅ 工具匹配器（正则/通配符）
- ✅ JSON 上下文（stdin）
- ✅ 阻塞 hooks (exit code 2)

**测试要点**:
- Hook 在正确时机触发
- 阻塞 hook 能阻止工具调用

---

## 10. 自定义命令和技能

**参考**: [docs/custom-commands.md](./custom-commands.md), [proposals/0011-custom-commands.md](./proposals/0011-custom-commands.md), [proposals/0021-skills-system.md](./proposals/0021-skills-system.md)

### 自定义命令
- ✅ Markdown 格式斜杠命令
- ✅ 变量扩展: `$ARGUMENTS`, `$1`, `$2`
- ✅ 文件包含: `@file`
- ✅ 模型覆盖
- ✅ 工具预授权

### 技能系统
- ✅ `SKILL.md` 文件定义
- ✅ 分层合并 (user + project)
- ✅ 嵌套目录支持

**测试要点**:
```
/test-command arg1 arg2
What skills are available?
```

---

## 11. 计划模式

**参考**: [proposals/0004-plan-mode.md](./proposals/0004-plan-mode.md)

- ✅ 设计优先的任务处理
- ✅ 只读工具访问（计划阶段）
- ✅ 预批准权限系统
- ✅ 两阶段执行：计划 → 执行
- ✅ 检查点和恢复

**测试要点**:
```
Enter plan mode and create a plan for implementing feature X
```

---

## 12. 成本跟踪

**参考**: [proposals/0025-cost-tracking.md](./proposals/0025-cost-tracking.md), [docs/cost-tracking-comparison.md](./cost-tracking-comparison.md)

- ✅ 输入/输出 token 计数
- ✅ 缓存读取 token (如支持)
- ✅ 所有供应商成本计算
- ✅ 实时成本累计
- ✅ 会话成本存储

**测试要点**:
- Token 统计显示
- 成本估算合理

---

## 13. CLI 界面

**参考**: [proposals/0038-interactive-cli-ui.md](./proposals/0038-interactive-cli-ui.md)

### UI 组件
- ✅ React + Ink 终端 UI
- ✅ Header (Logo + 模型信息)
- ✅ Markdown 渲染 + 代码高亮
- ✅ 流式输出
- ✅ 加载 Spinner

### 交互组件
- ✅ 权限提示 UI
- ✅ 问题提示 UI
- ✅ 计划审批 UI
- ✅ Todo 列表 UI
- ✅ 命令建议和模糊搜索
- ✅ 输入历史导航

### 其他
- ✅ 主题支持 (浅色/深色)
- ✅ 模型选择器
- ✅ Provider 管理器

**测试要点**:
- UI 正确渲染
- 交互组件工作正常
- 流式输出流畅

---

## 14. 安全特性

- ✅ Zod schema 输入验证
- ✅ SSRF 防护（内网 IP）
- ✅ 命令注入防护
- ✅ 路径遍历防护
- ✅ 工作区隔离

**测试要点**:
```
Fetch content from http://127.0.0.1  # 应被阻止
Read ../../../etc/passwd  # 应被阻止或警告
```

---

## 15. 错误处理

- ✅ 清晰的错误消息
- ✅ 上下文信息
- ✅ 解决建议
- ✅ 优雅降级
- ✅ 不崩溃

**测试要点**:
```
Read /nonexistent/file.txt  # 显示友好错误
```

---

## 16. 兼容性

### Claude Code 兼容
- ✅ 配置文件格式
- ✅ 命令格式
- ✅ 权限规则
- ✅ Hooks 配置
- ✅ 记忆系统 (CLAUDE.md)
- ✅ 双重配置路径 (`.gen/` + `.claude/`)

### 平台支持
- ✅ macOS
- ✅ Linux
- ⏳ Windows (实验性)

### Node.js 版本
- ✅ Node.js 18+
- ✅ Node.js 20+
- ✅ Node.js 22+

---

## 快速回归测试（5 分钟）

发布前最小测试集：

```bash
# 1. 启动
gen

# 2. 基础对话
> hi

# 3. 文件读取
> Read the package.json file

# 4. 文件创建
> Create a test file /tmp/smoke-test.txt with content "OK"

# 5. 搜索
> Find all .ts files in src/

# 6. Bash 执行
> Run: ls -la

# 7. 会话保存
Ctrl+C

# 8. 会话恢复
gen
> /sessions
> /resume <latest-id>
> What was my last command?

# 清理
rm /tmp/smoke-test.txt
```

**所有步骤通过** ✅ → 可以发布
**任何步骤失败** ❌ → 需要修复

---

## 测试优先级

### P0 - 必须 100% 通过
- 启动和基础对话
- 供应商切换
- 11 个内置工具基本功能
- 会话保存和恢复
- 核心回归场景

### P1 - 必须 95%+ 通过
- 权限系统
- CLI 界面
- 错误处理
- 配置系统
- 安全特性

### P2 - 建议 80%+ 通过
- MCP 集成
- Hooks 系统
- 子代理系统
- 计划模式
- 成本跟踪

### P3 - 建议 60%+ 通过
- 自定义命令/技能
- 记忆系统
- 跨平台兼容

---

## 发布检查清单

在发布新版本前，确认：

- [ ] 快速回归测试 100% 通过
- [ ] P0 功能 100% 通过
- [ ] P1 功能 95%+ 通过
- [ ] 无 Critical 级别问题
- [ ] 文档更新完整
- [ ] CHANGELOG.md 已更新
- [ ] 版本号已更新

---

## 相关文档

### 核心功能文档
- [providers.md](./providers.md) - 供应商管理
- [permissions.md](./permissions.md) - 权限系统
- [mcp.md](./mcp.md) - MCP 集成
- [hooks.md](./hooks.md) - Hooks 系统
- [memory-system.md](./memory-system.md) - 记忆系统
- [session-compression.md](./session-compression.md) - 会话压缩
- [custom-commands.md](./custom-commands.md) - 自定义命令
- [custom-agents.md](./custom-agents.md) - 自定义代理

### Proposals
- [proposals/](./proposals/) - 功能提案和详细设计

### 测试指南
- [manual-testing-guide.md](./manual-testing-guide.md) - 手动测试
- [interactive-testing-guide.md](./interactive-testing-guide.md) - 交互测试

---

**维护者**: GenCode Team
**版本**: 0.4.1
**文档日期**: 2026-01-19
