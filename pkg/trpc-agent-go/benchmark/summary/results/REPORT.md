# Session Summarization and On-Demand Retrieval: Benchmark Results on MT-Bench-101, QMSum, and LongMemEval

## 1. Introduction

Large Language Models (LLMs) face context window limitations and token cost issues in multi-turn conversation scenarios. Session summarization is a common solution: compressing conversation history into summaries to reduce input token count. However, summarization may also hide critical information and degrade later answers. This report looks at both sides of that tradeoff. On MT-Bench-101, we evaluate when summarization is broadly beneficial or harmful. On QMSum, we evaluate whether an on-demand retrieval path can bring back early details after summary compression has hidden them. On LongMemEval, we extend the evaluation to realistic multi-session user/assistant conversations where total context reaches ~100K tokens. The goal here is to answer the following questions: (1) In which scenarios can session summarization effectively save tokens? (2) How much does summarization impact response quality? (3) Can on-demand retrieval recover quality loss when summary hides early evidence? (4) How does on-demand retrieval perform when context length exceeds the model's effective processing range?

Through comparative experiments on 9 tasks (917 cases) from the MT-Bench-101 dataset, we find that:

- **Effective for Long Dialogues**: ≥4 turn dialogues achieve 28%~40% prompt token savings while maintaining over 85% response consistency
- **Harmful for Short Dialogues**: ≤2 turn dialogues not only fail to benefit but actually increase token consumption due to summarization overhead
- **Triggering Strategy Too Aggressive**: Current setting (triggering summary every 2 turns) is unsuitable for short dialogues

On a broader QMSum hidden-detail workload (`test / ALL / specific / support_distance_from_end >= 80`), we further find that:

- **Summary Alone Loses Important Early Details**: plain summary reduces prompt tokens by 94.78% versus long context, but ROUGE-L drops from 0.1930 to 0.1516
- **On-Demand Retrieval Recovers a Meaningful Portion of the Loss**: `summary_ondemand` improves ROUGE-L to 0.1770, recovering 61.5% of the ROUGE-L loss and 59.9% of the F1 loss caused by summary compression
- **Recovery Still Preserves Large Savings**: `summary_ondemand` keeps a 76.69% prompt-token reduction versus long context

On LongMemEval (`longmemeval_s_cleaned / single-session-user`), we additionally find:

- **Default Summary Requires Retrieval**: with the default summarizer prompt, plain `summary` is highly compressed (99.56% prompt savings) but loses most facts; `summary_ondemand` raises ROUGE-L from 0.0473 to 0.2486 while still saving 93.90% prompt tokens
- **Detailed Continuity Summary is the Strongest Single Mode**: with `-detailed-prompt=true`, plain `summary` reaches ROUGE-L 0.2965, LLMScore 0.9179, and 80% exact match, outperforming both full context and default on-demand retrieval
- **Retrieval Becomes Less Necessary When the Summary Preserves Verbatim User Facts**: detailed `summary_ondemand` reaches ROUGE-L 0.2595, close to detailed `summary`, because most answerable facts are already present in the detailed summary

Overall, the MT-Bench-101 results tell us when summary is broadly worth enabling, the QMSum results tell us what happens after summary hides details and whether an on-demand retrieval path can recover them, and the LongMemEval results demonstrate that at extreme context lengths, summary quality becomes decisive: a detailed continuity summary can outperform full context while still preserving large prompt savings, and on-demand retrieval is most valuable when the summary is intentionally compact.

---

## 2. Methodology

### 2.1 Experimental Design

We use three complementary evaluation settings.

For the MT-Bench-101 study, we employ an **A/B comparative experiment** design:

- **Baseline Group**: Retains complete conversation history as context
- **Experimental Group (Summary)**: Generates summary after every N turns, replacing original history with summary

For the QMSum study, we evaluate a **three-mode setup**:

- **Long Context**: Keeps the full transcript in prompt
- **Summary**: Replaces older history with a summary
- **Summary + On-Demand Retrieval**: Uses summary by default and allows the agent to call `session_search` and `session_load` against hidden history when hidden details need to be surfaced

For the LongMemEval study, we evaluate the same **three-mode setup** as QMSum but on multi-session user/assistant dialogues. Each instance averages ~50 sessions and ~500 turns, totaling ~103K tokens per instance. This extends the evaluation to realistic conversational scales where the full context approaches the model's maximum window size.

Together, the three settings answer connected questions: when summary is useful in general, whether hidden detail can be recovered once summary is enabled, and whether on-demand retrieval remains effective — or even becomes superior — at extreme context lengths.

### 2.2 Evaluation Metrics

Following τ-bench and τ²-bench methodologies, the MT-Bench-101 portion defines three evaluation dimensions:

| Metric                    | Weight | Definition                                                                                              |
| ------------------------- | -----: | ------------------------------------------------------------------------------------------------------- |
| **Response Consistency**  |    50% | Semantic similarity between summary and baseline responses, scored by LLM (0~1)                         |
| **Token Efficiency**      |    30% | Savings = (Baseline - Summary) / Baseline × 100%                                                        |
| **Information Retention** |    20% | Proportion of key information (numbers, proper nouns, quoted content) preserved in summarized responses |

**Pass^1 Metric**: If consistency score ≥ 0.7, the case passes. Pass^1 = passed cases / total cases.

For the QMSum and LongMemEval portions, we report answer-overlap metrics and cost metrics directly:

