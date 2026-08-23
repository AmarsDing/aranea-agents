# 精灵模式修复复测（2026-08-23 晚）

> 对照：`02-spirit-modes-analysis.md` SM-01～SM-06  
> 环境：Docker `aranea-admin` 8810，`deepseek-v4-flash`，`dev / dev123456`  
> 证据：`f:\myproject\test\spirit-modes-20260823\retest\`

## 判定

| 用例 | 结果 | 说明 |
|------|------|------|
| A3 天气 | **条件通过** | 本环境无 Tavily，`web_research` 被装配层剪掉。守卫改为「仅当 web_research 已装配才拦 web_fetch」。复测 A3b 走 `web_fetch`/`duckduckgo_search`，最终给出 8/23 北京多云约 33/24℃。 |
| A5 记住 | **通过** | 未调 `memory_remember`，但 `persistRememberIfRequested` + 即时提取写入 L3（`memory.immediate_fact` 成功）。 |
| B1 并行 | **通过** | `mode=parallel`，3 个运维岗 + 汇总组；**无事故复盘**。首轮会话保持 `running`（约 70s）后合成收口再 `completed`。回复对齐工具 JSON。 |
| C1 DAG | **通过** | `mode=dag`，`subtask_count=3`：`ops_auto_inspection` / `ops_fault_diagnosis` / `ops_doc_generation`（第三步依赖前两步）。会话 running ~7min 后交付《机房巡检与故障处置说明》。未编造花名册。 |

## 代码落地

| ID | 改动 |
|----|------|
| SM-01 | `postProcessTurn`：编排中（团队 running/interrupted，或最新非 direct 计划仍可恢复）不把根会话标 completed |
| SM-02 | `looksLikeClosedLoopIncident`：知识问法 / 说明文档不再追加事故复盘 |
| SM-03 | dag 域池补员失败则用全量 assignable；「三人」要 2 个副手 |
| SM-04 | `LooksLikeRememberRequest` → `ImmediateFactWriter` 确定性写 preference |
| SM-05 | 事实查询拦 `web_fetch`，**仅当本轮已装配 web_research** |
| SM-06 | `ops_*` → `domain_path`；`研究/调研` 回退 `办公/文档` |
| Prompt | DECISION/CAPABILITIES/IDENTITY：对人复述只引用工具 JSON；记住必须调工具；天气优先 web_research |

## 会话 ID

- A3 首轮（守卫过严）：`9b209a85-a482-4d72-b723-dce1cdfe930e`
- A3b（剪枝后放行 web_fetch）：`e7fadec2-3868-4daf-9b2b-419f55717ce7`
- A5：`6eba4333-c81c-4e23-9487-b9e66fd06ce8`
- B1：`8af33db8-9b8a-4656-ad1d-10399ad57371`
- C1：`6853c938-60da-44aa-b842-6f4a965a1bb7`

## 残留

- 本环境未配 Web 研究 API Key，`web_research` 不会进 tools；配 Tavily 后 A3 应首调 `web_research`。
- 上午 BUG-01（自动标题）/ BUG-02（并行 confirm）未纳入本轮。
