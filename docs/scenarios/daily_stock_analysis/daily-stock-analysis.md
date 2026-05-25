# Daily Stock Analysis — 场景化需求文档

> **场景代号**：`daily_stock_analysis`
> **场景定位**：基于 Aranea-Agents 多智能体编排平台构建的**开源每日股票分析助手**。
> **文档版本**：v1.1（2026-05-25；补充 Postgres 存储、实时性分级、多 Agent 数据协作）
> **相关文档**：[设计文档](./daily-stock-analysis.design.md) · [开发计划](./daily-stock-analysis-development.md) · [场景索引](./README.md)
> **依赖平台模块**：Agent / Team / Graph / Tools / Skill / Knowledge / Cron / Channel / Artifact / Memory / Session / Plugin / Evaluation

---

## 0. 文档边界

| 项 | 内容 |
|----|------|
| **本文档写什么** | 用户故事、功能规格、Agent 团队角色、数据源能力清单、验收标准、非功能需求 |
| **本文档不写** | 实现细节、代码片段、Proto 定义、数据库 DDL、调度算法（→ 设计文档） |
| **场景与平台关系** | 本场景是 Aranea-Agents 的**应用层产物**，不修改平台核心；所有能力通过平台已有的 Agent / Tool / Skill / Team / Graph 配置组合而成；缺失的能力（数据源接口、领域知识库）以**新增 Tool + Skill + Knowledge Source** 形式贡献到平台 |

---

## 1. 场景定位与价值

### 1.1 一句话定义

> **Daily Stock Analysis** 是一个开源的、可自托管的 AI 股票分析助手。用户通过自然语言提问、定时任务、或飞书/邮件渠道，触发一个由多个专业角色 Agent 组成的「分析师团队」完成数据采集、技术/基本面/资金面/消息面/情绪面分析、风险评估、综合评级与可视化报告输出。

### 1.2 目标用户

| 用户画像 | 核心诉求 | 典型动作 |
|----------|----------|----------|
| **个人投资者** | 每日盘前/盘后获取 watchlist 的快速分析与异动提醒 | 配置自选股；订阅每日 09:00 盘前简报；问答式深度分析 |
| **量化研究员** | 把多源数据汇总和初步分析自动化；将自然语言研究问题转为可复用工作流 | 自定义因子分析 Agent；接入自家数据；调用 Python 回测 Skill |
| **财经自媒体/分析师** | 快速生成结构化的分析报告草稿，节省案头时间 | 输入主题 → 生成 markdown 报告 → 飞书推送 → 人工二次编辑 |
| **金融教育用户** | 在演示环境中学习多智能体协作的实战范式 | 浏览运行轨迹（Team Run / Graph 节点）；观察分析师推理过程 |

### 1.3 业务价值与差异化

| 维度 | 价值点 |
|------|--------|
| **开源与自托管** | 用户数据、API 密钥、分析结果全部留在本地 **PostgreSQL**（场景业务表）+ 平台 SQLite（Agent/Session 等）+ 本地 Artifact，避免上传敏感持仓 |
| **多智能体协作** | 不同于单一 LLM 「全知全能」，本场景将分析任务拆分到 8~12 个专业角色 Agent，每个 Agent 拥有独立 system_prompt、工具集与知识库白名单，可独立评估和迭代 |
| **可观测可追溯** | 复用平台的 Team Run / Step / Tool Invocation / Memory L0~L3 / Telemetry，每一次分析都可回放、可审计、可评估 |
| **国内数据源原生支持** | 通过新增 Tool 接入 AKShare / Tushare / efinance / baostock 等开源数据源，无需付费 API |
| **可扩展工作流** | 借助 Graph 工作流模板，用户可自定义「研究流水线」（如：行业 → 龙头股 → 同业对比 → 估值 → 结论） |

### 1.4 非目标 / 明确排除

- ❌ 不提供**投资建议**或交易执行能力（仅生成分析与观点；交易决策由用户承担）
- ❌ 不内置**实时高频行情**推送（数据源以日线/分钟线为主，准实时 ≥ 1 min）
- ❌ 不做**美股期权 / 加密货币 / 商品期货**（首版聚焦 A 股 + 港股 + 美股股票现货）
- ❌ 不实现**自动调仓 / 券商接口**（开源版本不附带任何交易通道）
- ❌ 不替代专业 Bloomberg / Wind / Choice 终端

### 1.5 实时性分级（已确认）

本场景**不提供**交易所 Tick / WebSocket 级高频推送；「实时」指**准实时轮询**，由 Tool 在运行时拉取，再经 Agent 编排传递。

