# Daily Stock Analysis — 场景化设计文档

> **对应需求**：[daily-stock-analysis.md](./daily-stock-analysis.md)
> **遵循规范**：[`AI-DEVELOPMENT-SPECIFICATION.md`](../../guides/AI-DEVELOPMENT-SPECIFICATION.md)
> **平台架构**：[`0 系统框图.md`](../../需求/0%20系统框图.md)
> **文档版本**：v1.1（2026-05-25；PostgreSQL `stockx` schema、实时性、多 Agent 数据通路）

---

## 0. 设计原则

| # | 原则 | 体现 |
|---|------|------|
| 1 | **不修改平台核心** | 场景代码尽量通过「Agent 配置 + Skill 文件 + Tool 注册 + Cron 配置 + Knowledge 数据」组合而成；必须新增的能力以「内置工具贡献到 `internal/tools/...`」「Skill 包贡献到 `docs/skills/...` 或运行时 skill 目录」的方式入仓 |
| 2 | **分层不越界**（R1） | 数据源工具实现位于 `internal/tools/stockdata/`，仅暴露 `tool.Tool` 接口；biz 层不直接 import 第三方数据 SDK |
| 3 | **Postgres 场景库**（R2 变体） | 场景业务表走 `data.Postgres()` + `stockx` schema SQL 迁移；**不**为 stockx 新增 Ent Schema，避免牵动平台 SQLite Ent 主路径 |
| 4 | **不手写 HTTP 路由**（R3/R4） | 新页面通过 `api/kratos/stockx/v1/*.proto` 定义；server 注册函数复用 `api/**` 生成代码 |
| 5 | **goroutine 安全**（R10） | 任何后台采集/缓存刷新使用 `pkg/safego.Go` |
| 6 | **配置即代码** | Team / Agent / Cron / Skill 通过 YAML / JSON 「场景安装包」一键导入，避免手工配置 |
| 7 | **可关闭** | 整个场景通过 `STOCK_SCENARIO_ENABLED=0` 环境变量关闭，关闭后不注册任何相关 Tool / Cron / Channel |

---

## 1. 总体架构

### 1.1 场景在平台中的定位

```
┌─────────────────────────────────────────────────────────────────────────┐
│  接入层                                                                  │
│  ┌──────────┐  ┌──────────┐  ┌──────────────┐  ┌──────────────┐       │
│  │ Web UI   │  │ Cron     │  │ Channel 飞书 │  │ Channel 邮件 │       │
│  │ Chat /   │  │ 4 个内置 │  │ Webhook 推送  │  │ SMTP         │       │
│  │ Watchlist│  │ 任务     │  │              │  │              │       │
│  └────┬─────┘  └────┬─────┘  └──────┬───────┘  └──────┬───────┘       │
│       │             │               │                  │                │
│       ▼             ▼               ▼                  ▼                │
├─────────────────────────────────────────────────────────────────────────┤
│  传输层 — Kratos v2（已有，无需新增协议）                                  │
├─────────────────────────────────────────────────────────────────────────┤
│  Service 层 — 新增 StockxService（场景专用）                              │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ ChatService（沿用）  AgentService（沿用）  TeamService（沿用）   │   │
│  │ CronService（沿用）  ArtifactService（沿用）                     │   │
│  │ StockxService（新增）— Watchlist / Holdings / Report 索引       │   │
│  └─────────────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────────────┤
│  Biz 层                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ WatchlistUsecase / HoldingsUsecase / StockReportUsecase（新增）  │   │
│  │ TradingCalendarUsecase（新增；A 股节假日）                       │   │
│  │ 其他 Usecase 沿用                                                │   │
│  └─────────────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────────────┤
│  Data 层                                                                │
│  ┌────────────────────────────┐  ┌────────────────────────────────┐    │
│  │ SQLite（平台沿用）          │  │ PostgreSQL（场景 + 向量）       │    │
│  │ Agent/Team/Session/Cron…   │  │ schema stockx:                  │    │
│  │ seed-stockx-org 写此库      │  │ ├ watchlist / holdings          │    │
│  └────────────────────────────┘  │ ├ stock_meta / quote_cache      │    │
│                                   │ ├ news_cache / report           │    │
│                                   │ └ kb_*（pgvector，平台 Knowledge）│    │
│                                   └────────────────────────────────┘    │
├─────────────────────────────────────────────────────────────────────────┤
│  Agent 运行时（沿用 trpc-agent-go）                                       │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ LLMAgent / Team / Graph / Runner / Session / Memory / Tool      │   │
│  └─────────────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────────────┤
│  Tool 层 — 新增 internal/tools/stockdata/                                │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ 行情 Provider（akshare/yfinance）  财务 Provider                 │   │
│  │ 资金 Provider                       消息/情绪 Provider           │   │
│  │ 指标计算（纯 Go 或调 CodeExecutor） 图表（调 CodeExecutor）       │   │
│  │ 节假日 / 概念 / 行业链              交易日历 Tool                │   │
│  └─────────────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────────────┤
│  Provider 模型层（沿用）                                                  │
│  OpenAI / Anthropic / Gemini / DeepSeek / Hunyuan / Ollama / ...      │
└─────────────────────────────────────────────────────────────────────────┘
```

### 1.2 关键流程总览

```
[盘前简报场景]
  Cron(08:30) ──► CronRunner ──► TradingCalendarUsecase.IsTradingDay?
                                     │ yes
                                     ▼
                          ChatService.RunCronTurn(team_premarket_brief, watchlist)
                                     │
                                     ▼
                          BuildTRPCTeam(coordinator)
                          ├─► agent_coordinator 拆分任务
                          │     └─► AgentTool(agent_data_collector) ──► stock_quote_realtime / stock_news_individual
                          │     └─► AgentTool(agent_technical_analyst) ──► indicator_compute
                          │     └─► AgentTool(agent_news_analyst)
                          │     └─► ...
                          └─► agent_report_writer 汇总
                                     │
                                     ▼
                          Artifact.Save(markdown) ──► stock_report 表
                                     │
                                     ▼
                          ChannelService.Push(feishu, summary_card)
```

```
[个股深度分析场景 — Graph 编排]
  ChatService.SendChatMessage(team_id=team_stock_deep_dive, "分析 600519")
                                     │
                                     ▼
                          GraphRuntime.Execute(graph_stock_deep_dive)
                          ┌───────────────────────────────────────┐
                          │ Node 1: Function ─ 解析股票代码        │
                          │ Node 2: Tool    ─ stock_quote_history │
                          │ Node 3: Parallel{                     │
                          │   Agent: agent_technical_analyst,     │
                          │   Agent: agent_fundamental_analyst,   │
                          │   Agent: agent_money_flow_analyst,    │
                          │   Agent: agent_news_analyst,          │
                          │   Agent: agent_sentiment_analyst      │
                          │ }                                     │
                          │ Node 4: Agent ─ agent_risk_assessor   │
                          │ Node 5: Tool  ─ chart_render_kline    │
                          │ Node 6: Agent ─ agent_report_writer   │
                          │ Node 7: Function ─ persist report     │
                          └───────────────────────────────────────┘
                                     │
                                     ▼
                          流式投影 ──► WS Envelope ──► 前端 Chat 面板
```

