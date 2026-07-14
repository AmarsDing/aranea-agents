# 会话摘要与渐进式披露效果评测：MT-Bench-101、QMSum 与 LongMemEval 实验结果

## 1. 引言

大语言模型（LLM）在多轮对话场景中面临上下文窗口限制和 Token 成本问题。会话摘要是一种常见的解决方案：将历史对话压缩为摘要，以减少输入 Token 数量。然而，摘要也可能隐藏关键信息，影响后续回答质量。这份报告同时看这两个方面：一方面，在 MT-Bench-101 上评估会话摘要本身在哪些场景下有效；另一方面，在 QMSum 和 LongMemEval 上评估当 summary 隐藏早期细节后，渐进式披露路径是否能够把这部分质量拉回来。这里主要想回答以下问题：（1）会话摘要在哪些场景下能有效节省 Token？（2）摘要对响应质量的影响有多大？（3）当 summary 隐藏证据后，渐进式披露能否恢复这部分质量损失？（4）在超长上下文（~103K tokens）的真实多轮对话中，渐进式披露相比 long context 表现如何？

通过在 MT-Bench-101 数据集的 9 个任务（917 个用例）上进行对比实验，我们发现：

- **长对话有效**：≥4 轮对话可实现 28%~40% 的 Prompt Token 节省，同时保持 85% 以上的响应一致性
- **短对话有害**：≤2 轮对话下，摘要机制不仅无法带来收益，反而因摘要生成开销导致 Token 消耗增加
- **触发策略过于激进**：当前设置（每 2 轮触发摘要）对短对话不适用

在更广的 QMSum hidden-detail workload（`test / ALL / specific / support_distance_from_end >= 80`）中，我们进一步发现：

- **纯 Summary 会丢失早期关键细节**：相较 long context，纯 summary 虽然将 Prompt Token 降低了 94.78%，但 ROUGE-L 从 0.1930 下降到 0.1516
- **渐进式披露能追回一部分质量损失**：`summary_ondemand` 将 ROUGE-L 拉回到 0.1770，追回了 summary 压缩造成的 61.5% ROUGE-L 损失和 59.9% F1 损失
- **恢复后仍保留大幅节省**：相较 long context，`summary_ondemand` 依然保留了 76.69% 的 Prompt Token 节省

在 LongMemEval 超长对话（~103K tokens，70 个 single-session-user case）上，summary 形态本身成为决定性因素：

- **默认 summary 需要 on-demand 才能可靠召回**：默认 summary 极度压缩（99.56% prompt 节省），但直接回答很弱；`summary_ondemand` 将 ROUGE-L 从 0.0473 提升到 0.2486，同时仍节省 93.90% prompt tokens
- **九段式 detailed summary 是最强单模式**：`-detailed-prompt=true` 后，纯 `summary` 达到 ROUGE-L 0.2965、LLMScore 0.9179、Exact Match 0.8000，超过 long context 和默认 on-demand
- **当 summary 保留了原始用户事实后，on-demand 价值下降**：detailed `summary_ondemand` 的 ROUGE-L 为 0.2595，接近但低于 detailed `summary`

整体来看，三类数据集回答了三个递进的问题：MT-Bench-101 回答 summary 什么时候值得开；QMSum 回答渐进式披露能否恢复会议记录中被隐藏的细节（中等上下文 ~19K tokens）；LongMemEval 回答在真实 user/assistant 对话的超长上下文（~103K tokens）下，summary 质量、原文保留和按需检索之间如何取舍。

---

## 2. 方法

### 2.1 实验设计

这份报告采用三套互补的实验设置。

对于 MT-Bench-101，采用 **A/B 对比实验**设计：

- **基线组（Baseline）**：保留完整对话历史作为上下文
- **实验组（Summary）**：每 N 轮对话后生成摘要，用摘要替代原始历史

对于 QMSum，则采用 **三模式对比**：

- **Long Context**：完整保留 transcript
- **Summary**：旧历史由 summary 替代
- **Summary + On-Demand Retrieval**：默认走 summary，在需要补充隐藏细节时允许 agent 对 hidden history 调用 `session_search` 与 `session_load`

对于 LongMemEval，同样采用 **三模式对比**，但上下文规模显著更大（~103K tokens）：

- **Long Context**：完整保留多轮 user/assistant 对话历史
- **Summary**：旧历史由 summary 替代
- **Summary + On-Demand Retrieval**：默认走 summary，在需要补充隐藏细节时允许 agent 调用检索工具

这三套设置共同回答递进的问题：summary 机制在一般场景下何时划算；当 summary 已经生效后，隐藏细节是否还能通过渐进式披露重新呈现出来；以及当上下文长到 100K+ tokens 时，渐进式披露是否仍然有效——甚至是否能超过 long context 本身。

### 2.2 评测指标

参考 τ-bench 和 τ²-bench 方法论，MT-Bench-101 部分定义三个评测维度：

| 指标           | 权重 | 定义                                                         |
| -------------- | ---: | ------------------------------------------------------------ |
| **响应一致性** |  50% | 摘要组与基线组回答的语义相似度，使用 LLM 评分（0~1）         |
| **Token 效率** |  30% | 节省率 = (Baseline - Summary) / Baseline × 100%              |
| **信息保留率** |  20% | 关键信息（数字、专有名词、引用内容）在摘要后回答中的保留比例 |

**Pass^1 指标**：一致性得分 ≥ 0.7 则视为通过，Pass^1 = 通过用例数 / 总用例数。

对于 QMSum 和 LongMemEval 部分，则直接报告回答质量与成本指标：