| 层级 | 能力 | 典型延迟 | 典型用途 |
|------|------|----------|----------|
| L0 Tick / 交易所推送 | **不在范围** | ms~s | 程序化交易、盘口 |
| L1 准实时报价 | `stock_quote_realtime` | **1–3 min**（数据源 + 限频） | 盘前简报、自选列表展示 |
| L2 分钟线 | `stock_quote_intraday` | 1–5 min 级 | 盘中形态、异动 |
| L3 日线 / 历史 | `stock_quote_history` | 可缓存（当日 TTL） | 深度分析、复盘 |
| L4 定时批处理 | Cron + Team Run | 单次全流程 **≤5 min**（盘前 20 只自选） | 盘前/盘后报告 |

**用户可见说明**：Web 自选页展示价格时，须标注「数据延迟约 1–3 分钟，仅供参考」；报告中须标注 `data_cutoff_at`（数据截止时间）。

**端到端延迟**：单次「个股深度分析」除行情 API 外，瓶颈通常在 **多 Agent LLM 推理 + 多轮 AgentTool**（常见 2–8 min），非行情接口本身。

### 1.6 多 Agent 市场数据协作（已确认）

用户期望：**Tools 获取市场信息 → 喂给当前 AI → 再传递给其他 AI**。平台支持该模式，但**不是** Agent 间直连行情广播，而是以下三层通路：

```
行情源 → stock_* Tool → JSON（tool_result）
         ↓
      当前 Agent 的 LLM 上下文
         ↓
      该 Agent 输出（文本/JSON，成员 Agent 默认 ≤800 token）
         ↓
   Coordinator（AgentTool 返回值）或 Graph state_schema
         ↓
      下一 Agent 的 prompt（任务描述 + 上游摘要/状态字段）
```

| 编排模式 | 数据如何跨 Agent 传递 | 适用场景 | 实时数据重复拉取风险 |
|----------|----------------------|----------|----------------------|
| **Coordinator + AgentTool** | 主控依次调用成员；成员自行调 Tool；结果以 AgentTool 返回值回传主控 | 盘前简报 `team_premarket_brief` | 较高（各成员可能重复调 `stock_quote_*`） |
| **Graph（推荐深度分析）** | Tool 节点写入 `state`；并行 Agent 经 `input_mapper` 共读；`report_writer` 读聚合 state | 个股深度 `team_stock_deep_dive` | 较低（`fetch_quote` 一次，fan_out 共享） |
| **Parallel + Synthesizer** | 各 Agent 并行、互不可见；仅 synthesizer 合并 | 持仓诊断 `team_portfolio_doctor` | 中等 |

**需求约束**：

- 成员 Agent 默认 `history_scope=isolated`，**不**共享彼此完整对话，由主控或 Graph 状态传递结构化结论。
- 每条分析结论须能追溯到 **Tool Invocation**（`tool_refs_json` / `data_ref`）。
- MVP 验收：`team_stock_deep_dive` 须覆盖技术、基本面、资金、消息、风险至少五维；数据经 Tool 或 Graph state 注入，禁止无工具结果的臆断。

---

## 2. 用户场景与故事

### 2.1 场景一：盘前晨会简报（Cron 触发）

**用户故事**：作为个人投资者，我希望每个交易日 08:30 自动收到一份覆盖我自选股的盘前简报，并通过飞书机器人推送到群里，以便我在开盘前 30 分钟做好决策。

**触发**：Cron 定时任务（`0 30 8 * * MON-FRI`，跳过节假日）
**输入**：自选股列表（来自 Memory L4 持久知识 或 Watchlist 表）
**Agent 团队**：`team_premarket_brief`（Coordinator 模式）
**输出**：
- 飞书富文本卡片（重点异动 + 一句话点评）
- Markdown 完整简报（落到 Artifact，可点击查看）
- Session 中保留对话上下文，用户可后续追问

**验收**：
- 简报必须覆盖每只自选股的「昨日收盘价、涨跌幅、关键消息、今日关注点」四要素
- 全流程 ≤ 5 分钟完成
- 模型 token 用量记录在 Token / Usage 模块
- 失败时通过 Cron 失败次数与监控告警可见

### 2.2 场景二：个股深度分析（Chat 交互）

**用户故事**：作为研究员，我希望在 Chat 页面输入「帮我分析 600519 贵州茅台 2026 Q1 财报后的走势机会」，团队能够并行调用技术/基本面/资金面/情绪面分析师，最终汇总为一份结构化研究报告。

**触发**：用户在 Chat 中选中 `team_stock_deep_dive` 发送消息
**输入**：自然语言查询（含股票代码或名称）
**编排模式**：Graph 工作流（默认）或 Team Coordinator（简化版）
**输出**：
- 流式展示每个分析师的中间结论（成员级 `member_message_*` 事件）
- 最终汇总报告（落地为 Markdown Artifact，含 K 线图 / 财务表 base64 嵌入）
- 评级标签：买入 / 增持 / 观望 / 减持 / 卖出（仅供参考，附风险提示）

**验收**：
- 必须包含至少 5 个维度的分析（技术、基本面、资金、消息、风险）
- 每条结论必须引用数据源（工具调用记录可追溯）
- 用户可在 Team Run 详情页查看完整执行轨迹
- 用户可二次提问，团队复用 Session Memory L0~L2 进行追问

