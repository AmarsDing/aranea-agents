# ADR-E: 评测态 profile（P3-4）——声明式固定模型/工具/生成参数，与生产同内核

## 状态：已接受（2026-08-14）

## 背景

差距矩阵（plan §三）：DSH 以 preset（Standard/Minimal/Creator 共享同一内核）保证评测态与生产态一致；本项目评测域（`internal/evaluation/`）只覆盖**单 agent chat** 路径（chatRunnerAdapter → ChatService turn），Team/Graph 编排运行没有评测态概念——

1. **模型不可复现**：run 级联（P2-1 cascade）、model_router 插件、cost_guard 等动态路由使同一 definition 两次运行可能走不同模型；
2. **生成参数不可固定**：无 run 级 seed/temperature 钉住机制（GenerationConfig 在 agent build 期固化）；
3. **工具集不可约束**：成员工具集由各自 agent 构建期装配，评测无法声明「本次只允许这些工具」。

理论支持：

- **DSH preset 原则**：评测态与生产态共享同一内核，profile 是声明式组合而非 fork 内核；
- **DFAH（IBM, arXiv:2601.15322）**：4700+ 次 agentic run 实证——determinism 与 accuracy 不相关（r=-0.11, p=0.63），确定性必须作为**独立维度**测量与保障；钉住确定性旋钮（seed/temp）是测量前提；
- **业界 eval 实践**（genai-notes / pysolutions / Inspect AI 对比）：固定 temperature=0 + seed、每任务 3-5 次运行取中位数+方差、trajectory 断言（must_call/must_not_call/max_steps）优于黄金输出比对——三者都以「运行配置可声明、可复现」为前提；
- **DSH fail-loud 能力缝**：能力不满足即报错，禁止静默降级——评测在错误模型上跑出「看似有效」的结果是最差结局。

机制确认（代码核实）：

| 机制 | 位置 | 性质 |
|------|------|------|
| run 级 `ModelSelector` 优先于 agent build 级 selector | `pkg/trpc-agent-go/agent/invocation.go:700` | 单次赋值，后写胜出 |
| graph runtime 把父 RunOptions **整结构**传播到成员节点 | `pkg/trpc-agent-go/graph/state_graph.go:3759` | 一次安装全链生效 |
| `WithModelRequestExtraFields` 合并进每个模型请求且覆盖 model 级字段 | `invocation.go:279` + `internal/flow/processor/basic.go:85` | seed/temperature 钉住通道 |
| run 级 `ToolFilter` 由 llmflow 消费；`NewIncludeToolNamesFilter` 现成 | `internal/flow/llmflow/llmflow.go:1481`、`tool/filter.go:70` | 可见性即权限 |
| 所有图执行路径收敛于 Team 入口 run-opts 装配点 | `internal/team/runner_team_trpc.go:262-281`（team_graph runtime、DAG 派步团队均经此） | 单一接入口 |
| 既有范式 | P2-1 `ModelCascadeDef` → `cascadeRunOption`（team_cascade.go） | 声明式 definition JSON → run-level option |

## 决策

### D1: `EvalProfileDef` 挂在 Team Definition JSON（`eval_profile` 字段）

```json
{
  "eval_profile": {
    "provider": "openai",
    "model": "gpt-4o",
    "tool_allowlist": ["search_docs", "get_deliverable"],
    "extra_model_fields": { "seed": 42, "temperature": 0 }
  }
}
```

- `provider`/`model`：固定**全部成员**（含 leader/synthesizer）的调用模型；
- `tool_allowlist`：工具可见性白名单（空 = 不过滤；框架工具 transfer/knowledge_search 按框架契约不受滤）；
- `extra_model_fields`：provider 级请求字段（seed/temperature/reasoning_effort），经 `WithModelRequestExtraFields` 注入每个请求；
- 随 `DefinitionSnapshotJSON` 持久化——评测运行**自描述、可复现**（哪个 profile 产出哪个 run 可审计）。

### D2: 同内核生效——入口层装配 run-level options，不 fork 内核

`Runner.evalProfileRunOptions(ctx, def, sessID)` 在 `runner_team_trpc.go` run-opts 装配点安装：

1. **模型钉住**：新增 `agent.PinnedModelSelector(provider, model, catalog, rt, lg)`——任意 AgentName（leader 与 member 一视同仁）都返回固定模型；
   - **fail-loud**：模型构建失败返回 error 让 run 快速失败（selector 契约：返回 error 在请求构建前失败当前调用）。与 cascade 的保守 nil 语义**刻意不同**——评测跑错模型产出看似有效的无效结果，比失败更糟（DSH fail-loud 能力缝）；
   - 配置残缺（provider/model 任一为空）= 该子项不生效（nil option），不报错——评测 profile 允许只钉工具/参数。
2. **优先级**：`eval_profile` 与 `model_cascade` 同时配置时 **eval 胜出**（跳过 cascade 安装）+ warn 进程日志。cascade 是成本优化策略，与可复现性目标互斥；显式配置冲突不应静默叠加。
3. **生成参数钉住**：`WithModelRequestExtraFields(extra_model_fields)`（框架保证请求级覆盖 model 级）。
4. **工具白名单**：`WithToolFilter(tool.NewIncludeToolNamesFilter(...))`（可见性即权限，llmflow 消费）。

### D3: FlowLog 审计

`team.eval_profile.applied`（done）：记录 pinned provider/model、白名单工具数、extra 字段键集合——评测轨迹可审计（K1 入口节点）。

### D4: 覆盖范围与边界

- **覆盖**：Team 入口即全图覆盖——team_graph runtime 与 DAG 派步团队均收敛于 Team runner 的 run-opts 装配点；graph runtime 传播父 RunOptions 到成员节点；
- **不覆盖**：单 agent chat 评测（evaluation 域既有 chat runner + judge GenerationConfig 已覆盖）；judge/user-simulator 侧确定性由评测域自身管理；
- **非目标**：追求严格逐 token 复现——provider 侧 seed 仅尽力而为（DFAH：前沿模型 determinism 50-96%），评测结论应基于多次运行统计（与 `internal/evaluation/multirun.go` pass@k 既有能力互补）。

## 后果

正面：
- 编排评测可复现、自描述（definition snapshot 即 profile 档案）；
- 零内核 fork（DSH preset 原则），实现复用框架 run-level option 三件套 + P2-1 装配范式；
- fail-loud 防止「错误模型上的有效假象」评测结论。

负面：
- 钉住模型绕过 cost_guard/cascade → 评测运行成本可能高于生产；可接受（评测为显式 opt-in）；
- `tool_allowlist` 过窄可能使成员无法完成职责 → 评测配置错误以 run 失败形式暴露，属预期行为。

## 替代方案

| 方案 | 未选原因 |
|------|---------|
| 复用 openclaw `runtimeprofile.Profile` | 该机制面向子代理隔离（workspace/凭证/技能仓），经请求扩展解析，与 Team definition 声明式模型不匹配；引入会把 OpenClaw 概念泄漏进 team 域 |
| 评测域内新建 Team 评测执行器 | 违反同内核原则（fork 内核 = 评测与生产行为漂移，DSH preset 反例） |
| build 级 selector 钉模型 | build 级 selector 会被 run 级 cascade 覆盖且 agent 构建有缓存共享（BuildCache），钉住会污染生产运行 |
| cascade 式保守 nil 回退 | 评测语义要求 fail-loud；静默回退产生无效评测结论（DSH 能力缝原则） |