### 1.3 实时性设计

| 组件 | 行为 |
|------|------|
| `stock_quote_realtime` | 运行时 HTTP 调 Sidecar/Provider；**不写入长期缓存**；返回带 `ts` 字段 |
| `stock_quote_history` / `intraday` | 读 `stockx.quote_cache`；miss 时拉源并 upsert |
| Web Watchlist | StockxService 批量报价；服务端可选 60s TTL 写入 `quote_cache`；前端 30–60s 轮询 |
| Team Run | 不维持行情推送订阅；每次 Run 按需 Tool 拉取 |

延迟预算见需求文档 §1.5。v1.x 可选：Redis pub/sub 或 Sidecar 订阅写 `quote_tick`（**非 v1.0**）。

### 1.4 多 Agent 市场数据通路

**Coordinator 路径**（`team_premarket_brief`）：

```
agent_coordinator
  └─ AgentTool(agent_data_collector)
        └─ stock_quote_realtime / stock_news_individual  → JSON
        └─ 返回摘要文本 → coordinator 上下文
  └─ AgentTool(agent_technical_analyst)
        └─ 可再次 stock_quote_history / indicator_compute
  └─ AgentTool(agent_report_writer)  ← 汇总多成员输出
```

**Graph 路径**（`team_stock_deep_dive`，**推荐**）：

```
resolve (function) → fetch_quote (tool) → fan_out (parallel agents)
  state.quote_history 经 input_mapper 注入各分析师
  output_mapper 写回 state.fundamentals / state.news / …
  → risk (agent) → chart (tool) → report (agent) → persist (function → stockx.report)
```

成员 `history_scope=isolated`（`internal/team/trpc_build.go`）；跨成员数据**仅**经 AgentTool 返回值或 Graph `state_schema`，禁止假设成员互读 Session 历史。

---

## 2. 目录结构与代码归属

### 2.1 后端新增/修改

```
internal/
├── tools/
│   └── stockdata/                       ★ 新增
│       ├── registry.go                   ─ ToolKey 注册到 tools.Registry()
│       ├── quote/                        ─ 行情
│       │   ├── interface.go              ─ QuoteProvider 抽象
│       │   ├── akshare_provider.go       ─ AKShare 实现（HTTP 调本地 Python or 内置 Go 客户端）
│       │   ├── yfinance_provider.go      ─ yfinance 实现
│       │   ├── cache.go                  ─ stock_quote_cache 读写
│       │   └── tool.go                   ─ tool.Tool 适配（NewQuoteTool）
│       ├── fundamental/                  ─ 基本面
│       ├── moneyflow/                    ─ 资金面
│       ├── news/                         ─ 消息面
│       ├── sentiment/                    ─ 情绪面
│       ├── industry/                     ─ 行业
│       ├── indicator/                    ─ 指标计算（纯 Go）
│       ├── chart/                        ─ 图表（拼装 Python，调 CodeExecutor）
│       ├── calendar/                     ─ 交易日历 / 节假日
│       └── README.md                     ─ Provider 接入说明
│
├── biz/
│   ├── stock_watchlist_usecase.go        ★ 新增
│   ├── stock_holdings_usecase.go         ★ 新增（可选）
│   ├── stock_report_usecase.go           ★ 新增
│   └── trading_calendar_usecase.go       ★ 新增
│
├── data/
│   ├── stockx/                           ★ 新增
│   │   ├── migrations/                   ─ 001_init.sql（stockx schema + 表）
│   │   ├── watchlist_repo.go             ─ 复用 data.Postgres()
│   │   ├── report_repo.go
│   │   ├── meta_repo.go
│   │   └── cache_repo.go                 ─ quote + news
│   └── stockx_install.go                 ★ 新增 ─ 启动时 EnsureSchema（STOCK_SCENARIO_ENABLED）
│
├── service/
│   └── stockx.go                         ★ 新增 ─ StockxService(Watchlist/Holdings/Report)
│
├── scenario/
│   └── stockanalysis/                    ★ 新增 ─ 场景安装包入口
│       ├── install.go                    ─ 启动时检测 enable=true 后注册全部资源
│       ├── agents.yaml                   ─ 10+ Agent 定义（system_prompt / tools / skills）
│       ├── teams.yaml                    ─ 6 个 Team 定义
│       ├── graphs/                       ─ Graph 工作流 JSON
│       │   ├── stock_deep_dive.graph.json
│       │   └── research_pipeline.graph.json
│       ├── crons.yaml                    ─ 4 个 Cron 任务
│       └── README.md
│
└── server/
    └── register_stockx.go                ★ 新增 ─ 注册 stockx 路由（如不放在 register_chat.go）

api/
└── kratos/
    └── stockx/
        └── v1/
            └── stockx.proto              ★ 新增 ─ Watchlist / Holdings / Report / Calendar RPC

docs/
└── skills/                               ★ 新增 13 个 Skill 包
    ├── stock-data-normalize/SKILL.md
    ├── stock-technical-patterns/SKILL.md
    ├── stock-financial-ratio/SKILL.md
    ├── stock-fund-signal/SKILL.md
    ├── stock-news-classify/SKILL.md
    ├── stock-sentiment-score/SKILL.md
    ├── stock-industry-chain/SKILL.md
    ├── stock-risk-framework/SKILL.md
    ├── stock-factor-lib/SKILL.md
    ├── stock-chart-style/SKILL.md
    ├── stock-report-template/SKILL.md
    ├── stock-report-critic/SKILL.md
    └── stock-task-planning/SKILL.md
```

### 2.2 前端新增

```
web/src/
├── pages/
│   ├── stockx/
│   │   ├── WatchlistPage.vue              ★ 新增
│   │   ├── HoldingsPage.vue               ★ 新增（可选）
│   │   ├── StockDetailPage.vue            ★ 新增
│   │   └── ReportsPage.vue                ★ 新增
│   └── EcosystemPage.vue                  （沿用；后续上架本场景模板）
├── features/
│   └── stockx/
│       ├── api.ts                         ─ axios client（对接 stockx.proto）
│       ├── types.ts
│       ├── useWatchlist.ts                ─ composable
│       ├── useStockReport.ts
│       ├── useTradingCalendar.ts
│       └── __tests__/
├── components/
│   └── stockx/
│       ├── WatchlistTable.vue
│       ├── WatchlistImportDialog.vue
│       ├── StockKlineChart.vue            ─ ECharts K 线
│       ├── ReportListCard.vue
│       ├── ReportDetailDrawer.vue
│       └── SectorHeatmap.vue
└── router/
    └── stockx-routes.ts                   ★ 新增 ─ 注册到主 router
```