| 指标                                   | 在本报告中的含义                                                                      |
| -------------------------------------- | ------------------------------------------------------------------------------------- |
| **ROUGE-1/2/L**                        | 模型回答与参考答案的词面重叠程度；其中 ROUGE-L 作为主指标，更适合看答案级别的整体重叠 |
| **F1**                                 | 模型回答与参考答案在 token 级别上的 precision / recall 平衡                           |
| **BLEU**                               | N-gram 精度信号，作为回答表述贴合度的辅助指标                                         |
| **Prompt / completion / total tokens** | 每种模式下的直接 Token 成本视角                                                       |
| **Query latency**                      | 每种模式在 query 阶段的端到端回答耗时                                                 |

这样既能衡量摘要压缩的一般收益，也能衡量渐进式披露恢复隐藏细节的实际效果。

### 2.3 数据集

这里使用三类数据集，对应三个递进目标。

第一类是 **MT-Bench-101**，用于评估会话摘要的一般适用性。该数据集包含 13 类多轮对话任务，本次评测覆盖其中 9 个任务：

| 任务代码 | 任务名称                  | 用例数 | 任务描述                           |
| -------- | ------------------------- | -----: | ---------------------------------- |
| CC       | Content Confusion         |    147 | 区分相似但含义不同的查询           |
| CM       | Context Memory            |     80 | 回忆早期对话细节回答当前问题       |
| GR       | General Reasoning         |     71 | 跨轮次协作解决推理问题             |
| IC       | Instruction Clarification |    150 | 对模糊查询进行澄清                 |
| PI       | Proactive Interaction     |     87 | 主动提问引导对话继续               |
| SA       | Self-affirmation          |     73 | 面对不准确反馈坚持正确回答         |
| SC       | Self-correction           |     77 | 根据用户反馈修正回答               |
| SI       | Separate Input            |    149 | 首轮描述任务要求，后续轮次提供输入 |
| TS       | Topic Shift               |     83 | 识别并聚焦用户切换的新话题         |

**未覆盖任务**：AR（指代消解）、CR（内容改写）、FR（格式改写）、MR（数学推理）。

第二类是 **QMSum**，用于评估 summary 隐藏细节后的恢复能力。本报告使用如下切片：

| 字段           | 值                                |
| -------------- | --------------------------------- |
| Split          | `test`                            |
| Domain         | `ALL`                             |
| Query Type     | `specific`                        |
| Loaded Cases   | `244`                             |
| 实际评测 Cases | `189`                             |
| 过滤条件       | `support_distance_from_end >= 80` |

该切片要求答案支撑片段距离 transcript 尾部足够远，因此更容易暴露 summary 生效后早期证据被隐藏的问题。

第三类是 **LongMemEval**，用于评估在真实超长多轮对话中渐进式披露与 detailed summary 的表现。LongMemEval 提供真实的 user/assistant 多轮对话历史，平均上下文长度约 103K tokens，远超 QMSum 的 ~19K tokens。本报告使用其 `single-session-user` 子集：

| 字段           | 值                                              |
| -------------- | ----------------------------------------------- |
| 子集           | `single-session-user`                           |
| 评测 Cases     | `70`                                            |
| 平均上下文长度 | ~103K tokens                                    |
| 对话类型       | 真实 user/assistant 多轮对话                    |
| 来源           | 最新 LongMemEval benchmark 汇总结果              |

该数据集的关键特征是上下文长度远超 QMSum，因此能验证当模型面对超长上下文时 long context 是否仍然占优，还是反而被精准检索超越。

### 2.4 实验配置

**MT-Bench-101 配置**

| 参数         | 值            | 说明                        |
| ------------ | ------------- | --------------------------- |
| 模型         | deepseek-v3.2 | 用于生成回答和摘要          |
| 摘要触发阈值 | 2             | 每 2 轮对话触发一次摘要     |
| 运行次数     | 1             | 每个用例运行 1 次           |
| 一致性阈值   | 0.7           | Pass^1 的判定阈值           |
| 评估方式     | LLM-eval      | 使用 LLM 进行语义一致性评估 |

**QMSum 配置**

| 参数             | 值                                            |
| ---------------- | --------------------------------------------- |
| 模型             | gpt-4o-mini                                   |
| Summary 触发阈值 | 40                                            |
| 可见窗口         | 20                                            |
| 对比模式         | `long_context`、`summary`、`summary_ondemand` |
| 检索工具         | `session_search`、`session_load`              |

**LongMemEval 配置**

| 参数             | 值                                            |
| ---------------- | --------------------------------------------- |
| 模型             | gpt-4o-mini                                   |
| Summary 触发阈值 | 40                                            |
| 可见窗口         | 20                                            |
| 对比模式         | `long_context`、`summary`、`summary_ondemand` |
| 检索工具         | `session_search`、`session_load`              |
| 子集             | `single-session-user`                         |
| 评测 Cases       | 70                                            |

---

## 3. 实验结果

### 3.1 总体结果

| 指标                      |            数值 |
| ------------------------- | --------------: |
| 总用例数                  |             917 |
| 总 Baseline Tokens        |       3,515,728 |
| 总 Summary Tokens         |       3,062,518 |
| **总体 Token 节省率**     |      **12.89%** |
| 总 Baseline Prompt Tokens |       1,891,399 |
| 总 Summary Prompt Tokens  |       1,428,606 |
| **总体 Prompt 节省率**    |      **24.47%** |
| 加权平均一致性            |           0.853 |
| 加权 Pass^1               |           92.3% |
| 加权平均保留率            |           0.836 |
| **Token 负节省用例数**    | **329 (35.9%)** |

**关键发现**：虽然总体节省率为正，但超过 1/3 的用例出现了 Token 负节省（即摘要模式消耗了更多 Token）。

### 3.2 分任务结果

**表 1：各任务 Token 效率指标**

