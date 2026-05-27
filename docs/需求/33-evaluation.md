# M17: Evaluation 评估 — 需求文档

> **版本**：2026-05-21 | **状态**：🟢 Phase 5 完整（扩展指标 + LLM UserSim + 趋势/对比 + Eval LLM 系统配置）
> **设计**：[33 evaluation.design.md](./33%20evaluation.design.md) · **开发计划**：[33-evaluation-development.md](./33-evaluation-development.md)

---

## 1. 模块定位

Evaluation 评估模块对 Agent 的输出质量进行结构化评估，支持自动化评估（含 LLM-as-Judge）和人工评估，评估结果用于 Agent 迭代优化。

**核心价值**：将 Agent 质量从"人工感觉"转变为"可量化、可追溯、可对比"的客观指标。

---

## 2. 用户故事

### US-1 评估数据集管理

**作为** Agent 开发者，**我希望** 创建和管理评估数据集（包含输入/期望输出对），**以便** 为 Agent 质量评估提供标准化的测试用例。

**验收标准**：
- 可创建评估数据集（名称、描述）
- 可上传评估用例（JSON 数组，每条包含 input + expected_output）
- 可查看、删除数据集
- 数据集显示用例数量

### US-2 运行评估

**作为** Agent 开发者，**我希望** 对指定 Agent 运行评估，**以便** 获取 Agent 在评估数据集上的质量分数。

**验收标准**：
- 可选择数据集和 Agent 启动评估
- 评估异步执行，不阻塞调用方
- 可查看评估运行状态（pending → running → completed/failed）
- 可查看评估运行进度（已完成用例数/总用例数）
- 可选择运行的评估指标子集

### US-3 评估指标

**作为** Agent 开发者，**我希望** 通过多种指标评估 Agent 输出质量，**以便** 从不同维度了解 Agent 表现。

**验收标准**：
- 精确匹配（exact_match）：不区分大小写的字符串相等判定
- 包含匹配（contains_match）：期望输出出现在实际输出中
- LLM-as-Judge（llm_as_judge）：LLM 评判模型打分 [0,1]
- 工具调用准确率（tool_call_accuracy）：期望工具在输出中被提及的比例
- 可按需选择指标子集运行

### US-4 评估结果查看

**作为** Agent 开发者，**我希望** 查看评估运行的详细结果，**以便** 了解每个用例的具体表现。

**验收标准**：
- 可查看运行汇总分数（各指标平均分）
- 可查看逐用例结果（输入、期望输出、实际输出、各指标得分）
- 可按数据集/Agent/状态筛选运行列表

### US-5 自动评估触发

**作为** Agent 开发者，**我希望** Agent 运行后自动触发评估，**以便** 无需手动操作即可持续监控 Agent 质量。

**验收标准**：
- Agent 完成对话 turn 后可自动触发关联的评估
- 可配置自动评估的触发条件和关联数据集

> **实现状态**：✅ `NativeTurnAfterHook` + `config_json.evaluation.auto_after_turn`（min_interval_sec 限流）

### US-6 人工评估标注

**作为** Agent 开发者，**我希望** 对自动评估结果进行人工标注，**以便** 纠正自动评估的偏差并积累高质量标注数据。

**验收标准**：
- 可对评估结果进行人工通过/不通过标注
- 可添加人工评语
- 人工标注结果与自动评估结果并存展示

> **实现状态**：✅ `AnnotateCaseResult` API + Results 对话框（EVAL-02）

### US-7 评估报告与趋势

**作为** Agent 开发者，**我希望** 查看评估报告和趋势，**以便** 了解 Agent 质量的变化趋势和对比不同版本的表现。

**验收标准**：
- 可生成评估报告（汇总分数、用例分布）
- 可查看同一 Agent 不同时间运行的分数趋势
- 可导出评估报告

> **实现状态**：✅ 客户端 CSV/JSON 导出；`GetAgentEvalTrend` + `CompareEvalRuns` API；前端 `EvaluationAnalyticsPanel` 趋势表 + Run 多选 A/B 对比

### US-8 高级评估能力（超越层）

**作为** Agent 开发者，**我希望** 使用更高级的评估能力，**以便** 更全面地评估复杂 Agent 行为。

**验收标准**：
- 支持多轮对话评估（UserSimulation）
- 支持多次运行取平均（MultiRun），减少随机性
- 支持 Tool Trajectory 评估（工具调用序列匹配）
- 支持 Final Response 多维度评估（文本/JSON/ROUGE/XML）
- 支持 pass@k / pass^k 指标
- 支持 A/B 对比不同 Agent 版本

