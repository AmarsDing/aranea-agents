## ADDED Requirements

### Requirement: Ecosystem preset load API
系统 SHALL 提供 `POST /api/v1/admin/ecosystem/preset/load` API，允许管理员一键加载系统附带的行业生态数据（Agent、Team、分类树）。

#### Scenario: 首次加载全部行业生态
- **WHEN** 管理员调用 `POST /api/v1/admin/ecosystem/preset/load` 且 `ecosystem_loaded` 中所有行业均为 `loaded: false`
- **THEN** 系统依次加载 finance/selfmedia/softwaredev 三个行业的分类树、Agent、Team，Agent Kind 设为 `ecosystem_preset`，Team Kind 设为 `ecosystem_preset`，更新 `ecosystem_loaded` JSON 中各行业状态为 `loaded: true` 并记录加载时间和数量，返回各行业加载结果

#### Scenario: 加载指定行业
- **WHEN** 管理员调用 `POST /api/v1/admin/ecosystem/preset/load` 且请求体包含 `industries: ["finance"]`
- **THEN** 系统仅加载 finance 行业的分类树、Agent、Team，其他行业不受影响

#### Scenario: 重复加载已加载行业
- **WHEN** 管理员调用加载 API 且某行业在 `ecosystem_loaded` 中已为 `loaded: true`
- **THEN** 该行业被跳过，加入响应的 `already_loaded` 列表，不影响已加载数据

#### Scenario: 部分加载失败
- **WHEN** 加载过程中某行业失败（如数据库错误）
- **THEN** 已成功加载的行业状态正常更新，失败行业状态保持 `loaded: false`，API 返回错误信息包含失败行业名称和原因

### Requirement: Ecosystem loaded status query
系统 SHALL 在 `system_settings` 中存储附带生态加载状态，前端可读取并展示。

#### Scenario: 查询加载状态
- **WHEN** 前端请求系统设置
- **THEN** 响应包含 `ecosystem_loaded` JSON 字段，列出每个行业的加载状态（loaded/loaded_at/agents/teams）

#### Scenario: 未加载任何行业
- **WHEN** 系统首次启动且未调用过加载 API
- **THEN** `ecosystem_loaded` 为 `{}`（空 JSON），前端显示"未加载"状态

### Requirement: Ecosystem preset reload
系统 SHALL 支持重新加载已加载的行业生态。

#### Scenario: 重新加载单个行业
- **WHEN** 管理员调用 `POST /api/v1/admin/ecosystem/preset/load` 且请求体包含 `industries: ["finance"]` 且 `force: true`
- **THEN** 系统重置 finance 行业的 `ecosystem_loaded` 状态为 `loaded: false`，重新执行 Pack 导入（ConflictOverwrite 策略），更新加载状态

### Requirement: Ecosystem preset Agent/Team permissions
`ecosystem_preset` 类型的 Agent 和 Team SHALL 与 `user` 类型同权（可编辑、可删除），但前端显示"预设"徽章以区分来源。

#### Scenario: 编辑预设 Agent
- **WHEN** 用户编辑 Kind 为 `ecosystem_preset` 的 Agent
- **THEN** 系统允许编辑所有字段，与编辑 user Agent 行为一致

#### Scenario: 删除预设 Agent
- **WHEN** 用户删除 Kind 为 `ecosystem_preset` 的 Agent
- **THEN** 系统正常执行删除，不阻止操作

#### Scenario: 删除系统内置 Agent 被阻止
- **WHEN** 用户尝试删除 Kind 为 `system_builtin` 的 Agent
- **THEN** 前端隐藏删除按钮，若通过 API 直接调用则返回 403 错误

#### Scenario: 删除系统内置 Team 被阻止
- **WHEN** 用户尝试删除 Kind 为 `system_builtin` 的 Team
- **THEN** 前端隐藏删除按钮，若通过 API 直接调用则返回 403 错误

### Requirement: Ecosystem preset unload API
系统 SHALL 提供 `POST /api/v1/admin/ecosystem/preset/unload` API，允许管理员卸载指定行业的附带生态数据（删除该行业所有 Agent、Team 和分类节点）。

#### Scenario: 卸载单个行业
- **WHEN** 管理员调用 `POST /api/v1/admin/ecosystem/preset/unload` 且请求体包含 `industries: ["finance"]`，且 finance 在 `ecosystem_loaded` 中为 `loaded: true`
- **THEN** 系统软删除 finance 行业下所有分类节点（industry → departments → positions），软删除 Kind 为 `ecosystem_preset` 且属于该行业分类的 Agent，软删除 Kind 为 `ecosystem_preset` 且成员 Agent 全部属于该行业的 Team，更新 `ecosystem_loaded` 中 finance 状态为 `loaded: false`，返回删除统计

#### Scenario: 卸载未加载的行业
- **WHEN** 管理员调用卸载 API 且指定行业在 `ecosystem_loaded` 中为 `loaded: false`
- **THEN** 系统返回错误，提示该行业未加载

#### Scenario: 卸载时 Team 成员跨行业
- **WHEN** 卸载某行业时存在跨行业 Team（成员 Agent 分属多个行业）
- **THEN** 该 Team 不被删除，但从中移除属于被卸载行业的 Agent 成员

#### Scenario: 卸载确认对话框
- **WHEN** 用户在前端点击某行业的"卸载"按钮
- **THEN** 弹出确认对话框，显示"卸载将删除该行业下所有 Agent（XX 个）、Team（XX 个）和分类节点（XX 个），此操作不可撤销。确定要卸载吗？"，用户确认后才调用卸载 API