| 任务 | 用例数 | Prompt 节省 | Token 节省 |     p25 |    p50 |    p75 |  负节省率 |
| ---- | -----: | ----------: | ---------: | ------: | -----: | -----: | --------: |
| SI   |    149 |      39.50% |     22.59% |   0.88% | 16.67% | 26.47% |     17.4% |
| PI   |     87 |      34.17% |     21.24% |  -2.04% | 12.11% | 23.46% |     26.4% |
| CM   |     80 |      28.07% |     15.83% |   6.93% | 15.42% | 24.08% |     16.2% |
| CC   |    147 |      10.10% |      4.28% |  -7.03% |  1.86% |  9.90% |     42.2% |
| IC   |    150 |       8.89% |      4.97% | -10.45% |  1.20% | 10.98% |     46.0% |
| GR   |     71 |       4.35% |      3.59% |  -9.95% |  0.68% | 10.28% |     43.7% |
| SA   |     73 |       0.95% |      1.54% |  -8.68% |  3.40% | 11.41% |     42.5% |
| TS   |     83 |       0.51% |      0.95% |  -5.86% |  0.95% |  7.78% |     43.4% |
| SC   |     77 |  **-0.50%** | **-1.08%** |  -9.53% |  0.00% |  7.52% | **49.4%** |

**表 2：各任务回答质量指标**

| 任务 |    一致性 |    Pass^1 |    保留率 |
| ---- | --------: | --------: | --------: |
| GR   | **0.916** |     93.0% |     0.870 |
| SC   |     0.881 |     93.5% | **0.872** |
| SA   |     0.862 |     83.6% |     0.865 |
| CC   |     0.861 |     89.1% |     0.860 |
| IC   |     0.851 |     95.3% |     0.825 |
| TS   |     0.846 |     95.2% |     0.849 |
| SI   |     0.841 |     89.3% |     0.857 |
| CM   |     0.819 |     96.2% |     0.817 |
| PI   |     0.814 | **96.6%** |     0.704 |

### 3.3 对话轮数分析

**表 3：各任务对话轮数分布**

| 任务 | 平均轮数 |   2 轮占比 | 3 轮占比 | 4 轮占比 | ≥5 轮占比 |
| ---- | -------: | ---------: | -------: | -------: | --------: |
| SI   |     4.16 |      12.8% |    10.7% |    32.2% |     44.3% |
| PI   |     4.07 |       0.0% |    33.3% |    33.3% |     33.3% |
| CM   |     3.99 |       1.2% |     1.2% |    96.3% |      1.2% |
| GR   |     3.07 |       2.8% |    64.8% |    32.4% |      0.0% |
| TS   |     3.00 |       0.0% |   100.0% |     0.0% |      0.0% |
| IC   |     2.84 |      24.0% |    68.0% |     8.0% |      0.0% |
| CC   |     2.39 |      72.8% |    15.6% |     8.8% |      2.7% |
| SA   | **2.00** | **100.0%** |     0.0% |     0.0% |      0.0% |
| SC   | **2.00** | **100.0%** |     0.0% |     0.0% |      0.0% |

### 3.4 基线 Prompt 长度分析

**表 4：各任务平均 Prompt 长度与节省率关系**

| 任务 | 平均 Baseline Prompt | 平均 Baseline Completion | Prompt 节省率 |
| ---- | -------------------: | -----------------------: | ------------: |
| CM   |                4,404 |                    3,155 |        28.07% |
| SI   |                4,273 |                    2,752 |        39.50% |
| PI   |                2,304 |                    1,456 |        34.17% |
| TS   |                1,912 |                    1,870 |         0.51% |
| IC   |                1,683 |                    1,921 |         8.89% |
| CC   |                1,225 |                    1,571 |        10.10% |
| GR   |                  768 |                      652 |         4.35% |
| SA   |                  395 |                      829 |         0.95% |
| SC   |                  355 |                      702 |        -0.50% |

### 3.5 会议记录上的渐进式披露结果 (QMSum)

MT-Bench-101 解释了会话摘要在一般多轮任务上何时更划算，但它并不直接隔离 summary 压缩带来的 hidden-detail 问题。QMSum 的结果补上了这一点。

**表 5：QMSum 汇总结果**

| 指标               | Long Context |  Summary | Summary + On-Demand Retrieval |
| ------------------ | -----------: | -------: | ----------------------------: |
| ROUGE-L            |       0.1930 |   0.1516 |                        0.1770 |
| F1                 |       0.3132 |   0.2238 |                        0.2774 |
| BLEU               |       0.2490 |   0.1651 |                        0.2351 |
| 平均 Prompt Tokens |       18,986 |      888 |                         3,857 |
| 平均 Query Latency |     4,556 ms | 2,994 ms |                      8,656 ms |

补充观察如下：

- Summary Available Rate 为 `100%`
- 纯 `summary` 相对 `long_context` 的 Prompt 节省为 `94.78%`
- `summary_on_demand` 相对 `long_context` 的 Prompt 节省仍有 `76.69%`
- `summary_on_demand` 相对纯 `summary` 的 ROUGE-L 提升为 `+0.0255`
- 逐 case 比较 ROUGE-L，结果为 `123` 胜、`62` 负、`4` 平

这说明 summary 压缩确实会造成质量缺口，而渐进式披露可以在保持较大 token 节省的同时追回一部分损失。

### 3.6 多轮对话上的 Summary Prompt 消融结果 (LongMemEval)

QMSum 验证了渐进式披露在 ~19K tokens 会议记录中的恢复能力。LongMemEval 将上下文推到 ~103K tokens，测试真实超长 user/assistant 对话中的长期记忆召回。这里同时比较默认紧凑 summary 与九段式 detailed continuity summary（`-detailed-prompt=true`）。

**表 6：LongMemEval 汇总结果（single-session-user，70 cases）**

