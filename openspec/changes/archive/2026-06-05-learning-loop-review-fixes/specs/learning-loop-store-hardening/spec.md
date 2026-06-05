## ADDED Requirements

### Requirement: Store loading 使用请求计数器
`useLearningLoopStore` SHALL 使用请求计数器模式管理 loading 状态，而非单一布尔 ref。当有任意数量的请求在进行时，`loading` computed 属性 SHALL 返回 `true`；当所有请求完成时 SHALL 返回 `false`。

#### Scenario: 并发请求期间 loading 为 true
- **WHEN** `fetchObservations` 和 `fetchPatterns` 同时发起
- **THEN** `loading` SHALL 为 `true`，直到两个请求都完成

#### Scenario: 先完成的请求不会提前清除 loading
- **WHEN** `fetchObservations` 先完成而 `fetchPatterns` 仍在进行
- **THEN** `loading` SHALL 仍为 `true`

#### Scenario: 所有请求完成后 loading 为 false
- **WHEN** 所有并发请求都已完成
- **THEN** `loading` SHALL 为 `false`

### Requirement: Store mutation 方法统一错误处理
`useLearningLoopStore` 的 `approveProposal`、`rejectProposal`、`runLoop` 方法 SHALL 在 catch 中设置 `error` ref，与 `fetchObservations`/`fetchPatterns`/`fetchProposals` 保持一致的错误处理模式。

#### Scenario: approveProposal 失败设置 error
- **WHEN** `approveProposal` 调用 API 失败
- **THEN** Store 的 `error` ref SHALL 被设置为错误信息

#### Scenario: rejectProposal 失败设置 error
- **WHEN** `rejectProposal` 调用 API 失败
- **THEN** Store 的 `error` ref SHALL 被设置为错误信息

#### Scenario: runLoop 失败设置 error
- **WHEN** `runLoop` 调用 API 失败
- **THEN** Store 的 `error` ref SHALL 被设置为错误信息
