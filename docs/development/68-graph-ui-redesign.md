# 68 — Graph 编辑器 UI 重构

> 基于 Langflow 设计思想的适配方案，聚焦业务友好度和操作效率

---

## 1. 背景与动机

当前 Graph 编辑器存在三个核心体验问题：

1. **节点信息密度低**：节点只显示 2 个无名 Handle（左入右出），无法表达 State 字段读写关系，用户无法直观理解数据流
2. **资源选择原始**：Agent 名称是纯文本输入（无验证、无下拉），Tool 选择是平铺 key 列表（无分类），Function 引用也是纯文本
3. **编辑-执行割裂**：运行 Graph 需跳转到独立页面，无法在编辑器内完成"编辑→执行→调试"闭环

调研报告（`docs/reports/2026-06-09-research-langflow-ui-replication.md`）对 Langflow 前端进行了 9 轮深度评审，识别出 5 个值得复刻的设计思想和 5 个关键取舍决策。

## 2. 目标

| 目标 | 衡量标准 |
|------|---------|
| 节点可读性 | 用户无需点开 PropertyPanel 即可理解节点的读写字段和执行状态 |
| 资源选择效率 | Agent/Tool/Function 选择从"手动输入"升级为"分类选择器" |
| 编辑执行闭环 | 在编辑器内完成运行→查看状态→调试的完整流程 |
| 降低入门门槛 | 新用户 5 分钟内可完成第一个 Graph 的创建和运行 |

## 3. 范围

### 3.1 在范围内

- P0：节点重构（State-Aware 多端口 + 折叠）、5 种运行时状态、RunPanel、资源选择器
- P1：侧边栏重构、NodeToolbar、连线引导、State Schema 可视化
- P2：路径可达性校验、Reducer 冲突检测、子图嵌套、State Diff、便签节点

### 3.2 不在范围内

- 后端 API 改造（全部就绪，零改造）
- 新增节点类型（保持 7 种）
- MCP 节点类型（MCP 通过 Agent 间接使用，详见决策 4）
- Langflow 的 Python 代码编辑器、组件市场、自定义组件

## 4. 关键设计决策

### 决策 1：端口显示字段名而非类型

- **选择**：Handle 旁显示 State 字段名（如 `messages`），不显示类型（如 `Message[]`）
- **理由**：Aranea 只有 6 种 State 字段类型，类型信息区分度低；字段名本身是最佳语义说明
- **类型信息**：放在 Handle Tooltip 中（悬停 1s 显示字段名+类型+Reducer）

### 决策 2：5 种核心状态

- **选择**：idle / running / success / error / interrupted
- **合并**：queued→running、inactive→idle（灰化）、active→running
- **理由**：5 种颜色记忆负担低，后端事件映射清晰

### 决策 3：路径可达性降级为离线校验

- **选择**：连接时只做结构性检查（自连接/重复边），路径可达性作为"校验"按钮的离线检查
- **理由**：实时验证增加延迟且初学者不理解，离线校验更灵活

### 决策 4：MCP 通过 Agent 间接使用

- **选择**：不增加 MCP 节点类型，Agent 选择器显示 MCP 策略信息
- **理由**：与现有架构一致（MCP 在 Agent 级别配置），7 种节点类型已够多

### 决策 5：节点折叠放宽约束

- **选择**：所有节点均可折叠，折叠后显示 Header + 汇总端口（R3/W2）
- **理由**：Agent 节点多端口，Langflow 严格 isMinimal 规则下几乎无法折叠

## 5. 需求清单

### P0 — 核心体验

#### P0-1：State-Aware 可折叠节点

**用户故事**：作为流程设计者，我希望节点直接显示读写哪些 State 字段，这样我无需点开属性面板就能理解数据流。

**功能需求**：

| ID | 需求 | 验收标准 |
|----|------|---------|
| P0-1.1 | 节点按 State Schema 动态生成 READS/WRITES Handle | 每个 State 字段对应一个 Handle，Handle id 编码为 `r:{fieldName}` 或 `w:{fieldName}` |
| P0-1.2 | Handle 旁显示字段名标签 | 左侧 Handle 旁显示 "读取: fieldName"，右侧显示 "写入: fieldName" |
| P0-1.3 | Handle Tooltip 显示详细信息 | 悬停 1s 显示：字段名、类型、Reducer、描述 |
| P0-1.4 | 节点可折叠/展开 | 双击 Header 切换折叠态；折叠后只显示 Header + 汇总端口（R3/W2） |
| P0-1.5 | 折叠态汇总端口 | 折叠后左侧 1 个 READS 汇总 Handle，右侧 1 个 WRITES 汇总 Handle |
| P0-1.6 | 折叠/展开时 Handle 位置更新 | 调用 `updateNodeInternals()` 刷新 Handle 边界 |
| P0-1.7 | Edge 引用 sourceHandle/targetHandle | EdgeDef 增加 sourceHandle/targetHandle 字段 |
| P0-1.8 | 连接时结构性检查 | 禁止自连接、重复边；字段名匹配提示（非阻断） |
| P0-1.9 | 节点类型色标 | 7 种节点类型各有独立色标，驱动 Handle 边框色和 Header 色条 |