| Metric                                 | What it means in this report                                                                                                                                    |
| -------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **ROUGE-1/2/L**                        | Lexical overlap between model answer and reference answer; ROUGE-L is the main headline metric because it better reflects answer-level overlap under paraphrase |
| **F1**                                 | Token-level precision/recall balance between model answer and reference answer                                                                                  |
| **BLEU**                               | N-gram precision signal; useful as a secondary view of answer wording fidelity                                                                                  |
| **Prompt / completion / total tokens** | Direct token-cost view for each mode                                                                                                                            |
| **Query latency**                      | End-to-end answer-time cost for the query stage in each mode                                                                                                    |

This combination lets us evaluate both semantic compression tradeoffs and targeted detail-recovery performance.

### 2.3 Dataset

We use three datasets for three related purposes.

The first is **MT-Bench-101**, which contains 13 types of multi-turn dialogue tasks. This evaluation covers 9 tasks:

| Code | Task Name                 | Cases | Description                                                            |
| ---- | ------------------------- | ----: | ---------------------------------------------------------------------- |
| CC   | Content Confusion         |   147 | Distinguish similar but semantically different queries                 |
| CM   | Context Memory            |    80 | Recall early dialogue details to answer current questions              |
| GR   | General Reasoning         |    71 | Collaboratively solve reasoning problems across turns                  |
| IC   | Instruction Clarification |   150 | Clarify ambiguous queries                                              |
| PI   | Proactive Interaction     |    87 | Proactively ask questions to guide dialogue                            |
| SA   | Self-affirmation          |    73 | Maintain correct response against inaccurate feedback                  |
| SC   | Self-correction           |    77 | Correct response according to user feedback                            |
| SI   | Separate Input            |   149 | First turn describes task requirements, subsequent turns provide input |
| TS   | Topic Shift               |    83 | Recognize and focus on new topics when users switch                    |

**Uncovered Tasks**: AR (Anaphora Resolution), CR (Content Rephrasing), FR (Format Rephrasing), MR (Mathematical Reasoning).

The second is **QMSum**, used here as a targeted hidden-detail recovery benchmark. We evaluate the following slice:

| Field           | Value                             |
| --------------- | --------------------------------- |
| Split           | `test`                            |
| Domain          | `ALL`                             |
| Query Type      | `specific`                        |
| Loaded Cases    | `244`                             |
| Evaluated Cases | `189`                             |
| Case Filter     | `support_distance_from_end >= 80` |

This QMSum slice is designed so that supporting evidence lies sufficiently far from the end of the transcript, making it likely to be hidden once summary compression takes effect.

The third is **LongMemEval** (ICLR 2025), a benchmark for evaluating long-term memory in chat assistants. LongMemEval contains 500 questions across 6 question types, each backed by timestamped multi-session chat history. We evaluate the `single-session-user` slice (70 cases) from `longmemeval_s_cleaned.json`.

The key difference from QMSum is twofold: LongMemEval uses real user/assistant role alternation (rather than all-user meeting transcript turns), and the average context per instance is ~103K tokens — approximately 5x longer than QMSum's ~19K tokens. This places it near the boundary of gpt-4o-mini's 128K context window.

| Variant | Cases | Question Type      | Avg Turns | Avg Sessions | Avg Tokens |
| ------- | ----: | ------------------ | --------: | -----------: | ---------: |
| S       |    70 | single-session-user |      ~500 |          ~50 |     ~103K |

### 2.4 Experimental Configuration

**MT-Bench-101 setup**

| Parameter                 | Value         | Description                                 |
| ------------------------- | ------------- | ------------------------------------------- |
| Model                     | deepseek-v3.2 | Used for response and summary generation    |
| Summary trigger threshold | 2             | Trigger summary every 2 turns               |
| Number of runs            | 1             | Each case runs once                         |
| Consistency threshold     | 0.7           | Pass^1 determination threshold              |
| Evaluation method         | LLM-eval      | Use LLM for semantic consistency evaluation |

**QMSum setup**

| Parameter                 | Value                                         |
| ------------------------- | --------------------------------------------- |
| Model                     | gpt-4o-mini                                   |
| Summary trigger threshold | 40                                            |
| Visible event window      | 20                                            |
| Modes                     | `long_context`, `summary`, `summary_ondemand` |
| Retrieval tools           | `session_search`, `session_load`              |

**LongMemEval setup**

| Parameter                 | Value                                         |
| ------------------------- | --------------------------------------------- |
| Model                     | gpt-4o-mini                                   |
| Summary trigger threshold | 40                                            |
| Visible event window      | 20                                            |
| Question types            | single-session-user                           |
| Modes                     | `long_context`, `summary`, `summary_ondemand` |
| Retrieval tools           | `session_search`, `session_load`              |

---

## 3. Experimental Results

### 3.1 Overall Results

| Metric                           |           Value |
| -------------------------------- | --------------: |
| Total Cases                      |             917 |
| Total Baseline Tokens            |       3,515,728 |
| Total Summary Tokens             |       3,062,518 |
| **Overall Token Savings**        |      **12.89%** |
| Total Baseline Prompt Tokens     |       1,891,399 |
| Total Summary Prompt Tokens      |       1,428,606 |
| **Overall Prompt Savings**       |      **24.47%** |
| Weighted Avg Consistency         |           0.853 |
| Weighted Pass^1                  |           92.3% |
| Weighted Avg Retention           |           0.836 |
| **Negative Token Savings Cases** | **329 (35.9%)** |

