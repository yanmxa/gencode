# 成本跟踪实现对比：Claude Code vs OpenCode vs GenCode

## 一、三个工具的成本跟踪实现对比

### 1.1 OpenCode 的实现（生产级别）

OpenCode 是用 TypeScript/Go 实现的 AI 编码助手，有**完整的成本跟踪系统**。

#### 核心特点
- ✅ 支持多种 token 类型（输入/输出/推理/缓存读/缓存写）
- ✅ 使用 Decimal.js 确保财务计算精度
- ✅ 支持 Prompt Cache（Anthropic 的缓存功能）
- ✅ 支持 200k+ 上下文的分阶段定价
- ✅ 实时 UI 显示成本

#### 数据结构
```typescript
// 每个步骤完成时记录的成本信息
StepFinishPart {
  type: "step-finish",
  cost: 0.0234,              // USD 成本
  tokens: {
    input: 1234,             // 输入 token
    output: 567,             // 输出 token
    reasoning: 50,           // 推理 token（O1 模型）
    cache: {
      read: 100,             // 缓存命中读取
      write: 20,             // 缓存写入
    }
  }
}

// 每条助手消息的累积成本
AssistantMessage {
  cost: 0.0456,              // 这条消息的总成本
  tokens: { ... },           // 累积的 token 使用
  // ... 其他字段
}
```

#### 定价表存储方式
定价直接嵌入在 Model 对象中：
```typescript
Model {
  id: "claude-3-5-sonnet-20241022",
  cost: {
    input: 3.00,             // 每 1M tokens 价格（美元）
    output: 15.00,
    cache: {
      read: 0.30,            // 缓存读取便宜 10 倍
      write: 3.75,           // 缓存写入略贵
    },
    experimentalOver200K: {  // 超过 200k 上下文的特殊定价
      input: 6.00,           // 价格翻倍
      output: 30.00,
      cache: { ... }
    }
  }
}
```

---

### 1.2 GenCode 当前状态（基础设施已有）

GenCode 是我们正在开发的工具，**已有 token 跟踪，但缺少成本计算**。

#### 已有功能
- ✅ 所有 Provider 都返回 token usage
- ✅ Session 存储 token 使用量
- ✅ Example 中已展示如何显示 token

#### 缺失功能
- ❌ 没有成本计算逻辑
- ❌ 没有定价表
- ❌ CLI 不显示成本
- ❌ 不支持高级 token 类型（推理、缓存）
- ❌ 没有预算系统

#### 当前数据结构
```typescript
// Provider 返回的响应
CompletionResponse {
  content: [...],
  stopReason: "end_turn",
  usage?: {
    inputTokens: 1234,
    outputTokens: 567,
    // 缺少：reasoningTokens, cacheReadTokens, cacheWriteTokens
  }
}

// Session 存储
SessionMetadata {
  tokenUsage?: {
    input: 1234,
    output: 567,
    // 缺少：成本信息
  }
}
```

---

### 1.3 GenCode Proposal 0025 的设计（完整方案）

Proposal 设计了一套**完整的成本跟踪系统**。

#### 设计特点
- ✅ 完整的 token 类型支持（包括推理、缓存）
- ✅ 成本追踪和聚合
- ✅ 预算系统（每消息/每会话/每日/每月）
- ✅ 成本报告和对比
- ✅ 多 Provider 成本对比

#### 核心类型定义
```typescript
// Token 使用详情
interface TokenUsage {
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  reasoningTokens?: number;      // 新增：推理 token
  cacheReadTokens?: number;      // 新增：缓存读取
  cacheWriteTokens?: number;     // 新增：缓存写入
}

// 成本估算
interface CostEstimate {
  inputCost: number;             // 输入成本
  outputCost: number;            // 输出成本
  totalCost: number;             // 总成本
  currency: string;              // "USD"
}

// 每条消息的成本记录
interface MessageCost {
  messageId: string;
  timestamp: Date;
  model: string;                 // "claude-sonnet-4"
  provider: string;              // "anthropic"
  tokens: TokenUsage;
  cost: CostEstimate;
}

// 会话级别的成本聚合
interface SessionCost {
  sessionId: string;
  messages: MessageCost[];
  totals: {
    tokens: TokenUsage,          // 累积 token
    cost: CostEstimate,          // 累积成本
  }
}
```

