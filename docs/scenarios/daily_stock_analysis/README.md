# Daily Stock Analysis — 场景索引

> **场景代号**：`daily_stock_analysis` ｜ **代码命名空间**：`stockx`
> **平台**：基于 [Aranea-Agents](../../README.md) 多智能体编排平台
> **定位**：开源、可自托管的 AI 股票分析助手；多 Agent 协作 + 多渠道接入 + 可观测可追溯
> **状态**：📐 设计中（v1.1 需求/设计/开发计划已对齐：Postgres、准实时、多 Agent 数据通路）

---

## 1. 场景一句话

> 由 10+ 个专业分析师 Agent（数据采集、技术、基本面、资金、消息、情绪、行业、风险、报告写作、主控）组成的「AI 投研团队」，通过 Chat / Cron / 飞书 / 邮件 等入口，自动完成 **盘前简报 / 个股深度分析 / 板块扫描 / 持仓诊断 / 盘后复盘** 等开源股票分析工作流。

---

## 2. 文档导航

| # | 文档 | 用途 |
|---|------|------|
| 0 | [cmd/seed-stockx-org/README.md](../../cmd/seed-stockx-org/README.md) | **种子数据 CLI** — 组织 / Agent / Team 写入 SQLite 的使用说明 |
| 1 | [daily-stock-analysis.md](./daily-stock-analysis.md) | **需求文档** — 用户故事、Agent 团队、功能规格、数据源清单、验收标准、非功能需求 |
| 2 | [daily-stock-analysis.design.md](./daily-stock-analysis.design.md) | **设计文档** — 架构、目录结构、Tool/Skill/Agent/Team/Graph 详细设计、数据模型、Proto、安全 |
| 3 | [daily-stock-analysis-development.md](./daily-stock-analysis-development.md) | **开发计划** — 8 个 Phase、6 个里程碑、56 个 EP-STOCKX-* 任务清单、风险与依赖 |

阅读顺序建议：**README（本文）→ 需求 → 设计 → 开发计划**。

---

## 3. 场景关键能力

| 能力 | 说明 |
|------|------|
| **多 Agent 协作** | 10+ 专业角色 Agent，6 种 Team 编排模式（coordinator / sequential / parallel / critic_loop / graph） |
| **国内数据源原生** | AKShare（A 股）、Tushare（财务）、yfinance（港美股）、东方财富（资金）、雪球（情绪）开箱即用 |
| **可视化报告** | Markdown + 飞书卡片 + 邮件附件，含 K 线 / 财务 / 行业 / 组合图表 |
| **定时调度** | 4 个内置 Cron（盘前简报 / 盘后复盘 / 周板块扫描 / 月组合诊断），节假日自动跳过 |
| **多渠道推送** | 飞书 Webhook + 邮件 SMTP（Web UI 原生展示） |
| **可观测可追溯** | 复用平台 Telemetry / Monitor / Team Run / Tool Invocation；Grafana 面板 |
| **可扩展** | 通过 Graph 工作流自定义研究流水线；通过 Tools 管理页新增数据源 |
| **合规 & 安全** | 持仓数据本地化；API Key 加密；强制免责声明；不提供任何下单接口 |
| **存储** | 场景业务表在 **PostgreSQL `stockx` schema**；平台 Agent/Session 仍 SQLite |
| **准实时行情** | Tool 轮询 ≥1min（非 Tick）；Web 自选 30–60s 刷新并标注延迟 |
| **跨 Agent 数据** | Tool → 当前 Agent → Coordinator/Graph state → 下一 Agent（见需求 §1.6） |

---

## 3.1 技术决策摘要（v1.1）