### 2.3 配置示例（部分）

`internal/scenario/stockanalysis/agents.yaml`（节选）：

```yaml
- key: agent_technical_analyst
  display_name: 技术分析师
  category: stock_analyst
  system_prompt_file: prompts/technical_analyst.md
  provider: openai
  model: gpt-4o-mini
  context_window: 128000
  tools_allow:
    - stock_quote_history
    - indicator_compute
    - chart_render_kline
  tools_deny:
    - workspace_exec
    - filesystem
  skills_allow:
    - stock-technical-patterns
    - stock-chart-style
  runtime_settings:
    tools_profile: stock_analyst
    parallel_tools: true
    output_schema_json: ${file:schemas/technical_analyst_output.json}
```

`internal/scenario/stockanalysis/teams.yaml`（节选）：

```yaml
- key: team_premarket_brief
  display_name: 盘前简报团队
  mode: coordinator
  description: 盘前 30 分钟自动生成自选股简报
  definition:
    timeout_seconds: 300
    member_tool_config:
      stream_inner: false
      history_scope: isolated
    members:
      - agent_key: agent_coordinator
        role: coordinator
        sort_order: 0
      - agent_key: agent_data_collector
        role: worker
        sort_order: 10
      - agent_key: agent_technical_analyst
        role: worker
        sort_order: 20
      - agent_key: agent_news_analyst
        role: worker
        sort_order: 30
      - agent_key: agent_money_flow_analyst
        role: worker
        sort_order: 40
      - agent_key: agent_report_writer
        role: synthesizer
        sort_order: 90
    synthesizer_agent_key: agent_report_writer
```

`internal/scenario/stockanalysis/crons.yaml`：

```yaml
- key: cron_premarket_brief
  name: 盘前简报
  schedule_type: cron
  expression: 0 30 8 * * MON-FRI
  team_key: team_premarket_brief
  trigger_payload_json: |
    {
      "instruction": "请基于我的自选股列表生成今日盘前简报",
      "trading_day_only": true,
      "watchlist_source": "kb_user_watchlist"
    }
  enabled: true
```

---

## 3. 数据模型

### 3.1 PostgreSQL `stockx` schema（DDL 摘要）

迁移文件：`internal/data/stockx/migrations/001_init.sql`。安装时 `stockx.EnsureSchema(ctx, data.Postgres())`（`STOCK_SCENARIO_ENABLED=1` 且 `data.postgres.source` 已配置）。

```sql
CREATE SCHEMA IF NOT EXISTS stockx;

CREATE TABLE stockx.watchlist (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL DEFAULT '',
    symbol      TEXT NOT NULL,
    market      TEXT NOT NULL CHECK (market IN ('a','hk','us')),
    name        TEXT,
    group_name  TEXT NOT NULL DEFAULT 'default',
    note        TEXT,
    sort_order  INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, symbol, market)
);

CREATE TABLE stockx.report (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL DEFAULT '',
    session_id      TEXT NOT NULL,
    team_id         TEXT,
    team_key        TEXT,
    symbol          TEXT,
    market          TEXT,
    report_type     TEXT NOT NULL,
    title           TEXT NOT NULL,
    summary_md      TEXT,
    artifact_id     TEXT NOT NULL,
    symbols_json    JSONB,
    data_cutoff_at  TIMESTAMPTZ,
    tool_refs_json  JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_report_user_type_created ON stockx.report (user_id, report_type, created_at DESC);

CREATE TABLE stockx.quote_cache (
    cache_key   TEXT PRIMARY KEY,
    symbol      TEXT NOT NULL,
    market      TEXT NOT NULL,
    period      TEXT NOT NULL,
    adjust      TEXT NOT NULL DEFAULT 'none',
    data_json   JSONB NOT NULL,
    provider    TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_quote_cache_expires ON stockx.quote_cache (expires_at);

-- 同理：stockx.holdings、stockx.stock_meta、stockx.news_cache
```

**Repo 约定**（对齐 `internal/data/knowledge.go`）：

- 仅通过 `*sql.DB`（`Data.Postgres()`）访问；biz 层依赖 `biz.StockWatchlistRepo` 等接口。
- `internal/tools/stockdata/*/cache.go` 注入 `StockQuoteCacheRepo`，**禁止**在 Tool 内 `sql.Open`。
- 清理任务：每日 03:00 `DELETE FROM stockx.quote_cache WHERE expires_at < now()`（`safego.Go`）。

### 3.2 平台 SQLite 与场景 Postgres 分工

| 写入方 | 目标库 | 内容 |
|--------|--------|------|
| `cmd/seed-stockx-org` | SQLite | 组织树、Agent、Team 定义 |
| `scenario/stockanalysis/install.go` | SQLite | Upsert Agent/Team/Cron（幂等） |
| `StockxService` / stockdata cache | Postgres `stockx` | 自选、报告索引、行情/新闻缓存 |
| `KnowledgeService` | Postgres + pgvector | 公司库、行业链 |
| `ArtifactService` | SQLite + 文件存储 | 报告 Markdown 正文 |

### 3.3 Knowledge 数据装载

| 知识库 | 数据来源 | 摄取频率 | 落地 |
|--------|----------|----------|------|
| `kb_listed_companies` | AKShare `stock_info_a_code_name` + `stock_info_hk_name` + `stock_info_us_name` | 每日一次（场景安装时首次全量；Cron 增量更新） | `kb_documents` + `kb_chunks` + `kb_embeddings`（pgvector） |
| `kb_index_constituents` | AKShare `index_stock_cons` | 每周一次 | 同上 |
| `kb_industry_chain` | 手工维护 + AI 标注（首版静态） | 手动 | Markdown 文档摄取 |
| `kb_research_glossary` | 手工 Markdown | 手动 | 摄取一次 |

---

## 4. Tool 设计

### 4.1 Provider 抽象（以 Quote 为例）