### 2.3 场景三：行业/板块扫描（Cron + Channel）

**用户故事**：作为板块轮动策略研究者，我希望每周一 18:00 自动扫描全市场行业板块的资金流向、龙头股表现、估值变化，并把异动板块发送到飞书。

**触发**：Cron（`0 0 18 * * MON`）
**Agent 团队**：`team_sector_rotation`（Sequential 模式）
**输出**：
- 板块涨跌榜 + 资金净流入榜 + 估值百分位排名
- 异动板块（涨跌幅 > 阈值）详细解读
- 投资逻辑（产业链上下游、政策面、技术面共振）
- 飞书卡片摘要 + Markdown 完整报告

### 2.4 场景四：持仓组合诊断（按需触发）

**用户故事**：作为持有 10+ 只股票的投资者，我希望上传/录入持仓后，系统给出组合层面的风险评估（行业集中度、相关性、最大回撤模拟、Beta 暴露）和优化建议。

**触发**：Chat 中触发 / API 调用
**Agent 团队**：`team_portfolio_doctor`（Parallel + Synthesizer）
**输出**：
- 组合诊断报告（行业暴露热力图、相关性矩阵、波动率分解）
- 单股贡献度排名
- 风险敞口预警
- 优化建议（仅观点，不下单）

### 2.5 场景五：复盘报告（盘后 Cron）

**用户故事**：作为长期投资者，我希望每个交易日 17:00 自动生成市场复盘报告，覆盖大盘走势、热点板块、龙虎榜异动、北向资金流向、明日关注点。

**触发**：Cron（`0 0 17 * * MON-FRI`）
**Agent 团队**：`team_market_recap`
**输出**：盘后日报（Markdown + 飞书卡片）

### 2.6 场景六：自定义研究工作流（Graph）

**用户故事**：作为量化研究员，我希望通过 Graph 工作流画布自由编排「行业景气筛选 → 龙头股提取 → 财务质量筛选 → 估值排序 → 人工审批 → 输出 watchlist」，每步可中断和回放。

**触发**：手动 / 定时 / API
**编排**：Graph 工作流（HITL 节点 + 检查点）
**输出**：候选股池 + 每步检查点

---

## 3. Agent 团队角色清单

### 3.1 核心角色矩阵

| Agent Key | 中文名 | 角色定位 | 主要工具 | 必备 Skill | 默认模型偏好 |
|-----------|--------|----------|----------|------------|--------------|
| `agent_data_collector` | 数据采集员 | 统一拉取行情/财务/资金/新闻/公告数据，归一化输出 | `stock_quote_*`、`stock_fundamental_*`、`stock_news_*`、`stock_money_flow_*` | `skill.data_normalize` | 快速廉价模型（mini/flash/lite） |
| `agent_technical_analyst` | 技术分析师 | K 线形态、均线、MACD / KDJ / RSI / 布林带、量价关系、趋势/支撑/压力 | `stock_quote_history`、`indicator_compute` | `skill.technical_patterns` | 中等推理模型 |
| `agent_fundamental_analyst` | 基本面分析师 | 财报、ROE / PE / PB / PEG、盈利质量、行业地位 | `stock_fundamental_*`、`finance_statement_*` | `skill.financial_ratio` | 强推理模型 |
| `agent_money_flow_analyst` | 资金面分析师 | 北向资金、龙虎榜、主力净流入、大单成交、融资融券 | `stock_money_flow_*`、`hsgt_flow`、`dragon_tiger` | `skill.fund_signal` | 中等模型 |
| `agent_news_analyst` | 消息面分析师 | 公司公告、研报、政策、行业新闻、突发事件 | `stock_news_*`、`stock_announcement`、`web_search`、`research_report_search` | `skill.news_classify` | 中等模型 |
| `agent_sentiment_analyst` | 情绪面分析师 | 雪球 / 股吧 / 财经媒体舆情、热度趋势、关键词共现 | `stock_sentiment_*`、`web_search` | `skill.sentiment_score` | 快速模型 |
| `agent_industry_analyst` | 行业分析师 | 行业景气、产业链上下游、板块轮动、政策驱动 | `industry_*`、`stock_concept_*` | `skill.industry_chain` | 中等模型 |
| `agent_risk_assessor` | 风险评估师 | 波动率、最大回撤、Beta、VaR、行业/集中度风险、ST / 退市预警 | `risk_metric_*`、`backtest_simple` | `skill.risk_framework` | 中等模型 |
| `agent_quant_factor` | 因子计算员（可选） | 多因子计算（动量/反转/质量/价值/盈利预期） | `indicator_compute`、`factor_compute`、`codeexecutor`（Python） | `skill.factor_lib` | 强推理模型 |
| `agent_chart_builder` | 图表构建员 | 生成 K 线图、财务图表、组合热力图（输出 SVG / PNG / Artifact） | `chart_render_*`、`codeexecutor`（Python matplotlib） | `skill.chart_style` | 快速模型 |
| `agent_report_writer` | 报告撰写员 | 把多 Agent 输出汇总为结构化 Markdown / 飞书卡片 | `artifact_save`、`channel_push` | `skill.report_template` | 强推理模型 |
| `agent_coordinator` | 主控调度员 | 任务拆分、调度成员 Agent、决定是否需要追问 | （以成员 Agent 作为 AgentTool 调用） | `skill.task_planning` | 强推理模型 |
| `agent_critic` | 评审员（可选，用于 critic_loop） | 对报告草稿打分、提出修改意见 | — | `skill.report_critic` | 中等模型 |