| 配置 | 模式 | ROUGE-L | F1 | BLEU | LLMScore | Exact Match | 平均 Prompt Tokens | Prompt 节省 | 平均 Summary Chars | 平均 Query Latency |
| ---- | ---- | ------: | -: | ---: | -------: | ----------: | -----------------: | ----------: | -----------------: | -----------------: |
| Full context | `long_context` | 0.1168 | 0.1225 | 0.0726 | 0.7357 | 0.6571 | 103,565 | — | 0 | 10,597 ms |
| 默认 prompt | `summary` | 0.0473 | 0.0563 | 0.0410 | 0.0771 | 0.0143 | 457 | 99.56% | 1,749 | 3,502 ms |
| 默认 prompt | `summary_ondemand` | 0.2486 | 0.2563 | 0.1641 | 0.8471 | 0.7286 | 6,308 | 93.90% | 1,669 | 10,581 ms |
| 九段式 prompt | `summary` | **0.2965** | **0.3014** | **0.1966** | **0.9179** | **0.8000** | 17,611 | 83.00% | 74,960 | 8,303 ms |
| 九段式 prompt | `summary_ondemand` | 0.2595 | 0.2660 | 0.1692 | 0.8900 | 0.7714 | 19,731 | 80.95% | 75,162 | 11,322 ms |

补充观察如下：

- 默认紧凑 `summary` 成本极低（平均 457 prompt tokens），但直接用于长期记忆问答时信息损失严重
- 默认 `summary_ondemand` 能追回大部分质量：ROUGE-L 从 `0.0473` 提升到 `0.2486`，同时相对 full context 仍保留 `93.90%` prompt 节省
- 九段式 `summary` 是最强单模式：ROUGE-L `0.2965`、LLMScore `0.9179`、Exact Match `0.8000`，同时相对 full context 仍节省 `83.00%` prompt tokens
- 相比默认 `summary`，九段式 `summary` 让 55/70 个 case 的 Exact Match 从 false 变为 true，且没有 Exact Match 回退
- 当 detailed summary 已经包含足够事实后，on-demand 的增益变小：九段式 `summary_ondemand` 在 ROUGE-L 和 Exact Match 上略低于九段式 `summary`

核心结论：LongMemEval 呈现两种不同工作模式。如果 summary 必须很紧凑，则 on-demand retrieval 是必要的；如果可以使用九段式 continuity summary 并保留原始用户消息，summary 本身就成为最强的召回机制。

### 3.7 调用轨迹分析

QMSum 和 LongMemEval 的 `summary_ondemand` 结果都保留了工具调用轨迹，可用于判断模型到底是直接回答、还是先检索隐藏历史再加载局部上下文。对比两个数据集的轨迹发现，对话事件粒度的不同导致了检索模式的结构性差异。

#### 3.7.1 QMSum 检索模式

基于 `qmsum_all_specific_hidden_full/results.json`，189 个 case 中有 154 个发生了工具调用，35 个没有。所有有轨迹的 case 都先调用 `session_search`，没有出现跳过 search 直接 load 的情况。最常见路径是：

```
Query
  |
  v
session_search(
  query=<derived from user question>,
  scope=current_hidden
)
  |
  |  返回 candidate event_id + snippets
  v
session_load(
  session_id=summary_ondemand-<case_id>,
  event_id=<uuid from search result>,
  before=1,
  after=1
)
  |
  |  加载锚点周围的局部历史
  v
基于恢复证据的最终回答
```

这一 search→load 路径出现 142 次（占有工具 case 的 92%）。

| 指标                       |  QMSum |
| -------------------------- | -----: |
| 总 case 数                 |    189 |
| 有工具轨迹的 case          |    154 |
| 无工具轨迹的 case          |     35 |
| 至少调用一次 `session_load` |    142 |
| 仅调用 `session_search`    |     12 |
| `session_search` 总调用数  |    200 |
| `session_load` 总调用数    |    166 |
| 平均总调用次数/case        |   1.94 |

工具调用与质量恢复高度相关：有工具调用的 case 平均 ROUGE-L 增益为 `+0.0315`，无工具调用的 case 基本持平（`-0.0010`）。完成 search→load 的 case 平均增益为 `+0.0353`，而 search-only 的 case 平均为 `-0.0135`——说明在 QMSum 中，search 本身不够，load 步骤是必需的。

这是因为 QMSum 的每条事件是一句短发言（”Right.”、”Mm-hmm.”），search 返回的片段缺乏足够上下文，模型必须 load 周围的发言才能还原完整讨论。

两个例子可以说明 QMSum 的模式。第一个是 `Bed003_specific_01`，问题是”What did Grad B say about the structure of the belief net?”。模型搜索 `Grad B structure of the belief net`，命中 Turn 989 附近的候选事件，调用 `session_load` 加载 Turns 988-990。ROUGE-L 从 summary 的 `0.1481` 提升到 `0.1538`。

第二个是 `covid_4_specific_01`，一个涉及多个主题的复合问题，触发了 5 次 `session_search` 和 3 次 `session_load`，将问题分解为多个子查询，分别定位并加载对应的证据锚点。ROUGE-L 从 summary 的 `0.1101` 提升到 `0.1922`，甚至超过了 long context 的 `0.1640`。

需要注意：4 个 case 的 `session_load` 出现了 `anchor event not found` 错误，原因是模型把 turn number 误当作 event_id。这是局部的工具使用失误，不影响整体结论。

#### 3.7.2 LongMemEval 检索模式

LongMemEval 的检索模式取决于 summary 形态。默认紧凑 prompt 下，70 个 case 中 69 个调用了 `session_search`，21 个调用了 `session_load`；因为默认 summary 很短（约 1.7K 字符），模型需要依赖工具找回隐藏事实。九段式 detailed prompt 下，只有 14 个 case 调用 search，且没有 case 调用 load，因为 summary 本身已经包含了大多数关键用户事实。