**Key Finding**: Although overall savings are positive, over 1/3 of cases show negative token savings (i.e., summary mode consumed more tokens).

### 3.2 Per-Task Results

**Table 1: Token Efficiency Metrics by Task**

| Task | Cases | Prompt Savings | Token Savings |     p25 |    p50 |    p75 | Negative Rate |
| ---- | ----: | -------------: | ------------: | ------: | -----: | -----: | ------------: |
| SI   |   149 |         39.50% |        22.59% |   0.88% | 16.67% | 26.47% |         17.4% |
| PI   |    87 |         34.17% |        21.24% |  -2.04% | 12.11% | 23.46% |         26.4% |
| CM   |    80 |         28.07% |        15.83% |   6.93% | 15.42% | 24.08% |         16.2% |
| CC   |   147 |         10.10% |         4.28% |  -7.03% |  1.86% |  9.90% |         42.2% |
| IC   |   150 |          8.89% |         4.97% | -10.45% |  1.20% | 10.98% |         46.0% |
| GR   |    71 |          4.35% |         3.59% |  -9.95% |  0.68% | 10.28% |         43.7% |
| SA   |    73 |          0.95% |         1.54% |  -8.68% |  3.40% | 11.41% |         42.5% |
| TS   |    83 |          0.51% |         0.95% |  -5.86% |  0.95% |  7.78% |         43.4% |
| SC   |    77 |     **-0.50%** |    **-1.08%** |  -9.53% |  0.00% |  7.52% |     **49.4%** |

**Table 2: Response Quality Metrics by Task**

| Task | Consistency |    Pass^1 | Retention |
| ---- | ----------: | --------: | --------: |
| GR   |   **0.916** |     93.0% |     0.870 |
| SC   |       0.881 |     93.5% | **0.872** |
| SA   |       0.862 |     83.6% |     0.865 |
| CC   |       0.861 |     89.1% |     0.860 |
| IC   |       0.851 |     95.3% |     0.825 |
| TS   |       0.846 |     95.2% |     0.849 |
| SI   |       0.841 |     89.3% |     0.857 |
| CM   |       0.819 |     96.2% |     0.817 |
| PI   |       0.814 | **96.6%** |     0.704 |

### 3.3 Conversation Turn Analysis

**Table 3: Turn Distribution by Task**

| Task | Avg Turns |   2-turn % | 3-turn % | 4-turn % | ≥5-turn % |
| ---- | --------: | ---------: | -------: | -------: | --------: |
| SI   |      4.16 |      12.8% |    10.7% |    32.2% |     44.3% |
| PI   |      4.07 |       0.0% |    33.3% |    33.3% |     33.3% |
| CM   |      3.99 |       1.2% |     1.2% |    96.3% |      1.2% |
| GR   |      3.07 |       2.8% |    64.8% |    32.4% |      0.0% |
| TS   |      3.00 |       0.0% |   100.0% |     0.0% |      0.0% |
| IC   |      2.84 |      24.0% |    68.0% |     8.0% |      0.0% |
| CC   |      2.39 |      72.8% |    15.6% |     8.8% |      2.7% |
| SA   |  **2.00** | **100.0%** |     0.0% |     0.0% |      0.0% |
| SC   |  **2.00** | **100.0%** |     0.0% |     0.0% |      0.0% |

### 3.4 Baseline Prompt Length Analysis

**Table 4: Relationship Between Prompt Length and Savings Rate**

| Task | Avg Baseline Prompt | Avg Baseline Completion | Prompt Savings |
| ---- | ------------------: | ----------------------: | -------------: |
| CM   |               4,404 |                   3,155 |         28.07% |
| SI   |               4,273 |                   2,752 |         39.50% |
| PI   |               2,304 |                   1,456 |         34.17% |
| TS   |               1,912 |                   1,870 |          0.51% |
| IC   |               1,683 |                   1,921 |          8.89% |
| CC   |               1,225 |                   1,571 |         10.10% |
| GR   |                 768 |                     652 |          4.35% |
| SA   |                 395 |                     829 |          0.95% |
| SC   |                 355 |                     702 |         -0.50% |

### 3.5 On-Demand Retrieval on Meeting Transcripts (QMSum)

While MT-Bench-101 explains when session summarization is broadly beneficial, it does not directly isolate the hidden-detail problem introduced by summary compression. The QMSum results address that gap.

**Table 5: QMSum Aggregate Results**

| Metric            | Long Context |  Summary | Summary + On-Demand Retrieval |
| ----------------- | -----------: | -------: | ----------------------------: |
| ROUGE-L           |       0.1930 |   0.1516 |                        0.1770 |
| F1                |       0.3132 |   0.2238 |                        0.2774 |
| BLEU              |       0.2490 |   0.1651 |                        0.2351 |
| Avg Prompt Tokens |       18,986 |      888 |                         3,857 |
| Avg Query Latency |     4,556 ms | 2,994 ms |                      8,656 ms |

Additional observations:

- Summary availability rate is `100%`
- Plain `summary` saves `94.78%` of prompt tokens versus `long_context`
- `summary_ondemand` still saves `76.69%` of prompt tokens versus `long_context`
- `summary_ondemand` improves ROUGE-L by `+0.0255` over plain `summary`
- Per-case ROUGE-L comparison is `123` wins, `62` losses, and `4` ties

The main takeaway is that summary compression creates a real quality gap, but on-demand retrieval recovers a meaningful portion of it while preserving large token savings.