### 3.2 Team 编排模式映射

| Team Key | 模式（mode） | 成员组合 | 适用场景 |
|----------|--------------|----------|----------|
| `team_premarket_brief` | `coordinator` | `agent_coordinator` 调度 `agent_data_collector` + 5 个分析师 + `agent_report_writer` | 场景 2.1 盘前简报 |
| `team_stock_deep_dive` | `graph`（首选）或 `coordinator` | 含全部 8 个分析师 + critic_loop 二次精修 | 场景 2.2 深度分析 |
| `team_sector_rotation` | `sequential` | `agent_industry_analyst` → `agent_money_flow_analyst` → `agent_news_analyst` → `agent_report_writer` | 场景 2.3 板块扫描 |
| `team_portfolio_doctor` | `parallel` + `synthesizer` | `agent_risk_assessor` + `agent_fundamental_analyst` + `agent_quant_factor` → `agent_report_writer` | 场景 2.4 组合诊断 |
| `team_market_recap` | `sequential` | 4 个分析师 → `agent_chart_builder` → `agent_report_writer` | 场景 2.5 盘后复盘 |
| `team_research_pipeline` | `graph`（自定义） | 用户拖拽编辑 | 场景 2.6 自定义工作流 |

> **底线**：每个 Team 必须有一个 `synthesizer_agent_id`（默认 = `agent_report_writer`）确保有最终统一输出。

---

## 4. 功能需求

### 4.1 数据源接入（Tool 层）

| 工具组 | 必备工具键 | 数据范围 | 默认数据源建议 |
|--------|------------|----------|----------------|
| **行情** | `stock_quote_realtime`、`stock_quote_history`、`stock_quote_intraday` | 实时报价（≥1min）、日线、分钟线 | AKShare（A 股）、yfinance（美股/港股） |
| **基本面** | `stock_fundamental_overview`、`finance_statement_income`、`finance_statement_balance`、`finance_statement_cashflow`、`finance_indicator` | 公司概览、三大报表、关键指标 | AKShare、Tushare（需用户配 token） |
| **资金面** | `hsgt_flow`、`dragon_tiger`、`stock_money_flow_individual`、`margin_balance` | 北向、龙虎榜、个股资金流、两融 | AKShare、东方财富 |
| **消息面** | `stock_announcement`、`stock_news_individual`、`research_report_search`、`policy_search` | 公告、新闻、研报、政策 | AKShare、巨潮资讯 |
| **情绪面** | `stock_sentiment_xueqiu`、`stock_sentiment_guba` | 雪球热度、股吧舆情 | 雪球 H5、东方财富股吧 |
| **行业** | `industry_classification`、`industry_money_flow`、`stock_concept_list` | 行业分类、板块资金、概念股 | AKShare |
| **指标计算** | `indicator_compute`（MA / MACD / KDJ / RSI / BOLL / ATR / OBV …） | 技术指标 | TA-Lib（可选）/ 纯 Python |
| **可视化** | `chart_render_kline`、`chart_render_pie`、`chart_render_heatmap` | K 线、饼图、热力图 | matplotlib + mplfinance（Skill 调 CodeExecutor 实现） |
| **回测/因子（可选）** | `backtest_simple`、`factor_compute` | 简单回测、因子计算 | Backtrader / 自研 |
| **基础设施** | `web_search`、`httpfetch`、`workspace_exec` | 已存在于平台 | — |

#### 4.1.1 数据源 Provider 模式

- 每个工具组定义**抽象接口** + **多 Provider 实现**（AKShare、Tushare、yfinance、自建爬虫）
- 用户在 **Tools 管理页**为每个工具组选择 provider 和配置 API key
- Provider 缺失（如未配 Tushare token）时，工具应返回明确错误而非崩溃
- 提供 **默认免费 Provider 自动 fallback** 链路（首选 AKShare → 退化到爬虫）

#### 4.1.2 缓存策略

- 行情历史数据：默认缓存到 **PostgreSQL `stockx.quote_cache`**（key=`{symbol}|{market}|{period}|{adjust}|{date_range}`，TTL 当日，可配置 `STOCK_SCENARIO_CACHE_TTL_QUOTE`）
- 财报数据：缓存 24 小时（`stockx` 侧或 Tool 内存，除非新公告触发）
- **准实时报价**（`stock_quote_realtime`）：**不缓存**；Web 自选页可由 StockxService 批量拉取后短 TTL（≤60s）写入 `quote_cache` 仅供展示，须标注延迟
- 缓存命中应通过 Tool Invocation 的 `cache_hit` 字段可观测