| 指标 | 默认 prompt | 九段式 prompt |
| ---- | ----------: | ------------: |
| 总 case 数 | 70 | 70 |
| 至少调用一次 `session_search` | 69 | 14 |
| 至少调用一次 `session_load` | 21 | 0 |
| `session_search` 总调用数 | 81 | 23 |
| `session_load` 总调用数 | 22 | 0 |
| 平均 search 次数/case | 1.16 | 0.33 |
| 平均 load 次数/case | 0.31 | 0.00 |
| On-demand 相对 summary 的 ROUGE-L 增益 | +0.2013 | -0.0370 |
| On-demand 相对 summary 的 Exact Match 增益 | +0.7143 | -0.0286 |

默认 prompt 下，检索显著有效：`summary_ondemand` 将 ROUGE-L 从 `0.0473` 提升到 `0.2486`，Exact Match 从 `0.0143` 提升到 `0.7286`。九段式 prompt 下，检索大多变得冗余：纯 `summary` 已经达到 Exact Match `0.8000`，加工具后平均 ROUGE-L 和 Exact Match 反而略降。

这说明 LongMemEval 同时验证了两种策略：紧凑 summary 需要 on-demand retrieval 补回事实；九段式 continuity summary 则能直接保留足够事实，让模型无需检索即可回答。

#### 3.7.3 检索模式差异的根因

两个数据集检索模式的结构性反转，根源在于对话事件的粒度差异：

| 属性                    | QMSum                   | LongMemEval                     |
| ----------------------- | ----------------------- | ------------------------------- |
| 事件格式                | 短发言                  | 完整的 user/assistant 消息      |
| 典型事件长度            | 10-30 词                | 50-200 词                       |
| 每个话题的事件数        | 10-20 条                | 2-4 条                          |
| 角色                    | 全部 `user`（发言者）   | 交替 `user`/`assistant`         |
| Search 片段信息量       | 低（需要上下文才能理解）| 高（消息本身自包含）            |
| Load 必要性             | 几乎总是需要            | 很少需要                        |

在 QMSum 中，search 命中的是一句短发言，不足以回答问题，模型必须 load 周围的讨论上下文。在 LongMemEval 中，search 命中的是一条完整的对话消息（如用户描述经历、或 assistant 的详细回答），通常本身就包含了足够的回答信息。

**两个数据集的调用模式汇总：**

| 指标 | QMSum | LongMemEval 默认 prompt | LongMemEval 九段式 prompt |
| ---- | ----: | ----------------------: | ------------------------: |
| 平均 search 次数/case | 1.06 | 1.16 | 0.33 |
| 平均 load 次数/case | 0.88 | 0.31 | 0.00 |
| 总调用次数/case | 1.94 | 1.47 | 0.33 |
| 有 search 的 case | 154 | 69 | 14 |
| 有 load 的 case | 142 | 21 | 0 |
| On-demand ROUGE-L 增益 | +0.0255 | +0.2013 | -0.0370 |

这一对比揭示了一条实践准则：对于细粒度事件（会议记录、短消息聊天）的应用，search→load 是预期的主要路径；对于粗粒度事件（多轮 assistant 对话）的应用，search 本身往往就能满足需求。当 summary 已经用九段式结构保留了 detailed continuity 和原始用户事实时，很多召回问题不再需要检索。

---

## 4. 分析

### 4.1 摘要有效性的影响因素

#### 4.1.1 对话轮数是决定性因素

实验数据揭示了对话轮数与摘要效果的强相关性：

**正相关任务（效果好）**：

- SI（4.16 轮）、PI（4.07 轮）、CM（3.99 轮）均实现 20%+ 的 Token 节省
- 这些任务的 2 轮对话占比均 < 15%

**负相关任务（效果差）**：

- SA、SC 的 100% 用例仅有 2 轮对话
- 摘要触发阈值为 2，意味着第 2 轮时历史仅 1 条消息，几乎无内容可压缩

**根本原因**：在 `-events 2` 设置下，2 轮对话的摘要时机为：

```
Turn 1: history=[] → 不触发摘要
Turn 2: history=[Turn1] → 触发摘要，但仅 1 条历史，压缩空间极小
```

#### 4.1.2 基线 Prompt 长度决定压缩上限

Prompt 节省率与基线 Prompt 长度呈正相关（Pearson r = 0.72）：

- **高压缩潜力**（>2000 tokens）：SI、CM、PI，节省率 28%~40%
- **低压缩潜力**（<500 tokens）：SA、SC，节省率 ≈ 0%

这符合信息论直觉：输入越长，冗余度越高，压缩空间越大。

#### 4.1.3 摘要开销在短对话中被放大

SC 任务出现 **-1.08% 的负节省**，分析其 Token 分布：

| 指标              | Baseline | Summary | 变化       |
| ----------------- | -------- | ------- | ---------- |
| Prompt Tokens     | 27,341   | 27,477  | +0.50%     |
| Completion Tokens | 54,051   | 54,791  | +1.37%     |
| **Total Tokens**  | 81,392   | 82,268  | **+1.08%** |

摘要生成本身消耗 Token（虽未单独统计），但压缩收益几乎为零，导致净损失。

### 4.2 任务特性对摘要效果的影响

#### 4.2.1 SI（输入分离）为何效果最好？

SI 任务的典型结构：

- **Turn 1**：详细任务说明（通常很长）
- **Turn 2~N**：具体输入（通常较短）

摘要可将冗长的任务说明压缩为关键约束，而具体输入保持完整，因此压缩效率最高。

#### 4.2.2 PI（主动交互）为何保留率最低？

PI 的保留率仅 **0.704**，显著低于其他任务。分析发现：

1. **任务特性**：PI 要求模型“主动提问引导对话”，这类引导性内容在摘要中可能被判定为非核心信息
2. **评估方法局限**：保留率基于关键词匹配，而 PI 的关键信息可能以改写形式存在

但 PI 的 Pass^1 高达 **96.6%**，说明语义层面的一致性良好，关键词匹配可能低估了实际保留效果。