#### 定价表设计
```typescript
interface ProviderPricing {
  provider: string;              // "anthropic"
  model: string;                 // "claude-sonnet-4"
  inputPer1M: number;            // 3.00 USD
  outputPer1M: number;           // 15.00 USD
  effectiveDate: string;         // "2025-01-01"
}

// 2025 年最新定价
const pricing = [
  // Anthropic
  { provider: 'anthropic', model: 'claude-opus-4-5',
    inputPer1M: 15, outputPer1M: 75 },
  { provider: 'anthropic', model: 'claude-sonnet-4',
    inputPer1M: 3, outputPer1M: 15 },
  { provider: 'anthropic', model: 'claude-haiku-3-5',
    inputPer1M: 0.25, outputPer1M: 1.25 },

  // OpenAI
  { provider: 'openai', model: 'gpt-4o',
    inputPer1M: 2.5, outputPer1M: 10 },
  { provider: 'openai', model: 'o1',
    inputPer1M: 15, outputPer1M: 60 },

  // Google Gemini
  { provider: 'gemini', model: 'gemini-2.0-flash',
    inputPer1M: 0.075, outputPer1M: 0.30 },
  { provider: 'gemini', model: 'gemini-1.5-pro',
    inputPer1M: 1.25, outputPer1M: 5 },
];
```

#### 预算系统设计
```typescript
interface CostConfig {
  displayMode: 'always' | 'summary' | 'never';
  currency: string;
  budgets?: {
    perMessage?: number;         // 单条消息最大成本
    perSession?: number;         // 会话最大成本
    daily?: number;              // 每日预算
    monthly?: number;            // 每月预算
  };
  alerts?: {
    threshold: number;           // 告警阈值（如 0.8 = 80%）
    action: 'warn' | 'confirm' | 'block';
  };
}
```

---

## 二、数据流程图对比

### 2.1 OpenCode 的完整数据流

