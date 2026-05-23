# Team Graph M53 Phase 7 Review & 优化清单

> **评分**：80 / 100 | **风险等级**：P1  
> **审查时间**：2026-05-23  
> **代码锚点**：`internal/team/` · `internal/service/team_dead_letter.go` · `web/src/components/chat/ChatBackgroundJobsPanel.vue`  
> **对照**：[`m53-graph-team-multiagent-enterprise-blueprint.md`](../需求/53%20team-graph-orchestration.design.md#附录企业级蓝图与-ai-落地指南)

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 17 | 20 | Phase 7 主链已通；TG-RT-PARITY 仍为 build 级；Graph 首跑 step 语义待收敛 |
| 架构一致性 | 22 | 25 | biz/team/service 边界正确；Coordinator 单例 + Finisher 注入合理 |
| 后端实现质量 | 17 | 20 | Resume 对齐已补；超时/evict/Resolve 校验本轮优化 |
| 前端实现质量 | 13 | 15 | FP-04 死信 Tab MVP |
| 测试与验证 | 6 | 10 | 单测覆盖主路径；缺 run-level parity E2E |
| 文档一致性 | 5 | 10 | changelog / execution-plan 已同步（本轮） |

---

## 问题清单（按优先级）

### P1 — 本轮已修复

| ID | 问题 | 影响域 | 修复 |
|----|------|--------|------|
| **BL-01** | Graph HITL defer 前对全部 member bulk `persistStep`，与 graph 事件语义冲突 | Team Run steps / Summary / Observatory | defer 判定移到 bulk 之前；HITL 时跳过 bulk |
| **BL-02** | Resume watch 30min 超时静默退出，run 可能永久 `running` | Team Run 生命周期 | 超时 → `finalizeTeamRun(failed)` |
| **BE-02** | `FinalizeGraphTeamRun` 成功未写 `output_preview` / tokens | team_summary / RPC | 从 `team_run_steps` 聚合回填 |
| **ARCH-02** | Coordinator `sessions` map 无 evict | 内存 / 长跑 | finalize 后 `evictSession` |
| **BE-01** | `PersistResumeGraphStep` 失败静默 | 排障 | FlowLog warn |
| **BE-05** | `ResolveTaskDeadLetter` 无 pending 校验 | FP-04 Admin | 仅 pending 可 resolve；已 resolved 幂等返回 |
| **DOC-01** | Phase 7 changelog / execution-plan 与实现脱节 | 文档 | 同步已知缺口与验收 |

### P2 — 下一迭代

| ID | 问题 | 状态 |
|----|------|------|
| **BL-03** | Graph 非 HITL bulk persist | ✅ `StartGraphStepWatch` + Native-only bulk |
| **TG-RT-PARITY** | run 级 token/steps/WS 对比 | ⏳ build 级 ✅ · [diff 文档](../需求/53-team-graph-orchestration-development.md#9-native-vs-graph-paritytg-rt-parity) |
| **ARCH-01** | Finisher 暴露 session | ✅ `GraphRunStepContext` DTO |
| **FP-04-UI** | 死信无跳转 / payload | ✅ Observatory 深链 + payload 展开 |
| **FP-02** | Circuit breaker half-open | ✅ 状态机补全 |

---

## 回归风险说明

| 变更 | 风险 | 缓解 |
|------|------|------|
| BL-01 defer 顺序 | Graph HITL run 的 steps 变少（正确行为） | 依赖 resume finisher + ActivityFlusher；已有 coordinator 单测 |
| BL-02 超时 finalize | 长跑 HITL 可能标记 error | 30min 与 graph watch 一致；FlowLog + run error_message |
| BE-05 Resolve | 重复 resolve 返回原 row | 幂等，不报错 |
| Native bulk persist | 无变更（`graphExecID==""` 路径） | 保持原循环 |

---

## 验证

```bash
go test ./internal/team/... ./internal/service/... -run 'TeamGraph|TaskDeadLetter|Parity|Finisher' -count=1
go build ./...
```