```go
// internal/tools/stockdata/quote/interface.go
package quote

type Provider interface {
    Name() string
    HistoricalDaily(ctx context.Context, req HistoricalRequest) (*HistoricalResult, error)
    Realtime(ctx context.Context, symbols []SymbolRef) (*RealtimeResult, error)
    Intraday(ctx context.Context, req IntradayRequest) (*IntradayResult, error)
}

type SymbolRef struct {
    Symbol string  // 600519
    Market string  // a / hk / us
}

type HistoricalRequest struct {
    Symbol   SymbolRef
    Period   string  // daily / 1min / 5min / 15min / 30min / 60min
    Adjust   string  // qfq / hfq / none
    DateFrom time.Time
    DateTo   time.Time
}

type HistoricalResult struct {
    Symbol  SymbolRef
    Period  string
    Adjust  string
    Bars    []OHLCV     // 归一化字段
    Source  string
    Cached  bool
}

type OHLCV struct {
    Time   time.Time
    Open   float64
    High   float64
    Low    float64
    Close  float64
    Volume float64
    Amount float64  // 成交额
}
```

```go
// internal/tools/stockdata/quote/tool.go
package quote

import (
    trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func NewHistoricalTool(providers []Provider, cache Cache) trpctool.Tool {
    return trpctool.NewFunctionTool(trpctool.FunctionToolConfig{
        Name:        "stock_quote_history",
        Description: "获取股票历史 K 线（A/H/U 股；日/分钟线；前复权/后复权）。",
        Schema:      historicalSchemaJSON,
        Call: func(ctx context.Context, args json.RawMessage) (any, error) {
            req, err := parseHistoricalArgs(args)
            if err != nil {
                return nil, err
            }
            if v, ok := cache.Get(ctx, req); ok {
                return v, nil
            }
            for _, p := range providers {
                res, err := p.HistoricalDaily(ctx, req)
                if err == nil {
                    cache.Set(ctx, req, res)
                    return res, nil
                }
            }
            return nil, errors.New("all quote providers failed")
        },
    })
}
```

### 4.2 工具注册

```go
// internal/tools/stockdata/registry.go
package stockdata

func Register(reg tools.Registry, deps Deps) {
    reg.Add(tools.Spec{
        Key:         "stock_quote_history",
        Category:    "stock_data",
        Description: "股票历史 K 线",
        RiskLevel:   "low",
        Provider:    "stockdata.quote",
        Factory:     func() tool.Tool { return quote.NewHistoricalTool(deps.QuoteProviders, deps.QuoteCache) },
    })
    // ... 其他工具
}
```

注册时机：`internal/data/data.go` 的 `ensureBuiltinPlatformTools()` 之后，如果 `STOCK_SCENARIO_ENABLED=1`，则调用 `stockdata.Register(...)`。

### 4.3 工具清单与 Schema 摘要

| Tool Key | Args 关键字段 | 返回结构 | Provider |
|----------|---------------|----------|----------|
| `stock_quote_realtime` | `symbols[]` | `[{symbol, price, change, change_pct, volume, amount, bid, ask, ts}]` | akshare / yfinance |
| `stock_quote_history` | `symbol, market, period, adjust, date_from, date_to` | `{bars: OHLCV[], source}` | akshare / yfinance / baostock |
| `stock_quote_intraday` | `symbol, market, period, date` | `{bars: OHLCV[]}` | akshare |
| `stock_fundamental_overview` | `symbol, market` | `{name, industry, concept, pe, pb, roe, dividend_yield, ...}` | akshare / tushare |
| `finance_statement_income` | `symbol, market, period, periods` | `{rows: FinancialRow[]}` | tushare / akshare |
| `finance_statement_balance` | 同上 | 同上 | 同上 |
| `finance_statement_cashflow` | 同上 | 同上 | 同上 |
| `finance_indicator` | `symbol, market, indicators[]` | `{indicators: {key: value}}` | tushare / akshare |
| `hsgt_flow` | `date_from, date_to` | `{rows: HSGTRow[]}` | akshare |
| `dragon_tiger` | `date` | `{rows: DragonTigerRow[]}` | akshare |
| `stock_money_flow_individual` | `symbol, market, date_range` | `{rows: MoneyFlowRow[]}` | akshare |
| `margin_balance` | `symbol, date_range` | `{rows: MarginRow[]}` | akshare |
| `stock_announcement` | `symbol, market, date_range, limit` | `{items: AnnouncementItem[]}` | akshare / 巨潮 |
| `stock_news_individual` | `symbol, market, date_range, limit` | `{items: NewsItem[]}` | akshare |
| `research_report_search` | `symbol or keyword, date_range, limit` | `{items: ReportItem[]}` | akshare |
| `policy_search` | `keyword, date_range, limit` | `{items: PolicyItem[]}` | akshare / 政府网爬虫 |
| `stock_sentiment_xueqiu` | `symbol, market` | `{posts: PostItem[], heat_score}` | 雪球 H5 |
| `stock_sentiment_guba` | `symbol, market, limit` | `{posts: PostItem[]}` | 东方财富股吧 |
| `industry_classification` | — | `{industries: [{code, name, parent}]}` | akshare |
| `industry_money_flow` | `date` | `{rows: IndustryFlowRow[]}` | akshare |
| `stock_concept_list` | — | `{concepts: ConceptItem[]}` | akshare |
| `indicator_compute` | `bars: OHLCV[], indicators: [{name, params}]` | `{series: {name: number[]}}` | 内置纯 Go |
| `chart_render_kline` | `symbol, bars, overlays, output_format` | `{artifact_id, mime, base64}` | CodeExecutor + matplotlib |
| `chart_render_pie` | `data, title, output_format` | 同上 | 同上 |
| `chart_render_heatmap` | `matrix, x_labels, y_labels, output_format` | 同上 | 同上 |
| `is_trading_day` | `date, market` | `{is_trading: bool, reason}` | 内置交易日历 |
| `next_trading_day` | `date, market, offset` | `{date}` | 内置 |
| `stock_resolve` | `query`（中文名或简拼） | `{candidates: [{symbol, market, name, score}]}` | Knowledge 检索 + 模糊匹配 |

### 4.4 工具治理

- 所有新增工具走平台 **Tools 管理** 流程：默认 `risk_level=low`，写入类（如缓存刷新工具）`medium`
- 每个工具有 `parameters_schema_json`（JSON Schema），由 Tool Adapter 自动注入到 LLM tool definition
- **AfterTool 钩子**自动写入 `tool_invocations` 表，前端可在工具调用面板看到耗时、参数（脱敏）、是否缓存命中
- 失败重试策略：HTTP/网络错误自动重试 2 次（exp backoff），数据缺失不重试

### 4.5 与 CodeExecutor 集成（图表/因子）

- `chart_render_kline` 内部构造 Python 代码片段（基于 mplfinance），通过 CodeExecutor 执行：
  ```python
  # 自动生成
  import mplfinance as mpf
  import pandas as pd
  df = pd.DataFrame(__bars__).set_index('Date')
  mpf.plot(df, type='candle', style='charles', volume=True,
           savefig=dict(fname='/tmp/kline.png', dpi=150))
  ```