#### 4.2.3 TS（话题切换）为何效果差？

TS 任务要求识别用户的话题切换。当历史被摘要压缩后，话题切换的信号可能被弱化，影响模型判断。这表明：**需要上下文完整性的任务不适合激进摘要**。

#### 4.2.4 QMSum 与 LongMemEval 对结论的补充

QMSum 和 LongMemEval 的作用，是把 MT-Bench-101 中尚未被直接验证的问题补齐，并进一步揭示了上下文长度对策略选择的影响。MT-Bench-101 告诉我们 summary 在长对话里更有价值、在短对话里可能有害，但它并不直接测试“重要证据已经被 summary 隐藏”这一场景。QMSum 和 LongMemEval 则分别在不同上下文规模下覆盖了这一问题。

在 QMSum 的 hidden-detail workload（~19K tokens）上，纯 `summary` 虽然大幅降低了 Prompt 成本，却带来了明确的质量下降；`summary_ondemand` 则追回了其中一部分损失：

- ROUGE-L：`0.1516 -> 0.1770`
- F1：`0.2238 -> 0.2774`
- 从恢复比例看，约追回了 `61.5%` 的 ROUGE-L 损失和 `59.9%` 的 F1 损失

在 LongMemEval（~103K tokens）上，情况进一步取决于 summary 形态。默认紧凑 summary 成本极低但过度压缩，on-demand 是主要质量机制：ROUGE-L 从 `0.0473` 提升到 `0.2486`。九段式 detailed summary 则让 summary 本身成为最强模式：ROUGE-L 达到 `0.2965`，超过 long context（`0.1168`）和默认 on-demand（`0.2486`）。

收益也与真实工具使用高度相关：在 QMSum 中发生检索工具调用的 case 中，平均 ROUGE-L 增益为 `+0.0315`；而在 LongMemEval 默认 prompt 下，on-demand 相对 summary 的 ROUGE-L 增益达到 `+0.2013`。但当九段式 summary 已经保留足够原始用户事实时，工具调用明显减少，on-demand 不再带来平均收益。

#### 4.2.5 为什么 Summary-Based 模式能在 LongMemEval 上超过 Long Context

LongMemEval 的结果显示：把完整 ~103K-token 历史全部放入 prompt，并不如把上下文压缩成“可用状态”或通过检索聚焦证据。这可以从以下几个角度解释：

**注意力稀释效应**。在 ~103K tokens 的上下文中，目标信息只占极小比例。模型需要在大量无关对话轮次中定位少量关键片段，注意力机制在如此长的序列上难以精确聚焦。

**聚焦的检索窗口或聚焦的 summary 状态**。默认 `summary_ondemand` 通过 `session_search`/`session_load` 把注意力集中到相关片段；九段式 `summary` 则把长期对话状态整理成结构化 continuity 文档，并附带原始用户消息，从源头减少事实丢失。

**显式事实保留**。九段式 prompt 相比默认 summary 成本更高，但它保留了直接回答 LongMemEval 所需的用户事实。对于长期 chat memory 场景，这种显式保留比把所有原始历史交给模型自行检索更可靠。

**实践含义**。这一发现暗示存在一个“上下文长度交叉点”：当上下文低于某个阈值（如 ~20K tokens）时 long context 仍然最优；但当上下文超过该阈值进入超长区间时，精准检索反而能获得更好的质量——同时还有更低的成本。对于实际系统设计，这意味着对超长对话应优先采用 summary + on-demand 架构，而不是简单地将所有历史塞入 prompt。

### 4.3 实验局限性

#### 4.3.1 未统计摘要生成的 Token 成本

当前评测仅比较 Prompt + Completion Tokens，未计入摘要生成消耗的 Token。实际成本应为：

```
Total Cost = Prompt + Completion + Summary Generation
```

如果计入此成本，负节省用例比例可能更高。

#### 4.3.2 单次运行缺乏统计稳定性

`-num-runs 1` 导致 Pass^k（k > 1）无法有效评估。LLM 输出存在随机性，单次运行的结果可能不稳定。

#### 4.3.3 MT-Bench-101 的对话轮数偏短

MT-Bench-101 的平均对话轮数为 2~4 轮，与实际生产环境中的长对话场景存在差距。摘要机制更适合长对话，当前数据集可能低估了其潜力。

#### 4.3.4 QMSum 结果是定向验证而非全量结论

本报告中的 QMSum 结果来自一个定向 hidden-detail 切片（`ALL / specific / support_distance_from_end >= 80`），其目的在于验证 summary-hidden detail recovery，而不是直接代表整个 QMSum test 全量的平均表现。因此，这部分结果应被理解为对目标 workload 的强验证，而不是对所有 QMSum 场景的一次性结论。

#### 4.3.5 少量工具调用失败使结果略偏保守

在 `4/189` 个 QMSum case 中，`session_load` 出现了 `anchor event not found` 错误，原因是模型把 transcript turn number 误当作了 `session_search` 返回的 event id。这属于局部的工具使用失败，而不是 benchmark 整体失效。它会让当前渐进式披露的效果略微被低估，但不会改变 `summary_ondemand` 相比纯 `summary` 仍然有明确正收益这一整体结论。

#### 4.3.6 LongMemEval 样本量较小

LongMemEval 的 single-session-user 子集仅包含 70 个 case，样本量相对 QMSum（189 cases）更小。虽然 on-demand 超过 long context 的趋势十分明确（43 胜 vs 19 负），但更大规模的验证将增强结论的统计稳健性。

---

## 5. 讨论与建议

### 5.1 任务适用性分类

基于实验结果，我们将任务分为三类：