```
┌─────────────────────────────────────────────────────────────────────┐
│ Step 1: LLM API 返回                                                 │
│ ┌─────────────────────────────────────────────────────────────┐    │
│ │ Anthropic/OpenAI/Gemini API Response                         │    │
│ │ {                                                             │    │
│ │   id: "msg_123",                                              │    │
│ │   content: [...],                                             │    │
│ │   usage: {                                                    │    │
│ │     input_tokens: 1234,                                       │    │
│ │     output_tokens: 567,                                       │    │
│ │     cache_read_input_tokens: 100,  // Anthropic 特有         │    │
│ │     cache_creation_input_tokens: 20, // Anthropic 特有       │    │
│ │   }                                                            │    │
│ │ }                                                              │    │
│ └─────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
                                ↓
┌─────────────────────────────────────────────────────────────────────┐
│ Step 2: 提取和标准化 Token                                           │
│ ┌─────────────────────────────────────────────────────────────┐    │
│ │ Session.getUsage()                                            │    │
│ │                                                               │    │
│ │ 1. 处理 Prompt Cache                                          │    │
│ │    - Anthropic: 缓存 token 已从 input_tokens 中扣除          │    │
│ │    - 其他: 需要手动扣除                                       │    │
│ │                                                               │    │
│ │ 2. 标准化输出                                                 │    │
│ │    tokens = {                                                 │    │
│ │      input: 1234,                                             │    │
│ │      output: 567,                                             │    │
│ │      reasoning: 50,        // O1 模型                         │    │
│ │      cache: {                                                 │    │
│ │        read: 100,                                             │    │
│ │        write: 20,                                             │    │
│ │      }                                                         │    │
│ │    }                                                           │    │
│ └─────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
                                ↓
┌─────────────────────────────────────────────────────────────────────┐
│ Step 3: 选择定价表                                                   │
│ ┌─────────────────────────────────────────────────────────────┐    │
│ │ if (input + cache.read > 200,000) {                           │    │
│ │   // 使用 200k+ 特殊定价（更贵）                              │    │
│ │   costInfo = model.cost.experimentalOver200K                  │    │
│ │ } else {                                                      │    │
│ │   // 标准定价                                                 │    │
│ │   costInfo = model.cost                                       │    │
│ │ }                                                              │    │
│ │                                                               │    │
│ │ costInfo = {                                                  │    │
│ │   input: 3.00,      // USD per 1M tokens                     │    │
│ │   output: 15.00,                                              │    │
│ │   cache.read: 0.30,                                           │    │
│ │   cache.write: 3.75                                           │    │
│ │ }                                                              │    │
│ └─────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
                                ↓
┌─────────────────────────────────────────────────────────────────────┐
│ Step 4: 计算成本（使用 Decimal.js 确保精度）                        │
│ ┌─────────────────────────────────────────────────────────────┐    │
│ │ cost = new Decimal(0)                                         │    │
│ │   .add((tokens.input / 1,000,000) * costInfo.input)          │    │
│ │   .add((tokens.output / 1,000,000) * costInfo.output)        │    │
│ │   .add((tokens.cache.read / 1,000,000) * costInfo.cache.read)│    │
│ │   .add((tokens.cache.write / 1,000,000) * costInfo.cache.write)  │
│ │   .add((tokens.reasoning / 1,000,000) * costInfo.output)     │    │
│ │   .toNumber()                                                 │    │
│ │                                                               │    │
│ │ 例如：                                                        │    │
│ │   input: (1234 / 1,000,000) * 3.00 = $0.003702               │    │
│ │   output: (567 / 1,000,000) * 15.00 = $0.008505              │    │
│ │   cache.read: (100 / 1,000,000) * 0.30 = $0.000030           │    │
│ │   cache.write: (20 / 1,000,000) * 3.75 = $0.000075           │    │
│ │   ────────────────────────────────────────────────────       │    │
│ │   总计: $0.012312                                             │    │
│ └─────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
                                ↓
┌─────────────────────────────────────────────────────────────────────┐
│ Step 5: 存储到数据库                                                 │
│ ┌─────────────────────────────────────────────────────────────┐    │
│ │ SessionProcessor.process()                                    │    │
│ │                                                               │    │
│ │ 1. 累加到消息                                                 │    │
│ │    assistantMessage.cost += 0.012312                         │    │
│ │    assistantMessage.tokens = { input: 1234, ... }            │    │
│ │                                                               │    │
│ │ 2. 创建 StepFinishPart（每个步骤记录）                        │    │
│ │    new StepFinishPart({                                       │    │
│ │      type: "step-finish",                                     │    │
│ │      cost: 0.012312,                                          │    │
│ │      tokens: { input: 1234, ... }                            │    │
│ │    })                                                          │    │
│ │                                                               │    │
│ │ 3. 持久化到存储                                               │    │
│ │    - Message 表记录消息级别成本                               │    │
│ │    - Parts 表记录步骤级别成本                                 │    │
│ └─────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
                                ↓
┌─────────────────────────────────────────────────────────────────────┐
│ Step 6: UI 实时显示                                                  │
│ ┌─────────────────────────────────────────────────────────────┐    │
│ │ Header Component (header.tsx)                                 │    │
│ │                                                               │    │
│ │ 会话总成本：                                                  │    │
│ │   cost = messages                                             │    │
│ │     .filter(m => m.role === "assistant")                     │    │
│ │     .reduce((sum, m) => sum + m.cost, 0)                     │    │
│ │                                                               │    │
│ │   显示: $0.45                                                 │    │
│ │                                                               │    │
│ │ 最后一条消息的上下文使用率：                                  │    │
│ │   totalTokens = input + output + reasoning +                 │    │
│ │                 cache.read + cache.write                      │    │
│ │   percentage = (totalTokens / model.limit.context) * 100     │    │
│ │                                                               │    │
│ │   显示: 1,234 tokens (45%)                                    │    │
│ └─────────────────────────────────────────────────────────────┘    │
│                                                                      │
│ ┌────────────────────────────────────────────────┐                  │
│ │  OpenCode                          $0.45  ⚡    │                  │
│ │  ─────────────────────────────────────────     │                  │
│ │  Session: my-session                           │                  │
│ │  Context: 1,234 tokens (45%)                   │                  │
│ └────────────────────────────────────────────────┘                  │
└─────────────────────────────────────────────────────────────────────┘
```

