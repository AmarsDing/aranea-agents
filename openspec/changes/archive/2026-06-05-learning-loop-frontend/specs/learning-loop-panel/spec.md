## ADDED Requirements

### Requirement: 学习闭环 Tab 页面入口
Agent 详情页（AgentSettingsPage）SHALL 新增"学习闭环"Tab，与"进化"Tab 并列，作为学习闭环功能的唯一前端入口。

#### Scenario: Tab 可见且可切换
- **WHEN** 用户打开 Agent 详情页
- **THEN** 页面 SHALL 在 Tab 栏中显示"学习闭环"Tab，位于"进化"Tab 之后

#### Scenario: 切换到学习闭环 Tab 加载面板
- **WHEN** 用户点击"学习闭环"Tab
- **THEN** 系统 SHALL 渲染 `AgentLearningLoopPanel` 组件，传入当前 agentId

### Requirement: 学习闭环独立数据流
学习闭环模块 SHALL 遵循 API → Store → Composable → Page → Component 数据流铁律，使用独立的 API 文件、类型文件和 Store，不扩展现有 agents 模块的文件。

#### Scenario: 独立 API 文件
- **WHEN** 前端需要调用学习闭环后端接口
- **THEN** SHALL 通过 `api.learning.ts` 中的函数调用，该文件 SHALL 独立于 `api.ts`

#### Scenario: 独立 Store
- **WHEN** 学习闭环模块需要管理领域状态
- **THEN** SHALL 使用 `useLearningLoopStore`（`stores/learningLoop/index.ts`），该 Store SHALL 独立于 `useAgentDetailStore`

#### Scenario: Composable 封装 Store 调用
- **WHEN** 页面组件需要消费学习闭环数据
- **THEN** SHALL 通过 `useLearningLoopPanel` Composable 获取响应式状态和操作方法，Composable 内部通过 computed 从 Store 获取领域数据

### Requirement: 闭环概览统计卡片
`LearningLoopOverview` 组件 SHALL 展示 4 个统计卡片：观察数、模式数、待审批提议数、已注册知识数，并提供"运行学习闭环"按钮。

#### Scenario: 统计卡片展示
- **WHEN** 学习闭环面板加载完成
- **THEN** 概览区域 SHALL 展示 4 个统计卡片，分别显示观察记录总数、模式总数、状态为 validated 的提议数（待审批）、状态为 applied 的提议数（已注册知识）

#### Scenario: 统计数字实时计算
- **WHEN** Store 中的 observations、patterns、proposals 数据变化
- **THEN** 统计卡片 SHALL 通过 computed 属性自动更新数字

### Requirement: 手动触发 RunLoop
用户 SHALL 能通过"运行学习闭环"按钮手动触发闭环运行，按钮 SHALL 具有加载状态防止重复点击。

#### Scenario: 点击运行闭环
- **WHEN** 用户点击"运行学习闭环"按钮
- **THEN** 系统 SHALL 调用 `runLearningLoop` API，按钮进入 loading 状态，防止重复点击

#### Scenario: 运行完成后刷新数据
- **WHEN** RunLoop API 调用成功返回
- **THEN** 系统 SHALL 自动重新加载 observations、patterns、proposals 列表数据，按钮恢复可用状态

#### Scenario: 运行失败提示错误
- **WHEN** RunLoop API 调用失败
- **THEN** 系统 SHALL 显示错误通知，按钮恢复可用状态

### Requirement: 模式列表展示与筛选
`LearningPatternList` 组件 SHALL 展示已识别的模式（Pattern）列表，支持按状态筛选，显示频率、置信度和描述信息。

#### Scenario: 展示模式列表
- **WHEN** 学习闭环面板加载完成且存在模式数据
- **THEN** 模式列表 SHALL 展示每个模式的 kind badge、描述、频率、置信度进度条和检测时间

#### Scenario: 按状态筛选模式
- **WHEN** 用户在模式列表的状态筛选器中选择 detected / confirmed / dismissed
- **THEN** 系统 SHALL 重新调用 `fetchPatterns` 传入对应状态参数，仅展示匹配的模式

#### Scenario: 清除筛选显示全部
- **WHEN** 用户在模式列表的状态筛选器中选择"全部"
- **THEN** 系统 SHALL 调用 `fetchPatterns` 不传状态参数，展示所有模式

