# GenCode vs Claude Code 交互式对比测试指南

## 测试环境设置

### Tmux Session: Val
- **Window 0 (gencode)**: GenCode 运行窗口
- **Window 1 (claude)**: Claude Code 运行窗口（2 panes）
- **Window 2 (opencode)**: OpenCode 参考窗口

## 快速重启命令

### GenCode
```bash
# 方法 1: 使用 npm 脚本（推荐）
npm run restart

# 方法 2: 手动构建和启动
npm run build && npm start

# 方法 3: 开发模式（不需要重启，自动重编译）
npm run dev  # 在另一个终端
npm run start:dev  # 在测试终端
```

### Claude Code
```bash
# Claude Code 通常不需要重启，除非修改了配置
claude  # 直接启动
```

## 对比测试项目

### 1. 启动和欢迎界面

#### 测试步骤
1. 在 gencode 窗口启动 `npm start`
2. 在 claude 窗口启动 `claude`
3. 截图对比欢迎界面

#### 观察要点
- [ ] Logo 和品牌展示
- [ ] 启动时间
- [ ] 模型显示（GenCode: gemini-3-pro-preview, Claude Code: claude-sonnet-4）
- [ ] 工作目录显示
- [ ] 帮助提示
- [ ] 错误/警告信息（如 MCP 警告）

#### GenCode 当前输出示例
```
 ██████╗ ███████╗███╗   ██╗ ██████╗ ██████╗ ██████╗ ███████╗
██╔════╝ ██╔════╝████╗  ██║██╔════╝██╔═══██╗██╔══██╗██╔════╝
...

gemini-3-pro-preview · ~/Workspace/ideas/gencode
? for help · Ctrl+C to exit
```

### 2. 基本对话测试

#### 测试用例 1: 简单问候
```
用户输入: hi
```

**观察要点**:
- [ ] 响应时间
- [ ] 响应格式（markdown 渲染）
- [ ] Token 使用统计
- [ ] 状态指示器（GenCode: "Woven for 6s"）

#### 测试用例 2: 代码解释
```
用户输入: Explain what the Skill tool does in this codebase
```

**观察要点**:
- [ ] 是否正确读取文件
- [ ] 工具调用的显示方式
- [ ] 流式输出体验
- [ ] 最终答案的清晰度

#### 测试用例 3: 文件操作
```
用户输入: List all TypeScript files in src/skills/
```

**观察要点**:
- [ ] Glob 或 Bash 工具使用
- [ ] 工具调用的可见性
- [ ] 结果展示格式
- [ ] 错误处理

### 3. Skills 系统测试

#### 测试用例: 激活 Skill
```bash
# GenCode (如果实现了 Skill tool)
用户输入: Use the test-skill to verify functionality

# Claude Code
用户输入: /skill test-skill
```

**观察要点**:
- [ ] Skill 激活方式（GenCode: Skill tool, Claude Code: /skill 命令）
- [ ] Skill 内容展示
- [ ] 参数传递
- [ ] 错误提示

### 4. Commands 系统测试

#### 测试用例: 执行自定义命令
```bash
# GenCode
用户输入: Execute the test-command

# Claude Code
用户输入: /test-command
```

**观察要点**:
- [ ] 命令识别和执行
- [ ] 模板变量展开
- [ ] 工具预授权效果
- [ ] Model override 是否生效

### 5. 工具调用可见性测试

#### 测试用例: 读取文件
```
用户输入: Read the package.json file and tell me the version
```

**观察要点**:
- [ ] 工具调用是否显示在 UI 中
- [ ] Claude Code: 工具调用有专门的视觉元素
- [ ] GenCode: 如何展示工具使用？
- [ ] 工具结果的格式化

### 6. 错误处理测试

#### 测试用例 1: 无效文件路径
```
用户输入: Read the file /nonexistent/file.txt
```

#### 测试用例 2: 权限错误
```
用户输入: Write to /etc/passwd
```

