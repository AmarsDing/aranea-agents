# Proposal: Chat 非 ReAct 模式紧凑时间线（方案 B · Trae 风格）

> **日期**：2026-06-10 | **版本**：v1.0 | **状态**：M1 实施中
> **关联需求**：[59-chat-ui-optimization.md](../development/59-chat-ui-optimization.md)（合并自原 M59 + M69）
> **前置报告**：[2026-06-09-proposal-chat-execution-card-folding.md](./2026-06-09-proposal-chat-execution-card-folding.md)
> **竞品参考**：Trae IDE（实时跟随 + 步骤状态 + 内联动作）

---

## 摘要

替换 `TurnBlock.vue` 中 non-ReAct 分支的 `interleavedRounds` 1:1 配对渲染（存在段落切分脆弱、配对无因果、工具均分失真三大缺陷），改为**紧凑时间线**：按"思考 → 工具 → 回复"三层结构顺序渲染，借鉴 Trae IDE 的步骤状态徽章与内联动作交互。最小侵入（M1 仅新增 2 文件 + 改 1 文件），不破坏 ReAct 模式，不引入新组件到 `features/`，遵守项目红线与 UX 主题规范。

---

## 一、问题回顾

### 1.1 现状

`TurnBlock.vue:395-436` 的 `interleavedRounds` 计算属性：

```typescript
const reasoningParts = assistantReasoning.value.split(/\n{2,}/).map(s => s.trim()).filter(Boolean);
const replyParts      = assistantBody.value.split(/\n{2,}/).map(s => s.trim()).filter(Boolean);
const maxRounds = Math.max(reasoningParts.length, replyParts.length);
const toolsPerRound = Math.ceil(allToolEvents.length / maxRounds);  // 工具均分
for (let i = 0; i < maxRounds; i++) {
  rounds.push({ thinking: reasoningParts[i], reply: replyParts[i], tools: allToolEvents.slice(i*toolsPerRound, (i+1)*toolsPerRound) });
}
```

### 1.2 三大缺陷

| # | 缺陷 | 影响 |
|---|------|------|
| P-1 | **段落切分脆弱**：`split(/\n{2,}/)` 把 Markdown 双换行当语义边界，LLM 输出的列表/表格恰好换行被误切 | 思考/回复被割裂到错误位置 |
| P-2 | **配对无因果**：`reasoning[i] ↔ reply[i]` 是位置配对，非实际事件配对 | 用户看到"思考 1 → 回复 1"，但真实事件流是思考 1 → 工具 A → 思考 2 → 工具 B → 回复 |
| P-3 | **工具均分失真**：`Math.ceil(tools/maxRounds)` 用位运算把工具按 round 数均分，丢失"哪个工具跟在哪个思考之后"的因果 | 工具被错放到不相关的思考 round 下 |

---

## 二、方案 B 设计（Trae 风格紧凑时间线）

### 2.1 核心隐喻

放弃"配对回合"模型，改为**事件流**：

```
thinking  ←─ 整段（不切分）
tool      ←─ 1 个
tool      ←─ 1 个
reply     ←─ 整段
```

每层是一个独立的 UI 节点（`TimelineNodeRow`），状态由消息自身字段驱动。

### 2.2 视觉规范

每个节点一行（Trae 风格 `▸` 折叠 + 状态徽章 + 耗时）：

```
┌─ 💬 分析最近的销售趋势                          10:42 · 3.8s ─┐
│ 💭 思考 · 0.4s                                          ⧉ ▸ │
│    用户问的是趋势分析，需要先看月度数据，再做同比        │
│ 🔧 get_sales · 1.8s   ✓                              ⧉ ▸ │
│    24 条记录                                              │
│ 💬 回复 · 1.2s                                         ⧉ ↻ │
│    近 12 个月销售额呈现逐月增长趋势…                      │
└─────────────────────────────────────────────────────────┘
```

- **横向铺开**：每节点单行（图标 + 动作 + 标签 + 状态 + 耗时 + 动作按钮）
- **节点间**：4px 间距，无独立卡片
- **摘要内容**：inline 在节点下方，≤2 行
- **状态徽章**：`✓ ✕ ⏳ ⏸` 走现有 Quasar `text-positive/negative` 等
- **内联动作**（M2 引入）：hover 显 `⧉ 复制 · ↻ 重跑 · ▸ 展开`

### 2.3 数据契约

新增类型（`features/chat/compactTimeline.ts`）：

```typescript
export type CompactNode =
  | { kind: 'thinking'; text: string; messageId: string }
  | { kind: 'tool'; event: ToolUseEvent }
  | { kind: 'reply'; renderedHtml: string; messageId: string; streaming: boolean };

export function buildCompactNodes(args: {
  reasoning: string;
  bodyMarkdown: string;
  toolEvents: ToolUseEvent[];
  messageId: string;
  isStreaming: boolean;
  renderMarkdown: (text: string, key: string, streaming: boolean) => string;
}): CompactNode[];
```