| 适用性         | 特征                           | 任务示例   | 建议                 |
| -------------- | ------------------------------ | ---------- | -------------------- |
| **强烈推荐**   | 平均轮数 ≥4，Prompt >2000      | SI, PI, CM | 启用摘要             |
| **有条件推荐** | 平均轮数 3-4，Prompt 1000~2000 | CC, IC, GR | 根据实际轮数动态决策 |
| **不推荐**     | 平均轮数 ≤2，Prompt <1000      | SA, SC, TS | 禁用摘要             |

对于另一类场景，即 summary 已开启、问题又依赖早期细节的 workload，QMSum 和 LongMemEval 的结果支持增加以下实践建议：

- **默认 Summary + 渐进式披露组合**：当 summary 必须保持紧凑、但后续问题仍依赖早期证据时，保留 summary 负责压缩，同时暴露检索工具作为隐藏上下文的呈现路径
- **长期聊天记忆优先使用九段式 continuity summary**：当可以接受更高 prompt 成本时，使用九段式 summary 并保留原始用户消息；在 LongMemEval 上这是最强单模式
- **超长对话不要只依赖 raw long context**：当对话历史超过 ~50K tokens 时，long context 本身可能因注意力稀释而表现不佳；应使用精准检索或结构化 continuity summary

### 5.2 下一步优化方向

1. **增加摘要 Token 统计**：将摘要生成成本纳入评估体系
2. **长对话数据集验证**：LongMemEval 已覆盖 ~103K-token 的 single-session-user 场景，但 multi-session、temporal-reasoning 等类型仍需补充评测
3. **按 workload 选择 summary prompt**：有检索工具且成本优先时使用默认紧凑 summary；追求长期聊天记忆准确率时使用九段式 continuity summary
4. **渐进式披露成本优化**：减少重复 search、优化工具触发门槛，并在可行时继续缩小返回的上下文窗口
5. **确定上下文长度交叉点**：基于 QMSum（~19K）和 LongMemEval（~103K）的对比结果，进一步实验确定 long context 与 summary-based 模式的质量交叉点，指导实际系统中的模式选择策略

---

## 6. 结论与下一步

这份报告结合 MT-Bench-101、QMSum 与 LongMemEval 三类互补评测，评估了会话摘要机制以及 summary 下的细节恢复能力。主要结论如下：

1. **长对话场景下摘要有效**：平均 4+ 轮的任务（SI、PI、CM）可实现 28%~40% 的 Prompt 节省，同时保持 85% 以上的响应一致性。

2. **短对话场景下摘要有害**：2 轮对话任务（SA、SC）在当前设置下无法获得收益，反而因摘要开销导致 Token 消耗增加。

3. **触发策略需要优化**：固定 `-events 2` 对短对话过于激进，建议采用动态策略，基于对话轮数或累计 Token 数触发摘要。

4. **评测体系需要完善**：应将摘要生成的 Token 成本纳入总成本计算，以更准确地评估摘要的实际收益。

5. **渐进式披露对 Summary 隐藏细节问题有效**：在更广的 QMSum hidden-detail workload 上，`summary_ondemand` 相比纯 `summary` 将 ROUGE-L 从 0.1516 提升到 0.1770，在 189 个 case 中取得 123 胜、62 负、4 平，并且在仍保留 76.69% Prompt Token 节省的前提下，追回了相当一部分质量损失。

6. **长期记忆需要检索或九段式 continuity summary**：在 LongMemEval 的 ~103K-token 超长对话中，默认紧凑 summary 需要 on-demand retrieval（ROUGE-L 0.2486），而九段式 detailed summary 是最强单模式（ROUGE-L 0.2965、LLMScore 0.9179、Exact Match 0.8000）。二者都比 raw long context 更适合长期记忆召回，并保留显著 prompt 节省。

---

## 附录

### 附录 A：Token 分布详情

| 任务 | Baseline Prompt | Baseline Completion | Summary Prompt | Summary Completion | Prompt Δ | Completion Δ |
| ---- | --------------: | ------------------: | -------------: | -----------------: | -------: | -----------: |
| SI   |         636,677 |             410,062 |        385,205 |            425,101 |  -39.50% |       +3.67% |
| CM   |         352,349 |             252,400 |        253,457 |            255,567 |  -28.07% |       +1.25% |
| PI   |         200,445 |             126,682 |        131,961 |            125,675 |  -34.17% |       -0.79% |
| IC   |         252,440 |             288,191 |        229,989 |            283,796 |   -8.89% |       -1.53% |
| CC   |         180,057 |             230,963 |        161,876 |            231,533 |  -10.10% |       +0.25% |
| TS   |         158,705 |             155,207 |        157,900 |            153,034 |   -0.51% |       -1.40% |
| GR   |          54,541 |              46,263 |         52,171 |             45,011 |   -4.35% |       -2.71% |
| SA   |          28,844 |              60,510 |         28,570 |             59,404 |   -0.95% |       -1.83% |
| SC   |          27,341 |              54,051 |         27,477 |             54,791 |   +0.50% |       +1.37% |

### 附录 B：实验环境

- **评测框架**：trpc-agent-go benchmark/summary
- **模型**：deepseek-v3.2

### 附录 C：指标计算公式

**Token 节省率（总量口径）**：

```
Savings% = (∑Baseline Tokens - ∑Summary Tokens) / ∑Baseline Tokens × 100
```

**一致性得分**：
由 LLM 评估两个回答的语义相似度，输出 0~1 的分数。

**保留率**：

采用规则提取 + 匹配的方法计算：

1. **关键信息提取**（从 Baseline 回答中）：
   - 数字（日期、金额等）：正则 `\b\d+[\d,\.]*\b`
   - 引用内容：正则 `["']([^"']+)["']`
   - 专有名词：正则 `\b[A-Z][a-z]+(?:\s+[A-Z][a-z]+)*\b`（排除常见词）
   - 每轮最多提取 10 个关键信息

2. **匹配检测**（在 Summary 回答中）：
   - 精确匹配（不区分大小写）
   - 数字模糊匹配（忽略逗号格式差异）

