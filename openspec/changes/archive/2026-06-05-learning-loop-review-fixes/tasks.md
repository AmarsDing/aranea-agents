## 1. Store 层健壮性修复

- [x] 1.1 将 `stores/learningLoop/index.ts` 的 `loading: ref(false)` 改为 `pendingCount: ref(0)` 请求计数器，新增 `loading: computed(() => pendingCount.value > 0)`，所有 fetch 方法开头 `pendingCount.value++`、finally 中 `pendingCount.value--`。DoD: 并发 fetch 时 loading 不提前清除
- [x] 1.2 为 `approveProposal`、`rejectProposal`、`runLoop` 添加 try/catch，在 catch 中设置 `error.value` 并 re-throw。DoD: mutation 方法失败时 `store.error` 被正确设置
- [x] 1.3 验证 Store 改动：`cd web && pnpm build` 通过

## 2. Composable 类型导入修复

- [x] 2.1 将 `useLearningLoopPanel.ts` 第 4 行 `import type { ... } from './api.learning'` 改为 `import type { ... } from './learning.types'`。DoD: 类型导入来源为 learning.types，FL4 合规
- [x] 2.2 验证类型导入：`cd web && pnpm build` 通过

## 3. Dialog 样式修复

- [x] 3.1 在 `useLearningLoopPanel.ts` 的 `$q.dialog` 调用中添加 `class: 'app-dialog-card app-dialog-card--sm'`。DoD: 审批确认对话框使用 app-dialog-card 样式，FU4 合规
- [x] 3.3 验证 Dialog 样式：`cd web && pnpm build` 通过

## 4. API 层类型语法改进

- [x] 4.1 在 `api.learning.ts` 顶部添加 `import type { LearningObservation, LearningPattern, LearningProposal } from './learning.types'`，将 3 个 normalize 函数的返回类型从 `import('./learning.types').Xxx` 改为直接使用顶部导入的类型名。DoD: 无 `import()` 内联类型语法
- [x] 4.2 验证 API 层改动：`cd web && pnpm build` 通过

## 5. formatDate 去重

- [x] 5.1 新建 `web/src/features/agents/learning.utils.ts`，导出 `formatDate(iso: string): string` 函数（逻辑与现有组件内实现一致）。DoD: 共享 util 文件存在且导出 formatDate
- [x] 5.2 修改 `LearningPatternList.vue`：删除组件内 `formatDate` 函数，改为 `import { formatDate } from '../../features/agents/learning.utils'`。DoD: 组件内无局部 formatDate 定义
- [x] 5.3 修改 `LearningProposalList.vue`：同上。DoD: 组件内无局部 formatDate 定义
- [x] 5.4 修改 `LearningObservationList.vue`：同上。DoD: 组件内无局部 formatDate 定义
- [x] 5.5 验证 formatDate 去重：`cd web && pnpm build` 通过

## 6. 全量验证

- [x] 6.1 运行 `cd web && pnpm lint`，确认 0 errors
- [x] 6.2 运行 `cd web && pnpm build`，确认构建通过
- [x] 6.3 手动验证：Agent 详情页"学习闭环"Tab 功能正常（概览/模式/提议/观察/运行闭环/审批/拒绝）
