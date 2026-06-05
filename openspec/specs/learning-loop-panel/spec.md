# learning-loop-panel Specification

## Purpose
TBD - created by archiving change learning-loop-review-fixes. Update Purpose after archive.
## Requirements
### Requirement: 学习闭环独立数据流
学习闭环模块 SHALL 遵循 API → Store → Composable → Page → Component 数据流铁律，使用独立的 API 文件、类型文件和 Store，不扩展现有 agents 模块的文件。Composable 中的类型导入 SHALL 直接从 `learning.types.ts` 导入，而非穿透 API 层。

#### Scenario: Composable 类型导入来源
- **WHEN** `useLearningLoopPanel` Composable 需要使用 `LearningObservation`、`LearningPattern`、`LearningProposal` 类型
- **THEN** SHALL 从 `./learning.types` 直接导入，而非从 `./api.learning` 间接导入

#### Scenario: 独立 API 文件
- **WHEN** 前端需要调用学习闭环后端接口
- **THEN** SHALL 通过 `api.learning.ts` 中的函数调用，该文件 SHALL 独立于 `api.ts`

#### Scenario: 独立 Store
- **WHEN** 学习闭环模块需要管理领域状态
- **THEN** SHALL 使用 `useLearningLoopStore`（`stores/learningLoop/index.ts`），该 Store SHALL 独立于 `useAgentDetailStore`

#### Scenario: Composable 封装 Store 调用
- **WHEN** 页面组件需要消费学习闭环数据
- **THEN** SHALL 通过 `useLearningLoopPanel` Composable 获取响应式状态和操作方法，Composable 内部通过 computed 从 Store 获取领域数据

### Requirement: 提议审批操作
用户 SHALL 能对状态为 validated 的提议执行审批操作，审批前 SHALL 弹出确认对话框，对话框 SHALL 使用 `app-dialog-card` 样式类。

#### Scenario: 审批前弹出确认对话框
- **WHEN** 用户点击"审批"按钮
- **THEN** 系统 SHALL 弹出使用 `app-dialog-card` 样式类的确认对话框，要求用户确认审批操作

#### Scenario: 审批按钮仅对 validated 状态可见
- **WHEN** 提议状态为 validated
- **THEN** 该提议行 SHALL 显示"审批"操作按钮

#### Scenario: 确认审批后调用 API
- **WHEN** 用户在确认对话框中点击确认
- **THEN** 系统 SHALL 调用 `approveLearningProposal` API，审批按钮进入 loading 状态

#### Scenario: 审批成功后刷新数据
- **WHEN** 审批 API 调用成功
- **THEN** 系统 SHALL 更新 Store 中对应提议的状态，并重新加载 patterns 和 proposals 列表

#### Scenario: 用户取消审批
- **WHEN** 用户在确认对话框中点击取消
- **THEN** 系统 SHALL 关闭对话框，不执行任何 API 调用

### Requirement: 共享 formatDate 工具函数
学习闭环组件 SHALL 使用共享的 `formatDate` 工具函数，而非在各组件中重复定义。

#### Scenario: 组件使用共享 formatDate
- **WHEN** LearningPatternList、LearningProposalList、LearningObservationList 组件需要格式化日期
- **THEN** SHALL 从 `learning.utils.ts` 导入共享的 `formatDate` 函数

#### Scenario: 组件内无重复 formatDate 定义
- **WHEN** 审查 Learning 组件代码
- **THEN** SHALL 不存在组件内局部定义的 `formatDate` 函数

### Requirement: API 归一化函数使用顶部 import type
`api.learning.ts` 中的归一化函数 SHALL 使用顶部 `import type` 语法导入类型，而非 `import()` 内联类型语法。

#### Scenario: 归一化函数返回类型使用顶部导入
- **WHEN** `normalizeObservation`/`normalizePattern`/`normalizeProposal` 函数声明返回类型
- **THEN** SHALL 使用顶部 `import type` 导入的类型，而非 `import('./learning.types').Xxx` 内联语法