> **实现状态**：✅ 多轮 turns、脚本化/LLM UserSimulation、MultiRun + pass@k、ROUGE/XML/JSON FinalResponse、完整 ToolTrajectory（`tool_trajectory`）、趋势/A/B API 与前端

---

## 3. 功能规格

### 3.1 评估数据集

| 属性 | 说明 |
|------|------|
| 名称 | 必填，数据集标识 |
| 描述 | 选填，数据集说明 |
| 用例数 | 自动统计 |
| 用例格式 | JSON 数组；`metadata_json` 可含 `turns`（多轮）、`user_simulation`（`script` 脚本 / `use_llm`+`conversation_plan` LLM 驱动）、`expected_tools` / `expected_tool_calls`（ToolTrajectory） |

### 3.2 评估运行

| 属性 | 说明 |
|------|------|
| 关联数据集 | 必填 |
| 关联 Agent | 必填 |
| 运行状态 | pending → running → completed / failed |
| 指标选择 | 逗号分隔的指标键名，空值运行 4 种核心指标；扩展指标 opt-in（见 §3.3） |
| num_runs | MultiRun 重复次数（默认 1）；`>1` 时写入 pass@k / pass^k |
| use_user_simulation | 启用 UserSimulation（脚本或 LLM，见 case metadata） |

| 执行方式 | 异步，创建后立即返回，后台执行 |

### 3.3 评估指标

#### 核心指标（metrics 为空时默认全开）

| 指标 | 键 | 计算方式 |
|------|-----|----------|
| 精确匹配 | `exact_match` | FinalResponse 精确文本匹配 |
| 包含匹配 | `contains_match` | FinalResponse 包含期望文本 |
| LLM-as-Judge | `llm_as_judge` | Judge 模型对 (input, expected, actual) 打分 [0,1] |
| 工具调用准确率 | `tool_call_accuracy` | 期望工具在输出中被提及的比例（legacy）；或 ToolTrajectory 简化版 |

#### 扩展指标（opt-in，逗号加入 metrics）

| 指标 | 键 | 说明 |
|------|-----|------|
| JSON 匹配 | `json_match` | FinalResponse JSON 结构匹配 |
| XML 匹配 | `xml_match` | FinalResponse XML 结构匹配 |
| ROUGE-L | `rouge_l` | FinalResponse ROUGE-L（阈值 0.5） |
| 工具轨迹 | `tool_trajectory` | 顺序敏感完整 ToolTrajectory 匹配 |

扩展分数字段：`eval_runs.scores_json` / `eval_case_results.scores_json`（键 → 分数 map）；legacy 四列仍同步写入。

### 3.4 Eval LLM 模型配置

UserSim 与 LLM-as-Judge 共用 Provider 目录凭证；模型解析优先级：**env > system_settings > 目录 mini/flash**。

| 来源 | 键 | 说明 |
|------|-----|------|
| 环境变量 | `KRATOS_EVAL_SIM_PROVIDER` / `KRATOS_EVAL_SIM_MODEL` | UserSim；Judge 未配时可回退 |
| 环境变量 | `KRATOS_EVAL_JUDGE_PROVIDER` / `KRATOS_EVAL_JUDGE_MODEL` | Judge 专用 |
| 系统设置 | `system_settings.eval_sim_*` / `eval_judge_*` | **Settings 页**持久化；前端默认 `openai` / `gpt-4o-mini`（Sim） |

### 3.5 评估结果

| 属性 | 说明 |
|------|------|
| 汇总分数 | 各指标在全部用例上的平均分 |
| 逐用例结果 | 每条用例的 actual_output + 各指标得分/判定 |
| 执行耗时 | 运行开始到结束的时间 |

---

## 4. 验收标准总览

1. 可创建评估数据集并上传用例 ✅
2. 可对指定 Agent 运行评估（异步）✅
3. 四种内置指标可按需选择运行 ✅
4. 可查看运行汇总分数和逐用例结果 ✅
5. Agent 运行后可自动触发评估（US-5）✅
6. 评估结果可人工标注（US-6）✅
7. 可查看评估报告和趋势（US-7）✅ 客户端导出 + 趋势/A/B API
8. 高级评估：多轮/UserSim/扩展指标/pass@k/趋势对比（US-8）✅
9. Eval LLM 可在系统设置页配置并持久化 ✅

---

## 5. 非功能需求

| 项 | 要求 |
|----|------|
| 性能 | 100 用例数据集在 5 分钟内完成评估 |
| 可观测 | 评估运行次数和用例执行耗时通过 Prometheus 暴露 |
| 成本 | LLM-as-Judge 消耗 LLM Token，需支持按需启用/禁用 |
| 可靠性 | 评估运行失败不影响正常 Chat 流程 |