**技术约束**：
- Vue Flow Handle 支持 `id` 属性和默认 slot，可渲染字段名标签
- `updateNodeInternals()` 在 Handle 动态变更时必须调用
- Handle id 编码方案：`r:{fieldName}` / `w:{fieldName}`，避免特殊字符

#### P0-2：5 种运行时状态

**用户故事**：作为流程设计者，我希望运行 Graph 时实时看到每个节点的执行状态，这样我能快速定位问题。

**功能需求**：

| ID | 需求 | 验收标准 |
|----|------|---------|
| P0-2.1 | 5 种状态视觉 | idle(默认)/running(蓝色脉冲)/success(绿色)/error(红色)/interrupted(橙色) |
| P0-2.2 | WS 事件→状态映射 | graph_node_start→running, graph_node_end→success, graph_node_error→error, checkpoint.interrupt→interrupted |
| P0-2.3 | 状态图标 | running=Loader2旋转, success=Check, error=CircleAlert, interrupted=CirclePause |
| P0-2.4 | 边动画波浪推进 | running 状态的节点，其出边显示流动动画点 |
| P0-2.5 | 批量状态更新 | 使用 `clearAndSetEdgesRunning()` 批量更新边动画，避免逐条更新卡顿 |

#### P0-3：RunPanel

**用户故事**：作为流程设计者，我希望在编辑器内直接运行 Graph 并查看结果，这样我不需要跳转到独立页面。

**功能需求**：

| ID | 需求 | 验收标准 |
|----|------|---------|
| P0-3.1 | 右侧可折叠 RunPanel | 默认收起，点击运行按钮展开；宽度可拖拽（300-500px） |
| P0-3.2 | 执行控制 | 运行/暂停/停止按钮；"运行到此"从选中节点开始 |
| P0-3.3 | State 快照 | 显示当前 State 的所有字段和值，实时更新 |
| P0-3.4 | Checkpoint 导航 | 列出所有 Checkpoint，点击可 TimeTravel |
| P0-3.5 | 与 PropertyPanel 共存 | RunPanel 在右侧，PropertyPanel 在节点旁浮动（借鉴 Langflow InspectionPanel + Playground 共存） |

**技术约束**：
- 复用现有 `useGraphExecutionStream` 和 `useGraphTimeTravel` composable
- RunPanel 使用 `q-drawer` 或自定义 flex 面板

#### P0-4：资源分类选择器

**用户故事**：作为流程设计者，我希望通过下拉选择器选择 Agent/Tool/Function，而不是手动输入名称。

**功能需求**：

| ID | 需求 | 验收标准 |
|----|------|---------|
| P0-4.1 | Agent 选择器 | 替代纯文本输入；按 Kind 分组（自建/系统/A2A Proxy）；支持搜索 |
| P0-4.2 | Tool 选择器 | 替代平铺 key 列表；按 Category 分组（系统工具/MCP 工具/自定义工具）；多选 |
| P0-4.3 | Function 选择器 | 替代纯文本输入；列出 Registry 注册的函数 |
| P0-4.4 | 节点创建引导 | 拖入节点后自动弹出对应选择器；跳过则显示"未配置"警告 |
| P0-4.5 | Agent MCP 信息 | 选中 Agent 后显示其 MCP 策略（通过 GetAgentEffectiveTools API） |
| P0-4.6 | 选择器验证 | 选择的资源不存在时显示红色警告 |

**数据源**：

| 选择器 | API | Store | 已有？ |
|--------|-----|-------|--------|
| Agent | `listAgents()` | `useAgentsCatalogStore` | 是，需增加 Kind 分组 |
| Tool | `listTools()` | `useToolsStore` | 是，需增加 Category 分组 |
| Function | `listTools()` (filter) | `useToolsStore` | 复用 Tool API |
| Agent MCP | `getAgentEffectiveTools()` | 新建 | API 已有，需封装 |

### P1 — 体验打磨

#### P1-1：侧边栏重构