- 执行结果（PNG）保存到 Artifact，工具返回 `artifact_id`
- Skill `stock-chart-style` 提供风格预设与水印
- 沙箱默认使用 Docker；本地 LocalExec 仅用于开发

---

## 5. Agent 设计

### 5.1 通用约束

每个分析师 Agent 的 `system_prompt` 都包含：

1. **角色定义**：你是 XX 分析师，专注 YY，输出风格 ZZ
2. **数据声明**：你只能基于「明确返回的工具结果」做分析，禁止凭空捏造
3. **输出契约**：必须使用结构化 Markdown（H2 段落 + 表格），每个结论附 `data_ref: <tool_call_id>`
4. **风险护栏**：禁止给出具体买卖价位；禁止表述确定性收益；必须附风险提示
5. **协作**：当被作为 AgentTool 调用时，输出长度限制（默认 ≤ 800 token），由 coordinator 决定是否需要扩展

### 5.2 关键 Agent 详解

#### 5.2.1 `agent_coordinator`

| 项 | 设计 |
|----|------|
| 模型 | 强推理（gpt-4o / claude-opus / deepseek-r1） |
| 工具 | 仅 `AgentTool(...)` 调度成员 |
| Skill | `stock-task-planning`、`stock-report-template` |
| 输出契约 | 先输出 **任务拆分 JSON**，再按拆分调用 AgentTool；最后将多 Agent 输出整合为统一回复（synthesizer 缺席时） |
| 上下文管理 | history_scope = isolated；防止成员 Agent 之间 token 串污 |

#### 5.2.2 `agent_technical_analyst`

| 项 | 设计 |
|----|------|
| 模型 | 中等（gpt-4o-mini / claude-haiku） |
| 工具 | `stock_quote_history`、`indicator_compute`、`chart_render_kline` |
| Skill | `stock-technical-patterns`、`stock-chart-style` |
| 输出结构 | 趋势判断 / 形态识别 / 关键支撑压力 / 量价配合 / 短期方向 |
| Output Schema | JSON（trend, pattern, support, resistance, volume_signal, short_term_view, evidence） |

#### 5.2.3 `agent_report_writer`

