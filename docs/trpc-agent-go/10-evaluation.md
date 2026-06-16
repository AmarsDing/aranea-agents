# 评估（Evaluation）— 框架对齐分析

> 模块路径：`pkg/trpc-agent-go/evaluation/`
> 项目实现路径：`internal/evaluation/`、`internal/biz/evaluation/`、`internal/data/evaluation.go`
> 当前对齐度：★★★☆☆（原报告 ☆☆☆☆☆，经代码审查修正）

---

## 一、框架能力全景

### 1.1 核心接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `AgentEvaluator` | `Evaluate(ctx, evalSetID, ...Option) (*EvaluationResult, error)` | 顶层评估入口，编排推理+评估两阶段 |
| `AgentEvaluator` | `Close() error` | 释放资源 |
| `evalset.Manager` | `Get/Create/List/Delete/GetCase/AddCase/UpdateCase/DeleteCase/Close` | 评估集 CRUD（9 方法） |
| `metric.Manager` | `List/Get/Add/Delete/Update/Close` | 指标 CRUD（6 方法） |
| `evalresult.Manager` | `Save/Get/List/Close` | 结果持久化（4 方法） |
| `evaluator.Evaluator` | `Name()/Description()/Evaluate(ctx, actuals, expecteds, metric) (*EvaluateResult, error)` | 评估器核心接口 |
| `evaluator.Registry` | `Register(name, Evaluator)/Get(name)/List()` | 评估器注册中心 |
| `metric.Registry` | `RegisterTextCompare/RegisterJSONCompare/RegisterToolTrajectoryCompare/RegisterFinalResponseCompare/RegisterRougeTokenizer/Resolve` | 准则运行时注册中心（5 类扩展点） |
| `service.Service` | `Inference(ctx, *InferenceRequest, ...Option) ([]*InferenceResult, error)` | 推理阶段服务 |
| `service.Service` | `Evaluate(ctx, *EvaluateRequest, ...Option) (*EvalSetRunResult, error)` | 评估阶段服务 |
| `usersimulation.Simulator` | `Start(ctx, *StartRequest) (Conversation, error)` | 用户模拟器入口 |
| `usersimulation.Conversation` | `Next(ctx, *TurnRequest) (*Decision, error)/Close()` | 模拟对话控制 |

### 1.2 关键类型

| 类型 | 说明 |
|------|------|
| `EvalSet` | 评估集，含 `EvalSetID`、`Name`、`EvalCases[]` |
| `EvalCase` | 评估用例，含 `EvalID`、`EvalMode`、`Conversation[]`、`ConversationScenario`、`SessionInput`、`Rubrics` |
| `Invocation` | 单轮调用，含 `UserContent`、`FinalResponse`、`Tools[]`、`IntermediateResponses` |
| `Tool` | 工具调用，含 `ID`、`Name`、`Arguments`、`Result` |
| `ConversationScenario` | 对话场景，含 `Driver`、`StartingPrompt`、`ConversationPlan`、`StopSignal`、`MaxAllowedInvocations` |
| `SessionInput` | 会话初始化，含 `AppName`、`UserID`、`State` |
| `EvalCaseRubric` | 用例级别 Rubric，含 `MetricName`、`ID`、`Content`、`Description`、`Type` |
| `EvalMetric` | 评估指标，含 `MetricName`、`EvaluatorName`、`Threshold`、`Criterion` |
| `Criterion` | 评估准则，含 `ToolTrajectory`、`FinalResponse`、`LLMJudge` 三类 |
| `ToolTrajectoryCriterion` | 工具轨迹比较，含 `DefaultStrategy`、`ToolStrategy`、`OrderSensitive`、`SubsetMatching` |
| `FinalResponseCriterion` | 最终响应比较，含 `Text`、`JSON`、`Rouge`、`XML` |
| `LLMCriterion` | LLM Judge 准则，含 `Rubrics`、`JudgeModel`、`JudgeRunnerOptions`、`Template` |
| `EvaluationResult` | 顶层结果，含 `AppName`、`EvalSetID`、`OverallStatus`、`EvalCases`、`EvalResult` |
| `EvaluationCaseResult` | 用例结果（多轮聚合），含 `EvalCaseID`、`OverallStatus`、`MetricResults`、`RunDetails` |
| `EvalSetResultSummary` | 多轮结果摘要，含 `OverallStatus`、`NumRuns`、`RunSummaries`、`EvalCaseSummaries` |
| `EvalStatus` | 状态枚举：`unknown`/`passed`/`failed`/`not_evaluated` |

### 1.3 扩展点

| 扩展点 | 机制 | 适用场景 |
|--------|------|---------|
| 实现 `evaluator.Evaluator` 接口 | 自定义评估器注册到 Registry | 业务特定评估逻辑（如合规性检查） |
| `metric.Registry.RegisterTextCompare` | 自定义文本比较函数 | 替代 exact/contains/regex 匹配 |
| `metric.Registry.RegisterJSONCompare` | 自定义 JSON 比较函数 | 特定 JSON 字段比较逻辑 |
| `metric.Registry.RegisterToolTrajectoryCompare` | 自定义工具轨迹比较函数 | 自定义工具匹配策略 |
| `metric.Registry.RegisterFinalResponseCompare` | 自定义最终响应比较函数 | 自定义响应评分逻辑 |
| `metric.Registry.RegisterRougeTokenizer` | 自定义 ROUGE 分词器 | 中文分词（jieba）等 |
| `service.Callbacks` | 8 个生命周期回调 | 进度追踪、监控、日志 |
| `usersimulation.Simulator` 接口 | 自定义用户模拟器 | 特定用户行为模拟 |
| `evalset/recorder.Plugin` | 运行时录制 Agent 交互 | 自动生成评估用例 |
| `server/evaluation.RouteRegistrar` | 自定义 HTTP 路由 | 扩展评估 API |