---

### 2.2 GenCode 当前数据流（仅 Token）

```
┌─────────────────────────────────────────────────────────────────────┐
│ Step 1: LLM API 返回                                                 │
│ ┌─────────────────────────────────────────────────────────────┐    │
│ │ Anthropic/OpenAI/Gemini API Response                         │    │
│ │ {                                                             │    │
│ │   // ... 内容 ...                                             │    │
│ │   usage: {                                                    │    │
│ │     input_tokens: 1234,                                       │    │
│ │     output_tokens: 567,                                       │    │
│ │   }                                                            │    │
│ │ }                                                              │    │
│ └─────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
                                ↓
┌─────────────────────────────────────────────────────────────────────┐
│ Step 2: Provider 标准化                                              │
│ ┌─────────────────────────────────────────────────────────────┐    │
│ │ AnthropicProvider.complete()                                  │    │
│ │ OpenAIProvider.complete()                                     │    │
│ │ GeminiProvider.complete()                                     │    │
│ │                                                               │    │
│ │ return {                                                      │    │
│ │   content: [...],                                             │    │
│ │   stopReason: "end_turn",                                     │    │
│ │   usage: {                                                    │    │
│ │     inputTokens: 1234,                                        │    │
│ │     outputTokens: 567,                                        │    │
│ │   }                                                            │    │
│ │ }                                                              │    │
│ └─────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
                                ↓
┌─────────────────────────────────────────────────────────────────────┐
│ Step 3: Session 存储（可选）                                         │
│ ┌─────────────────────────────────────────────────────────────┐    │
│ │ SessionManager.addMessage()                                   │    │
│ │                                                               │    │
│ │ session.tokenUsage = {                                        │    │
│ │   input: session.tokenUsage.input + 1234,                    │    │
│ │   output: session.tokenUsage.output + 567,                   │    │
│ │ }                                                              │    │
│ │                                                               │    │
│ │ 存储到: ~/.gen/sessions/{id}.json                         │    │
│ └─────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
                                ↓
┌─────────────────────────────────────────────────────────────────────┐
│ Step 4: 基础显示（仅在 example 中）                                 │
│ ┌─────────────────────────────────────────────────────────────┐    │
│ │ examples/basic.ts                                             │    │
│ │                                                               │    │
│ │ if (response.usage) {                                         │    │
│ │   console.log(                                                │    │
│ │     `Usage: ${response.usage.inputTokens} input, ` +         │    │
│ │     `${response.usage.outputTokens} output`                  │    │
│ │   );                                                          │    │
│ │ }                                                              │    │
│ │                                                               │    │
│ │ 输出: Usage: 1234 input, 567 output                           │    │
│ └─────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘

❌ 缺少：成本计算
❌ 缺少：定价表
❌ 缺少：CLI 中的成本显示
❌ 缺少：会话级别的成本聚合
❌ 缺少：预算系统
```

---

### 2.3 GenCode Proposal 设计的数据流

