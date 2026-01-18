# GenCode 交互式测试检查清单

## 测试会话信息
- **日期**: ___________
- **GenCode 版本**: 0.4.1
- **测试目标**: _______________________

---

## 1. 启动测试 ✓

### GenCode
```bash
cd /Users/myan/Workspace/ideas/gencode
npm run restart
```

- [ ] 启动成功
- [ ] Logo 正确显示
- [ ] 模型信息显示: gemini-3-pro-preview
- [ ] 工作目录显示正确
- [ ] 无致命错误
- [ ] 启动时间: _____ 秒

**观察到的问题**:
```



```

### Claude Code
```bash
claude
```

- [ ] 启动成功
- [ ] 欢迎信息清晰
- [ ] 模型信息显示: claude-sonnet-4
- [ ] 启动时间: _____ 秒

**对比差异**:
```



```

---

## 2. 基础对话测试 ✓

### Test Case 2.1: 简单问候
**输入**: `hi`

#### GenCode
- [ ] 响应成功
- [ ] 响应时间: _____ 秒
- [ ] Token 统计显示
- [ ] Markdown 渲染正确

**输出质量** (1-5): _____

#### Claude Code
- [ ] 响应成功
- [ ] 响应时间: _____ 秒
- [ ] Token 统计显示
- [ ] Markdown 渲染正确

**输出质量** (1-5): _____

**对比观察**:
```



```

---

### Test Case 2.2: 代码查询
**输入**: `What TypeScript files are in the src/skills/ directory?`

#### GenCode
- [ ] 使用了正确的工具: [ ] Glob [ ] Bash [ ] Read [ ] Other: _____
- [ ] 工具调用是否可见: [ ] 是 [ ] 否
- [ ] 结果准确
- [ ] 响应时间: _____ 秒

**截图/输出**:
```



```

#### Claude Code
- [ ] 使用了正确的工具: [ ] Glob [ ] Bash [ ] Read [ ] Other: _____
- [ ] 工具调用是否可见: [ ] 是 [ ] 否
- [ ] 结果准确
- [ ] 响应时间: _____ 秒

**截图/输出**:
```



```

**UX 对比**:
- [ ] GenCode 工具调用更清晰
- [ ] Claude Code 工具调用更清晰
- [ ] 无明显差异

---

## 3. 文件操作测试 ✓

### Test Case 3.1: 读取文件
**输入**: `Read the package.json file and tell me the main entry point`

#### GenCode
- [ ] 成功读取文件
- [ ] 正确提取信息
- [ ] 工具调用显示: [ ] 清晰 [ ] 不清晰 [ ] 没有显示
- [ ] 错误处理: [ ] N/A [ ] 处理得当 [ ] 需改进

#### Claude Code
- [ ] 成功读取文件
- [ ] 正确提取信息
- [ ] 工具调用显示: [ ] 清晰 [ ] 不清晰 [ ] 没有显示

**差异**:
```



```

---

### Test Case 3.2: 写入文件
**输入**: `Create a test file /tmp/gencode-test.txt with content "Hello from GenCode"`

#### GenCode
- [ ] 成功创建文件
- [ ] 内容正确
- [ ] 权限处理: [ ] 正确 [ ] 有问题 [ ] N/A
- [ ] 确认提示: [ ] 有 [ ] 无

#### Claude Code
- [ ] 成功创建文件
- [ ] 内容正确
- [ ] 确认提示: [ ] 有 [ ] 无

**验证**:
```bash
cat /tmp/gencode-test.txt
# 清理: rm /tmp/gencode-test.txt
```

---

## 4. Skills 系统测试 ✓

### Test Case 4.1: 列出 Skills
**GenCode 输入**: `What skills are available?`
**Claude Code 输入**: `/skills`

#### GenCode
- [ ] 成功列出 skills
- [ ] Skill 数量: _____
- [ ] 显示格式: [ ] 清晰 [ ] 可改进

**Skills 列表**:
```



```

#### Claude Code
- [ ] 成功列出 skills
- [ ] Skill 数量: _____
- [ ] 显示格式: [ ] 清晰 [ ] 可改进

**对比**:
- [ ] GenCode 的 skills 更多
- [ ] Claude Code 的 skills 更多
- [ ] 相似

---

### Test Case 4.2: 激活 Skill
**GenCode 输入**: `Use the Skill tool to activate test-skill`
**Claude Code 输入**: `/skill test-skill` 或 `Use skill test-skill`

#### GenCode
- [ ] Skill 成功激活
- [ ] 内容正确注入
- [ ] 参数传递: [ ] 支持 [ ] 不支持 [ ] N/A