**观察要点**:
- [ ] 错误信息的清晰度
- [ ] 错误恢复能力
- [ ] 用户引导

### 7. 长对话和上下文测试

#### 测试步骤
1. 进行多轮对话（5-10 轮）
2. 引用之前的对话内容
3. 观察上下文管理

**观察要点**:
- [ ] 上下文保持能力
- [ ] 会话历史显示
- [ ] Token 限制处理
- [ ] 性能（响应时间随对话增长的变化）

### 8. 快捷键和命令测试

#### GenCode
- `?` - 帮助
- `Ctrl+C` - 退出

#### Claude Code
- `/help` - 帮助
- `/clear` - 清除会话
- `/sessions` - 会话管理
- 等等

**观察要点**:
- [ ] 快捷键响应
- [ ] 命令自动补全
- [ ] 帮助文档完整性

### 9. UI/UX 细节对比

#### 视觉设计
- [ ] 颜色方案
- [ ] 字体和排版
- [ ] 分隔线和边框
- [ ] 图标使用

#### 交互体验
- [ ] 输入延迟感知
- [ ] 流式输出的流畅度
- [ ] 错误反馈的及时性
- [ ] 状态指示清晰度

#### 信息密度
- [ ] 单屏信息量
- [ ] 空白利用
- [ ] 可读性

## 测试记录模板

### 测试日期: YYYY-MM-DD
### 测试项目: [项目名称]

#### GenCode 表现
- **版本**: 0.4.1
- **模型**: gemini-3-pro-preview
- **截图**: [描述或路径]
- **观察**:
  - 优点:
  - 缺点:
  - Bug:

#### Claude Code 表现
- **版本**: [版本号]
- **模型**: claude-sonnet-4
- **截图**: [描述或路径]
- **观察**:
  - 优点:
  - 缺点:

#### 改进建议
1.
2.
3.

## 快速截图命令

```bash
# macOS
# Cmd+Shift+4, 然后空格键点击窗口

# 或使用 tmux 捕获
tmux capture-pane -t Val:0 -p > gencode-output.txt
tmux capture-pane -t Val:1 -p > claude-output.txt

# 对比
diff gencode-output.txt claude-output.txt
```

## 持续改进流程

### 1. 发现问题
- 通过对比测试发现 UX 差异
- 记录用户体验问题
- 收集 bug 和错误

### 2. 分类优先级
- **P0 (Critical)**: 功能性 bug，阻碍使用
- **P1 (High)**: 明显的 UX 问题，影响体验
- **P2 (Medium)**: 改进建议，优化体验
- **P3 (Low)**: Nice to have

### 3. 实施改进
- 修改代码
- `npm run restart` 重启测试
- 验证改进效果
- 再次对比

### 4. 文档更新
- 更新 CHANGELOG
- 更新相关文档
- 记录设计决策

## 常见改进方向

### UI 改进
1. **颜色和主题**
   - 统一颜色方案
   - 支持 dark/light 主题
   - 语法高亮

2. **布局优化**
   - 更好的信息层级
   - 清晰的视觉分隔
   - 合理的留白

3. **状态指示**
   - 加载状态
   - 进度反馈
   - 错误高亮

### 功能改进
1. **工具可见性**
   - 显示工具调用
   - 工具参数展示
   - 工具结果格式化

2. **交互优化**
   - 自动补全
   - 快捷键支持
   - 命令历史

3. **错误处理**
   - 友好的错误信息
   - 错误恢复建议
   - 调试信息

### 性能优化
1. 启动时间
2. 响应延迟
3. 内存使用
4. Token 效率

## 下一步行动

1. 启动两个系统进行并行测试
2. 按照测试清单逐项对比
3. 记录发现的问题和改进点
4. 实施高优先级的改进
5. 重复测试验证

---

**提示**: 使用 `tmux select-window -t Val:0` 和 `tmux select-window -t Val:1` 快速切换窗口进行对比。