### 4.2 Skill 包

| Skill Key | 用途 | 类型 |
|-----------|------|------|
| `skill.data_normalize` | 数据结构归一化规范（字段命名、单位、时区） | Knowledge / Markdown 规范 |
| `skill.technical_patterns` | 经典技术形态库（头肩顶、双底、突破、缺口…） | Markdown 规范 + 示例 |
| `skill.financial_ratio` | 财务指标解读手册 | Markdown |
| `skill.fund_signal` | 资金面信号解读 | Markdown |
| `skill.news_classify` | 新闻分类与重要性评分规则 | Markdown |
| `skill.sentiment_score` | 舆情打分规则 | Markdown |
| `skill.industry_chain` | A 股主要行业产业链图谱 | Markdown + JSON |
| `skill.risk_framework` | 风险评估框架（行业集中度、Beta、回撤计算公式） | Markdown |
| `skill.factor_lib` | 多因子库（动量/价值/质量等）+ Python 实现示例 | Markdown + Python |
| `skill.chart_style` | 统一图表风格（配色、字体、水印、暗色模式） | Markdown |
| `skill.report_template` | 报告模板（盘前简报 / 深度分析 / 板块扫描 / 复盘 / 组合诊断） | Markdown |
| `skill.report_critic` | 报告评审规则（结构完整性、数据引用、风险提示） | Markdown |
| `skill.task_planning` | 任务拆分与调度心法 | Markdown |

> **加载策略**：每个 Skill 在 Agent 配置中按需启用；默认仅注入与该 Agent 角色匹配的 Skill，避免上下文膨胀。

### 4.3 Knowledge 知识库

| 知识库 | 内容 | 用途 |
|--------|------|------|
| `kb_listed_companies` | A 股 / 港股 / 美股上市公司基础信息（行业、概念、主营业务） | 模糊匹配股票名称、概念→个股映射 |
| `kb_index_constituents` | 主要指数成分股（沪深 300、中证 500、上证 50、纳指 100…） | 指数级分析 |
| `kb_industry_chain` | 行业产业链 + 龙头股映射 | 行业分析时检索 |
| `kb_research_glossary` | 研究术语词典（PEG、ROIC、PB-ROE 模型…） | 报告写作时统一表达 |
| `kb_user_watchlist`（用户级） | 用户自选股 + 备注 | Cron 触发时获取目标 |
| `kb_user_holdings`（用户级，可选） | 用户持仓（脱敏） | 组合诊断 |

### 4.4 报告输出

#### 4.4.1 报告类型

| 报告类型 | 默认格式 | 落地方式 |
|----------|----------|----------|
| 盘前简报 | 飞书卡片 + Markdown | Channel 推送 + Artifact |
| 个股深度报告 | Markdown（含嵌入图表） | Artifact + 可选 PDF 导出 |
| 板块扫描 | Markdown + 排行榜表格 | Artifact + Channel |
| 盘后复盘 | Markdown 长报 | Artifact + Channel |
| 组合诊断 | Markdown + 多张图表 | Artifact |

#### 4.4.2 报告通用结构

每份报告至少包含：
1. **元信息**：生成时间、数据截止时间、分析师（Team Key）、用户问题
2. **核心结论**（TL;DR）：3~5 行
3. **分维度分析**：每个分析师的子模块
4. **数据引用**：列出本次调用的工具与数据源（含 `tool_call.id`）
5. **风险提示**：固定文案 + 个性化补充
6. **附录**：图表、原始数据表（可折叠）

#### 4.4.3 图表要求

- 主图：K 线 + 均线 + 成交量（默认 90 日）
- 次图：MACD、KDJ、RSI（可选）
- 资金图：北向资金 / 龙虎榜买卖前五
- 财务图：营收/净利润趋势、ROE / 毛利率趋势
- 行业图：行业涨跌幅 TOP10、资金流入热力图
- 组合图：行业暴露饼图、相关性矩阵、持仓贡献条形

### 4.5 调度与触发

#### 4.5.1 Cron 内置任务

| Cron Key | 表达式 | 触发场景 |
|----------|--------|----------|
| `cron_premarket_brief` | `0 30 8 * * MON-FRI` | 盘前简报 |
| `cron_postmarket_recap` | `0 0 17 * * MON-FRI` | 盘后复盘 |
| `cron_weekly_sector_scan` | `0 0 18 * * MON` | 周一板块扫描 |
| `cron_monthly_portfolio_review` | `0 0 9 1 * *` | 月度组合诊断 |

> **节假日跳过**：需新增 Skill `skill.cn_trading_calendar`（或工具 `is_trading_day`）确保 A 股节假日不触发。

#### 4.5.2 渠道推送