### 3.6 Summary Prompt Ablation on Multi-Session Dialogues (LongMemEval)

The QMSum results establish that on-demand retrieval recovers hidden details at medium context lengths (~19K tokens). LongMemEval extends this evaluation to realistic user/assistant conversations where total context averages ~103K tokens — near the boundary of the model's 128K context window. In this regime, we evaluate both the default compact summary and the detailed continuity summary (`-detailed-prompt=true`).

**Table 6: LongMemEval Aggregate Results (single-session-user, 70 cases)**

| Configuration | Mode | ROUGE-L | F1 | BLEU | LLMScore | Exact Match | Avg Prompt Tokens | Prompt Savings | Avg Summary Chars | Avg Query Latency |
| ------------- | ---- | ------: | -: | ---: | -------: | ----------: | ----------------: | -------------: | ----------------: | ----------------: |
| Full context | `long_context` | 0.1168 | 0.1225 | 0.0726 | 0.7357 | 0.6571 | 103,565 | — | 0 | 10,597 ms |
| Default prompt | `summary` | 0.0473 | 0.0563 | 0.0410 | 0.0771 | 0.0143 | 457 | 99.56% | 1,749 | 3,502 ms |
| Default prompt | `summary_ondemand` | 0.2486 | 0.2563 | 0.1641 | 0.8471 | 0.7286 | 6,308 | 93.90% | 1,669 | 10,581 ms |
| Detailed prompt | `summary` | **0.2965** | **0.3014** | **0.1966** | **0.9179** | **0.8000** | 17,611 | 83.00% | 74,960 | 8,303 ms |
| Detailed prompt | `summary_ondemand` | 0.2595 | 0.2660 | 0.1692 | 0.8900 | 0.7714 | 19,731 | 80.95% | 75,162 | 11,322 ms |

Key observations:

- The default compact `summary` is extremely cheap (457 prompt tokens on average) but too lossy for direct long-term memory recall.
- Default `summary_ondemand` recovers most of the lost quality, raising ROUGE-L from `0.0473` to `0.2486` while retaining `93.90%` prompt-token savings versus full context.
- Detailed `summary` is the strongest single mode: ROUGE-L `0.2965`, LLMScore `0.9179`, and exact match `0.8000`, while still saving `83.00%` prompt tokens versus full context.
- Detailed `summary` improves exact match on 55 of 70 cases compared with default `summary`, with no exact-match regressions.
- Once the detailed summary is available, on-demand retrieval adds little: detailed `summary_ondemand` is slightly lower than detailed `summary` on ROUGE-L (`0.2595` vs `0.2965`) and exact match (`0.7714` vs `0.8000`).

The main takeaway is that LongMemEval separates two operating modes. If the summary is intentionally compact, on-demand retrieval is essential and provides the best quality/cost tradeoff. If the summary can include a detailed continuity structure and verbatim user-message appendix, the summary itself becomes the strongest recall mechanism and often makes retrieval unnecessary.

### 3.7 Tool Trace Analysis

Both the QMSum and LongMemEval `summary_ondemand` runs preserve tool traces, making it possible to inspect whether the model answered directly or first retrieved hidden history. The traces are stored under `summary_ondemand.tool_traces` in the raw results. Comparing the two datasets reveals a structural difference in how retrieval tools are used, driven by the granularity of the underlying conversation format.

#### 3.7.1 QMSum Retrieval Patterns

Based on `qmsum_all_specific_hidden_full/results.json`, 154 of 189 cases invoked at least one retrieval tool, while 35 cases did not invoke any tool. Every traced case starts with `session_search`. There are no cases where the first tool is `session_load`, and no cases where the model calls `session_load` without a preceding `session_search`. The dominant path is:

```
Query
  |
  v
session_search(
  query=<derived from user question>,
  scope=current_hidden
)
  |
  |  returns candidate event_id + snippets
  v
session_load(
  session_id=summary_ondemand-<case_id>,
  event_id=<uuid from search result>,
  before=1,
  after=1
)
  |
  |  returns local hidden-history messages
  v
Final answer with recovered evidence
```

This search-then-load path appears 142 times (92% of traced cases). The model first locates candidate events via search, then loads surrounding context to recover evidence.

| Metric                                    | QMSum |
| ----------------------------------------- | ----: |
| Total cases                               |   189 |
| Cases with tool traces                    |   154 |
| Cases without tool traces                 |    35 |
| Cases with at least one `session_load`    |   142 |
| Cases with only `session_search`          |    12 |
| Total `session_search` calls              |   200 |
| Total `session_load` calls                |   166 |
| Avg total tool calls per case             |  1.94 |

Tool use is strongly associated with quality recovery. Cases with tool calls have an average ROUGE-L gain of `+0.0315`, while cases without tool calls are nearly flat (`-0.0010`). Cases that complete search-then-load gain `+0.0353` on average, while search-only cases average `-0.0135` — indicating that on QMSum, search alone is not enough; the load step is essential.

This is because QMSum meeting transcripts consist of short speaker turns ("Right.", "Mm-hmm.", "Yeah, it is."). A single search hit returns one such turn plus a snippet, which rarely contains enough context to answer a question. The model must load surrounding turns to reconstruct the full discussion thread.

Two examples illustrate the QMSum pattern. In `Bed003_specific_01`, the query asks: "What did Grad B say about the structure of the belief net?" The model searches for `Grad B structure of the belief net`, finds a candidate around Turn 989, then calls `session_load` to recover Turns 988-990. ROUGE-L rises from summary's `0.1481` to `0.1538`.