### Requirement: 模式状态颜色映射
模式列表 SHALL 按状态显示不同颜色的 badge：detected 为 orange（已检测）、confirmed 为 positive/绿色（已确认）、dismissed 为 grey（已忽略）。

#### Scenario: detected 状态显示橙色
- **WHEN** 模式状态为 detected
- **THEN** 该模式的状态 badge SHALL 显示为 orange 颜色，标签为"已检测"

#### Scenario: confirmed 状态显示绿色
- **WHEN** 模式状态为 confirmed
- **THEN** 该模式的状态 badge SHALL 显示为 positive 颜色，标签为"已确认"

#### Scenario: dismissed 状态显示灰色
- **WHEN** 模式状态为 dismissed
- **THEN** 该模式的状态 badge SHALL 显示为 grey 颜色，标签为"已忽略"

### Requirement: 提议列表展示与筛选
`LearningProposalList` 组件 SHALL 展示提议（Proposal）列表，支持按状态筛选，显示标题、内容摘要和状态 badge。

#### Scenario: 展示提议列表
- **WHEN** 学习闭环面板加载完成且存在提议数据
- **THEN** 提议列表 SHALL 展示每个提议的 kind badge、标题、内容摘要、状态 badge 和操作按钮

#### Scenario: 按状态筛选提议
- **WHEN** 用户在提议列表的状态筛选器中选择特定状态（draft / validated / approved / applied / rejected / conflict）
- **THEN** 系统 SHALL 重新调用 `fetchProposals` 传入对应状态参数，仅展示匹配的提议

#### Scenario: 清除筛选显示全部
- **WHEN** 用户在提议列表的状态筛选器中选择"全部"
- **THEN** 系统 SHALL 调用 `fetchProposals` 不传状态参数，展示所有提议

### Requirement: 提议状态颜色映射
提议列表 SHALL 按状态显示不同颜色的 badge：draft 为 grey（草稿）、validated 为 blue（已验证）、approved 为 teal（已审批）、rejected 为 negative/红色（已拒绝）、applied 为 positive/绿色（已应用）、conflict 为 warning/黄色（冲突）、expired 为 grey（已过期）。

#### Scenario: validated 状态显示蓝色
- **WHEN** 提议状态为 validated
- **THEN** 该提议的状态 badge SHALL 显示为 blue 颜色，标签为"已验证"

#### Scenario: approved 状态显示 teal
- **WHEN** 提议状态为 approved
- **THEN** 该提议的状态 badge SHALL 显示为 teal 颜色，标签为"已审批"

#### Scenario: rejected 状态显示红色
- **WHEN** 提议状态为 rejected
- **THEN** 该提议的状态 badge SHALL 显示为 negative 颜色，标签为"已拒绝"

#### Scenario: applied 状态显示绿色
- **WHEN** 提议状态为 applied
- **THEN** 该提议的状态 badge SHALL 显示为 positive 颜色，标签为"已应用"

#### Scenario: conflict 状态显示黄色
- **WHEN** 提议状态为 conflict
- **THEN** 该提议的状态 badge SHALL 显示为 warning 颜色，标签为"冲突"

### Requirement: 提议审批操作
用户 SHALL 能对状态为 validated 的提议执行审批操作，审批前 SHALL 弹出确认对话框。

#### Scenario: 审批按钮仅对 validated 状态可见
- **WHEN** 提议状态为 validated
- **THEN** 该提议行 SHALL 显示"审批"操作按钮

#### Scenario: 审批前弹出确认对话框
- **WHEN** 用户点击"审批"按钮
- **THEN** 系统 SHALL 弹出确认对话框，要求用户确认审批操作

#### Scenario: 确认审批后调用 API
- **WHEN** 用户在确认对话框中点击确认
- **THEN** 系统 SHALL 调用 `approveLearningProposal` API，审批按钮进入 loading 状态

#### Scenario: 审批成功后刷新数据
- **WHEN** 审批 API 调用成功
- **THEN** 系统 SHALL 更新 Store 中对应提议的状态，并重新加载 patterns 和 proposals 列表

#### Scenario: 用户取消审批
- **WHEN** 用户在确认对话框中点击取消
- **THEN** 系统 SHALL 关闭对话框，不执行任何 API 调用

### Requirement: 提议拒绝操作
用户 SHALL 能对状态为 validated 的提议执行拒绝操作，拒绝前 SHALL NOT 弹出确认对话框，直接执行。