| 渠道 | 用途 |
|------|------|
| 飞书机器人（Lark Webhook） | 主要推送渠道（卡片 + 文本） |
| 邮件 SMTP | 长报告附件 |
| Web UI Chat 历史 | 用户主动查看 |

### 4.6 Web UI 功能要求

| 页面 | 功能 |
|------|------|
| **Chat 页（沿用平台）** | 在「Team 列表」中可看到本场景内置 6 个 Team；选中后即可对话 |
| **Stock Watchlist 页**（新增） | 维护自选股；标签分组；批量导入（粘贴代码、CSV） |
| **Stock Detail 页**（新增） | 单股看板：基础信息 + K 线 + 最近一次分析报告链接 |
| **Reports 页**（复用 Artifact） | 历史报告列表（按 Team / 日期 / 股票筛选） |
| **Cron 管理页（沿用）** | 启用/禁用 4 个内置 Cron；查看运行历史 |
| **Tools 管理页（沿用）** | 配置数据源 Provider 和 API Key（Tushare、雪球 cookie 等） |

### 4.7 用户配置项

| 配置 | 描述 |
|------|------|
| 默认市场 | A 股 / 港股 / 美股 |
| 默认时区 | Asia/Shanghai |
| 默认推送渠道 | 飞书机器人 webhook URL / 邮件地址 |
| 自选股 | 在 Watchlist 页维护 |
| 数据源 API Key | Tushare token、雪球 cookie 等 |
| 报告模板偏好 | 简版 / 标准 / 详细 |
| 风险偏好提示 | 显示在每份报告底部的固定文案 |

---

## 5. 数据需求

### 5.0 存储架构（已确认：PostgreSQL）

| 数据类别 | 存储 | 说明 |
|----------|------|------|
| 平台 CRUD（Agent / Team / Session / Cron 等） | **SQLite**（Ent，沿用平台） | 不修改平台 `data.go` 主路径；`seed-stockx-org` 仍写 SQLite |
| 场景业务表（watchlist / report / cache / meta） | **PostgreSQL `stockx` schema** | 复用 `configs/config.yaml` 中 `data.postgres` 连接；raw SQL Repo（对齐 Knowledge 模块） |
| 向量知识库（公司库 / 行业链） | **PostgreSQL + pgvector** | 与平台 Knowledge 共用实例 |
| 报告正文 | **Artifact**（平台） | `stockx.report` 仅存索引与 `artifact_id` |
| 全平台 Ent 迁 PG | **非本场景范围** | 若需 Session/Agent 也迁 PG，单独立项 |

**配置要求**：部署须配置 `data.postgres.source`；场景安装前执行 `migrations/stockx/*.sql`（见设计文档 §3）。

### 5.1 新增数据表（PostgreSQL `stockx` schema）

| 表 | 字段（核心） | 说明 |
|----|--------------|------|
| `stockx.watchlist` | `id, user_id, symbol, market, group_name, note, sort_order, created_at` | 用户自选股 |
| `stockx.holdings` | `id, user_id, symbol, market, shares, cost_price, note, created_at`（可选） | 持仓 |
| `stockx.stock_meta` | `symbol, market, name, industry, concepts_json, list_date, updated_at` | 上市公司元数据（缓存） |
| `stockx.quote_cache` | `cache_key, symbol, market, period, data_json, provider, expires_at` | 行情历史缓存；展示层短 TTL 可选 |
| `stockx.news_cache` | `id, symbol, title, source, url, published_at, content_summary, expires_at` | 新闻缓存 |
| `stockx.report` | `id, user_id, team_id, session_id, symbol, report_type, title, summary_md, artifact_id, data_cutoff_at, tool_refs_json, created_at` | 报告索引 |

> **沿用平台已有表**（SQLite）：`sessions` / `messages` / `team_runs` / `team_run_steps` / `tool_invocations` / `cron_tasks` / `cron_task_runs` / `artifacts` / `usages` / `platform_channels`。

### 5.2 数据源接入清单（前置依赖）

| 数据源 | 类型 | 是否需要 API Key | 优先级 | 用途 |
|--------|------|-------------------|--------|------|
| AKShare | Python 库（HTTP wrapper） | 否（部分功能可选 token） | P0 | A 股全量数据 |
| Tushare | HTTP / Python | 是（用户注册免费） | P1 | 财务数据更细粒度 |
| yfinance | Python | 否 | P1 | 港股 / 美股 |
| efinance | Python | 否 | P2 | 东方财富数据补充 |
| baostock | Python | 否 | P2 | 历史行情备份 |
| 雪球 / 股吧 | H5 爬虫 | 是（用户 cookie） | P2 | 情绪数据 |
| 巨潮资讯 | HTTP | 否 | P2 | 公告原文 |
| 用户自定义 HTTP API | — | 看自定义 | P3 | 接入企业内部数据 |

---

## 6. 非功能需求

### 6.1 性能