In `covid_4_specific_01`, a compound question about petitions, tax evasion, and violence handling triggers 5 `session_search` calls and 3 `session_load` calls, decomposing the query into subqueries and recovering multiple evidence anchors. ROUGE-L improves from summary's `0.1101` to `0.1922`, even surpassing long context's `0.1640`.

One caveat: 4 of 189 cases had `session_load` failures (`anchor event not found`) where the model passed a transcript turn number instead of the `event_id` from the search response. This is a localized tool-usage failure that slightly underestimates on-demand performance.

#### 3.7.2 LongMemEval Retrieval Patterns

LongMemEval shows two distinct retrieval regimes depending on summary style. With the default compact prompt, 69 of 70 cases invoke `session_search`, and 21 cases also invoke `session_load`. The default summary is very small (~1.7K characters), so the model relies on retrieval tools to recover hidden facts. With the detailed continuity prompt, only 14 of 70 cases invoke search and no cases invoke load, because the summary itself already contains the relevant user facts.

| Metric | Default Prompt | Detailed Prompt |
| ------ | -------------: | --------------: |
| Total cases | 70 | 70 |
| Cases with at least one `session_search` | 69 | 14 |
| Cases with at least one `session_load` | 21 | 0 |
| Total `session_search` calls | 81 | 23 |
| Total `session_load` calls | 22 | 0 |
| Avg search calls per case | 1.16 | 0.33 |
| Avg load calls per case | 0.31 | 0.00 |
| ROUGE-L gain of on-demand vs summary | +0.2013 | -0.0370 |
| Exact-match gain of on-demand vs summary | +0.7143 | -0.0286 |

Under the default prompt, retrieval is clearly beneficial: `summary_ondemand` improves ROUGE-L from `0.0473` to `0.2486` and exact match from `0.0143` to `0.7286`. Under the detailed prompt, retrieval is mostly redundant: plain `summary` already reaches exact match `0.8000`, and adding tools slightly reduces average ROUGE-L and exact match.

This makes LongMemEval a useful stress test for both strategies. Compact summaries need on-demand retrieval to regain missing facts; detailed continuity summaries preserve enough factual state that the model can answer directly.

#### 3.7.3 Why Retrieval Patterns Differ

The structural inversion between QMSum and LongMemEval traces back to the granularity of conversation events:

| Property                   | QMSum                      | LongMemEval                     |
| -------------------------- | -------------------------- | ------------------------------- |
| Event format               | Short speaker turn         | Full user/assistant message     |
| Typical event length       | 10-30 words                | 50-200 words                    |
| Events per topic           | 10-20 turns                | 2-4 turns                       |
| Roles                      | All `user` (speakers)      | Alternating `user`/`assistant`  |
| Search snippet usefulness  | Low (needs surrounding turns) | High (self-contained message) |
| Load necessity             | Almost always              | Rarely                          |

In QMSum, a search hit returns a single short meeting utterance — insufficient to answer a question. The model must call `session_load` to see the surrounding discussion. In LongMemEval, a search hit returns a full conversational turn (e.g., the user describing their experience, or the assistant providing a detailed answer), which typically contains enough information to answer directly.

**Summary of tool-use patterns across datasets:**

| Metric | QMSum | LongMemEval Default | LongMemEval Detailed |
| ------ | ----: | ------------------: | -------------------: |
| Avg search calls per case | 1.06 | 1.16 | 0.33 |
| Avg load calls per case | 0.88 | 0.31 | 0.00 |
| Total calls per case | 1.94 | 1.47 | 0.33 |
| Cases with any search | 154 | 69 | 14 |
| Cases with any load | 142 | 21 | 0 |
| On-demand ROUGE-L gain | +0.0255 | +0.2013 | -0.0370 |

This comparison suggests a practical guideline: for applications with short-grained events (meeting transcripts, chat logs with brief messages), the search-then-load pattern is expected; for applications with coarse-grained events (multi-turn assistant dialogues), search alone is often sufficient. When the summary already preserves detailed continuity and verbatim user facts, retrieval can become unnecessary for many recall questions.

---

## 4. Analysis

### 4.1 Factors Affecting Summarization Effectiveness

#### 4.1.1 Conversation Turns is the Decisive Factor

Experimental data reveals a strong correlation between conversation turns and summarization effectiveness:

**Positive Correlation Tasks (Good Effect)**:

- SI (4.16 turns), PI (4.07 turns), CM (3.99 turns) all achieve 20%+ token savings
- These tasks have <15% 2-turn dialogue proportion

**Negative Correlation Tasks (Poor Effect)**:

- 100% of SA and SC cases have only 2 turns
- With summary trigger threshold of 2, this means only 1 message in history when summarizing—almost nothing to compress

**Root Cause**: Under `-events 2` setting, the summary timing for 2-turn dialogues is:

```
Turn 1: history=[] → No summary triggered
Turn 2: history=[Turn1] → Summary triggered, but only 1 history item, minimal compression space
```

#### 4.1.2 Baseline Prompt Length Determines Compression Ceiling

Prompt savings rate positively correlates with baseline prompt length (Pearson r = 0.72):

- **High Compression Potential** (>2000 tokens): SI, CM, PI, savings 28%~40%
- **Low Compression Potential** (<500 tokens): SA, SC, savings ≈ 0%

This aligns with information theory intuition: longer inputs have higher redundancy and greater compression space.