#### Claude Code
- [ ] Skill 成功激活
- [ ] 内容正确注入
- [ ] 参数传递: [ ] 支持 [ ] 不支持 [ ] N/A

**发现的问题**:
```



```

---

## 5. Commands 系统测试 ✓

### Test Case 5.1: 列出 Commands
**GenCode 输入**: `What commands are available?`
**Claude Code 输入**: `/help` 或 `help`

#### GenCode
- [ ] 成功列出 commands
- [ ] Command 数量: _____
- [ ] 帮助信息: [ ] 完整 [ ] 不完整

#### Claude Code
- [ ] 成功列出 commands
- [ ] Command 数量: _____
- [ ] 帮助信息: [ ] 完整 [ ] 不完整

---

### Test Case 5.2: 执行 Command
**GenCode 输入**: `Execute /test-command with arguments "test arg"`
**Claude Code 输入**: `/test-command test arg`

#### GenCode
- [ ] Command 识别成功
- [ ] 参数正确传递
- [ ] 模板展开正确
- [ ] 工具预授权生效: [ ] 是 [ ] 否 [ ] 未知

#### Claude Code
- [ ] Command 识别成功
- [ ] 参数正确传递
- [ ] 模板展开正确

**注意事项**:
```



```

---

## 6. 错误处理测试 ✓

### Test Case 6.1: 无效文件路径
**输入**: `Read the file /this/does/not/exist.txt`

#### GenCode
- [ ] 错误信息清晰
- [ ] 提供了帮助建议
- [ ] 不会崩溃

**错误消息**:
```



```

#### Claude Code
- [ ] 错误信息清晰
- [ ] 提供了帮助建议
- [ ] 不会崩溃

**对比**:
- 哪个错误处理更好？_______________

---

### Test Case 6.2: 无效命令
**GenCode 输入**: `Execute /nonexistent-command`
**Claude Code 输入**: `/nonexistent-command`

#### GenCode
- [ ] 给出错误提示
- [ ] 建议类似命令: [ ] 是 [ ] 否

#### Claude Code
- [ ] 给出错误提示
- [ ] 建议类似命令: [ ] 是 [ ] 否

---

## 7. 性能测试 ✓

### 测试场景: 连续 5 次对话
输入序列:
1. `hi`
2. `List files in src/`
3. `Read src/index.ts`
4. `What does this codebase do?`
5. `How is the tool system implemented?`

#### GenCode
- [ ] 所有响应成功
- [ ] 平均响应时间: _____ 秒
- [ ] Token 累计: _____
- [ ] 上下文保持良好

**性能问题**:
```



```

#### Claude Code
- [ ] 所有响应成功
- [ ] 平均响应时间: _____ 秒
- [ ] Token 累计: _____
- [ ] 上下文保持良好

---

## 8. UI/UX 详细评分 ✓

评分标准: 1-5 (1=差, 5=优秀)

### GenCode
| 方面 | 评分 | 说明 |
|-----|-----|-----|
| 启动体验 | ___ | |
| 视觉设计 | ___ | |
| 信息清晰度 | ___ | |
| 工具调用可见性 | ___ | |
| 错误提示 | ___ | |
| 流式输出流畅度 | ___ | |
| 响应速度感知 | ___ | |
| 整体用户体验 | ___ | |

**GenCode 总分**: _____ / 40

### Claude Code
| 方面 | 评分 | 说明 |
|-----|-----|-----|
| 启动体验 | ___ | |
| 视觉设计 | ___ | |
| 信息清晰度 | ___ | |
| 工具调用可见性 | ___ | |
| 错误提示 | ___ | |
| 流式输出流畅度 | ___ | |
| 响应速度感知 | ___ | |
| 整体用户体验 | ___ | |

**Claude Code 总分**: _____ / 40

---

## 9. 发现的问题汇总 ✓

### P0 - Critical (立即修复)
1.
2.
3.

### P1 - High (本周修复)
1.
2.
3.

### P2 - Medium (本月修复)
1.
2.
3.

### P3 - Low (未来考虑)
1.
2.
3.

---

## 10. 改进建议 ✓

### UI 改进
1.
2.
3.

### 功能改进
1.
2.
3.

### 性能优化
1.
2.
3.

---

## 测试结论

### GenCode 优势
1.
2.
3.

### Claude Code 优势
1.
2.
3.

### 最重要的改进方向
1.
2.
3.

---

## 下次测试计划

**日期**: ___________
**重点**: ___________
**预期改进**: ___________

---

**测试完成时间**: ___________
**测试人员**: ___________