```
┌─────────────────────────────────────────────────────────────────────┐
│ Step 1: LLM API 返回（同当前）                                       │
│ [同当前流程]                                                         │
└─────────────────────────────────────────────────────────────────────┘
                                ↓
┌─────────────────────────────────────────────────────────────────────┐
│ Step 2: Provider 标准化并计算成本（新增）                            │
│ ┌─────────────────────────────────────────────────────────────┐    │
│ │ import { calculateCost } from '../pricing/calculator';       │    │
│ │                                                               │    │
│ │ AnthropicProvider.complete() {                                │    │
│ │   const response = await this.client.messages.create(...);   │    │
│ │                                                               │    │
│ │   const tokens = {                                            │    │
│ │     inputTokens: response.usage.input_tokens,                │    │
│ │     outputTokens: response.usage.output_tokens,              │    │
│ │   };                                                          │    │
│ │                                                               │    │
│ │   // 新增：计算成本                                           │    │
│ │   const cost = calculateCost({                                │    │
│ │     provider: 'anthropic',                                    │    │
│ │     model: this.model,                                        │    │
│ │     tokens,                                                   │    │
│ │   });                                                         │    │
│ │                                                               │    │
│ │   return {                                                    │    │
│ │     content: [...],                                           │    │
│ │     stopReason: "end_turn",                                   │    │
│ │     usage: tokens,                                            │    │
│ │     cost,  // 新增字段                                        │    │
│ │   };                                                          │    │
│ │ }                                                              │    │
│ └─────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
                                ↓
┌─────────────────────────────────────────────────────────────────────┐
│ Step 3: CostTracker 记录（新增）                                    │
│ ┌─────────────────────────────────────────────────────────────┐    │
│ │ Agent.run() {                                                 │    │
│ │   const response = await this.provider.complete(...);        │    │
│ │                                                               │    │
│ │   // 新增：记录成本                                           │    │
│ │   if (response.usage && response.cost) {                     │    │
│ │     costTracker.recordUsage(                                 │    │
│ │       this.sessionId,                                         │    │
│ │       this.model,                                             │    │
│ │       this.provider.name,                                     │    │
│ │       response.usage,                                         │    │
│ │       response.cost                                           │    │
│ │     );                                                        │    │
│ │                                                               │    │
│ │     // 检查预算                                               │    │
│ │     costTracker.checkBudget(this.sessionId);                 │    │
│ │   }                                                            │    │
│ │ }                                                              │    │
│ └─────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
                                ↓
┌─────────────────────────────────────────────────────────────────────┐
│ Step 4: Session 存储扩展                                             │
│ ┌─────────────────────────────────────────────────────────────┐    │
│ │ SessionManager.addMessage()                                   │    │
│ │                                                               │    │
│ │ session.tokenUsage = {                                        │    │
│ │   input: ...,                                                 │    │
│ │   output: ...,                                                │    │
│ │ };                                                            │    │
│ │                                                               │    │
│ │ // 新增：成本跟踪                                             │    │
│ │ session.totalCost = (session.totalCost || 0) +               │    │
│ │                      response.cost.totalCost;                 │    │
│ │                                                               │    │
│ │ 存储到: ~/.gen/sessions/{id}.json                         │    │
│ │   {                                                           │    │
│ │     "id": "...",                                              │    │
│ │     "tokenUsage": { "input": 1234, "output": 567 },          │    │
│ │     "totalCost": 0.0234,  // 新增                            │    │
│ │     "messages": [...]                                         │    │
│ │   }                                                            │    │
│ └─────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
                                ↓
┌─────────────────────────────────────────────────────────────────────┐
│ Step 5: CLI 显示（新增）                                             │
│ ┌─────────────────────────────────────────────────────────────┐    │
│ │ App.tsx - 在每次响应后显示                                    │    │
│ │                                                               │    │
│ │ {event.type === 'done' && event.usage && event.cost && (     │    │
│ │   <Box marginTop={1}>                                         │    │
│ │     <Text dimColor>                                           │    │
│ │       Tokens: {formatTokens(event.usage.inputTokens)} in /   │    │
│ │       {formatTokens(event.usage.outputTokens)} out           │    │
│ │       (~{formatCost(event.cost.totalCost)})                  │    │
│ │     </Text>                                                   │    │
│ │   </Box>                                                      │    │
│ │ )}                                                            │    │
│ │                                                               │    │
│ │ 显示效果：                                                    │    │
│ │   Tokens: 1.2K in / 567 out (~$0.02)                         │    │
│ │   Session total: $0.45                                        │    │
│ └─────────────────────────────────────────────────────────────┘    │
│                                                                      │
│ ┌────────────────────────────────────────────────┐                  │
│ │ > How can I help you?                          │                  │
│ │                                                │                  │
│ │ [AI 响应内容...]                               │                  │
│ │                                                │                  │
│ │ Tokens: 1.2K in / 567 out (~$0.02)            │                  │
│ │ Session total: $0.45                           │                  │
│ └────────────────────────────────────────────────┘                  │
└─────────────────────────────────────────────────────────────────────┘
                                ↓
┌─────────────────────────────────────────────────────────────────────┐
│ Step 6: 成本报告命令（新增）                                         │
│ ┌─────────────────────────────────────────────────────────────┐    │
│ │ /costs - 显示详细报告                                         │    │
│ │                                                               │    │
│ │ Cost Report - Current Session:                               │    │
│ │ ┌──────────────────────────────────────────────────────┐     │    │
│ │ │ Provider   Model          Messages  Tokens    Cost   │     │    │
│ │ ├──────────────────────────────────────────────────────┤     │    │
│ │ │ anthropic  claude-sonnet   12       45.2K     $0.23  │     │    │
│ │ │ anthropic  claude-haiku    3        8.1K      $0.01  │     │    │
│ │ ├──────────────────────────────────────────────────────┤     │    │
│ │ │ Total                      15       53.3K     $0.24  │     │    │
│ │ └──────────────────────────────────────────────────────┘     │    │
│ │                                                               │    │
│ │ Daily: $1.45 / $10.00 budget (14.5%)                         │    │
│ │ Monthly: $23.67 / $100.00 budget (23.7%)                     │    │
│ └─────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 三、具体例子对比

### 3.1 场景：用户发送一个简单问题

**用户输入**：
```
"请帮我写一个 TypeScript 函数，用于计算斐波那契数列"
```

#### OpenCode 的处理流程

```
1. 用户输入 → Agent
   输入: "请帮我写一个..." (约 20 tokens)