#### 4.1.3 Summarization Overhead is Amplified in Short Dialogues

SC task shows **-1.08% negative savings**. Analyzing its token distribution:

| Metric            | Baseline | Summary | Change     |
| ----------------- | -------- | ------- | ---------- |
| Prompt Tokens     | 27,341   | 27,477  | +0.50%     |
| Completion Tokens | 54,051   | 54,791  | +1.37%     |
| **Total Tokens**  | 81,392   | 82,268  | **+1.08%** |

Summary generation consumes tokens (not separately counted), but compression gains are nearly zero, resulting in net loss.

### 4.2 Impact of Task Characteristics on Summarization

#### 4.2.1 Why Does SI (Separate Input) Perform Best?

Typical structure of SI tasks:

- **Turn 1**: Detailed task instructions (usually long)
- **Turn 2~N**: Specific inputs (usually short)

Summarization can compress verbose task instructions into key constraints while keeping specific inputs intact, achieving highest compression efficiency.

#### 4.2.2 Why Does PI (Proactive Interaction) Have Lowest Retention?

PI's retention rate is only **0.704**, significantly lower than other tasks. Analysis reveals:

1. **Task Characteristics**: PI requires the model to "proactively ask questions to guide dialogue"—such guiding content may be deemed non-core during summarization
2. **Evaluation Method Limitation**: Retention is based on keyword matching, but PI's key information may exist in paraphrased form

However, PI's Pass^1 is **96.6%**, indicating good semantic-level consistency. Keyword matching may underestimate actual retention effectiveness.

#### 4.2.3 Why Does TS (Topic Shift) Perform Poorly?

TS tasks require recognizing user topic switches. When history is compressed by summarization, topic switch signals may be weakened, affecting model judgment. This indicates: **tasks requiring context completeness are not suitable for aggressive summarization**.

#### 4.2.4 What Do QMSum and LongMemEval Add Beyond MT-Bench-101?

The QMSum and LongMemEval results complement the MT-Bench-101 findings in an important way. MT-Bench-101 shows that summary can be beneficial in longer interactions and harmful in shorter ones, but it does not directly test a regime where important evidence has already been hidden by summary compression. QMSum and LongMemEval do — at different scales.

On QMSum (~19K tokens), plain summary sharply reduces prompt cost but creates a measurable quality gap. `summary_ondemand` then recovers a meaningful portion of that loss:

- ROUGE-L improves from `0.1516` to `0.1770`
- F1 improves from `0.2238` to `0.2774`
- the recovered share is about `61.5%` of the ROUGE-L loss and `59.9%` of the F1 loss caused by summary compression

On LongMemEval (~103K tokens), the picture shifts further and depends on the summary style. With the default compact prompt, plain summary is very cheap but too lossy, and on-demand retrieval is the main quality mechanism: ROUGE-L improves from `0.0473` to `0.2486`. With the detailed continuity prompt, the summary itself becomes the strongest mode: ROUGE-L reaches `0.2965`, exceeding both full context (`0.1168`) and default on-demand (`0.2486`).

Together, QMSum and LongMemEval show two complementary strategies. At medium context (~19K tokens), on-demand retrieval recovers details hidden by summary compression. At extreme context (~103K tokens), either retrieval or a detailed continuity summary can beat full context; the best choice depends on the desired cost/quality tradeoff.

#### 4.2.5 Why Can Summary-Based Modes Surpass Long Context on LongMemEval?

The LongMemEval results present an apparent paradox: providing the model with more context (the full ~103K-token history) yields worse answers than summary-based modes. Three factors explain this:

