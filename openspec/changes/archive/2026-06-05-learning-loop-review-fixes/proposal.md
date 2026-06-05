## Why

学习闭环前端可视化（learning-loop-frontend）代码审查发现 6 个建议项：类型导入来源违规（FL4）、Dialog 缺少 app-dialog-card（FU4）、Store loading 竞态风险、错误处理不一致、api.learning.ts 可读性问题、formatDate 函数重复。这些问题虽非阻断，但违反项目规范且存在潜在缺陷，需在下一迭代中修复。

## What Changes

- 修复 `useLearningLoopPanel.ts` 类型导入来源：从 `./api.learning` 改为 `./learning.types`（FL4 合规）
- 修复 `$q.dialog` 缺少 `app-dialog-card` class（FU4 合规）
- 重构 Store `loading` ref 为请求计数器模式，消除并发竞态风险
- 统一 Store 错误处理模式：mutation 方法（approveProposal/rejectProposal/runLoop）也设置 `store.error`
- 改进 `api.learning.ts` 归一化函数类型语法：将 `import()` 内联类型改为顶部 `import type`
- 抽取 `formatDate` 为共享 util，消除 3 个组件中的重复定义

## Capabilities

### New Capabilities

- `learning-loop-store-hardening`: Store 层健壮性增强——loading 竞态修复 + 错误处理统一

### Modified Capabilities

- `learning-loop-panel`: 类型导入来源修正、Dialog class 补充、formatDate 去重

## Impact

- **前端 features 层**：`api.learning.ts`（类型语法改进）、`useLearningLoopPanel.ts`（类型导入 + Dialog class）
- **前端 stores 层**：`stores/learningLoop/index.ts`（loading 计数器 + 错误处理统一）
- **前端 components 层**：`LearningPatternList.vue`、`LearningProposalList.vue`、`LearningObservationList.vue`（formatDate 去重）
- **可能新增**：共享 `formatDate` util 文件
- **不影响**：后端、Proto、API 契约、其他 Store
