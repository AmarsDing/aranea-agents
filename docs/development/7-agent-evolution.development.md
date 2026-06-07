# Agent 进化 — 开发计划

> **版本**：2026-06-06 | **状态**：✅ API + 指标 + Scanner + Learning Loop；🟡 趋势图 / diff / 护栏未通
> **需求**：[7 agent-evolution.md](./7%20agent-evolution.md) · **设计**：[7 agent-evolution.design.md](./7%20agent-evolution.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-07

---

## 1. 模块定位

Agent 自我进化：运行指标、改进建议、应用/拒绝；运行时开关在 `AgentRuntimeSettings`。

**代码锚点**：
- `internal/biz/evolution.go` — `EvolutionUsecase`
- `internal/data/evolution_metrics_repo.go` — 从 `tool_invocations` 聚合成功率/检索质量
- `internal/service/agent.go` — Evolution RPC（挂在 `AgentService`）
- `web/src/components/agents/AgentEvolutionPanel.vue` — 设置页「进化」Tab
- `web/src/features/agents/useAgentEvolutionPanel.ts` — evolution panel composable
- `web/src/features/agents/useAgentEvolutionSettings.ts` — evolution settings composable
- `web/src/features/agents/useLearningLoopPanel.ts` — learning loop composable
- `web/src/components/agents/AgentLearningLoopPanel.vue` — learning loop panel
- `web/src/components/agents/LearningLoopOverview.vue` — overview
- `web/src/components/agents/LearningObservationList.vue` — observations
- `web/src/components/agents/LearningPatternList.vue` — patterns
- `web/src/components/agents/LearningProposalList.vue` — proposals
- `web/src/features/agents/api.learning.ts` — learning loop API
- `web/src/stores/learningLoop/` — learning loop store
- `internal/agent/trpc_build.go` — 进化相关开关字段

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 指标 RPC | ✅ | `GetEvolutionMetrics` |
| 建议列表 / 应用 / 拒绝 | ✅ | `GetEvolutionSuggestions` / `Apply` / `Reject` |
| 指标真实计算 | ✅ | `evolutionMetricsRepo` 查 `tool_invocations`（非静态零值） |
| `EvolutionScanner` worker | ✅ | `internal/cronrunner/jobs/evolution_scanner.go`；Wire + `main` 启动；`EVOLUTION_SCANNER_DISABLED=1` 可关 |
| 自动生成建议 | ✅ | `EvolutionUsecase.ScanAll` / `ScanAgent` 阈值写入 `evolution_suggestions`（去重 pending 同 type） |
| `ApplySuggestion` type=prompt | ✅ | 写入 `AGENTS_CORE.md` / `AGENTS_*` 首匹配文件 |
| `ApplySuggestion` persona type | ✅ | 写入 `IDENTITY.md` ## Persona section |
| `ApplySuggestion` prompt type | ✅ | 写入 `AGENTS_CORE.md` or first match |
| Learning Loop API | ✅ | observation/pattern/proposal CRUD + run |
| `diff_preview` 生成 | ❌ | 多为空 |
| SOUL.md 自动演化 | ❌ | 开关有，无 Scanner 写文件 |
| 护栏运行时 | ❌ | `guardrail_*` 字段未参与扫描 |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 进化四项开关 | ✅ | `AgentEvolutionPanel` + settings 绑定 |
| 时间范围 7d/30d/90d | ✅ | `q-btn-toggle` |
| 指标卡片 | ✅ | 成功率 / 检索质量 / 待处理建议数 |
| 建议列表 | ✅ | 应用/拒绝按钮 |
| Learning loop panel | ✅ | 4 sub-components（Overview/Observation/Pattern/Proposal） |
| Evolution panel | ✅ | with metrics dashboard, suggestion list, guardrails |
| 折线图表 | ❌ | 无 ECharts/Chart.js（有序列数据 `*_series` 未绘图） |
| Diff preview | 🟡 | `AIRefineButton` has diff, but evolution suggestion `diff_preview` still empty |
| 指标/建议空态 | 🟡 | 零值时仍展示卡片，可增强文案 |

---

## 3. 差距与优化

| ID | 优先级 | 待优化项 |
|----|--------|----------|
| EVO-01 | P2 | `evolution_scanner.go` + Wire 启动 + `safego` | ✅ |
| EVO-02 | P2 | 阈值生成 `EvolutionSuggestion` | ✅ |
| EVO-03 | P3 | `diff_preview` 生成（🟡 `AIRefineButton` has diff, evolution suggestion still empty） |
| EVO-04 | P3 | SOUL.md 自动演化（仅风格段）（❌ 但 PGO V2 已 deprecated SOUL.md） |
| EVO-05 | P3 | 护栏运行时 + 前端图表 |
| EVO-06 | P3 | Learning Loop 与 Evolution 对齐（共享 Apply 逻辑） |
| EVO-07 | P3 | PGO V2 影响 — SOUL.md deprecated，evolution persona target 变更 |

---

## 4. 验收标准

- [x] 指标 API 可返回基于 `tool_invocations` 的数据
- [x] 前端可展示指标卡片与建议列表
- [x] EvolutionScanner 周期性运行（默认 30min）
- [x] 建议可由扫描自动生成（非仅手工/种子）
- [x] Learning Loop panel 可用（Overview/Observation/Pattern/Proposal）
- [x] `ApplySuggestion` persona type 写入 IDENTITY.md
- [ ] 前端趋势图展示 `tool_success_series` / `retrieval_quality_series`
- [ ] Evolution suggestion `diff_preview` 非空
- [ ] 护栏运行时参与扫描

---

## 5. 架构备注

### 5.1 Scanner

| 组件 | 路径 |
|------|------|
| 扫描逻辑 | `internal/biz/evolution_scan.go` |
| Worker | `internal/cronrunner/jobs/evolution_scanner.go` |
| 注册 | `cmd/admin/wire.go` → `provideEvolutionScanner` |

`ScanAgent` 读取 `evolution_suggestions_enabled` / `evo_enabled` 与 `evo_min_*`；失败时 `ScanAll` 聚合错误并由 worker 打 Warn 日志。**未做**：per-agent 节流 TTL、`evo_auto_apply` 自动应用。

### 5.2 Learning Loop

| 组件 | 路径 |
|------|------|
| API | `web/src/features/agents/api.learning.ts` |
| Store | `web/src/stores/learningLoop/` |
| Panel | `web/src/components/agents/AgentLearningLoopPanel.vue` |
| Overview | `web/src/components/agents/LearningLoopOverview.vue` |
| Observations | `web/src/components/agents/LearningObservationList.vue` |
| Patterns | `web/src/components/agents/LearningPatternList.vue` |
| Proposals | `web/src/components/agents/LearningProposalList.vue` |
| Composable | `web/src/features/agents/useLearningLoopPanel.ts` |

---

## 6. 依赖

| 模块 | 说明 |
|------|------|
| 5 设置 | 进化开关 |
| 6 文件 | Apply 建议改 IDENTITY.md / AGENTS_CORE.md（PGO V2 已 deprecated SOUL.md） |
| 23 Tools | `tool_invocations` 数据源 |
| Learning Loop | observation/pattern/proposal 与 evolution 建议共享 Apply 逻辑 |