#### Scenario: 拒绝按钮仅对 validated 状态可见
- **WHEN** 提议状态为 validated
- **THEN** 该提议行 SHALL 显示"拒绝"操作按钮

#### Scenario: 点击拒绝直接执行
- **WHEN** 用户点击"拒绝"按钮
- **THEN** 系统 SHALL 直接调用 `rejectLearningProposal` API，不弹出确认对话框，拒绝按钮进入 loading 状态

#### Scenario: 拒绝成功后刷新数据
- **WHEN** 拒绝 API 调用成功
- **THEN** 系统 SHALL 更新 Store 中对应提议的状态，并重新加载 patterns 和 proposals 列表

### Requirement: 已注册知识查看
提议列表中状态为 applied 的提议 SHALL 展示为已注册知识，显示审批人和注册时间。

#### Scenario: applied 状态提议显示注册信息
- **WHEN** 提议状态为 applied
- **THEN** 该提议行 SHALL 显示审批人（approved_by）和更新时间，不显示审批/拒绝操作按钮

#### Scenario: 概览卡片统计已注册知识数
- **WHEN** 存在状态为 applied 的提议
- **THEN** 概览卡片的"已注册"统计数字 SHALL 等于 applied 状态提议的数量

### Requirement: 观察记录列表
`LearningObservationList` 组件 SHALL 展示观察记录（Observation）列表，显示 kind 图标、内容摘要、Session ID 和观察时间。

#### Scenario: 展示观察记录列表
- **WHEN** 学习闭环面板加载完成且存在观察数据
- **THEN** 观察记录列表 SHALL 展示每条记录的 kind 图标、内容摘要、Session ID 和观察时间

#### Scenario: Kind 图标映射
- **WHEN** 观察记录的 kind 为 tool_call
- **THEN** 该记录 SHALL 显示 build 图标（blue 颜色）

#### Scenario: feedback 类型图标
- **WHEN** 观察记录的 kind 为 feedback
- **THEN** 该记录 SHALL 显示 chat 图标（purple 颜色）

#### Scenario: memory_hit 类型图标
- **WHEN** 观察记录的 kind 为 memory_hit
- **THEN** 该记录 SHALL 显示 psychology 图标（teal 颜色）

#### Scenario: memory_miss 类型图标
- **WHEN** 观察记录的 kind 为 memory_miss
- **THEN** 该记录 SHALL 显示 psychology 图标（grey 颜色）

### Requirement: 数据自动加载与响应式刷新
Composable SHALL 在 agentId 变化或筛选器变化时自动重新加载所有数据。

#### Scenario: agentId 变化自动重载
- **WHEN** 用户切换到不同 Agent 的学习闭环 Tab
- **THEN** Composable SHALL 自动调用 fetchAll 重新加载该 Agent 的 observations、patterns、proposals

#### Scenario: 筛选器变化自动重载
- **WHEN** 用户更改模式状态筛选器或提议状态筛选器
- **THEN** Composable SHALL 自动调用对应的 fetch 函数重新加载筛选后的数据

#### Scenario: 初始加载
- **WHEN** 学习闭环面板首次挂载且 agentId 有效
- **THEN** Composable SHALL 立即调用 fetchAll 加载所有数据

### Requirement: 加载状态展示
学习闭环面板 SHALL 在数据加载期间展示加载指示器，防止用户在数据未就绪时进行操作。

#### Scenario: 首次加载显示 loading
- **WHEN** 面板首次加载数据
- **THEN** 概览区域 SHALL 显示 `q-inner-loading` 加载指示器，子列表组件 SHALL 接收 loading 状态

#### Scenario: 操作期间按钮 loading
- **WHEN** 用户执行审批/拒绝/运行闭环操作
- **THEN** 对应的操作按钮 SHALL 进入 loading 状态，approvingId/rejectingId/runningLoop 标识当前操作的项

### Requirement: 不修改现有进化面板
学习闭环 Tab 的实现 SHALL NOT 修改 `AgentEvolutionPanel.vue` 的任何现有功能。

#### Scenario: 进化面板功能不受影响
- **WHEN** 学习闭环 Tab 被添加到 Agent 详情页
- **THEN** "进化"Tab 下的 AgentEvolutionPanel 功能 SHALL 保持不变