| ID | 需求 | 验收标准 |
|----|------|---------|
| P1-1.1 | 3 区段导航 | 节点类型 / 版本历史 / 设置（40px 图标条） |
| P1-1.2 | 节点骨架卡片 | 7 种节点类型 + 快速模板；拖拽创建 |
| P1-1.3 | 快速模板 | 审批流程/数据处理管线/条件路由 3 个预置模板 |

#### P1-2：NodeToolbar

| ID | 需求 | 验收标准 |
|----|------|---------|
| P1-2.1 | 浮动工具栏 | 选中节点上方显示；3 个直接按钮（运行到此/冻结/删除） |
| P1-2.2 | 更多菜单 | 复制/设为入口/HITL 配置/重试策略/缓存策略/Fallback Agent/运行时日志 |
| P1-2.3 | 条件性菜单项 | HITL 配置仅 HITL 节点；Fallback Agent 仅 Agent 节点 |

#### P1-3：连线引导

| ID | 需求 | 验收标准 |
|----|------|---------|
| P1-3.1 | Handle 字段名过滤 | 拖线时，同名 Handle 绿色脉冲，其他灰色 |
| P1-3.2 | 兼容高亮 | 从 "写入 messages" Handle 拖线 → 所有 "读取 messages" Handle 高亮 |
| P1-3.3 | 无兼容提示 | 无可连接目标时显示 "无兼容端口" 提示 |
| P1-3.4 | 双击快捷连线 | 双击 Handle 自动连接到最近的兼容 Handle |

#### P1-4：State Schema 可视化

| ID | 需求 | 验收标准 |
|----|------|---------|
| P1-4.1 | State 面板 | 右侧面板显示所有 State 字段及其读写节点 |
| P1-4.2 | 字段流向 | 每个字段显示哪些节点写入、哪些节点读取 |
| P1-4.3 | Reducer 自然语言 | default→"覆盖"、append→"追加"、cover→"可选更新"、merge→"合并" |

### P2 — 高级功能

| ID | 需求 | 优先级 |
|----|------|--------|
| P2-1 | 路径可达性离线校验面板 | P2 |
| P2-2 | Reducer 冲突检测 | P2 |
| P2-3 | 子图嵌套编辑器 | P2 |
| P2-4 | 运行时 State Diff | P2 |
| P2-5 | 便签节点 | P2 |
| P2-6 | 辅助对齐线 | P2 |
| P2-7 | 组件版本管理 | P2 |

## 6. 非功能需求

| 维度 | 要求 |
|------|------|
| 性能 | 50 节点图编辑器操作流畅（< 16ms/帧）；100 节点图状态更新 < 200ms |
| 兼容性 | Chrome 115+、Edge 115+、Firefox 120+ |
| 可访问性 | 所有交互支持键盘操作；Handle Tooltip 有 ARIA 标签 |
| 暗色模式 | 所有新增 UI 支持双主题（暖琥珀/科技青） |
| 响应式 | 最小支持 1280px 宽度；RunPanel 可折叠 |

## 7. 约束与假设

| 约束 | 说明 |
|------|------|
| 后端零改造 | 所有 API 已就绪，前端只需消费更多字段 |
| Vue Flow 版本 | @vue-flow/core v1.48.2，不升级 |
| Quasar 版本 | v2.19.3，不升级 |
| NodeToolbar 自建 | Vue Flow 不内置，需自行实现浮动定位 |
| 现有功能不回退 | 所有现有 Graph 编辑器功能必须保留 |

## 8. 依赖

| 依赖 | 状态 | 说明 |
|------|------|------|
| Graph CRUD API | ✅ 就绪 | 28 个 RPC 方法 |
| Graph 执行 API | ✅ 就绪 | ExecuteGraph/ResumeGraph/CancelGraph |
| Graph 校验 API | ✅ 就绪 | ValidateGraph |
| Graph State API | ✅ 就绪 | GetStateSnapshot/EditState/TimeTravel |
| Agent 列表 API | ✅ 就绪 | 含 agent_key/display_name/kind/agent_kind |
| Tool 列表 API | ✅ 就绪 | 含 key/display_name/category/source |
| Agent 有效工具 API | ✅ 就绪 | GetAgentEffectiveTools |
| WS 事件 | ✅ 就绪 | 7 种执行事件 |
| M54 任务系统 | 进行中 | 不阻塞 P0/P1 |

## 9. 参考资料

- [调研报告](../reports/2026-06-09-research-langflow-ui-replication.md) — Langflow UI 深度调研（v1.9）
- [Graph 工作流需求](36-graph-workflow.md) — 现有 Graph 模块需求文档
- [Graph 工作流设计](36-graph-workflow.design.md) — 现有 Graph 模块设计文档
