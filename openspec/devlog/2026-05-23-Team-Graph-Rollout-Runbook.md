# Team Graph Runtime Rollout Runbook

> **模块**：M53 Phase 5–7 · TG-RT-FLAG / TG-RT-RETIRE  
> **日期**：2026-05-23（Phase 7 更新）  
> **指标**：`aranea_team_graph_runtime_total{outcome,reason}` · Grafana「Team Runtime Path Distribution」

> **Phase 7（2026-05-23）**：Graph 为默认执行路径；`ARANEA_TEAM_GRAPH_RUNTIME` 空值即开启。Native 仅 `ARANEA_TEAM_NATIVE=1` 应急。详见 [changelog Phase 7](../changelog/2026-05-23-Team-Graph-M53-Phase7.md)。

## 目标

扩大 GraphAgent 执行占比；Phase 7 后 **不再 silent Native fallback**，监控 `native_emergency` / `native_fallback` 应接近 0。

## 前置条件

- [ ] `go test ./internal/team/... -run 'Parity|ParityRunE2E'` 六 mode 编译 + stub E2E 通过
- [ ] Grafana 面板可见 graph / native / fallback 三条曲线
- [ ] FlowLog 无 `team.graph_runtime.*` 异常尖峰

## Canary 步骤

| 阶段 | 环境变量 | Team 配置 | 观察期 |
|------|----------|-----------|--------|
| 0 | `ARANEA_TEAM_GRAPH_RUNTIME=0` | 任意 | 基线（全 Native，需进程可构建 Native） |
| 1 | `ARANEA_TEAM_GRAPH_CANARY_PERCENT=5` | 可选：显式 `runtime_engine=graph` 强制入组 | 3 天 |
| 2 | `=50` | 同上 | 1 周 |
| 3 | `=100`（默认） | 存量默认 Graph | 1 周 |
| 4 | Phase 7 ✅ | 新 Team 默认 graph + env gate 默认开 | — |

### 自动分桶（`ARANEA_TEAM_GRAPH_CANARY_PERCENT`）

- **空或未设置** = `100`（全量 Graph，Phase 7 默认）
- **1–99**：`hash(team_id) % 100 < percent` 的 Team 走 Graph；桶外 Team 自动走 **Native holdout**（`native_canary_holdout` 指标）
- **显式 `runtime_engine=graph`**：始终 Graph（手动 Canary 入组，不受分桶限制）
- **显式 `runtime_engine=native`**：始终 Native holdout

```bash
# Stage 1：5% 自动 Graph + 95% Native holdout
export ARANEA_TEAM_GRAPH_CANARY_PERCENT=5
# 重启 admin

# 观察 native_fallback 应 ≈ 0；native_canary_holdout 应 ≈ 95% runs
# Grafana「Team Runtime Path Distribution」→ native canary holdout 曲线
```

## 监控项

1. `native_fallback` rate 不应持续高于 graph success 的 5%
2. `team_run.graph_execution_id` 填充率应随 canary 比例上升
3. Observatory Activity 时间线无断片（Kanban「进行中」列 history ≥ 1）

## 回滚

```bash
# 立即关闭 Graph 路径（无需改代码）
export ARANEA_TEAM_GRAPH_RUNTIME=0
# 重启 admin 进程
```

可选：关闭 Activity 持久化（仅内存时间线）

```bash
export ARANEA_OBS_PERSIST=0
```

## 已知 diff（Native vs Graph）

- Graph 路径额外发射 `graph_node_*` envelope（Native 无）
- Adaptive/Swarm transfer 边在 runtime 编译时剥离（preview 仍保留 overlay）
- 终态 `team_summary` 字段应对齐；token 差异 ±5% 可接受（cache/并行调度）