**关键简化（vs 第一版）**：
- 去掉 `split(/\n{2,}/)` 段落切分
- 去掉 `Math.ceil` 工具均分
- 节点顺序：thinking → tools → reply（**如实反映实际事件流**）
- 单一 messageId 关联所有节点，stable key 避免流式闪烁

### 2.4 ReAct 模式不变

`reactSteps` / `reactReplyRounds` 仍由 `parseReactPlannerContent` 解析（基于 `/*PLANNING*/` 显式标签），是真实的结构信号，不在改造范围。

---

## 三、评审发现（7 项）

### 3.1 R-1：数据契约限制

`message.reasoning_markdown` 是累积字符串，无时间戳分段。

- **影响**：M1 不能真正按"思考 1 → 工具 A → 思考 2"分桶
- **决策**：M1 接受"整段 thinking"语义，**不做时间分桶**；如需真正的多段思考，依赖后端推送 `reasoning_delta` 增量事件（**不属本任务**）

### 3.2 R-2：ReAct 模式段落切分仍在

`reactReplyRounds` 仍用 `split(/\n{2,}/)`（`TurnBlock.vue:344-348`）。

- **影响**：M1 仅替换 non-ReAct 模板，ReAct 模式不变
- **决策**：保持现状，未来如 ReAct 也需改造，单独提 issue

### 3.3 R-3：流式 key 稳定性

当前 `interleavedRounds` 用数组 index 作 key，流式到达时整个列表重渲染。

- **影响**：用户看到内容闪烁
- **决策**：M1 用 `${messageId}-${kind}` 稳定 key（如 `m123-thinking`、`m123-tool-1`、`m123-reply`）

### 3.4 R-4：i18n 键复用

`chat.turn.block.thinkingLabel` / `thinkingLabelN` / `resultLabel` / `resultLabelN` 沿用。

### 3.5 R-5：展示组件红线

`CompactTimeline.vue` 仅接收 `props`（reasoning/body/tools/messageId/streaming）+ 触发 `emit`（不调 API/Store），符合红线 #1 #2。

### 3.6 R-6：路径规范

| 文件类型 | 路径 |
|----------|------|
| 纯函数 | `web/src/features/chat/compactTimeline.ts` |
| 展示组件 | `web/src/components/chat/CompactTimeline.vue` |
| 测试 | `web/src/features/chat/__tests__/compactTimeline.spec.ts` |

### 3.7 R-7：CSS 落点

新增样式加入 `web/src/css/theme/_chat-message-panel.sass`（与 TurnBlock 现有样式同文件），保持与项目其他时间线相关样式一致。

---

## 四、最终方案：实施路线

### 4.1 M1：数据契约 + 基础渲染（本次实施）

**目标**：消除 1:1 配对 + 段落切分，不引入新交互。

| 文件 | 改动 |
|------|------|
| `web/src/features/chat/compactTimeline.ts` | 新增纯函数 `buildCompactNodes` |
| `web/src/components/chat/CompactTimeline.vue` | 新增展示组件 |
| `web/src/features/chat/__tests__/compactTimeline.spec.ts` | 5 个测试 case |
| `web/src/components/chat/TurnBlock.vue` | non-ReAct 分支模板替换；删除 `interleavedRounds` 计算属性 |
| `web/src/css/theme/_chat-message-panel.sass` | 新增 `.compact-timeline*` 系列样式 |

**验收**：
- 思考/回复**不再被段落切分**
- 工具**不再被均分**到配对 round
- 节点渲染顺序：thinking → tools → reply
- 流式到达时节点不闪烁（稳定 key）
- 现有 ReAct 模式行为完全不变
- `pnpm lint && pnpm test && pnpm build` 通过

### 4.2 M2：状态徽章 + 内联动作（后续）

新增：
- `✓ ✕ ⏳ ⏸` 状态徽章（按 `ToolUseEvent.status` 渲染）
- hover 显 `⧉ 复制 · ↻ 重跑 · ▸ 展开` 动作
- 工具节点点击展开详情（重用 `ChatExecutionCard` 的详情逻辑）

### 4.3 M3：实时跟随 + 早期折叠（后续）

- 当前活动节点脉动圆点 + 强调带
- 超过 4 个完成节点时自动折叠早期（参考 Cursor）

---

## 五、改动清单（M1）

| 文件 | 类型 | 改动 |
|------|------|------|
| `features/chat/compactTimeline.ts` | 新增 | `buildCompactNodes()` 纯函数 + `CompactNode` 类型 |
| `components/chat/CompactTimeline.vue` | 新增 | 展示组件，仅 props/emits |
| `features/chat/__tests__/compactTimeline.spec.ts` | 新增 | 5 个 case：空、单 reply、thinking+reply、单 tool、多 tool |
| `components/chat/TurnBlock.vue` | 修改 | 删除 `interleavedRounds`（52 行）；模板 non-ReAct 分支替换为 `<CompactTimeline>` |
| `css/theme/_chat-message-panel.sass` | 修改 | 新增 `.compact-timeline*` 样式（~60 行） |