2. Agent → Anthropic API
   Request: {
     model: "claude-3-5-sonnet-20241022",
     messages: [
       { role: "system", content: "..." },  // 系统提示 ~500 tokens
       { role: "user", content: "请帮我写..." }  // 20 tokens
     ]
   }

3. Anthropic API → Response
   Response: {
     content: [{
       type: "text",
       text: "当然可以！下面是一个计算斐波那契数列的 TypeScript 函数：\n\n```typescript\nfunction fibonacci(n: number): number {\n  if (n <= 1) return n;\n  return fibonacci(n - 1) + fibonacci(n - 2);\n}\n```"
     }],
     usage: {
       input_tokens: 520,          // 系统 + 用户输入
       output_tokens: 85,          // 生成的代码和说明
       cache_creation_input_tokens: 500,  // 系统提示写入缓存
       cache_read_input_tokens: 0         // 首次调用，无缓存
     }
   }

4. Session.getUsage() 计算成本
   tokens = {
     input: 520,
     output: 85,
     cache: {
       write: 500,  // 首次缓存写入
       read: 0
     }
   }

   定价（claude-3-5-sonnet）：
   - input: $3.00 / 1M tokens
   - output: $15.00 / 1M tokens
   - cache.write: $3.75 / 1M tokens
   - cache.read: $0.30 / 1M tokens

   cost = (520 / 1,000,000) * 3.00        = $0.001560
        + (85 / 1,000,000) * 15.00        = $0.001275
        + (500 / 1,000,000) * 3.75        = $0.001875
        + (0 / 1,000,000) * 0.30          = $0.000000
        ─────────────────────────────────────────────
        总计                               = $0.00471

5. 存储
   StepFinishPart: { cost: 0.00471, tokens: {...} }
   AssistantMessage.cost = 0.00471

6. UI 显示
   ┌──────────────────────────────────────────────┐
   │ OpenCode                       $0.00 ⚡       │
   │ ───────────────────────────────────────      │
   │ > 请帮我写一个 TypeScript 函数...            │
   │                                              │
   │ 当然可以！下面是一个计算斐波那契数列的...    │
   │ [代码块显示]                                 │
   │                                              │
   │ Session: $0.00 | Context: 605 tokens (3%)   │
   └──────────────────────────────────────────────┘
```

**第二次调用（Prompt Cache 生效）**：
```
用户: "能否优化这个函数，使用动态规划？"

Anthropic API Response:
  usage: {
    input_tokens: 20,                    // 新用户输入
    output_tokens: 120,                  // 新输出
    cache_creation_input_tokens: 0,      // 无新缓存
    cache_read_input_tokens: 585         // 缓存命中！（系统提示 + 之前对话）
  }