| 场景 | 指标 | 备注 |
|------|------|------|
| 盘前简报（20 只自选股） | 端到端 ≤ 5 min | Coordinator 串行 AgentTool |
| 单股深度分析 | 端到端 ≤ 90 s（目标）；**可接受 ≤ 8 min**（多 Agent + 五维并行） | Graph 模式优先；超 90s 须在 UI 标注「分析中」 |
| 行业扫描 | 端到端 ≤ 3 min | |
| 单次 Tool 调用 | P95 ≤ 5 s（含缓存命中） | `stock_quote_realtime` 除外（受三方限频） |
| 自选页报价刷新 | 前端轮询 **30–60 s** | 非 SSE 交易所推送 |
| Web 加载 | 首屏 ≤ 2 s | |

### 6.2 可靠性

- 数据源故障必须有 fallback，绝不阻塞整个分析
- 单个 Agent 失败时，团队整体应继续（parallel 模式支持部分失败汇总；coordinator 模式由 critic_loop 兜底）
- Cron 失败应通过 Monitor 模块告警
- 报告生成失败应保留中间 Artifact + 失败原因

### 6.3 安全与合规

| 项 | 要求 |
|----|------|
| API 密钥 | 落库前必须加密；前端不回传明文 |
| 用户持仓 | 默认不上传任何外部服务；外发提示词中自动脱敏 |
| 报告免责声明 | 每份报告必须包含「以上内容仅供学习研究，不构成任何投资建议」 |
| 数据合规 | 接入第三方数据源必须遵守其 Robots / TOS；商业部署需用户自行获得授权 |
| 推送内容 | 飞书 / 邮件推送必须包含来源声明 |

### 6.4 可观测性

- 复用平台 Telemetry / Monitor：每次 Team Run、Tool Invocation、模型调用都有 trace
- 提供专用 Grafana 仪表盘：每日报告生成数、平均延迟、Token 用量、Tool 失败率、Cron 成功率
- 报告应记录数据时效性（数据截止时间 vs 报告生成时间）

### 6.5 国际化

- 首版默认中文（zh-CN）
- 报告模板预留英文（en-US）扩展位
- 美股报告默认输出英文（用户可切换）

### 6.6 兼容性

- 不破坏平台已有功能
- 所有新增 Tool / Skill / Knowledge 通过平台标准接口注册
- 场景禁用时不影响平台其它模块运行

---

## 7. 验收标准（MVP 完工定义）

### 7.1 必须达成（P0）

- [ ] 用户可在 Web UI 中看到内置的 6 个 Team
- [ ] 至少 3 个数据源工具（AKShare 行情 / 财务 / 新闻）真实可用
- [ ] 场景 2.2「个股深度分析」端到端走通：输入股票 → 团队执行 → 输出 Markdown 报告
- [ ] 场景 2.1「盘前简报」可手动触发（Cron 模拟），并通过飞书 webhook 推送
- [ ] 报告必须包含数据引用、风险提示、TL;DR 摘要
- [ ] Watchlist 页可增删自选股
- [ ] 报告页可按股票/日期检索历史报告
- [ ] 节假日跳过逻辑可用
- [ ] README + 安装指南覆盖：环境依赖、API key 配置、首启动指南

### 7.2 应当达成（P1）

- [ ] 场景 2.3「板块扫描」可用
- [ ] 场景 2.5「盘后复盘」可用
- [ ] 支持港股或美股至少一个市场
- [ ] K 线图自动嵌入报告
- [ ] Cron 定时任务可在 Web UI 启用/禁用并查看运行历史
- [ ] Tool Invocation 在 Monitor 页可观测
- [ ] Grafana 仪表盘 JSON 提供

### 7.3 可以达成（P2）

- [ ] 场景 2.4「组合诊断」端到端走通
- [ ] 场景 2.6「自定义研究工作流」通过 Graph 可拖拽
- [ ] 多因子模块（`agent_quant_factor` + `factor_compute`）
- [ ] 邮件渠道推送
- [ ] PDF 报告导出
- [ ] 国际化（en-US）
- [ ] Evaluation 模块支持「报告质量评估」（LLM-as-Judge 评估报告完整性、数据引用率、结论一致性）

### 7.4 未来扩展（P3）

- [ ] 接入 Wind / Choice / Bloomberg 数据
- [ ] 回测引擎（Backtrader 集成）
- [ ] 期权 / 商品 / 加密扩展
- [ ] 多用户协作 watchlist
- [ ] 移动端 App / 小程序

---

## 8. 风险与约束

