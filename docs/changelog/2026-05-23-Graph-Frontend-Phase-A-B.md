# Graph 前端 Phase A/B — 运行态闭环 + 设计态增强

**日期**：2026-05-23  
**模块**：Graph (36) · 前端

## 摘要

按 `36-graph-development.md` Sprint 1–2 落地 Graph UI 优化：运行态 WS 接线、HITL 对话框、执行摘要侧栏；设计态模板/校验/布局持久化；分层对齐 `frontend-guide.md`。

## Phase A — 运行态闭环

| 项 | 变更 |
|----|------|
| A-1 | 新增 `features/graph/runtime/`：`graphExecutionProjection.ts`、`useGraphExecutionStream.ts` |
| A-2 | `useGraphRunPage` 订阅 WS，`execNodeStates` 实时驱动画布 |
| A-3 | 新增 `GraphRunSidebar.vue`：执行详情 + ExecutionSummary + 步骤时间线 |
| A-4 | 新增 `GraphHitlDialog.vue`：`lineage_id` / `checkpoint_id` / `resume_map` 恢复 |
| A-5 | `useGraphStream` 增强：输出 `executionSummary`、`interrupt`、`clearInterrupt` |

## Phase B — 设计态增强

| 项 | 变更 |
|----|------|
| B-1 | `GraphTemplatePicker` + store `loadTemplates` / `instantiateTemplate` |
| B-2 | `GraphValidationPanel`；保存后自动 `validateGraph` |
| B-3 | 节点布局写入 `metadata.layout`；`GraphEditorCanvas` 拖拽持久化 |
| B-5 | 编辑器加载 Tools catalog 填充 tool 下拉 |
| B-6 | `hitl` 节点类型注册；Run 页 `readOnly` 画布 |

## 架构

- **SRP**：Canvas 纯展示；Store 管 API；composable 编排 WS + 页面状态
- **影响域**：`useEnvelopeStream.ts`（graph 分支 metadata）、`stores/graph`、`components/graph/*`、`pages/Graph*Page`

## 验证

```bash
cd web && pnpm test -- --run src/features/graph/runtime/graphExecutionProjection.spec.ts
cd web && pnpm build
```