成本计算：
  cost = (20 / 1,000,000) * 3.00         = $0.000060
       + (120 / 1,000,000) * 15.00       = $0.001800
       + (0 / 1,000,000) * 3.75          = $0.000000
       + (585 / 1,000,000) * 0.30        = $0.000176  ← 缓存命中便宜 10 倍！
       ─────────────────────────────────────────────
       总计                                = $0.002036

会话总成本: $0.00471 + $0.002036 = $0.006746

UI 显示:
  Session: $0.01 | Context: 725 tokens (4%)
```

---

#### GenCode 当前的处理（无成本计算）

```
1. 用户输入 → Agent
   [同上]

2. Agent → Anthropic API
   [同上]

3. Anthropic API → Response
   [同上，包含 usage 数据]

4. Provider 标准化
   return {
     content: [...],
     stopReason: "end_turn",
     usage: {
       inputTokens: 520,
       outputTokens: 85,
     }
   }

   ❌ 未计算成本
   ❌ 未跟踪缓存 token

5. Session 存储（可选）
   session.tokenUsage = {
     input: 520,
     output: 85,
   }

   ❌ 无成本信息

6. CLI 显示
   ┌──────────────────────────────────────────────┐
   │ > 请帮我写一个 TypeScript 函数...            │
   │                                              │
   │ 当然可以！下面是一个计算斐波那契数列的...    │
   │ [代码块显示]                                 │
   │                                              │
   │ ❌ 无成本显示                                │
   │ ❌ 无 token 显示                             │
   └──────────────────────────────────────────────┘

   仅在 examples/basic.ts 中有基础显示：
   > Usage: 520 input, 85 output
```

---

#### GenCode Proposal 的处理（完整成本跟踪）

```
1-3. [同当前流程]

4. Provider 计算成本（新增）
   import { calculateCost } from '../pricing/calculator';

   const cost = calculateCost({
     provider: 'anthropic',
     model: 'claude-sonnet-4',
     tokens: {
       inputTokens: 520,
       outputTokens: 85,
     }
   });

   // 定价表查询
   const pricing = {
     provider: 'anthropic',
     model: 'claude-sonnet-4',
     inputPer1M: 3.00,
     outputPer1M: 15.00,
   };

   // 成本计算
   cost = {
     inputCost: (520 / 1_000_000) * 3.00 = 0.00156,
     outputCost: (85 / 1_000_000) * 15.00 = 0.001275,
     totalCost: 0.002835,
     currency: 'USD'
   };

   return {
     content: [...],
     usage: { inputTokens: 520, outputTokens: 85 },
     cost: cost,  // ✅ 新增
   };

5. CostTracker 记录
   costTracker.recordUsage(
     sessionId: "session-123",
     model: "claude-sonnet-4",
     provider: "anthropic",
     tokens: { inputTokens: 520, outputTokens: 85 },
     cost: { totalCost: 0.002835, ... }
   );

   // 检查预算
   if (config.budgets?.perMessage && cost.totalCost > config.budgets.perMessage) {
     ui.warn("Message cost exceeds budget!");
   }

6. Session 存储
   session.tokenUsage = { input: 520, output: 85 };
   session.totalCost = 0.002835;  // ✅ 新增

7. CLI 显示
   ┌──────────────────────────────────────────────┐
   │ > 请帮我写一个 TypeScript 函数...            │
   │                                              │
   │ 当然可以！下面是一个计算斐波那契数列的...    │
   │ [代码块显示]                                 │
   │                                              │
   │ ✅ Tokens: 520 in / 85 out (~$0.00)         │
   │ ✅ Session total: $0.00                     │
   └──────────────────────────────────────────────┘

8. 成本报告（/costs 命令）
   > /costs

   Cost Report - Current Session:
   ┌──────────────────────────────────────────────┐
   │ Provider   Model          Messages  Cost     │
   ├──────────────────────────────────────────────┤
   │ anthropic  claude-sonnet  1         $0.00    │
   ├──────────────────────────────────────────────┤
   │ Total                     1         $0.00    │
   └──────────────────────────────────────────────┘