| 风险 | 影响 | 缓解 |
|------|------|------|
| 数据源接口变更（爬虫失效） | 工具不可用 | 多 Provider fallback + 单元/契约测试 + 监控 |
| 模型 token 成本失控 | 用户费用激增 | 默认用 mini/flash 模型；token 用量限额（沿用 Token 模块）；每个 Agent 都可配 model_selector |
| 报告时效性 | 用户依赖旧数据决策 | 强制在报告中标注数据截止时间；超过阈值时大字号警告 |
| 法律合规 | 投资建议风险 | 明确「非投资建议」声明；不提供下单接口 |
| 第三方数据 TOS | 商业部署违规 | 文档中明示用户需自行确认；默认仅免费数据源 |
| Cron 节假日 | 盘前简报在节假日发出 | 节假日跳过 Skill；通过 Telemetry 监控未触发是否符合预期 |
| 中文金融术语翻译 | 模型理解偏差 | 通过 Knowledge 注入术语表；评估集覆盖术语翻译 |
| 准实时误解为 Tick | 用户按秒级行情决策 | UI/报告强制标注延迟与 `data_cutoff_at`；文档明确 L0 不在范围 |
| Coordinator 重复拉行情 | 限频 / 超时 | 深度分析优先 Graph；盘前简报合并 data_collector 一次拉取 |

---

## 9. 与平台模块的依赖矩阵

| 平台模块 | 本场景依赖度 | 用法 | 备注 |
|----------|--------------|------|------|
| Agent (M2) | ★★★★★ | 注册 10+ 角色 Agent | 复用 LLMAgent.New 装配 |
| Team (M3) | ★★★★★ | 5+ Team（coordinator / sequential / parallel / critic_loop / graph） | 必须 |
| Graph (M4) | ★★★★★ | 场景 2.2 个股深度（**Graph 优先**）；2.6 自定义流水线 | Graph 执行引擎 ✅；MVP 可先用 coordinator 降级 |
| Session (M5) | ★★★★★ | 每次分析一个 session | 复用 |
| Memory (M6) | ★★★☆☆ | Watchlist / 用户偏好（L4） + 追问上下文（L0~L2） | L4 进化记忆有依赖 |
| Tool (M7) | ★★★★★ | 新增 30+ 数据源工具 | 主要贡献 |
| MCP (M8) | ★★☆☆☆ | 可选接入外部 MCP server（如 Wind MCP） | 可选 |
| Model (M9) | ★★★★☆ | 推荐 OpenAI / Anthropic / DeepSeek / Hunyuan | 用户自配 |
| Plugin (M10) | ★★★☆☆ | 高风险数据写入拦截、敏感字段脱敏 | 可选 |
| Planner (M11) | ★★★☆☆ | 复杂任务使用 ReAct Planner | 可选 |
| Artifact (M12) | ★★★★★ | 所有报告输出 | 必须 |
| Knowledge (M13) | ★★★★☆ | 行业链 / 术语 / 公司库 | 强依赖 |
| CodeExecutor (M14) | ★★★★☆ | 图表生成、因子计算（Python） | 强依赖 |
| A2A (M15) | ★★☆☆☆ | 可选跨实例协作 | 可选 |
| Cron | ★★★★★ | 4 个内置定时任务 | 必须 |
| Channel | ★★★★☆ | 飞书 / 邮件推送 | 强依赖 |
| Evaluation (M17) | ★★★☆☆ | 报告质量 LLM-as-Judge | P2 |
| Event (M18) | ★★★★★ | WS 流式 + member 事件 | 复用 |
| Runner (M20) | ★★★★★ | 主链路 | 复用 |

---

## 10. 术语表

| 术语 | 解释 |
|------|------|
| Aranea-Agents | 本场景所基于的多智能体编排平台（即本仓库） |
| Agent | 单一角色 LLM 智能体（System Prompt + 工具集 + 技能集） |
| Team | 多 Agent 编排单元（coordinator / sequential / parallel / critic_loop / swarm / graph） |
| Graph | 节点 + 边 + 状态的工作流图（确定性流程） |
| Skill | Agent 使用某类能力的说明、知识和操作规范（Markdown / JSON / 脚本） |
| Tool | Agent 可调用的具体能力（HTTP API / 本地函数 / MCP） |
| MCP | Model Context Protocol，外部工具协议 |
| Artifact | 持久化制品（报告 / 图表 / 临时文件） |
| Watchlist | 用户自选股列表 |
| TL;DR | Too Long; Didn't Read，报告摘要 |
| HITL | Human-in-the-Loop，人工介入节点 |
| Provider | 数据源具体实现（AKShare / Tushare / yfinance …） |
| Backtrader | Python 开源回测框架（可选集成） |

---

## 11. 文档治理

- **v1.1（2026-05-25）**：确认 Postgres `stockx` 存储、实时性分级（§1.5）、多 Agent 数据协作（§1.6）；与设计和开发计划对齐
- 本文档为「需求规格」，被代码反超时**只追加现状对齐注解**，不重写历史结论
- 实现细节、表 DDL、Proto 定义、Wire 连线 → 见 [设计文档](./daily-stock-analysis.design.md)
- 任务进度、里程碑、阶段目标 → 见 [开发计划](./daily-stock-analysis-development.md)
- 与平台核心模块的对齐冲突优先级：`AI-DEVELOPMENT-SPECIFICATION` > 核心需求 > 本场景需求