**新增组件数**：1
**新增纯函数文件**：1
**新增测试**：1
**总新增行数**：~280
**修改行数**：~60（TurnBlock 模板替换 + SASS 追加）

---

## 六、边界场景

| # | 场景 | 处理 |
|---|------|------|
| 1 | 仅有 thinking 无 reply | 渲染 [thinking]，reply 节点不渲染 |
| 2 | 仅有 reply 无 thinking | 渲染 [reply]，thinking 节点不渲染 |
| 3 | 有 thinking + 1 tool + reply | 渲染 [thinking, tool, reply] |
| 4 | 仅有 tools 无 thinking/reply | 渲染 [tool, tool, ...]（与 ToolStrip fallback 区分：本组件走 ChatExecutionCard 卡片，ToolStrip 走摘要条） |
| 5 | ReAct 模式（`reactSteps.length > 0`） | 完全不进入新组件，保留旧渲染 |
| 6 | 流式中（`isAssistantStreaming`） | reply 节点 `streaming: true` 触发光标 + 稳定 key |
| 7 | 消息失败（`status: 'failed'`） | reply 节点标记 failed，整段保留不隐藏 |

---

## 七、验证

### 7.1 单元测试

```
cd web && pnpm test -- compactTimeline
```

5 个 case：
1. 空（reasoning/body 都为空）→ `[]`
2. 仅 reply → `[reply]`
3. thinking + reply，无 tool → `[thinking, reply]`
4. thinking + reply + 1 tool → `[thinking, tool, reply]`
5. thinking + reply + 3 tools → `[thinking, tool, tool, tool, reply]`

### 7.2 类型检查与 Lint

```
pnpm lint
```

### 7.3 构建

```
pnpm build
```

### 7.4 手动验证

- 桌面 1280px：横向铺开单行模式
- 昼夜切换：状态徽章颜色对比度 ≥4.5:1
- 流式：节点稳定 key，光标正常显示
- ReAct 模式：与改造前完全一致
- 失败消息：reply 节点标记失败，不自动折叠

---

## 八、风险评估

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| 段落切分移除后，长 thinking 占据过多垂直空间 | 中 | 低 | M2 引入单行摘要 + ▸ 展开 |
| 工具渲染从 `q-ml-md` 缩进丢失 | 低 | 低 | 视觉上无大差异，工具卡片本身有足够视觉重量 |
| 现有用户在 non-ReAct 模式下已习惯配对视图 | 低 | 低 | M1 整体形态仍按"思考→工具→回复"展示，用户可接受 |
| ReAct 模式段落切分仍存在 | — | — | 不在本任务范围（见 R-2） |

---

## 附录 A：与原方案对比

| 维度 | 原 `interleavedRounds` | 新 `CompactTimeline` |
|------|----------------------|---------------------|
| 段落切分 | `split(/\n{2,}/)` | 无 |
| 配对机制 | `reasoning[i] ↔ reply[i]` 位置配对 | 无配对（顺序独立） |
| 工具分配 | `Math.ceil(tools/rounds)` 均分 | 顺序保持原位 |
| 节点 key | `round-${rIdx}` 数组 index | `${messageId}-${kind}` 稳定 key |
| ReAct 模式 | 不受影响 | 不受影响 |
| 流式闪烁 | 每次流式到达整列表重渲染 | append-only，单节点更新 |
| 新增组件 | — | 1 个展示组件 |
| 新增纯函数 | — | 1 个 `buildCompactNodes` |

## 附录 B：竞品模式映射

| Trae IDE 模式 | 我们的实现 |
|---------------|-----------|
| 步骤状态徽章 | M2 引入 `✓ ✕ ⏳ ⏸` |
| 内联动作按钮 | M2 引入 `⧉ 复制 · ↻ 重跑` |
| 实时跟随 | M3 引入脉动圆点 + scroll 跟随 |
| 5s 耗时守卫 | 已被 `ChatExecutionCard` 覆盖（见 2026-06-09 报告） |
| 面板自动切换 | 降级为节点 ▸ 展开（不做多面板） |

## 附录 C：相关文件路径

- 现状代码：`web/src/components/chat/TurnBlock.vue:113-170`（non-ReAct 模板分支）
- 现状代码：`web/src/components/chat/TurnBlock.vue:384-436`（`interleavedRounds` 计算属性）
- 现状代码：`web/src/components/chat/TurnBlock.vue:343-348`（`reactReplyRounds`，ReAct 模式，不动）
- 现状样式：`web/src/css/theme/_chat-message-panel.sass`（TurnBlock 已有 `.turn-block__section*` 系列）
- 数据契约：`web/src/features/chat/messagePlannerPresentation.ts:26-58`（`resolveAssistantPresentation`）