| 项 | 设计 |
|----|------|
| 模型 | 强推理（写作能力优先） |
| 工具 | `artifact_save`、`channel_push`（基于已有 Channel） |
| Skill | `stock-report-template`（按报告类型选择模板） |
| Input | 来自 coordinator/team 汇总后的多 Agent JSON 输出 |
| 流程 | 模板选择 → 段落生成 → 图表占位 → 数据引用注入 → 风险提示 → 保存 Artifact |
| 输出 | Markdown（含 ![](artifact://xxx) 引用） + 飞书卡片（structured JSON） |

### 5.3 配置即代码（场景安装）

启动时 `internal/scenario/stockanalysis/install.go` 做以下事情：

1. 检查 `STOCK_SCENARIO_ENABLED`，未启用直接返回
2. 加载 `agents.yaml` / `teams.yaml` / `crons.yaml` / `graphs/*.json`
3. 调用 `AgentUsecase.UpsertByKey(...)` 幂等创建/更新（按 `key` 唯一）
4. 调用 `TeamUsecase.UpsertByKey(...)` 同上
5. 调用 `CronUsecase.UpsertByKey(...)` 同上
6. 调用 `SkillUsecase.ImportFromDirectory(docs/skills/stock-*)` 触发 fsnotify 装载
7. 注册 Tool（在 `wireApp` 流程末尾）

所有 Upsert 都走 biz 层的 idempotent API，不破坏用户后续手动修改（用户修改后场景安装时按 `system_managed=true` 标记决定是否覆盖）。

---

## 6. Team 与 Graph 编排详细设计

### 6.1 `team_premarket_brief`（Coordinator）

```
agent_coordinator
   ├─ AgentTool(agent_data_collector)        ─ 获取 watchlist 当日开盘前数据
   ├─ AgentTool(agent_news_analyst)          ─ 检索昨晚至今晨的关键消息
   ├─ AgentTool(agent_money_flow_analyst)    ─ 北向资金 / 龙虎榜
   ├─ AgentTool(agent_technical_analyst)     ─ 关键个股技术形态
   └─ AgentTool(agent_report_writer)         ─ synthesizer
```

- `MemberToolConfig`：`stream_inner=false, history_scope=isolated, skip_summarization=false`
- `timeout_seconds=300`
- `SwarmConfig` 不适用（非 Swarm）

### 6.2 `team_stock_deep_dive`（Graph 优先）

Graph 定义（`graphs/stock_deep_dive.graph.json`）节选：

```json
{
  "key": "graph_stock_deep_dive",
  "version": 1,
  "state_schema": {
    "symbol": "string",
    "market": "string",
    "quote_history": "object",
    "fundamentals": "object",
    "money_flow": "object",
    "news": "object",
    "sentiment": "object",
    "industry": "object",
    "risk": "object",
    "chart_artifact_id": "string",
    "report_md": "string"
  },
  "nodes": [
    { "id": "resolve", "type": "function", "handler": "stock.resolve_symbol" },
    { "id": "fetch_quote", "type": "tool", "tool": "stock_quote_history" },
    { "id": "fan_out", "type": "parallel", "members": [
      { "id": "tech",  "type": "agent", "agent_key": "agent_technical_analyst",   "input_mapper": "stock.tech_input",  "output_mapper": "stock.tech_output" },
      { "id": "fund",  "type": "agent", "agent_key": "agent_fundamental_analyst","input_mapper": "stock.fund_input",  "output_mapper": "stock.fund_output" },
      { "id": "flow",  "type": "agent", "agent_key": "agent_money_flow_analyst", "input_mapper": "stock.flow_input",  "output_mapper": "stock.flow_output" },
      { "id": "news",  "type": "agent", "agent_key": "agent_news_analyst",       "input_mapper": "stock.news_input",  "output_mapper": "stock.news_output" },
      { "id": "senti", "type": "agent", "agent_key": "agent_sentiment_analyst",  "input_mapper": "stock.senti_input", "output_mapper": "stock.senti_output" }
    ]},
    { "id": "risk",   "type": "agent", "agent_key": "agent_risk_assessor",
      "input_mapper": "stock.risk_input", "output_mapper": "stock.risk_output" },
    { "id": "chart",  "type": "tool",  "tool": "chart_render_kline",
      "input_mapper": "stock.chart_input" },
    { "id": "report", "type": "agent", "agent_key": "agent_report_writer",
      "input_mapper": "stock.report_input", "output_mapper": "stock.report_output" },
    { "id": "persist","type": "function","handler": "stock.persist_report" }
  ],
  "edges": [
    { "from": "__start__", "to": "resolve" },
    { "from": "resolve",   "to": "fetch_quote" },
    { "from": "fetch_quote","to": "fan_out" },
    { "from": "fan_out",   "to": "risk" },
    { "from": "risk",      "to": "chart" },
    { "from": "chart",     "to": "report" },
    { "from": "report",    "to": "persist" },
    { "from": "persist",   "to": "__end__" }
  ]
}
```

- InputMapper/OutputMapper handler 注册在 `internal/scenario/stockanalysis/graphhandlers/`
- `__start__` 接收用户原始输入（自然语言），由 `resolve` 节点解析为 symbol+market
- 并行节点失败时遵循 Graph 框架的「fail_strategy=continue」策略，缺失维度由 `agent_report_writer` 显式标注

### 6.3 `team_sector_rotation`（Sequential）

`industry_analyst → money_flow_analyst → news_analyst → report_writer`，逐步累积上下文。

### 6.4 `team_portfolio_doctor`（Parallel + Synthesizer）

- 三个 worker 并行：`risk_assessor`、`fundamental_analyst`、`quant_factor`
- `synthesizer_agent_key=agent_report_writer`
- 接收持仓 JSON 作为输入（来自 `stock_holdings` 表或用户上传）

### 6.5 `team_market_recap`（Sequential）

收盘后流水线：行情快照 → 资金 → 板块 → 异动股 → 明日关注 → 撰写。

### 6.6 `team_research_pipeline`（自定义 Graph）

用户在 Graph 编辑器中自由编排（依赖 Graph 模块的画布前端）。

---

## 7. Cron 调度设计

### 7.1 任务定义

复用 `cron_tasks` 表，新增字段使用方式：

| 字段 | 内容 |
|------|------|
| `team_id` | `team_premarket_brief` 等 Team 的 ID |
| `trigger_payload_json` | `{instruction, watchlist_source, trading_day_only, market}` |
| `target_session_strategy` | `per_run_new`（每次新建 session） / `daily_singleton`（每天复用一个） |

### 7.2 节假日跳过

- 新增 `TradingCalendarUsecase`：维护 A/H/U 股节假日
- CronRunner 触发前调用 `IsTradingDay(date, market)`；非交易日记录 `cron_task_runs.status=skipped, reason=non_trading_day`
- 节假日数据加载：场景安装时全量拉一次未来 2 年（AKShare `tool_trade_date_hist_sina`），后续每月增量

### 7.3 失败处理

- 数据源失败 → Cron task 标记 `failure`，记录原因；失败次数 +1
- Channel 推送失败 → 报告仍写入 Artifact，但 Cron run 标记 `partial_success`
- 连续 3 次失败时通过 Monitor 模块告警

---

## 8. Channel 推送设计

### 8.1 飞书机器人

- 沿用 `platform_channels` 表，`channel_type=feishu_robot`
- 报告生成完成后，`agent_report_writer` 调用 `channel_push` 工具，参数：`{channel_key, card_json, fallback_text}`
- 卡片模板由 `stock-report-template` Skill 提供（覆盖 5 种报告类型）
- 卡片中嵌入指向 Artifact 详情页的深链（用户点击跳到 Web UI 查看完整报告）

### 8.2 邮件 SMTP（P1）

- 新增 channel_type `email_smtp`
- 长报告作为 `report.html` 附件；摘要作为正文
- 配置：SMTP server / port / TLS / from / 收件人列表

---

## 9. 前端设计

### 9.1 Watchlist 页

- 路径：`/stockx/watchlist`
- 组件：`WatchlistPage.vue` + `WatchlistTable.vue` + `WatchlistImportDialog.vue`
- 功能：
  - 表格：股票代码 / 名称 / 当前价 / 涨跌幅 / 所属分组 / 备注 / 操作
  - 顶部工具栏：搜索、分组筛选、批量导入（粘贴 CSV / 文本）、新增、刷新
  - 行内操作：编辑、删除、查看详情、立即分析（跳转 Chat 并预填）
  - 当前价：StockxService 批量调 `stock_quote_realtime` 或读 `quote_cache`（≤60s TTL）；前端 **30–60s 轮询**（非交易所 SSE）；页脚标注延迟
- 暗色模式：复用平台 `app-page-cream` + `body--dark`

### 9.2 Stock Detail 页

- 路径：`/stockx/detail/:market/:symbol`
- 区块：
  - 基础信息卡（名称 / 行业 / 概念 / 当前价 / 关键指标）
  - K 线图（默认 90 日，可切换周期；ECharts）
  - 最近报告列表（来自 `stock_report` 表，按时间倒序）
  - 「立即深度分析」按钮 → 跳转 Chat，自动选中 `team_stock_deep_dive`

### 9.3 Reports 页

- 路径：`/stockx/reports`
- 表格：标题 / 类型 / 涉及股票 / 数据截止时间 / 生成时间 / Artifact 链接
- 筛选：报告类型、日期范围、股票（联想搜索）
- 行内：查看（侧拉抽屉展示完整 Markdown，支持复制 / 下载 / 重新生成）

### 9.4 Cron 管理（沿用平台）

- 4 个内置 Cron 自动出现在 Cron 管理页
- 用户可启用/禁用、查看运行历史、查看失败原因

### 9.5 Chat 中的 Team 列表

- 内置 6 个 Team 通过场景安装写入 `teams` 表，自动出现在 Chat 左侧 Team 列表
- Team 头像通过 Avatar 模块上传/默认占位

---

## 10. Proto 与 RPC 设计

新增 `api/kratos/stockx/v1/stockx.proto`：

```protobuf
syntax = "proto3";
package kratos.stockx.v1;
import "google/api/annotations.proto";
import "google/api/field_behavior.proto";
import "google/protobuf/struct.proto";

option go_package = "aranea-agents/api/kratos/stockx/v1;v1";

service StockxService {
  // Watchlist
  rpc ListWatchlist (ListWatchlistRequest) returns (ListWatchlistResponse) {
    option (google.api.http) = { get: "/v1/stockx/watchlist" };
  }
  rpc UpsertWatchlistItem (UpsertWatchlistItemRequest) returns (WatchlistItem) {
    option (google.api.http) = { post: "/v1/stockx/watchlist", body: "*" };
  }
  rpc DeleteWatchlistItem (DeleteWatchlistItemRequest) returns (DeleteWatchlistItemResponse) {
    option (google.api.http) = { delete: "/v1/stockx/watchlist/{id}" };
  }
  rpc BatchImportWatchlist (BatchImportWatchlistRequest) returns (BatchImportWatchlistResponse) {
    option (google.api.http) = { post: "/v1/stockx/watchlist:batchImport", body: "*" };
  }

  // Reports
  rpc ListReports (ListReportsRequest) returns (ListReportsResponse) {
    option (google.api.http) = { get: "/v1/stockx/reports" };
  }
  rpc GetReport (GetReportRequest) returns (Report) {
    option (google.api.http) = { get: "/v1/stockx/reports/{id}" };
  }
  rpc TriggerAnalysis (TriggerAnalysisRequest) returns (TriggerAnalysisResponse) {
    option (google.api.http) = { post: "/v1/stockx/reports:trigger", body: "*" };
  }

  // Calendar
  rpc IsTradingDay (IsTradingDayRequest) returns (IsTradingDayResponse) {
    option (google.api.http) = { get: "/v1/stockx/calendar/is-trading-day" };
  }

  // Stock Resolve
  rpc ResolveStock (ResolveStockRequest) returns (ResolveStockResponse) {
    option (google.api.http) = { get: "/v1/stockx/stocks:resolve" };
  }
}
```

字段定义遵循平台风格（蛇形 / `field_behavior = REQUIRED`），具体不在此展开。

---

## 11. 安全与合规设计

| 风险 | 控制点 |
|------|--------|
| API Key 泄露 | 落库时用平台 Provider 加密机制；前端只回传遮罩值 |
| 用户持仓外发 | Plugin BeforeModel 钩子，将 `holdings_*` 字段在 prompt 中替换为 `<HOLDING_REDACTED>`，仅对 trusted model 解除 |
| 报告免责 | `stock-report-template` Skill 强制 footer 模板；report_writer 输出后由 critic 验证 |
| 高风险工具 | 凡是「数据下单 / 第三方账户操作」类工具一律不在本场景注册 |
| 抓取频率 | 内置令牌桶（基于 `pkg/safego` + `rate.Limiter`），每个 Provider 独立 |
| 来源声明 | 所有飞书/邮件推送末尾固定文案：「数据来源：AKShare / Tushare …。本内容由 AI 生成，仅供学习研究」 |

---

## 12. 可观测性设计

### 12.1 Telemetry / Trace

- 每次 Team Run 生成根 span：`stockx.team.run`，attributes：`team_key, session_id, user_id, symbols`
- 每个工具调用产生子 span：`stockx.tool.{tool_key}`，attributes：`provider, cached, latency_ms`
- 失败 span 标 `error.type` 与 `error.message`

### 12.2 Metrics（Prometheus）

| 指标 | 类型 | 标签 |
|------|------|------|
| `stockx_team_run_total` | counter | team_key, status |
| `stockx_team_run_duration_seconds` | histogram | team_key |
| `stockx_tool_call_total` | counter | tool_key, status, provider, cache_hit |
| `stockx_tool_call_duration_seconds` | histogram | tool_key, provider |
| `stockx_report_generated_total` | counter | report_type |
| `stockx_cron_run_total` | counter | cron_key, status |
| `stockx_data_source_failure_total` | counter | provider, tool_key |

### 12.3 Grafana 仪表盘

新增 `docs/observability/grafana-stockx.json`，面板：

- 今日报告生成数 / Cron 成功率
- 各 Team 平均耗时趋势
- 数据源失败 TOP10
- Token 用量（按 Team / Agent / Model）
- 缓存命中率

---

## 13. 配置与启停

### 13.1 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `STOCK_SCENARIO_ENABLED` | `0` | 总开关 |
| `STOCK_SCENARIO_DEFAULT_MARKET` | `a` | 默认市场 |
| `STOCK_SCENARIO_AUTO_INSTALL` | `1` | 首次启动是否自动 upsert Agent/Team/Cron/Skill |
| `STOCK_SCENARIO_REINSTALL_ON_START` | `0` | 每次启动是否覆盖（用于开发） |
| `STOCK_SCENARIO_TUSHARE_TOKEN` | — | Tushare token（生产推荐用 Secret） |
| `STOCK_SCENARIO_FEISHU_WEBHOOK` | — | 默认飞书 webhook |
| `STOCK_SCENARIO_CACHE_TTL_QUOTE` | `1h` | 行情缓存 TTL |
| `STOCK_SCENARIO_CHART_BACKEND` | `codeexecutor` | 图表后端（`codeexecutor` / `local`） |

### 13.2 启动顺序

```
main()
  ↓
wireApp()
  ├ NewData()
  │   ↓
  │   ensureBuiltinPlatformTools()
  │   ↓
  │   stockanalysis.RegisterTools(deps)        ← 仅当 ENABLED=1
  │   ↓
  │   ensureSessionMemorySchema()
  │   ↓
  │   ensureStockxSchema()                     ← 新增；幂等创建本场景表
  │
  ├ Biz / Service / Server 装配
  │
  ├ CronRunner.Start(...)                      ← 沿用，cron_tasks 表中本场景任务自动调度
  ├ SkillWatch.Start(...)                      ← 沿用
  │
  └ stockanalysis.RunInstall(ctx, deps)        ← 异步 safego.Go 执行
        ├ AgentUsecase.UpsertByKey(...)
        ├ TeamUsecase.UpsertByKey(...)
        ├ CronUsecase.UpsertByKey(...)
        ├ KnowledgeUsecase.EnsureKBs(...)
        └ log.Info("stockx scenario installed")
```

---

## 14. 测试设计

### 14.1 单元测试

| 模块 | 用例覆盖 |
|------|----------|
| `quote.akshare_provider` | 历史 K 线 / 实时 / 异常状态码 / 限流 / 解析失败 |
| `indicator.compute` | MA/MACD/KDJ/RSI/BOLL 等正确性（黄金数据集对比） |
| `calendar` | A 股节假日 / 调休 / 国庆 / 春节 |
| `stock_resolve` | 中文名 / 简拼 / 行业别名 |
| `report_persist` | 报告写入 / 与 Artifact 关联 |
| `cron_install` | 幂等性 / 覆盖策略 |

### 14.2 契约测试

- 对每个 Tool 编写 `tool_contract_test.go`：模拟 Provider 返回，验证返回字段、JSON Schema 一致
- 对 Team 编排编写 `team_smoke_test.go`：用 Mock Agent 验证 coordinator → members → synthesizer 调用链

### 14.3 端到端

- 提供 `examples/stockx/run_deep_dive.sh` 脚本：拉起本地 admin → 调用 RPC TriggerAnalysis → 等待报告生成 → 校验 Artifact 内容
- CI 阶段使用 `httpfetch` Mock 返回固定数据，避免依赖外网

### 14.4 评估（P2）

- 复用 Evaluation 模块，建立**报告质量评估集**：
  - 输入：10~20 个典型问题（个股 / 行业 / 复盘 / 组合）
  - 评分维度：结构完整性、数据引用率、风险提示存在性、TL;DR 准确性、术语正确性
  - 评估器：LLM-as-Judge（用 GPT-4o 评 GPT-4o-mini 报告）

---

## 15. 部署与运维

### 15.1 单机一键启动

提供 `docker-compose.stockx.yml`：

```yaml
services:
  aranea-admin:
    image: aranea/admin:latest
    environment:
      - STOCK_SCENARIO_ENABLED=1
      - STOCK_SCENARIO_DEFAULT_MARKET=a
      - STOCK_SCENARIO_FEISHU_WEBHOOK=${FEISHU_WEBHOOK}
    volumes:
      - ./data:/app/data
    ports: ["8000:8000", "8001:8001"]

  aranea-web:
    image: aranea/web:latest
    ports: ["5173:5173"]

  aranea-python-sandbox:
    image: aranea/codeexecutor-py:latest
    # mplfinance / pandas / akshare / yfinance 预装
```

### 15.2 升级策略

- 场景升级 = 升级 Aranea 平台 + 重新 install 场景配置
- `STOCK_SCENARIO_REINSTALL_ON_START=1` 时按 `system_managed=true` 标记强制覆盖；其余保留用户修改
- Cron / Team 的删除：场景包提供 `manifest.json` 列出**当前版本**所有资源 key，本地多余 `system_managed=true` 的资源在升级时移除（带 confirm 提示）

### 15.3 数据迁移

- 新增表通过 Ent 自动迁移（dev 模式）；生产模式提供 SQL 脚本 `docs/sql/stockx_init.sql`
- 缓存表过期清理由 `safego.Go` 定时任务执行（每天 03:00 清理过期）

---

## 16. 与平台演进的协同点

| 平台模块 | 本场景的反向需求 |
|----------|------------------|
| Cron (EP-BIZ-09) | 必须有可靠的调度引擎；本场景作为最先大规模使用 Cron 的应用，会驱动调度引擎完善 |
| Channel (EP-BIZ-08) | 飞书 webhook 推送与卡片渲染；本场景作为典型用户 |
| Knowledge (EP-KN-01/02) | `kb_listed_companies` 摄取需要稳定的 Embedder + 异步流水线 |
| Graph (EP-BIZ-10) | `team_stock_deep_dive` 与 `team_research_pipeline` 是 Graph 的主要场景验证 |
| Plugin (EP-CB-01) | 持仓脱敏 / 高风险数据屏蔽需要 Plugin Before* 钩子 |
| CodeExecutor (EP-BIZ-04) | 图表生成、因子计算依赖 Docker Sandbox |
| Evaluation | 报告质量 LLM-as-Judge 体系化落地 |

> 凡是平台尚未完成的能力，本场景以**最小可行 fallback**（如纯 Go 指标计算替代 Python；本地图表 fallback；Plain text 替代飞书卡片）保证 MVP 仍能跑通。

---

## 17. 关键设计取舍

| 取舍 | 选择 | 理由 |
|------|------|------|
| Tool vs Skill | 「能力」用 Tool，「方法论/规范」用 Skill | 工具是确定性 API，方法论是上下文文本，前者注入工具集，后者注入 prompt |
| Coordinator vs Graph | 简单场景用 coordinator，多步骤可分支用 Graph | coordinator 黑盒、易上手；Graph 显式、可观测、可暂停 |
| Python vs Go 计算指标 | 默认 Go，复杂回测 Python | 减少 CodeExecutor 依赖，常见指标在 Go 中实现性能好 |
| 场景业务表 SQLite vs Postgres | **Postgres `stockx` schema** | 与 Knowledge/pgvector 同实例；便于缓存、报告检索与后续多实例；平台 Ent 仍 SQLite |
| 行情缓存 Postgres vs Redis | Postgres（v1.0） | 与业务表同库；高并发展示层 v1.x 可后置 Redis |
| 深度分析 Coordinator vs Graph | **Graph 优先**（MVP 末 / M-S4） | Graph 共享 `state.quote_history`，减少重复拉行情与限频；盘前简报仍用 Coordinator |
| 数据源 SDK 直连 vs Sidecar | Sidecar Python 服务 + Go HTTP 调用 | 避免 cgo / 维护 SDK 多版本；同时 AKShare 等是 Python 生态 |
| 报告输出 Markdown vs JSON | Markdown 主，JSON 辅 | Markdown 直接给人看；同时 metadata 用 JSON 落 stock_report 表便于检索 |
| Watchlist 在 SQL 还是 Knowledge | SQL 主，Knowledge 辅（仅做 RAG 检索） | 结构化数据用 SQL；模糊匹配/语义检索时由 Cron 同步到 KB |

---

## 18. 关键名词与约定

- **Stockx**：本场景在代码层的命名 namespace（避免「stock」与三方库冲突；与平台模块前缀如 `chatx` 等保持一致）
- **场景安装包**：`internal/scenario/<scenario_key>/` 下的 YAML/JSON + Go install.go，描述一个完整可落地的场景应用
- **Provider**：数据源具体实现（区别于平台 Provider 模块的「LLM Provider」）
- **System Managed**：标记由场景安装包创建的资源，升级时可被覆盖；用户克隆出的副本不再带此标记

---

## 19. 文档变更记录

| 版本 | 日期 | 变更 |
|------|------|------|
| v1.0 | 2026-05-18 | 初版 |
| v1.1 | 2026-05-25 | 场景业务表改 Postgres `stockx`；§1.3 实时性；§1.4 多 Agent 通路；Ent→SQL 迁移 |

---

## 20. 参考文档

- [需求文档](./daily-stock-analysis.md)
- [开发计划](./daily-stock-analysis-development.md)
- 平台编码规范 [`AI-DEVELOPMENT-SPECIFICATION.md`](../../guides/AI-DEVELOPMENT-SPECIFICATION.md)
- 平台架构 [`0 系统框图.md`](../../需求/0%20系统框图.md)
- 平台 Team [`11 multi-agent.md`](../../需求/11%20multi-agent.md)
- 平台 Graph [`36 graph-workflow.md`](../../需求/36%20graph-workflow.md)
- 平台 Tools [`23 tools.md`](../../需求/23%20tools.md)
- 平台 Skill [`20 skill.md`](../../需求/20%20skill.md)
- 平台 Cron [`21 cron.md`](../../需求/21%20cron.md)
- 平台 Channel [`17 channel.md`](../../需求/17%20channel.md)
- 平台 Knowledge [`37 knowledge.md`](../../需求/37%20knowledge.md)
- 平台 Artifact [`27 artifact.md`](../../需求/27%20artifact.md)