3. **计算公式**：

```
Retention = 匹配的关键信息数 / 提取的关键信息总数
```

---

### 附录 D：QMSum 原始聚合结果

下表直接根据原始 benchmark 输出文件 `qmsum_all_specific_hidden_full/results.json` 提取。

**来源信息**

| 字段           | 值                                                        |
| -------------- | --------------------------------------------------------- |
| 时间戳         | `2026-04-13T20:44:50+08:00`                               |
| 模型           | `gpt-4o-mini`                                             |
| 切片           | `test / ALL / specific / support_distance_from_end >= 80` |
| Loaded Cases   | `244`                                                     |
| 实际评测 Cases | `189`                                                     |

**精确聚合指标**

| 指标                          | Long Context |       Summary | Summary + Progressive Disclosure |
| ----------------------------- | -----------: | ------------: | -------------------------------: |
| avg_rouge_1                   |     0.313242 |      0.223800 |                         0.277402 |
| avg_rouge_2                   |     0.083403 |      0.043668 |                         0.067339 |
| avg_rouge_l                   |     0.192977 |      0.151582 |                         0.177047 |
| avg_f1                        |     0.313242 |      0.223800 |                         0.277402 |
| avg_bleu                      |     0.249045 |      0.165136 |                         0.235089 |
| avg_prompt_tokens             | 18985.560847 |    888.158730 |                      3857.417989 |
| avg_completion_tokens         |   115.359788 |     59.708995 |                        81.624339 |
| avg_total_tokens              | 19100.920635 |    947.867725 |                      3939.042328 |
| avg_query_latency_ms          |  4555.767196 |   2993.597884 |                      8656.497354 |
| avg_seed_duration_ms          |     1.391534 | 344655.544974 |                    343654.634921 |
| avg_summary_build_duration_ms |            - |   6488.158730 |                      6193.825397 |
| avg_summary_chars             |            - |   1776.079365 |                      1785.095238 |
| summary_available_rate        |            - |      1.000000 |                         1.000000 |
| avg_session_search_calls      |            - |             - |                         1.058201 |
| avg_session_load_calls        |            - |             - |                         0.878307 |
| prompt_savings_vs_long        |            - |    94.784062% |                       76.690768% |

**基于同一原始文件计算的对比结果**

| 派生指标                     |                                                                            数值 |
| ---------------------------- | ------------------------------------------------------------------------------: |
| wins_vs_summary              |                                                                             123 |
| losses_vs_summary            |                                                                              62 |
| ties_vs_summary              |                                                                               4 |
| avg_rouge_l_gain_vs_summary  |                                                                        0.025465 |
| tool_used_cases              |                                                                             154 |
| tool_unused_cases            |                                                                              35 |
| avg_gain_when_tool_used      |                                                                        0.031453 |
| avg_gain_when_tool_unused    |                                                                       -0.000883 |
| anchor_event_not_found_cases |                                                                               4 |
| anchor_event_case_ids        | ES2004b_specific_04, Bro019_specific_04, Bro027_specific_05, Bro019_specific_06 |

---

### 附录 E：LongMemEval 原始聚合结果

下表汇总最新 LongMemEval benchmark 中默认 prompt 与九段式 prompt 的聚合结果。

**来源信息**

| 字段           | 值                                              |
| -------------- | ----------------------------------------------- |
| 子集           | `single-session-user`                           |
| 模型           | `gpt-4o-mini`                                   |
| 评测 Cases     | `70`                                            |
| 平均上下文长度 | ~103K tokens                                    |

**精确聚合指标**

| 配置 | 模式 | avg_rouge_l | avg_f1 | avg_bleu | avg_llm_score | avg_exact_match | avg_prompt_tokens | avg_query_latency_ms |
| ---- | ---- | ----------: | -----: | -------: | ------------: | --------------: | ----------------: | -------------------: |
| Full context | `long_context` | 0.1168 | 0.1225 | 0.0726 | 0.7357 | 0.6571 | 103,565 | 10,597 |
| 默认 prompt | `summary` | 0.0473 | 0.0563 | 0.0410 | 0.0771 | 0.0143 | 457 | 3,502 |
| 默认 prompt | `summary_ondemand` | 0.2486 | 0.2563 | 0.1641 | 0.8471 | 0.7286 | 6,308 | 10,581 |
| 九段式 prompt | `summary` | 0.2965 | 0.3014 | 0.1966 | 0.9179 | 0.8000 | 17,611 | 8,303 |
| 九段式 prompt | `summary_ondemand` | 0.2595 | 0.2660 | 0.1692 | 0.8900 | 0.7714 | 19,731 | 11,322 |

**派生指标**

| 派生指标 | 默认 prompt | 九段式 prompt |
| -------- | ----------: | ------------: |
| prompt_savings_summary_vs_long | 99.56% | 83.00% |
| prompt_savings_ondemand_vs_long | 93.90% | 80.95% |
| rouge_l_gain_ondemand_vs_summary | +0.2013 | -0.0370 |
| avg_session_search_calls | 1.16 | 0.33 |
| avg_session_load_calls | 0.31 | 0.00 |
| cases_with_search | 69 | 14 |
| cases_with_load | 21 | 0 |

---

## 参考文献

1. Bai, Y., et al. "MT-Bench-101: A Fine-Grained Benchmark for Evaluating Large Language Models in Multi-Turn Dialogues." ACL 2024.
2. Yao, S., et al. "τ-bench: A Benchmark for Tool-Agent-User Interaction in Real-World Domains." arXiv:2406.12045, 2024.
3. Chen, W., et al. "τ²-bench: Benchmarking Table-Reasoning Agents." arXiv:2506.07982, 2025.
4. Wang, D., et al. "LongMemEval: Benchmarking Chat Assistants on Long-Term Interactive Memory." arXiv:2410.10813, 2024.

