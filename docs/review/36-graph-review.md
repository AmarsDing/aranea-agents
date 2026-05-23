# 36 Graph 工作流 Review

> **评分**：77 / 100 | **风险等级**：P1  
> **文档**：[36-graph-development.md](../需求/36-graph-development.md)  
> **代码锚点**：`internal/graph/` · `internal/service/graph.go` · `internal/biz/graph.go` · `internal/graph/trpc/` · `web/src/pages/GraphEditorPage.vue` · `web/src/pages/GraphRunPage.vue`  
> **审查时间**：2026-05-21 · **Phase A–D 优化闭合**：[2026-05-23-Graph-Phase-A-D-Review.md](./2026-05-23-Graph-Phase-A-D-Review.md)（92/100，P2）

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 15 | 20 | 核心 Graph 框架可用；LLM/Tool 节点待补全；Checkpoint/HITL 产品化尚早期 |
| 架构一致性 | 21 | 25 | `internal/graph/trpc` 适配层正确；Graph runtime 与 biz 通过端口接口隔离；Data 层 checkpoint 绑定框架有待修复 |
| 后端实现质量 | 17 | 20 | Graph Builder + GraphAgent + ExecutionSummary ✅；节点类型不完整 |
| 前端实现质量 | 12 | 15 | Vue Flow 画布 + 节点面板 + 运行页 ✅；节点类型对应前端配置待补 |
| 测试与验证 | 6 | 10 | `pkg/trpc-agent-go/graph/*_test.go` 框架层有测试；Aranea 适配层测试薄弱 |
| 文档一致性 | 6 | 10 | `36-graph-development.md` 与代码状态对齐 |

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| Graph CRUD（创建/编辑/删除） | ✅ |
| Vue Flow 画布编辑器 | ✅ |
| Graph Builder（`BuildDeps` + `wireNode`） | ✅ I2 |
| `AddLLMNode` / `AddToolsNode` 接线 | ✅ `node_wiring.go` |
| `SetEntryPoint` / `SetFinishPoint` | ✅ |
| ExecutionSummary（`graph_execution_done` metadata） | ✅ I2 |
| Graph Run 页（执行态可视化） | ✅ |
| Graph EventBridge → EventBus | ✅ |
| Checkpoint（trpc-agent-go 框架层） | ✅ 框架；🟡 产品化 |
| HITL（Human-in-the-Loop） | 🟡 框架支持；产品化待补 |
| LLM 节点（`AddLLMNode` + model/tools resolve）| ✅ 已接线 |
| Tool 节点（`AddToolsNode`）| ✅ 已接线 |
| Agent 节点（按 catalog AgentName 动态创建）| ❌ 未实现 |
| Router 节点（条件路由）| 🟡 模板引用；Builder 未有独立 case |
| Function/FuncRef 节点（自定义代码） | ✅ 注册表驱动 |
| DAG 验证（循环检测） | ✅ 框架层 |

---

## 架构问题

### P1 — Data 层 checkpoint 耦合

**问题**：`internal/data.go` 直接绑定 trpc graph checkpoint saver（框架运行时类型），导致 data 层测试依赖 trpc-agent-go 框架。

**建议**：将 `provideGraphCheckpointSaver` 上移到 `internal/runtime` 或 Wire 阶段组装，data 层只提供 raw storage。

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| GRAPH-P1-01 | `internal/data.go` 绑定 trpc graph checkpoint — data 层运行时耦合 | `provideGraphCheckpointSaver` 上移到 Wire |
| GRAPH-P1-02 | Agent 节点（按 catalog AgentName 动态创建）未实现；Router 节点无独立 builder 分支 | 在 `node_wiring.go` 中增加 `case "agent"` / `case "router"` |
| GRAPH-P1-03 | `internal/biz/graph.go`（583 行）和 `internal/service/graph.go`（940 行）均无专项测试 | 补 biz/service 层 graph 测试 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| GRAPH-P2-01 | Checkpoint 产品化：用户无法从 UI 查看/恢复检查点 | 规划 Checkpoint 管理 UI |
| GRAPH-P2-02 | HITL 节点：需定义 `await_user_reply` 在 Graph 中的配置和 UX | 与 Chat AwaitUserReply 复用 |
| GRAPH-P2-03 | Function 节点（自定义代码）需与 CodeExecutor 集成 | 规划 FunctionNode → CodeExecutor 接线 |
| GRAPH-P2-04 | Graph 适配层无专项测试 | 补 Aranea 侧适配层测试 |

---

## 前端架构

```
GraphsPage.vue — 列表 + 进入
GraphEditorPage.vue — Vue Flow 画布
    ├─ GraphEditorCanvas.vue — 拖拽编辑
    ├─ GraphNodePalette.vue — 节点类型选择
    └─ GraphPropertyPanel.vue — 属性配置
GraphRunPage.vue — 执行态 + ExecutionSummary
stores/graph — 状态管理
features/graph/api.ts — HTTP 门面
```

**状态**：前端框架完整；节点属性面板是最大缺口。

---

## 建议优化路径

1. 将 Graph checkpoint provider 从 data.go 上移到 Wire（P1 架构修复）。
2. 实现 Agent 节点（`case "agent"` + catalog 动态装配）和 Router 节点（P1）。
3. 补 biz/service/adapter 层测试（P1）。
4. 规划 Checkpoint 管理 UI 和 HITL 节点配置（P2）。