### 1.4 配置选项

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithEvalSetManager(m)` | 评估集管理器 | `evalsetinmemory.New()` |
| `WithMetricManager(m)` | 指标管理器 | `metricinmemory.New()` |
| `WithEvalResultManager(m)` | 结果管理器 | `evalresultinmemory.New()` |
| `WithRegistry(r)` | 评估器注册中心 | `registry.New()` |
| `WithMetricRegistry(r)` | 准则运行时注册中心 | `metricregistry.New()` |
| `WithEvaluationService(s)` | 自定义评估服务 | 自动创建 `local.Service` |
| `WithUserSimulator(sim)` | 用户模拟器 | nil |
| `WithCallbacks(c)` | 生命周期回调 | nil |
| `WithJudgeRunner(judge)` | Judge Runner | nil |
| `WithJudgeRunnerNumSamples(n)` | Judge 采样次数 | nil |
| `WithExpectedRunner(r)` | 期望输出 Runner | nil |
| `WithNumRuns(n)` | 重复运行次数 | 1 |
| `WithEvalCaseIDs(ids...)` | 限定评估用例 | 空=全部 |
| `WithNumRunsParallelEnabled(bool)` | 多 Run 并行 | false |
| `WithEvalCaseParallelism(n)` | 用例并行度 | 1 |
| `WithEvalCaseParallelInferenceEnabled(bool)` | 推理并行 | false |
| `WithEvalCaseParallelEvaluationEnabled(bool)` | 评估并行 | false |
| `WithRunDetailsEnabled(bool)` | 记录推理详情 | false |
| `WithRunOptions(opt...)` | Agent.Run 选项 | nil |

### 1.5 框架内置实现

| 实现 | 路径 | 说明 |
|------|------|------|
| `evalsetinmemory.New()` | `evaluation/evalset/inmemory/` | 内存评估集存储 |
| `evalsetlocal.New()` | `evaluation/evalset/local/` | 本地文件评估集存储（JSON） |
| `evalsetmysql.New()` | `evaluation/evalset/mysql/` | MySQL 评估集存储 |
| `metricinmemory.New()` | `evaluation/metric/inmemory/` | 内存指标存储 |
| `metriclocal.New()` | `evaluation/metric/local/` | 本地文件指标存储（JSON） |
| `metricmysql.New()` | `evaluation/metric/mysql/` | MySQL 指标存储 |
| `evalresultinmemory.New()` | `evaluation/evalresult/inmemory/` | 内存结果存储 |
| `evalresultlocal.New()` | `evaluation/evalresult/local/` | 本地文件结果存储（JSON） |
| `evalresultmysql.New()` | `evaluation/evalresult/mysql/` | MySQL 结果存储 |
| `registry.New()` | `evaluation/evaluator/registry/` | 自动注册 9 个内置评估器 |
| `tooltrajectory.New()` | `evaluation/evaluator/tooltrajectory/` | 工具轨迹评估器（静态） |
| `finalresponse.New()` | `evaluation/evaluator/finalresponse/` | 最终响应评估器（静态） |
| `llmfinalresponse.New()` | `evaluation/evaluator/llm/finalresponse/` | LLM 最终响应评估器 |
| `rubriccritic.New()` | `evaluation/evaluator/llm/rubriccritic/` | Rubric 批评者评估器 |
| `rubricresponse.New()` | `evaluation/evaluator/llm/rubricresponse/` | Rubric 响应评估器 |
| `rubricreferencecritic.New()` | `evaluation/evaluator/llm/rubricreferencecritic/` | Rubric 参考批评者评估器 |
| `rubricknowledgerecall.New()` | `evaluation/evaluator/llm/rubricknowledgerecall/` | 知识召回评估器 |
| `hallucination.New()` | `evaluation/evaluator/llm/hallucination/` | 幻觉检测评估器 |
| `llmtemplate.New()` | `evaluation/evaluator/llm/template/` | 模板化 LLM Judge 评估器 |
| `usersimulation.New(simRunner)` | `evaluation/usersimulation/` | LLM 用户模拟器 |
| `recorder.New()` | `evaluation/evalset/recorder/` | 运行时录制器（Plugin） |
| `service.NewCallbacks()` | `evaluation/service/` | 回调链构造器 |
| `PassAtK(n, c, k)` | `evaluation/pass.go` | Codex/HumanEval 标准 pass@k |
| `PassHatK(n, c, k)` | `evaluation/pass.go` | pass^k 统计指标 |
| PromptIter Engine | `evaluation/workflow/promptiter/` | Prompt 自动优化工作流 |

---

## 二、项目实现现状

### 2.1 框架接口实现情况

| 框架接口/功能 | 项目实现 | 合规性 | 说明 |
|--------------|---------|--------|------|
| `AgentEvaluator.Evaluate()` | ✅ `FrameworkBridge.Execute()` | ✅ 完全实现 | 通过 `trpceval.New()` + `Evaluate()` 调用 |
| `evalset.Manager`（inmemory） | ✅ `evalsetinmemory.New()` | ✅ 完全实现 | 每次运行新建实例 |
| `metric.Manager`（inmemory） | ✅ `metricinmemory.New()` | ✅ 完全实现 | 每次运行新建实例 |
| `evalresult.Manager`（inmemory） | ✅ `evalresultinmemory.New()` | ✅ 完全实现 | 每次运行新建实例 |
| `evaluator.Registry` | ✅ `RegisterBuiltinEvaluators()` | ✅ 完全实现 | 注册全部 9 个内置评估器 |
| `metric.Registry`（criterion） | ✅ `registerFrameworkMetrics()` | ⚠️ 部分使用 | 使用 criterion 构造指标，但未注册自定义 Compare 函数 |
| `PassAtK` / `PassHatK` | ✅ `computePassMetrics()` | ✅ 完全实现 | 使用框架 `ParsePassNC` + `PassAtK` + `PassHatK` |
| `WithNumRuns` / 并行选项 | ✅ `MultiRunConfig.ToOptions()` | ✅ 完全实现 | 完整映射 6 个 Option |
| `usersimulation.Simulator` | ✅ `NewLLMUserSimulator()` | ✅ 完全实现 | 使用框架 `usersimulation.New()` |
| `WithUserSimulator` | ✅ `resolveUserSimulator()` | ✅ 完全实现 | 根据条件注入 |
| `runner.Runner` 适配 | ✅ `chatRunnerAdapter` | ✅ 完全实现 | 实现 `runner.Runner` 接口 |
| `service.Callbacks` | ❌ 未使用 | ❌ 不合规 | 框架 8 个生命周期回调均未使用 |
| `WithJudgeRunner` | ❌ 未使用 | ❌ 不合规 | 项目自建 LLMJudge 替代 |
| `WithExpectedRunner` | ❌ 未使用 | ❌ 不合规 | 未启用期望输出自动生成 |
| `WithEvaluationService` | ❌ 未使用 | ❌ 不合规 | 使用框架默认 Service |
| `evalset/recorder.Plugin` | ❌ 未使用 | ❌ 不合规 | 未启用运行时录制 |
| `EvalCase.SessionInput` | ❌ 未使用 | ❌ 不合规 | 未设置会话初始化参数 |
| `EvalCaseRubric` | ❌ 未使用 | ❌ 不合规 | 未使用用例级别 Rubric |
| `metric.Registry` 自定义注册 | ❌ 未使用 | ❌ 不合规 | 未注册自定义 Compare/Tokenizer |
| PromptIter 工作流 | ❌ 未使用 | ❌ 不合规 | 框架完整 Prompt 优化能力未启用 |
| `server/evaluation` HTTP 服务 | ❌ 未使用 | ❌ 不合规 | 项目自建 gRPC/HTTP 服务 |
| Langfuse 集成 | ❌ 未使用 | ❌ 不合规 | 未启用外部评估平台集成 |
| local/mysql 存储后端 | ❌ 未使用 | ❌ 不合规 | 仅使用 inmemory，每次运行新建 |

### 2.2 自建功能清单

| 自建功能 | 实现位置 | 替代框架功能 | 自建原因 |
|---------|---------|-------------|---------|
| 数据持久化（4 表 + Raw SQL） | `internal/data/evaluation.go` | 框架仅提供 inmemory/local/mysql | 项目使用 SQLite，框架无 SQLite 后端 |
| Biz 层 CRUD（Dataset/Case/Run/CaseResult） | `internal/biz/evaluation/evaluation.go` | 框架无业务层概念 | 项目需要独立的业务逻辑层 |
| Legacy 评估路径 | `internal/evaluation/runner_legacy.go`、`metrics.go` | 框架 AgentEvaluator | 降级路径，仅支持 4 个简单指标 |
| LLM-as-Judge | `internal/evaluation/llm_judge.go` | 框架 `WithJudgeRunner` + LLM 评估器 | 先于框架 LLM Judge 集成开发，更简单 |
| 脚本用户模拟器 | `internal/evaluation/scripted_simulator.go` | 框架无此功能 | 框架仅提供 LLM 驱动模拟器 |
| AfterTurn 自动评估触发器 | `internal/evaluation/after_turn.go` | 框架无此功能 | 项目特有需求：聊天后自动触发评估 |
| ChatRunner 适配器 | `internal/evaluation/chat_runner.go` | 框架 `runner.Runner` 接口 | 桥接项目 AgentRunner 到框架 Runner |
| Case 元数据解析 | `internal/evaluation/case_metadata.go` | 框架无此功能 | 扩展 eval_cases.metadata_json |
| EvalSet 适配 | `internal/evaluation/evalset_adapter.go` | 框架 `evalset.Manager` | 桥接 biz Case 到框架 EvalSet |
| 分数映射 | `internal/evaluation/scores.go` | 框架 `EvaluationResult` | 映射框架结果到 biz 字段 + scores_json |
| LLM 模型解析 | `internal/evaluation/eval_llm_resolve.go` | 框架 `criterion.llmJudge.judgeModel` | 项目统一模型解析链 |
| gRPC/HTTP 服务 | `internal/service/evaluation.go` | 框架 `server/evaluation` | 项目使用 Kratos 传输层 |
| DDL 初始化 | `internal/data/evaluation.go` EnsureEvalSchema | 框架 local/mysql 自管理 | 项目 SQLite 表需手动创建 |

### 2.3 未使用的框架功能

| 框架功能 | 未使用原因 | 是否需要启用 |
|---------|-----------|-------------|
| `service.Callbacks`（8 个生命周期回调） | 项目通过 safego 异步执行，未感知评估内部阶段 | 是 — 可用于进度追踪和监控 |
| `WithJudgeRunner` | 项目自建 LLMJudge，直接调用 LLM 模型 | 是 — 框架方案更完整（支持 Rubric/Hallucination 等） |
| `WithExpectedRunner` | 未了解此功能 | 评估中 — 可自动生成期望输出 |
| `EvalCaseRubric` | 未使用用例级别 Rubric | 是 — LLM Judge 评估的核心配置 |
| `EvalCase.SessionInput` | 项目通过 ChatRunnerAdapter 绕过 | 评估中 — 可配置会话初始状态 |
| `metric.Registry` 自定义注册 | 项目仅使用内置 criterion | 否 — 当前内置 criterion 足够 |
| PromptIter 工作流 | 未了解此功能 | 是 — 可实现 Prompt 自动优化闭环 |
| `evalset/recorder.Plugin` | 未了解此功能 | 是 — 可自动录制 Agent 交互生成评估用例 |
| `server/evaluation` HTTP 服务 | 项目使用 Kratos 传输层 | 否 — Kratos 服务层更符合项目架构 |
| Langfuse 集成 | 项目未使用 Langfuse | 否 — 暂无需求 |
| local/mysql 存储后端 | 项目使用 SQLite | 否 — 需自建 SQLite 后端或保持 inmemory |
| `WithEvaluationService` | 默认 Service 满足需求 | 否 — 无自定义需求 |

---

## 三、对比分析

### 3.1 框架优势（项目应采纳的）

| # | 框架优势 | 项目现状 | 对齐收益 |
|---|---------|---------|---------|
| 1 | **LLM Judge 评估器族**（7 个：rubric_critic/response/reference_critic/knowledge_recall/hallucination/final_response/template） | 项目自建简单 LLMJudge（单 prompt + 0-1 分数解析） | 评估质量大幅提升：支持 Rubric 多维度评分、幻觉检测、知识召回评估、模板化自定义 |
| 2 | **Callbacks 生命周期**（8 个钩子：Before/After Inference/Evaluate × Set/Case） | 项目无评估阶段感知，仅异步执行后写 DB | 可实现实时进度推送、Case 级监控、评估中断控制 |
| 3 | **PromptIter 工作流**（评估→反向传播→聚合→优化循环） | 项目无 Prompt 优化能力 | 实现 Prompt 自动优化闭环，减少人工调优成本 |
| 4 | **EvalSet Recorder**（Plugin 自动录制 Agent 交互为评估用例） | 项目手动创建评估用例 | 自动化用例生成，降低评估数据准备成本 |
| 5 | **EvalCaseRubric**（用例级别 Rubric 配置） | 项目 LLM Judge 使用全局固定 prompt | 每个 Case 可配置独立评分维度，评估更精准 |
| 6 | **WithJudgeRunner**（注入独立 Judge Runner） | 项目自建 LLMJudge 函数 | 框架自动管理 Judge 生命周期、采样、聚合 |
| 7 | **ConversationScenario + UserSimulation**（框架标准多轮对话评估） | 项目部分使用，但 LLM 模拟器初始化在 service 层 | 统一到框架标准模式，减少自建代码 |
| 8 | **local 存储后端**（JSON 文件持久化 EvalSet/Metric/Result） | 项目每次运行新建 inmemory 实例 | 评估配置和结果可跨运行持久化，支持历史对比 |

### 3.2 项目优势（框架缺失的）

| # | 项目优势 | 框架现状 | 建议处理 |
|---|---------|---------|---------|
| 1 | **SQLite 持久化**（4 表 + Raw SQL + Ent Schema） | 框架仅提供 inmemory/local/mysql 后端 | 贡献回框架 — 实现 `evalsetsqlite`/`metricsqlite`/`evalresultsqlite` |
| 2 | **脚本用户模拟器**（按预设脚本模拟用户消息） | 框架仅提供 LLM 驱动模拟器 | 贡献回框架 — 实现 `scriptedsimulation` 包 |
| 3 | **AfterTurn 自动评估触发器**（聊天后自动触发 + 频率控制） | 框架无此功能 | 贡献回框架 — 作为 Callback 或独立 Plugin |
| 4 | **Case 元数据扩展**（metadata_json 含 expected_tools/turns/user_simulation） | 框架 EvalCase 已有 Tools/Turns/Simulation 字段 | 保持自建适配层 — 项目元数据格式与框架不完全一致 |
| 5 | **LLM 模型解析链**（env → system_settings → catalog 自动选择 mini/flash） | 框架 `criterion.llmJudge.judgeModel` 需显式配置 | 评估中 — 可封装为框架 Option |
| 6 | **双路径架构**（Framework + Legacy 降级） | 框架无降级机制 | 保持自建 — 提升鲁棒性 |
| 7 | **人工标注**（human_pass/human_score/human_comment） | 框架无人工标注功能 | 贡献回框架 — 扩展 EvalCaseResult |

### 3.3 差异根因分析

| 差异点 | 根因 | 影响范围 |
|--------|------|---------|
| 自建 LLMJudge 而非使用框架 LLM 评估器 | **历史遗留** — LLMJudge 先于框架 LLM Judge 集成开发，当时框架 LLM 评估器可能尚不完善 | `llm_judge.go`、`framework.go` 中 LLM Judge 调用逻辑 |
| 未使用 Callbacks | **认知缺失** — 项目未充分了解框架 Callback 机制 | 评估进度追踪、监控能力缺失 |
| 未使用 PromptIter | **认知缺失** — 项目未了解框架 PromptIter 工作流 | Prompt 优化能力缺失 |
| 未使用 EvalSet Recorder | **认知缺失** — 项目未了解框架录制器 Plugin | 评估用例创建效率低 |
| 自建 SQLite 持久化 | **功能缺失** — 框架无 SQLite 后端 | `internal/data/evaluation.go` 全部 Raw SQL |
| 自建 AfterTurn 触发器 | **功能缺失** — 框架无聊天后自动评估触发机制 | `after_turn.go` + Wire 循环依赖解耦 |
| 自建脚本模拟器 | **功能缺失** — 框架仅提供 LLM 驱动模拟器 | `scripted_simulator.go` |
| Legacy 评估路径 | **架构决策** — 保留降级路径确保鲁棒性 | `runner_legacy.go`、`metrics.go` |
| 每次运行新建 inmemory Manager | **架构决策** — 项目自行持久化结果到 SQLite，框架 Manager 仅用于运行时 | `framework.go` 中 Manager 创建逻辑 |
| 未使用 SessionInput | **架构决策** — 项目通过 ChatRunnerAdapter 绕过框架 Session 机制 | `evalset_adapter.go` |
| 自建 gRPC/HTTP 服务 | **架构决策** — 项目使用 Kratos 传输层，不使用框架 HTTP Server | `internal/service/evaluation.go` |

---

## 四、对齐方案

### 4.1 对齐项清单

| # | 对齐项 | 类型 | 优先级 | 影响范围 | 预期收益 |
|---|--------|------|--------|---------|---------|
| 1 | 启用框架 LLM Judge 评估器替代自建 LLMJudge | 替换自建实现 | P1 | `llm_judge.go`、`framework.go`、`runner_legacy.go` | 代码减少约 80 行，评估质量提升 |
| 2 | 启用 Callbacks 生命周期回调 | 启用框架功能 | P1 | `framework.go`、`runner.go` | 实时进度追踪，评估可观测性提升 |
| 3 | 启用 EvalSet Recorder 自动录制 | 启用框架功能 | P2 | `internal/evaluation/`、Agent 运行时 | 自动化用例生成，降低评估数据准备成本 |
| 4 | 启用 PromptIter 工作流 | 启用框架功能 | P2 | 新增模块 | Prompt 自动优化闭环 |
| 5 | 移除 Legacy 评估路径 | 替换自建实现 | P2 | `runner_legacy.go`、`metrics.go` | 代码减少约 120 行，消除维护负担 |
| 6 | 贡献 SQLite 存储后端 | 贡献回框架 | P3 | `internal/data/evaluation.go` | 框架生态完善，项目可删除自建 DDL |
| 7 | 贡献脚本用户模拟器 | 贡献回框架 | P3 | `scripted_simulator.go` | 框架生态完善 |
| 8 | 贡献 AfterTurn 触发器 | 贡献回框架 | P3 | `after_turn.go` | 框架生态完善 |

### 4.2 对齐项详情

#### 对齐项 #1：启用框架 LLM Judge 评估器替代自建 LLMJudge

**类型**：替换自建实现

**现状**：
- 项目自建 `LLMJudge`（`llm_judge.go`）：单 prompt + LLM 调用 + 0-1 分数解析，约 70 行
- 框架提供 7 个 LLM 评估器：`llm_final_response`、`rubric_critic`、`rubric_response`、`rubric_reference_critic`、`rubric_knowledge_recall`、`hallucination`、`template`
- 项目已在 `evaluator_registry.go` 注册了全部 9 个评估器，但从未通过 Metric 配置使用 LLM 评估器
- `FrameworkBridge.Execute()` 中额外调用 `b.llmJudge()` 做二次评分（第 165-188 行），与框架评估器结果并存

**对齐方案**：
1. 在 `registerFrameworkMetrics()` 中为 `MetricLLMAsJudge` 配置框架 LLM Judge criterion（使用 `llm_final_response` 或 `template` 评估器）
2. 配置 `criterion.llmJudge.judgeModel` 或使用 `WithJudgeRunner` 注入 Judge Runner
3. 移除 `FrameworkBridge.Execute()` 中额外的 `b.llmJudge()` 调用（第 165-188 行）
4. 移除 `llm_judge.go` 文件
5. 移除 `Runner` struct 中的 `llmJudge LLMJudge` 字段
6. 更新 Wire 注入链，移除 LLMJudge 构造

**代码变更范围**：
- 修改：`framework_metrics.go`（添加 LLM Judge criterion 配置）
- 修改：`framework.go`（移除额外 LLMJudge 调用、移除 llmJudge 字段）
- 修改：`runner.go`（移除 llmJudge 字段和构造参数）
- 修改：`multirun.go`（如需配置 JudgeRunner Option）
- 删除：`llm_judge.go`
- 修改：`internal/service/evaluation_runner.go`（移除 LLMJudge 构造）
- 修改：`internal/service/evaluation.go`（更新 Wire 注入）

**兼容性风险**：
- LLM Judge 评分标准可能变化（从简单 0-1 分数变为 Rubric 多维度评分），历史数据对比可能不一致
- 框架 LLM Judge 需要 JudgeRunner 或 judgeModel 配置，当前项目的模型解析链需适配

**回退方案**：
- 保留 `LLMJudge` 函数类型定义，通过 feature flag 控制使用框架 LLM Judge 或自建 LLMJudge

**验证方法**：
- 运行含 `llm_as_judge` 指标的评估 Run，对比框架 LLM Judge 和自建 LLMJudge 的评分结果
- 验证 `scores_json` 中 `llm_as_judge` 字段值正确写入

**预期收益**：
- 代码减少：约 80 行（`llm_judge.go` + `framework.go` 中额外调用逻辑）
- 评估质量：支持 Rubric 多维度评分、幻觉检测等高级评估能力
- 维护成本：减少约 0.5 人天/季度（LLM Judge prompt 维护）

---

#### 对齐项 #2：启用 Callbacks 生命周期回调

**类型**：启用框架功能

**现状**：
- 项目评估异步执行（`safego.Go`），无评估阶段感知
- 前端无法实时获取评估进度（需轮询 Run 状态）
- 框架提供 8 个生命周期回调：`Before/After Inference/Evaluate × Set/Case`

**对齐方案**：
1. 创建 `evalCallbacks` 实现 `service.Callback` 接口
2. 在 `AfterInferenceCase` 回调中更新 `completed_cases` 计数和 Run 状态
3. 在 `AfterEvaluateCase` 回调中写入 CaseResult 到 DB
4. 通过 `WithCallbacks` 注入到 `AgentEvaluator`
5. 利用回调实现评估进度 WebSocket 推送

**代码变更范围**：
- 新增：`internal/evaluation/callbacks.go`（回调实现）
- 修改：`framework.go`（添加 `WithCallbacks` Option）
- 修改：`runner.go`（移除手动 completed_cases 更新逻辑，改由回调驱动）

**兼容性风险**：
- 低风险 — Callback 是纯增量功能，不影响现有逻辑

**回退方案**：
- 不注入 Callbacks 即可回退到当前行为

**验证方法**：
- 运行评估，验证 `AfterInferenceCase`/`AfterEvaluateCase` 回调正确触发
- 验证 Run 状态实时更新

**预期收益**：
- 代码减少：约 20 行（移除手动进度更新逻辑）
- 功能增强：实时评估进度推送、Case 级监控
- 维护成本：减少约 0.3 人天/季度

---

#### 对齐项 #3：启用 EvalSet Recorder 自动录制

**类型**：启用框架功能

**现状**：
- 项目评估用例完全手动创建（通过 API UploadCases）
- 框架提供 `evalset/recorder.Plugin`，可在 Agent 运行时自动录制交互为评估用例

**对齐方案**：
1. 在 Agent 运行时创建 `recorder.New()` Plugin
2. 配置 `EvalSetIDResolver` 和 `EvalCaseIDResolver` 自动分配 ID
3. 将 Recorder 注册为 Agent Plugin
4. 新增 API 端点：从录制结果创建 Dataset

**代码变更范围**：
- 新增：`internal/evaluation/recorder.go`（Recorder 配置和集成）
- 修改：Agent 运行时 Plugin 注册逻辑
- 修改：`internal/service/evaluation.go`（新增从录制创建 Dataset API）

**兼容性风险**：
- 中风险 — Recorder 作为 Plugin 注入 Agent 运行时，可能影响推理性能
- 需评估异步写入模式对延迟的影响

**回退方案**：
- 不注册 Recorder Plugin 即可回退

**验证方法**：
- 启用 Recorder 后运行 Agent 对话，验证自动生成 EvalCase
- 验证生成的 EvalCase 可用于后续评估

**预期收益**：
- 功能增强：自动化评估用例生成
- 效率提升：评估数据准备成本降低约 60%
- 代码增加：约 50 行（Recorder 配置和集成）

---

#### 对齐项 #4：启用 PromptIter 工作流

**类型**：启用框架功能

**现状**：
- 项目无 Prompt 优化能力，完全依赖人工调优
- 框架提供完整 PromptIter 工作流：评估→反向传播→聚合→优化循环

**对齐方案**：
1. 在 `internal/evaluation/` 下新增 `promptiter.go` 封装框架 PromptIter Engine
2. 新增 API 端点：创建/查询/停止 PromptIter 优化任务
3. 集成项目 LLM 模型解析链到 PromptIter 的 Optimizer Runner
4. 前端新增 PromptIter 管理界面

**代码变更范围**：
- 新增：`internal/evaluation/promptiter.go`
- 新增：`internal/biz/evaluation/prompt_iter.go`（Biz 接口）
- 新增：`internal/service/evaluation_promptiter.go`（API 端点）
- 新增：Proto 定义
- 新增：前端页面和组件

**兼容性风险**：
- 低风险 — 纯增量功能
- PromptIter 可能消耗较多 LLM Token，需设置预算控制

**回退方案**：
- 不暴露 API 端点即可

**验证方法**：
- 创建 PromptIter 任务，验证自动优化循环执行
- 验证优化后 Prompt 的评估分数提升

**预期收益**：
- 功能增强：Prompt 自动优化闭环
- 效率提升：Prompt 调优时间减少约 70%
- 代码增加：约 200 行（后端）+ 前端页面

---

#### 对齐项 #5：移除 Legacy 评估路径

**类型**：替换自建实现

**现状**：
- `Runner` 有两条执行路径：Framework（推荐）和 Legacy（降级）
- Legacy 路径仅支持 4 个简单指标（exact_match/contains_match/llm_as_judge/tool_call_accuracy）
- Legacy 路径约 120 行代码（`runner_legacy.go` + `metrics.go`）

**对齐方案**：
1. 确认 Framework 路径覆盖所有 Legacy 指标（已确认：4 个指标均有框架 criterion 对应）
2. 将 `Runner.framework` 和 `Runner.agent` 都为 nil 时的错误提示从 "agent runner not configured" 改为明确要求使用 Framework 路径
3. 移除 `executeLegacy` 方法和 `scoreLegacyCase` 函数
4. 移除 `metrics.go` 中的 legacy 计算逻辑
5. 移除 `Runner.agent`（AgentRunner）字段，强制使用 Framework 路径

**代码变更范围**：
- 删除：`runner_legacy.go`
- 删除：`metrics.go`
- 修改：`runner.go`（移除 agent 字段和 legacy 分支）
- 修改：`internal/service/evaluation_runner.go`（移除 AgentRunner 注入）

**兼容性风险**：
- 中风险 — 如果 Framework 路径因框架 bug 失败，无降级路径
- 需确保 Framework 路径稳定性

**回退方案**：
- 保留 `runner_legacy.go` 文件但标记 Deprecated，通过配置开关切换

**验证方法**：
- 运行全量评估，验证 Framework 路径结果与 Legacy 路径一致
- 特别验证 4 个核心指标的分数一致性

**预期收益**：
- 代码减少：约 120 行
- 依赖简化：移除 `internal/evaluation` 对 `AgentRunner` 类型的依赖
- 维护成本：减少约 0.5 人天/季度

---

#### 对齐项 #6：贡献 SQLite 存储后端

**类型**：贡献回框架

**现状**：
- 项目自建 4 张 SQLite 表 + Raw SQL Repo（`internal/data/evaluation.go`，约 400 行）
- 框架提供 inmemory/local/mysql 三种后端，无 SQLite
- 项目 DDL 通过 `EnsureEvalSchema` 手动创建，不在 DDL Migration Registry 体系中

**对齐方案**：
1. 在框架 `evaluation/evalset/sqlite/`、`evaluation/metric/sqlite/`、`evaluation/evalresult/sqlite/` 实现 SQLite 后端
2. 遵循框架 Manager 接口和 local 后端的实现模式
3. 项目切换到 SQLite 后端后，移除自建 Raw SQL Repo 和 `EnsureEvalSchema`
4. 将评估表纳入 Ent Schema + DDL Migration Registry 体系

**代码变更范围**：
- 新增（框架）：`evaluation/evalset/sqlite/`、`evaluation/metric/sqlite/`、`evaluation/evalresult/sqlite/`
- 修改（项目）：`framework.go`（替换 inmemory 为 sqlite Manager）
- 删除（项目）：`internal/data/evaluation.go` 中 Raw SQL 评估相关代码
- 修改（项目）：DDL 迁移注册

**兼容性风险**：
- 高风险 — 需要框架接受贡献，且数据迁移需谨慎
- SQLite 后端的事务行为与 inmemory 不同

**回退方案**：
- 保持 inmemory + 项目自建持久化

**验证方法**：
- 验证 SQLite 后端 CRUD 与 inmemory 行为一致
- 验证评估结果正确持久化和查询

**预期收益**：
- 代码减少：约 300 行（Raw SQL Repo + EnsureEvalSchema）
- 框架生态：其他 SQLite 项目可直接使用
- 维护成本：减少约 1 人天/季度

---

#### 对齐项 #7：贡献脚本用户模拟器

**类型**：贡献回框架

**现状**：
- 项目自建 `scriptedSimulator`（`scripted_simulator.go`，约 55 行），实现 `usersimulation.Simulator` 接口
- 框架仅提供 LLM 驱动模拟器，无脚本模拟器

**对齐方案**：
1. 在框架 `evaluation/usersimulation/scripted/` 实现脚本模拟器
2. 支持从 JSON 配置加载脚本
3. 项目切换到框架实现后删除自建代码

**代码变更范围**：
- 新增（框架）：`evaluation/usersimulation/scripted/`
- 修改（项目）：`llm_simulator.go`（`resolveUserSimulator` 改用框架实现）
- 删除（项目）：`scripted_simulator.go`

**兼容性风险**：
- 低风险 — 接口完全兼容

**回退方案**：
- 保持自建实现

**验证方法**：
- 验证脚本模拟器与自建实现行为一致

**预期收益**：
- 代码减少：约 55 行
- 框架生态：补充框架用户模拟器类型

---

#### 对齐项 #8：贡献 AfterTurn 触发器

**类型**：贡献回框架

**现状**：
- 项目自建 `AfterTurnTrigger`（`after_turn.go`，约 70 行），实现 `NativeTurnAfterHook` 接口
- 含频率控制（`MinIntervalSec`）和自动清理逻辑
- 框架无此功能

**对齐方案**：
1. 在框架中设计 AfterEval Trigger 机制（可作为 Callback 扩展或独立 Plugin）
2. 将频率控制逻辑抽象为通用 RateLimiter
3. 项目切换到框架实现后删除自建代码

**代码变更范围**：
- 新增（框架）：AfterEval Trigger 机制
- 删除（项目）：`after_turn.go`
- 修改（项目）：Wire 注入链

**兼容性风险**：
- 中风险 — 需要框架设计新的扩展点
- 与项目 `NativeTurnAfterHook` 接口解耦需谨慎

**回退方案**：
- 保持自建实现

**验证方法**：
- 验证 AfterTurn 自动评估触发和频率控制

**预期收益**：
- 代码减少：约 70 行
- 框架生态：补充框架自动评估触发能力

---

## 五、实施路线

### 5.1 阶段规划

| 阶段 | 对齐项 | 前置依赖 | 预计工作量 |
|------|--------|---------|-----------|
| Phase 1 | #1（LLM Judge 对齐）、#2（Callbacks 启用） | 无 | 中 |
| Phase 2 | #5（移除 Legacy 路径） | Phase 1（LLM Judge 对齐完成） | 小 |
| Phase 3 | #3（EvalSet Recorder）、#4（PromptIter） | Phase 1（Callbacks 启用） | 大 |
| Phase 4 | #6（SQLite 后端贡献）、#7（脚本模拟器贡献）、#8（AfterTurn 贡献） | Phase 2（Legacy 移除后架构清晰） | 大 |

### 5.2 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| 框架 LLM Judge 评分标准与自建不一致 | 中 | 高 | 先并行运行两者，对比结果后再切换 |
| 移除 Legacy 路径后框架路径不稳定 | 低 | 高 | 保留 Legacy 代码标记 Deprecated，配置开关切换 |
| PromptIter 消耗大量 LLM Token | 中 | 中 | 设置预算上限和优化轮次上限 |
| SQLite 后端贡献未被框架接受 | 中 | 低 | 保持 inmemory + 项目自建持久化 |
| Callbacks 回调中 DB 写入影响评估性能 | 低 | 中 | 使用异步写入（回调中发 channel，独立 goroutine 消费） |

---

## 六、附录

### A. 框架示例代码参考（必填）

| 示例 | 路径 | 关键 API | 初始化模式 | 与项目实现差异 |
|------|------|---------|-----------|--------------|
| inmemory | `examples/evaluation/inmemory/` | `evaluation.New()` + inmemory Manager 三件套 + `registry.New()` | 编程式创建 EvalSet/Metric | 项目同样使用 inmemory，但每次运行新建 Manager |
| local | `examples/evaluation/local/` | `evalsetlocal.New()` + `metriclocal.New()` + `evalresultlocal.New()` | JSON 文件驱动 | 项目未使用 local 后端，数据和配置存在 SQLite |
| callbacks | `examples/evaluation/callbacks/` | `service.NewCallbacks().Register()` + `WithCallbacks` | local + Callback 注入 | 项目未使用 Callbacks |
| usersimulation | `examples/evaluation/usersimulation/` | `usersimulation.New(simRunner)` + `WithUserSimulator` + `WithJudgeRunner` | 3 Runner（actual/sim/judge） | 项目使用 LLM 模拟器但未用 `WithJudgeRunner`，自建 LLMJudge |
| tooltrajectory | `examples/evaluation/tooltrajectory/` | `criterion.WithToolTrajectory()` + 精细策略配置 | local + ToolTrajectory criterion | 项目使用 `tooltrajectory.New(tooltrajectory.WithOrderSensitive(true))`，配置较简单 |
| llm/finalresponse | `examples/evaluation/llm/finalresponse/` | `criterion.llmJudge.judgeModel` + `llm_final_response` 评估器 | local + LLM Judge criterion | 项目自建 LLMJudge，未使用框架 LLM 评估器 |
| rouge | `examples/evaluation/rouge/` | `crouge.New(crouge.WithRougeType("rougeL"))` | local + ROUGE criterion | 项目已注册 ROUGE 指标 |
| evalsetrecorder | `examples/evaluation/evalsetrecorder/` | `recorder.New()` + Plugin 注册 | Runner + Recorder Plugin | 项目未使用 Recorder |
| promptiter/syncrun | `examples/evaluation/promptiter/syncrun/` | PromptIter Engine + Optimizer + Backwarder | 同步优化循环 | 项目未使用 PromptIter |

### B. 框架文档参考

| 文档 | 路径 |
|------|------|
| Evaluation 中文文档 | `docs/mkdocs/zh/evaluation.md` |
| Evaluation 英文文档 | `docs/mkdocs/en/evaluation.md` |
| PromptIter 中文文档 | `docs/mkdocs/zh/promptiter.md` |