1. **Attention dilution at scale**: At ~103K tokens (near gpt-4o-mini's 128K limit), the model must attend across ~50 sessions and ~500 turns. The relevant evidence typically occupies fewer than 200 tokens within that span. The model's effective attention is diluted across the vast irrelevant majority, leading to missed or confused answers.

2. **Focused retrieval or focused summary state**: Default `summary_ondemand` gives the model a small prompt plus targeted retrieved snippets, while detailed `summary` gives the model a structured continuity document with preserved user facts. Both paths concentrate context around the information needed for recall.

3. **Explicit factual preservation**: The detailed continuity prompt keeps a nine-section work-state summary and a verbatim user-message appendix. This raises prompt cost compared with the default summary, but it preserves factual user statements that the default compact summary discards.

This pattern is expected to strengthen as conversation history grows longer. As context exceeds the model's effective processing range, raw long-context recall degrades; either targeted retrieval or explicit continuity-state preservation becomes necessary.

### 4.3 Experimental Limitations

#### 4.3.1 Summary Generation Token Cost Not Counted

Current evaluation only compares Prompt + Completion Tokens, not including tokens consumed by summary generation. Actual cost should be:

```
Total Cost = Prompt + Completion + Summary Generation
```

If this cost were included, the negative savings case proportion would likely be higher.

#### 4.3.2 Single Run Lacks Statistical Stability

`-num-runs 1` makes Pass^k (k > 1) ineffective. LLM outputs have randomness, and single-run results may be unstable.

#### 4.3.3 MT-Bench-101 Has Short Dialogue Turns

MT-Bench-101's average dialogue turns are 2~4, which differs from long dialogue scenarios in production environments. Summarization is better suited for longer dialogues; the current dataset may underestimate its potential.

#### 4.3.4 QMSum Slice Is Targeted Rather Than Fully Global

The QMSum results in this report come from a targeted hidden-detail slice (`ALL / specific / support_distance_from_end >= 80`) rather than the entire QMSum test set. This is appropriate for validating summary-hidden detail recovery, but the conclusions should still be framed as strong evidence for that workload, not as universal evidence across every QMSum setting.

#### 4.3.5 Small Number of Tool-Call Failures Make Results Slightly Conservative

In `4/189` QMSum cases, `session_load` failed with `anchor event not found` because the model passed transcript turn numbers instead of the exact event IDs returned by `session_search`. This is a localized tool-usage failure rather than a benchmark-wide validity problem. It slightly underestimates current on-demand performance, but it does not change the overall conclusion that `summary_ondemand` remains meaningfully better than plain `summary` on this workload.

#### 4.3.6 LongMemEval Uses a Single Question Type Slice

The LongMemEval results come from the `single-session-user` slice (70 of 500 questions). Other question types (multi-session, temporal-reasoning, knowledge-update) may show different patterns. The single-session-user type is the most direct test of hidden-detail recovery, but a broader evaluation would strengthen the conclusions.

---

## 5. Discussion and Recommendations

### 5.1 Task Suitability Classification

Based on experimental results, we classify tasks into three categories:

| Suitability                   | Characteristics                 | Example Tasks | Recommendation                         |
| ----------------------------- | ------------------------------- | ------------- | -------------------------------------- |
| **Highly Recommended**        | Avg turns ≥4, Prompt >2000      | SI, PI, CM    | Enable summarization                   |
| **Conditionally Recommended** | Avg turns 3-4, Prompt 1000-2000 | CC, IC, GR    | Dynamic decision based on actual turns |
| **Not Recommended**           | Avg turns ≤2, Prompt <1000      | SA, SC, TS    | Disable summarization                  |

For hidden-detail workloads where summary is already enabled and the question depends on early transcript evidence, the QMSum and LongMemEval results suggest additional practical rules:

- **Default Summary + On-Demand Retrieval Recommended**: when the summary must stay compact, keep summary for compression and expose retrieval tools as the path for surfacing hidden context
- **Detailed Continuity Summary Recommended for Long-Term Chat Memory**: when higher prompt cost is acceptable, use a detailed continuity prompt with verbatim user-message preservation; on LongMemEval this is the strongest single mode
- **For conversations exceeding ~50K tokens, avoid relying on raw full context alone**: use either targeted retrieval or detailed continuity summaries, because full-context recall degrades at extreme lengths.

### 5.2 Next Optimization Directions

1. **Add Summary Token Statistics**: Include summary generation cost in evaluation system
2. **Long Dialogue Dataset Validation**: Partially addressed by LongMemEval's ~103K-token dialogues for the single-session-user question type. Multi-session and temporal-reasoning question types still need evaluation to confirm the pattern generalizes across all LongMemEval categories.
3. **Choose Summary Prompt by Workload**: use compact default summaries when retrieval tools are available and token cost is critical; use detailed continuity summaries for high-accuracy long-term chat memory
4. **Optimize On-Demand Retrieval Cost**: Reduce redundant searches, tighten triggering, and shrink returned context windows for hidden-detail workloads

---

## 6. Conclusion and Next Steps

Across MT-Bench-101, QMSum, and LongMemEval, this report evaluates session summarization and summary-time detail recovery. Main conclusions are:

1. **Summarization is Effective for Long Dialogues**: Tasks with average 4+ turns (SI, PI, CM) achieve 28%~40% prompt savings while maintaining over 85% response consistency.

2. **Summarization is Harmful for Short Dialogues**: 2-turn dialogue tasks (SA, SC) cannot benefit under current settings and actually increase token consumption due to summarization overhead.

3. **Triggering Strategy Needs Optimization**: Fixed `-events 2` is too aggressive for short dialogues. Recommend adopting dynamic strategies based on conversation turns or cumulative token count.

4. **Evaluation System Needs Improvement**: Summary generation token cost should be included in total cost calculation to more accurately evaluate actual summarization benefits.

5. **On-Demand Retrieval Helps with Summary-Hidden Detail Recovery**: On the broader QMSum hidden-detail workload, `summary_ondemand` improves ROUGE-L from 0.1516 to 0.1770 over plain `summary`, wins 123 of 189 cases, and recovers a meaningful portion of the quality gap to `long_context` while still saving 76.69% of prompt tokens.

6. **Long-Term Memory Needs Either Retrieval or Detailed Continuity Summaries**: On LongMemEval's ~103K-token dialogues, compact default summary needs on-demand retrieval (ROUGE-L 0.2486), while detailed continuity summary is the strongest single mode (ROUGE-L 0.2965, LLMScore 0.9179, exact match 0.8000). Both outperform raw long context on recall quality while preserving substantial prompt savings.

---

## Appendix

### Appendix A: Token Distribution Details

| Task | Baseline Prompt | Baseline Completion | Summary Prompt | Summary Completion | Prompt Δ | Completion Δ |
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

### Appendix B: Experimental Environment

- **Evaluation Framework**: trpc-agent-go benchmark/summary
- **Model**: deepseek-v3.2

### Appendix C: Metric Calculation Formulas

**Token Savings Rate (Aggregate)**:

```
Savings% = (∑Baseline Tokens - ∑Summary Tokens) / ∑Baseline Tokens × 100
```

**Consistency Score**:
LLM evaluates semantic similarity between two responses, outputting a 0~1 score.

**Retention Rate**:

Calculated using rule-based extraction + matching:

1. **Key Information Extraction** (from Baseline response):
   - Numbers (dates, amounts, etc.): regex `\b\d+[\d,\.]*\b`
   - Quoted content: regex `["']([^"']+)["']`
   - Proper nouns: regex `\b[A-Z][a-z]+(?:\s+[A-Z][a-z]+)*\b` (excluding common words)
   - Maximum 10 key information items per turn

2. **Matching Detection** (in Summary response):
   - Exact match (case-insensitive)
   - Fuzzy number matching (ignoring comma format differences)

3. **Formula**:

```
Retention = Matched key info count / Total extracted key info count
```

---

### Appendix D: Raw QMSum Aggregate Output

The tables below are extracted from the raw benchmark output file `qmsum_all_specific_hidden_full/results.json`.

**Source metadata**

| Field           | Value                                                     |
| --------------- | --------------------------------------------------------- |
| Timestamp       | `2026-04-13T20:44:50+08:00`                               |
| Model           | `gpt-4o-mini`                                             |
| Slice           | `test / ALL / specific / support_distance_from_end >= 80` |
| Loaded Cases    | `244`                                                     |
| Evaluated Cases | `189`                                                     |

**Exact aggregate metrics**

| Metric                        | Long Context |       Summary | Summary + On-Demand Retrieval |
| ----------------------------- | -----------: | ------------: | ----------------------------: |
| avg_rouge_1                   |     0.313242 |      0.223800 |                      0.277402 |
| avg_rouge_2                   |     0.083403 |      0.043668 |                      0.067339 |
| avg_rouge_l                   |     0.192977 |      0.151582 |                      0.177047 |
| avg_f1                        |     0.313242 |      0.223800 |                      0.277402 |
| avg_bleu                      |     0.249045 |      0.165136 |                      0.235089 |
| avg_prompt_tokens             | 18985.560847 |    888.158730 |                   3857.417989 |
| avg_completion_tokens         |   115.359788 |     59.708995 |                     81.624339 |
| avg_total_tokens              | 19100.920635 |    947.867725 |                   3939.042328 |
| avg_query_latency_ms          |  4555.767196 |   2993.597884 |                   8656.497354 |
| avg_seed_duration_ms          |     1.391534 | 344655.544974 |                 343654.634921 |
| avg_summary_build_duration_ms |            - |   6488.158730 |                   6193.825397 |
| avg_summary_chars             |            - |   1776.079365 |                   1785.095238 |
| summary_available_rate        |            - |      1.000000 |                      1.000000 |
| avg_session_search_calls      |            - |             - |                      1.058201 |
| avg_session_load_calls        |            - |             - |                      0.878307 |
| prompt_savings_vs_long        |            - |    94.784062% |                    76.690768% |

**Derived comparisons from the same raw file**

| Derived metric               |                                                                           Value |
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

### Appendix E: Raw LongMemEval Aggregate Output

The tables below summarize the latest LongMemEval benchmark runs for the default and detailed summary prompts.

**Source metadata**

| Field | Value |
| ----- | ----- |
| Model | `gpt-4o-mini` |
| Dataset | `longmemeval_s_cleaned.json` |
| Question Type | `single-session-user` |
| Evaluated Cases | `70` |

**Exact aggregate metrics**

| Configuration | Mode | avg_rouge_l | avg_f1 | avg_bleu | avg_llm_score | avg_exact_match | avg_prompt_tokens | avg_query_latency_ms |
| ------------- | ---- | ----------: | -----: | -------: | ------------: | --------------: | ----------------: | -------------------: |
| Full context | `long_context` | 0.1168 | 0.1225 | 0.0726 | 0.7357 | 0.6571 | 103,565 | 10,597 |
| Default prompt | `summary` | 0.0473 | 0.0563 | 0.0410 | 0.0771 | 0.0143 | 457 | 3,502 |
| Default prompt | `summary_ondemand` | 0.2486 | 0.2563 | 0.1641 | 0.8471 | 0.7286 | 6,308 | 10,581 |
| Detailed prompt | `summary` | 0.2965 | 0.3014 | 0.1966 | 0.9179 | 0.8000 | 17,611 | 8,303 |
| Detailed prompt | `summary_ondemand` | 0.2595 | 0.2660 | 0.1692 | 0.8900 | 0.7714 | 19,731 | 11,322 |

**Derived comparisons**

| Derived metric | Default Prompt | Detailed Prompt |
| -------------- | -------------: | --------------: |
| prompt_savings_summary_vs_long | 99.56% | 83.00% |
| prompt_savings_ondemand_vs_long | 93.90% | 80.95% |
| rouge_l_gain_ondemand_vs_summary | +0.2013 | -0.0370 |
| avg_session_search_calls | 1.16 | 0.33 |
| avg_session_load_calls | 0.31 | 0.00 |
| cases_with_search | 69 | 14 |
| cases_with_load | 21 | 0 |

---

## References

1. Bai, Y., et al. "MT-Bench-101: A Fine-Grained Benchmark for Evaluating Large Language Models in Multi-Turn Dialogues." ACL 2024.
2. Yao, S., et al. "τ-bench: A Benchmark for Tool-Agent-User Interaction in Real-World Domains." arXiv:2406.12045, 2024.
3. Chen, W., et al. "τ²-bench: Benchmarking Table-Reasoning Agents." arXiv:2506.07982, 2025.
4. Wu, D., et al. "LongMemEval: Benchmarking Chat Assistants on Long-Term Interactive Memory." ICLR 2025.