| 主题 | 决策 | 文档 |
|------|------|------|
| 数据库 | 场景表 + 缓存 + 报告索引 → **Postgres `stockx`**；平台 CRUD → SQLite | [需求 §5.0](./daily-stock-analysis.md#50-存储架构已确认postgresql) · [设计 §3](./daily-stock-analysis.design.md#3-数据模型) |
| 实时性 | L1 准实时 1–3min；不做 L0 Tick；深度分析端到端常 2–8min | [需求 §1.5](./daily-stock-analysis.md#15-实时性分级已确认) |
| 多 Agent 喂数 | Graph 优先（共享 K 线 state）；盘前用 Coordinator | [设计 §1.4](./daily-stock-analysis.design.md#14-多-agent-市场数据通路) |
| 实施节奏 | 推荐 **策略 B MVP**（3–4 周可演示） | [开发计划 §1](./daily-stock-analysis-development.md#1-推荐实施策略已确认) |

---

## 4. Agent 团队总览

```
                       ┌────────────────────────────┐
                       │       agent_coordinator     │  ← 主控（必选，调度全队）
                       └─────────────┬──────────────┘
                                     │ AgentTool 调用
        ┌────────────┬───────────────┼───────────────┬────────────┬────────────┐
        ▼            ▼               ▼               ▼            ▼            ▼
  data_collector  technical    fundamental      money_flow      news     sentiment
   数据采集员      技术分析师    基本面分析师       资金面分析师   消息面分析师  情绪面分析师
                                     │
                       ┌─────────────┼──────────────┐
                       ▼             ▼              ▼
                  industry        risk_assessor    quant_factor
                  行业分析师       风险评估师       因子计算员(可选)
                                     │
                       ┌─────────────┼──────────────┐
                       ▼             ▼              ▼
                  chart_builder   report_writer    critic
                   图表构建员      报告撰写员       评审员(可选)
```

详见 [需求文档 §3](./daily-stock-analysis.md#3-agent-团队角色清单)。

---

## 5. 内置 Team 矩阵

| Team Key | 模式 | 场景 | 触发方式 |
|----------|------|------|----------|
| `team_premarket_brief` | coordinator | 盘前 30 分钟简报 | Cron `0 30 8 * * MON-FRI` |
| `team_stock_deep_dive` | graph / coordinator | 个股深度分析（多维并行） | Chat |
| `team_sector_rotation` | sequential | 周板块扫描 | Cron `0 0 18 * * MON` |
| `team_portfolio_doctor` | parallel + synthesizer | 持仓组合诊断 | Chat / Cron |
| `team_market_recap` | sequential | 盘后复盘 | Cron `0 0 17 * * MON-FRI` |
| `team_research_pipeline` | graph (自定义) | 用户自定义研究流水线 | Chat / Graph 编辑器 |

---

## 6. 数据源能力

| 类型 | 工具数 | 主 Provider | 备 Provider |
|------|--------|-------------|--------------|
| 行情 | 3 | AKShare | yfinance / baostock |
| 基本面 | 5 | AKShare | Tushare |
| 资金面 | 4 | AKShare | 东方财富爬虫 |
| 消息面 | 4 | AKShare | 巨潮资讯 |
| 情绪面 | 2 | 雪球 H5 | 股吧爬虫 |
| 行业 | 3 | AKShare | — |
| 指标计算 | 1 | 内置 Go | — |
| 可视化 | 3 | CodeExecutor + matplotlib | 本地 SVG fallback |
| 工具类 | 2 | 内置（交易日历、stock_resolve） | — |
| **合计** | **27** | | |

详见 [设计文档 §4.3](./daily-stock-analysis.design.md#43-工具清单与-schema-摘要)。

---

## 7. 与平台模块的关系

本场景**不修改平台核心**，所有能力通过平台标准接口注入：

- **新增**：27 个数据源 Tool、13 个 Skill、4 个 Knowledge Base、Postgres `stockx` 5 张业务表、1 个 Service、1 个 proto、3 个前端页面
- **复用**：Agent / Team / Graph / Runner / Session / Memory / Tool / Skill / Cron / Channel / Artifact / Knowledge / CodeExecutor / Monitor / Telemetry / Evaluation

详见 [需求文档 §9](./daily-stock-analysis.md#9-与平台模块的依赖矩阵)。

---

## 8. 快速开始

### 8.1 开发环境：组织 / Agent / Team 种子（当前可用）

在平台源码仓库根目录，将场景组织树、13 个 Agent 与 7 个 Team 写入本地 SQLite：

```powershell
$env:KRATOS_HTTP_AUTH_DISABLED = "1"
$env:DEPLOY_ENV = "dev"
$env:ARANEA_SQLITE_PATH = "data/arenea.sqlite"

# 写入组织 + Agent + Team（写入前请停止 go run ./cmd/admin）
go run ./cmd/seed-stockx-org
```

完整参数、幂等策略与写入清单见 **[cmd/seed-stockx-org/README.md](../../cmd/seed-stockx-org/README.md)**。

验证：Web UI → 组织树出现「Stockx AI 投研」→ Agents / Teams 页可见对应条目 → Chat 选择 `team-stock-deep-dive` 联调。

### 8.2 设计目标（v1.0 发布后）

> 以下为 v1.0 发布后的目标体验；Tool / Skill / Cron 等按 [开发计划](./daily-stock-analysis-development.md) 推进。

```bash
# 1. 一键启动
docker compose -f docker-compose.stockx.yml up -d

# 2. 配置场景（须已配置 data.postgres.source）
echo "STOCK_SCENARIO_ENABLED=1" >> .env
echo "STOCK_SCENARIO_FEISHU_WEBHOOK=https://open.feishu.cn/open-apis/bot/v2/hook/xxxx" >> .env

# 3. 打开 Web UI
open http://localhost:5173

# 4. 在 Watchlist 页加入自选股 → 在 Chat 中选「个股深度分析」团队 → 输入「分析下贵州茅台」
```

---

## 9. 里程碑路线图（预计 9 周到 v1.0）

```
Phase 1  数据底座        ──► M-S1   (2 周)
Phase 2  Agent/Team      ──┐
Phase 3  Service/前端    ──┤
Phase 4  Cron/Channel    ──┴──► M-S2 MVP        (5 周) ──► v0.1.0-alpha
Phase 5  KB/可观测       ──────► M-S3 场景闭环  (7 周) ──► v0.5.0-beta
Phase 6  Graph 高级编排  ──────► M-S4           (8 周) ──► v0.9.0-rc
Phase 7  评估/安全/文档  ──────► M-S5 开源发布  (9 周) ──► v1.0.0
Phase 8  生态扩展        ──────► 持续迭代              ──► v1.x
```

详见 [开发计划 §6 里程碑](./daily-stock-analysis-development.md#6-里程碑)。

---

## 10. 关键约束

| 约束 | 含义 |
|------|------|
| **不修改平台核心** | 所有新增能力通过 Tool / Skill / Agent / Team / Cron / Knowledge 等标准接口 |
| **不提供投资建议** | 报告 footer 强制免责；不内置任何下单 / 券商接口 |
| **不上传用户数据** | 持仓、API Key、报告索引存本地 **PostgreSQL `stockx`** + Artifact；平台元数据在 SQLite |
| **开源协议** | 建议 Apache-2.0；商业部署用户自行承担三方数据 TOS 合规 |
| **可关闭** | `STOCK_SCENARIO_ENABLED=0` 时整个场景静默，不注册任何资源 |

---

## 11. 贡献与协作

- **问题反馈 / 功能讨论**：建议在主仓库 Issue 区使用 `[scenario:stockx]` 前缀
- **PR 规范**：遵循平台 [`AI-DEVELOPMENT-SPECIFICATION.md`](../../guides/AI-DEVELOPMENT-SPECIFICATION.md)；本场景代码集中在 `internal/tools/stockdata/`、`internal/scenario/stockanalysis/`、`internal/biz/stock_*`、`web/src/{pages,features,components}/stockx/`
- **金融领域贡献**：欢迎补充 Skill 包（如行业链、报告模板）；建议 PR 时附领域参考
- **数据源贡献**：欢迎新增 Provider；必须实现 `Provider` 接口并补契约测试

---

## 12. 法律与合规声明

- 本项目为**开源软件**，作者 / 维护者不对使用本工具产生的任何投资决策与损失负责
- 接入第三方数据源（AKShare / Tushare / yfinance / 雪球 / 东方财富 等）需遵守对应服务条款；商业部署需自行获得授权
- 本工具不构成**投资建议**；所有报告默认包含「仅供学习研究」声明
- 用户对自己的 API Key、cookies、持仓数据负责，建议本地化存储

---

## 13. 相关链接

- [Aranea-Agents 主仓库](../../README.md)
- [平台架构总览](../../需求/0%20系统框图.md)
- [Multi-Agent / Team 模块](../../需求/11%20multi-agent.md)
- [Graph 工作流模块](../../需求/36%20graph-workflow.md)
- [Tools 工具体系](../../需求/23%20tools.md)
- [Skill 技能体系](../../需求/20%20skill.md)
- [Cron 定时模块](../../需求/21%20cron.md)
- [Channel 渠道模块](../../需求/17%20channel.md)

