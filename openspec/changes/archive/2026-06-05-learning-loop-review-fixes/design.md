## Context

学习闭环前端可视化（learning-loop-frontend）已归档完成，代码审查发现 6 个建议项需要修复。当前实现功能正确、构建通过，但存在分层规范违规（FL4/FU4）和 Store 层健壮性问题（loading 竞态、错误处理不一致），以及代码重复（formatDate）和可读性问题（import() 内联类型语法）。

## Goals / Non-Goals

**Goals:**

- 修复 FL4 违规：Composable 类型导入来源从 api.learning 改为 learning.types
- 修复 FU4 违规：$q.dialog 补充 app-dialog-card class
- 消除 Store loading 竞态：改用请求计数器模式
- 统一 Store 错误处理：mutation 方法也设置 store.error
- 改进 api.learning.ts 可读性：import() 内联类型改为顶部 import type
- 消除 formatDate 重复：抽取共享 util

**Non-Goals:**

- 不修改后端 API 或 Proto
- 不修改 AgentEvolutionPanel.vue（其 formatDate 重复暂不处理，避免跨模块改动）
- 不重构 Store 整体架构（仅修复已识别问题）
- 不添加新功能

## Decisions

### D1: Store loading 改用请求计数器

**选择**：将 `loading: ref(false)` 改为 `pendingCount: ref(0)` 计数器模式。

**理由**：
- 方案 A（独立 loading per action）：需要 3 个 loading ref，增加复杂度
- 方案 B（请求计数器）：简单、通用，fetch 开始 +1、finally -1，为 0 即无请求
- 方案 C（AbortController + loading）：过度设计，当前无取消需求

**选择 B**，计算属性 `loading = computed(() => pendingCount > 0)` 保持对外 API 不变。

### D2: formatDate 抽取位置

**选择**：在 `web/src/features/agents/` 下新建 `learning.utils.ts`。

**理由**：
- 放在 `features/agents/` 域内，与学习闭环功能内聚
- 不放在全局 utils（当前仅 learning 组件使用，避免过早泛化）
- 如果后续其他 agents 组件也需要，可再提升到 `utils/`

### D3: api.learning.ts 类型语法改进

**选择**：顶部 `import type` 导入 + `export type { ... } from` 分离写法。

**理由**：当前 `import('./learning.types').LearningObservation` 内联语法可读性差。改为顶部导入后，re-export 需要用 `export type { X } from './learning.types'` 语法（TypeScript 支持 import + re-export 同名类型）。

## Risks / Trade-offs

- [Store loading 改动可能引入新 bug] → 改动范围小（仅替换 ref 为计数器），且 Composable 层自持 loading 未受影响，回归验证即可
- [formatDate 抽取可能影响其他组件] → 仅影响 3 个 Learning 组件，AgentEvolutionPanel 不在本次范围
- [api.learning.ts 类型语法改动可能影响 re-export] → TypeScript 允许 import type + export type from 共存，需验证编译通过