```

---

### 3.2 成本对比场景：多轮对话

**场景**：一个 5 轮对话的编码会话

| 轮次 | 输入 Token | 输出 Token | OpenCode 成本 | GenCode Proposal 成本 | 差异原因 |
|-----|-----------|-----------|--------------|---------------------|---------|
| 1 | 520 | 85 | $0.00471 | $0.00284 | OpenCode 包含缓存写入成本 |
| 2 | 20 + 585(cache) | 120 | $0.00204 | $0.00184 | OpenCode 缓存命中折扣 |
| 3 | 30 + 705(cache) | 200 | $0.00331 | $0.00309 | 同上 |
| 4 | 50 + 935(cache) | 150 | $0.00271 | $0.00255 | 同上 |
| 5 | 25 + 1085(cache) | 180 | $0.00303 | $0.00278 | 同上 |
| **总计** | **2955** | **735** | **$0.0158** | **$0.01310** | **Proposal 少 17%** |

**差异分析**：
- OpenCode：完整跟踪缓存 token，首次写入有额外成本，后续命中有折扣
- GenCode Proposal：简化版本，不跟踪缓存（初期实现），所以成本估算偏低

**如果 GenCode 实现缓存跟踪**（Phase 2）：
- 成本将与 OpenCode 一致
- 需要从 Anthropic API 提取 `cache_creation_input_tokens` 和 `cache_read_input_tokens`

---

## 四、关键技术对比总结

| 特性 | OpenCode | GenCode 当前 | GenCode Proposal |
|-----|----------|-------------|----------------|
| **Token 类型** | input/output/reasoning/cache.read/cache.write | input/output | 计划支持全部 |
| **精度保证** | Decimal.js | 标准 number | 标准 number |
| **定价表** | 内嵌 Model 对象 | 无 | 独立 ProviderPricing[] |
| **成本存储** | 每 Step + 每 Message | 无 | 每 Message |
| **UI 显示** | Header 实时显示 | 无 | 计划显示 |
| **预算系统** | 无（应用层） | 无 | 设计中 |
| **特殊定价** | 200k+ 分阶段 | 无 | 无（可扩展） |
| **缓存优化** | 完整支持 | 不支持 | Phase 2 |
| **实现复杂度** | 高 | - | 中 |

---

## 五、实现建议

基于对比分析，GenCode 应该分阶段实现：

### Phase 1: 基础成本跟踪（最小可行产品）
✅ 创建定价表（`src/pricing/models.ts`）
✅ 实现成本计算器（`src/pricing/calculator.ts`）
✅ Provider 返回成本（修改 `CompletionResponse`）
✅ CLI 显示成本（修改 `App.tsx`）

**工作量**：低
**价值**：高
**参考**：Proposal 0025 Phase 1

### Phase 2: 高级 Token 支持
- 扩展 `TokenUsage` 支持 reasoning, cacheRead, cacheWrite
- 更新所有 Provider 提取这些数据
- 更新定价计算逻辑

**工作量**：中
**价值**：中（Anthropic 用户受益大）
**参考**：OpenCode 的 getUsage() 实现

### Phase 3: 预算和告警
- 实现 `CostConfig` 和 `CostTracker`
- 会话/每日/每月预算
- 超预算告警

**工作量**：中
**价值**：高（企业用户必需）
**参考**：Proposal 0025 Budget System

### Phase 4: 高级特性
- 成本报告（/costs 命令）
- 多 Provider 成本对比
- 成本优化建议
- Decimal.js 精度保证（如需要）

**工作量**：高
**价值**：中（锦上添花）

---

## 六、总结

**OpenCode**：
- ✅ 生产级实现，功能完整
- ✅ 支持所有 token 类型和缓存优化
- ✅ 财务精度保证（Decimal.js）
- ⚠️ 实现复杂度高

**GenCode 当前**：
- ✅ Token 基础设施已完备
- ❌ 缺少成本计算层
- ❌ 缺少 UI 显示

**GenCode Proposal**：
- ✅ 设计合理，分阶段实现
- ✅ 覆盖核心用例
- ✅ 可扩展到高级特性
- ✅ 实现成本适中

**推荐路径**：
1. 先实现 Phase 1（基础成本显示）→ 快速交付价值
2. 根据用户反馈决定是否实现 Phase 2-4
3. 参考 OpenCode 的精确实现，但保持 GenCode 的简洁性
