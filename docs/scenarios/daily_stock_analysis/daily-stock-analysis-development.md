# Daily Stock Analysis — 开发计划

> **版本**：v1.0（2026-05-18）
> **需求**：[daily-stock-analysis.md](./daily-stock-analysis.md)
> **设计**：[daily-stock-analysis.design.md](./daily-stock-analysis.design.md)
> **执行基线**：[`guides/execution-plan.md`](../../guides/execution-plan.md)
> **场景代号**：`daily_stock_analysis`（代码命名空间 `stockx`）

---

## 0. 文档守则

| 项 | 内容 |
|----|------|
| **本文档写什么** | 模块状态矩阵、阶段计划、里程碑、任务清单、风险与依赖 |
| **本文档不写** | 用户故事（→ 需求）、实现细节与代码（→ 设计） |
| **进度真相** | 本文档跟踪「场景级」进度。涉及平台核心模块的进度仍以 `guides/execution-plan.md` 附录 A 为准 |
| **EP 编号** | 场景级任务前缀 `EP-STOCKX-XX`；与平台 EP-* 协同 |

---

## 1. 全局优先级

| 优先级 | 含义 |
|--------|------|
| **P0** | 阻塞 MVP，必须完成 |
| **P1** | 核心功能，影响主流程体验 |
| **P2** | 重要增强 |
| **P3** | 优化 / 远期 |

---

## 2. 模块状态速览

| # | 模块 | 目标状态 | 当前状态 | 依赖 |
|---|------|----------|----------|------|
| 1 | 数据源 Tool 层（行情/财务/资金/消息/情绪/行业） | ✅ 全量 | ❌ 未启动 | — |
| 2 | 指标计算（纯 Go） | ✅ | ❌ 未启动 | — |
| 3 | 图表渲染（CodeExecutor） | ✅ | ❌ 未启动 | EP-BIZ-04 |
| 4 | 交易日历 + 节假日 Skill | ✅ | ❌ 未启动 | — |
| 5 | 13 个 Skill 包（方法论 / 模板） | ✅ | ❌ 未启动 | — |
| 6 | 4 个 Knowledge Base（公司库 / 行业链 / 指数 / 术语） | ✅ | ❌ 未启动 | EP-KN-01/02 |
| 7 | 10+ Agent 配置 | ✅ | ❌ 未启动 | — |
| 8 | 6 个 Team 配置 | ✅ | ❌ 未启动 | — |
| 9 | 2 个 Graph 工作流 | ✅ | ❌ 未启动 | EP-BIZ-10 |
| 10 | 4 个 Cron 任务 | ✅ | ❌ 未启动 | EP-BIZ-09 |
| 11 | Channel 推送（飞书 + 邮件） | ✅ | ❌ 未启动 | EP-BIZ-08 |
| 12 | Stockx Service + 新增 5 张表 | ✅ | ❌ 未启动 | — |
| 13 | 前端 Watchlist / Detail / Reports 页 | ✅ | ❌ 未启动 | 12 |
| 14 | 安装包 install.go + 配置即代码 | ✅ | ❌ 未启动 | — |
| 15 | 部署（Docker Compose + Python Sidecar） | ✅ | ❌ 未启动 | — |
| 16 | 可观测（Grafana JSON + Metrics） | ✅ | ❌ 未启动 | 沿用平台 |
| 17 | 报告评估集（LLM-as-Judge） | ✅ | ❌ 未启动 | M17 |
| 18 | 文档（README / 安装 / 配置 / FAQ） | ✅ | ✅ 部分（需求/设计/开发计划完成） | — |

---

## 3. 平台前置依赖

| 平台 EP | 状态 | 本场景受影响项 | 兜底方案 |
|---------|------|----------------|----------|
| `EP-BIZ-04` CodeExecutor skill 路径走 Sandbox | 🟡 | 图表渲染、因子计算 | 本地图表 fallback（生成 SVG 字符串） |
| `EP-BIZ-08` Channel Webhook + 投递 | 🟡 | 飞书推送 | HTTP 直连 webhook 简易实现 |
| `EP-BIZ-09` Cron 调度引擎 | 🟡 | 4 个内置 Cron | 进程内 ticker 简易调度 |
| `EP-BIZ-10` Graph 执行引擎 | 🟡 | `team_stock_deep_dive` Graph 编排 | 退化为 coordinator 模式 |
| `EP-KN-01/02` Knowledge Embedder / 摄取 | 🟡 | 公司库 / 行业链 RAG | 本地内存索引 + 精确匹配 |
| `EP-CB-01` Callback 链（LLMAgent/Model） | 🟡 | 持仓脱敏 Plugin | Skill 内提示词软约束 |

> 任何**未达成**的平台依赖，必须在本场景对应任务上**显式标注降级方案**，确保即使平台模块滞后，场景 MVP 仍可独立运行。

---

## 4. 阶段计划

### Phase 1 — 数据底座（P0，目标 2 周）

**目标**：搭好 Tool / Skill / 节假日 / 缓存等底层能力，让任何 Agent 都能拉到稳定数据。

| # | 任务 | EP | 优先级 | 估时 | 验收 |
|---|------|-----|--------|------|------|
| 1.1 | 设计 `Provider` 接口 + Quote 模块骨架 | EP-STOCKX-01 | P0 | 2d | `internal/tools/stockdata/quote/` 单元测试通过 |
| 1.2 | AKShare Provider（行情、财务、资金、消息） | EP-STOCKX-02 | P0 | 5d | 离线 fixture 测试 + 在线冒烟 |
| 1.3 | yfinance Provider（港股 / 美股 行情） | EP-STOCKX-03 | P1 | 2d | 同上 |
| 1.4 | 指标计算（MA/MACD/KDJ/RSI/BOLL/ATR/OBV 纯 Go） | EP-STOCKX-04 | P0 | 3d | 与 talib 黄金对照 |
| 1.5 | 缓存层（quote / news；TTL；清理 Cron） | EP-STOCKX-05 | P0 | 2d | 命中率指标可观测 |
| 1.6 | 交易日历（A/H/U + 节假日加载） | EP-STOCKX-06 | P0 | 2d | 2024-2026 节假日正确 |
| 1.7 | 工具注册 + Tools 管理页可见 | EP-STOCKX-07 | P0 | 1d | 所有工具在 `/tools` 列出 |
| 1.8 | `stock_resolve` Tool（中文/拼音/简称匹配） | EP-STOCKX-08 | P0 | 2d | 100 样本召回 ≥ 95% |
| 1.9 | 工具单元 / 契约测试 | EP-STOCKX-09 | P0 | 3d | CI 通过 |

**出口标准**：
- 所有数据源 Tool 在 Tools 管理页可见、可调用、可禁用
- 在 Chat 中创建一个空白 Agent，挂载这些工具，能回答「茅台最近 30 天收盘价多少」「上周北向资金流入哪些股票」

---

### Phase 2 — Agent 与 Team 编排（P0，目标 1.5 周）

**目标**：让 6 个 Team 在 Chat 中可用，覆盖个股深度分析 + 盘前简报两大主线。

| # | 任务 | EP | 优先级 | 估时 | 验收 |
|---|------|-----|--------|------|------|
| 2.1 | 13 个 Skill 包编写 + 入仓 | EP-STOCKX-10 | P0 | 5d | `/skills` 全部可见，启用后 Agent 可引用 |
| 2.2 | 10+ Agent 定义 + system_prompt + 工具绑定 | EP-STOCKX-11 | P0 | 4d | `agents.yaml` 加载成功；Chat 中可单独对话 |
| 2.3 | `team_premarket_brief`（coordinator） | EP-STOCKX-12 | P0 | 2d | 输入 watchlist → 输出简报 Markdown |
| 2.4 | `team_stock_deep_dive`（coordinator 降级版） | EP-STOCKX-13 | P0 | 2d | 输入「600519」→ 输出深度报告 |
| 2.5 | `team_sector_rotation`（sequential） | EP-STOCKX-14 | P1 | 2d | 输出板块扫描报告 |
| 2.6 | `team_market_recap`（sequential） | EP-STOCKX-15 | P1 | 2d | 输出盘后复盘 |
| 2.7 | `team_portfolio_doctor`（parallel + synthesizer） | EP-STOCKX-16 | P2 | 3d | 输入持仓 JSON → 输出诊断 |
| 2.8 | 报告写作模板 + 飞书卡片 schema | EP-STOCKX-17 | P0 | 2d | 5 种报告类型卡片可渲染 |
| 2.9 | Critic Loop 二次精修（可选） | EP-STOCKX-18 | P2 | 2d | 报告评分 ≥ 阈值 |

**出口标准**：
- 6 个 Team 全部出现在 Chat 左侧列表
- 至少 `team_stock_deep_dive` + `team_premarket_brief` 端到端可用，包含数据引用 + 风险提示

---

### Phase 3 — 业务数据层与 Web UI（P0，目标 1.5 周）

**目标**：让用户能管理自选股、查看股票详情和历史报告。

| # | 任务 | EP | 优先级 | 估时 | 验收 |
|---|------|-----|--------|------|------|
| 3.1 | 5 张表 Ent Schema + 迁移 | EP-STOCKX-19 | P0 | 2d | `make wire && make build` 通过 |
| 3.2 | `WatchlistUsecase` / `StockReportUsecase` / `TradingCalendarUsecase` | EP-STOCKX-20 | P0 | 2d | 单元测试 |
| 3.3 | `api/kratos/stockx/v1/*.proto` + RPC 实现 | EP-STOCKX-21 | P0 | 2d | `make api` 通过；HTTP 路由可访问 |
| 3.4 | 前端 Watchlist 页（CRUD + 批量导入） | EP-STOCKX-22 | P0 | 3d | 增删改查 + CSV 导入 |
| 3.5 | 前端 Stock Detail 页（基础信息 + K 线 + 报告列表） | EP-STOCKX-23 | P1 | 3d | 价格刷新 + K 线渲染 |
| 3.6 | 前端 Reports 页（列表 + 筛选 + 详情抽屉） | EP-STOCKX-24 | P0 | 2d | 报告可查看可下载 |
| 3.7 | 路由 + 侧栏入口 + 暗色模式适配 | EP-STOCKX-25 | P0 | 1d | UX 一致 |
| 3.8 | 前端 i18n（zh-CN 基线） | EP-STOCKX-26 | P1 | 1d | 文案抽离 |

**出口标准**：
- 用户从零开始可在 Web UI 完成：登录 → 加入自选股 → 触发分析 → 看到报告
- 整套 Web 操作流可录屏 demo（5 分钟内完成）

---

### Phase 4 — 调度与渠道（P0，目标 1 周）

**目标**：4 个 Cron 在交易日自动跑通，结果推送到飞书。

| # | 任务 | EP | 优先级 | 估时 | 验收 |
|---|------|-----|--------|------|------|
| 4.1 | 4 个 Cron 任务安装（crons.yaml） | EP-STOCKX-27 | P0 | 1d | Cron 管理页可见 |
| 4.2 | Cron 节假日跳过逻辑 | EP-STOCKX-28 | P0 | 1d | 国庆节不触发 |
| 4.3 | 飞书 Channel 推送对接 + 卡片模板 5 套 | EP-STOCKX-29 | P0 | 3d | 收到富文本卡片 |
| 4.4 | 邮件 SMTP Channel | EP-STOCKX-30 | P1 | 2d | 邮件附件正常 |
| 4.5 | Cron 失败告警接入 Monitor | EP-STOCKX-31 | P1 | 1d | 连续 3 次失败有告警 |

**出口标准**：
- 4 个 Cron 在交易日自动触发；非交易日 skipped
- 收到飞书卡片 + 报告链接可点

---

### Phase 5 — 知识库与可观测（P1，目标 1 周）

**目标**：让 Agent 能检索行业链 / 公司库；上线监控仪表盘。

| # | 任务 | EP | 优先级 | 估时 | 验收 |
|---|------|-----|--------|------|------|
| 5.1 | `kb_listed_companies` 摄取（首次全量 + 增量） | EP-STOCKX-32 | P1 | 2d | 中文名 → symbol 召回 ≥ 95% |
| 5.2 | `kb_industry_chain`（首版静态 Markdown） | EP-STOCKX-33 | P1 | 2d | 行业 → 上下游股票可检索 |
| 5.3 | `kb_index_constituents` 摄取 | EP-STOCKX-34 | P2 | 1d | 沪深 300 成分股 |
| 5.4 | `kb_research_glossary` | EP-STOCKX-35 | P2 | 1d | 术语解释命中 |
| 5.5 | Grafana JSON `grafana-stockx.json` | EP-STOCKX-36 | P1 | 2d | 导入即用 |
| 5.6 | Prometheus 指标接入 | EP-STOCKX-37 | P1 | 1d | `/metrics` 可见 stockx_* 指标 |

**出口标准**：
- 用户在 Chat 中问「白酒龙头有哪些」→ Agent 能基于 KB 召回 + 个股数据回答
- Grafana 看板展示今日报告数、失败率、缓存命中率

---

### Phase 6 — Graph 工作流与高级编排（P1，目标 1 周）

**目标**：把 `team_stock_deep_dive` 升级为 Graph 模式；上线 `team_research_pipeline`。

| # | 任务 | EP | 优先级 | 估时 | 验收 |
|---|------|-----|--------|------|------|
| 6.1 | `graph_stock_deep_dive` JSON + handler 注册 | EP-STOCKX-38 | P1 | 3d | 5 个分析师并行；最终报告完整 |
| 6.2 | `graph_research_pipeline` 模板 + HITL 节点 | EP-STOCKX-39 | P2 | 3d | 用户可在 Graph 编辑器加载 |
| 6.3 | Graph 节点失败 fallback 策略（fan_out continue） | EP-STOCKX-40 | P1 | 1d | 缺失维度被显式标注 |
| 6.4 | Graph 运行轨迹可视化（沿用平台） | EP-STOCKX-41 | P2 | 1d | UX 复用 |

**出口标准**：
- 深度分析报告由 Graph 驱动，5 个分析师真正并行；前端可看到节点级流式事件

---

### Phase 7 — 评估、安全与文档（P1，目标 1 周）

| # | 任务 | EP | 优先级 | 估时 | 验收 |
|---|------|-----|--------|------|------|
| 7.1 | 报告评估集（20 用例）+ LLM-as-Judge | EP-STOCKX-42 | P2 | 3d | 集成到 M17 Evaluation |
| 7.2 | 持仓脱敏 Plugin（Before Model 钩子） | EP-STOCKX-43 | P1 | 2d | 持仓字段在 prompt 中被 mask |
| 7.3 | API Key 加密落库 | EP-STOCKX-44 | P1 | 1d | 列表页只显示遮罩值 |
| 7.4 | 数据源限流（每 Provider rate limiter） | EP-STOCKX-45 | P1 | 1d | 高并发不被封 IP |
| 7.5 | README / 安装指南 / FAQ / 视频 | EP-STOCKX-46 | P0 | 3d | 新用户 10 分钟启动 |
| 7.6 | Docker Compose + Python Sidecar 镜像 | EP-STOCKX-47 | P0 | 2d | `docker compose up` 即用 |
| 7.7 | 法律免责声明 / LICENSE 复审 | EP-STOCKX-48 | P0 | 1d | 报告含合规 footer |

**出口标准**：
- 任何外部用户可在 1 小时内完成自托管首跑
- 报告评估分数 ≥ 4/5（人工 + LLM-as-Judge 双重）

---

### Phase 8 — 优化与远期（P2/P3，持续）

| # | 任务 | EP | 优先级 | 估时 |
|---|------|-----|--------|------|
| 8.1 | 多因子模块（`agent_quant_factor` + factor_compute） | EP-STOCKX-50 | P2 | 5d |
| 8.2 | Backtrader 回测集成（CodeExecutor） | EP-STOCKX-51 | P3 | 5d |
| 8.3 | PDF 导出（wkhtmltopdf via CodeExecutor） | EP-STOCKX-52 | P2 | 2d |
| 8.4 | en-US 国际化（美股报告） | EP-STOCKX-53 | P2 | 3d |
| 8.5 | 微信 / Slack / Discord Channel | EP-STOCKX-54 | P3 | 各 2d |
| 8.6 | Watchlist 多分组管理 + 标签 | EP-STOCKX-55 | P2 | 2d |
| 8.7 | 智能选股策略库（动量 / 价值 / 质量） | EP-STOCKX-56 | P3 | 7d |
| 8.8 | 异常事件实时预警（重大公告 / 龙虎榜异动） | EP-STOCKX-57 | P3 | 5d |
| 8.9 | 接入 Ecosystem Marketplace 上架场景包 | EP-STOCKX-58 | P3 | 3d |

---

## 5. 里程碑

| 里程碑 | 内容 | 累计周期 | 出口标准 |
|--------|------|----------|----------|
| **M-S1：数据底座** | Phase 1 完成 | 2 周 | 工具单测 + 集成测试通过 |
| **M-S2：MVP 可用** | Phase 1-3 完成 + Phase 4 部分 | 5 周 | 个股深度分析 + 自选股管理 + 飞书推送可用 |
| **M-S3：场景闭环** | Phase 1-5 完成 | 7 周 | 4 个 Cron 上线 + KB 检索 + 监控仪表盘 |
| **M-S4：高级编排** | Phase 6 完成 | 8 周 | Graph 工作流可用 |
| **M-S5：开源发布** | Phase 7 完成 | 9 周 | 开源仓库 release v1.0 |
| **M-S6：生态扩展** | Phase 8 滚动迭代 | 持续 | 多因子 / 回测 / 多渠道 |

---

## 6. 关键路径与依赖图

```
Phase 1 (数据底座)
    │
    ├──► Phase 2 (Agent/Team)  ─────────────────┐
    │                                            │
    ├──► Phase 3 (Service/前端) ─────────────────┤
    │                                            │
    └──► Phase 4 (Cron/Channel) ─► M-S2 MVP ────┤
                                                 │
                  Phase 5 (KB/可观测) ───────────┤
                  Phase 6 (Graph) ───────────────┤── M-S5 Open Source v1.0
                  Phase 7 (评估/安全/文档) ─────┘
```

**关键路径**：Phase 1 → Phase 2 → Phase 3 → Phase 4（任何一个 Phase 延迟会直接顺延 MVP）。

---

## 7. 风险与缓解

| # | 风险 | 概率 | 影响 | 缓解 |
|---|------|------|------|------|
| R1 | AKShare 接口频繁变更，工具失效 | 高 | 高 | 多 Provider fallback；CI 中加 Provider 契约测试；维护 fixture |
| R2 | 平台 Cron / Channel / Graph 模块未完成 | 中 | 高 | 本场景兜底实现（ticker / 直连 webhook / coordinator 降级） |
| R3 | CodeExecutor Sandbox 未稳定 | 中 | 中 | 图表降级为内置 SVG；因子降级为纯 Go |
| R4 | 数据源被封禁 IP | 中 | 中 | 限流 + 重试 + 多源 + 用户自配代理 |
| R5 | Token 用量失控 | 中 | 中 | 默认 mini/flash 模型；Token 模块限额；上下文 isolated |
| R6 | 法律合规风险（投资建议） | 低 | 高 | 强制免责声明；不提供下单 |
| R7 | 性能不达标（深度分析 > 90s） | 中 | 中 | 缓存 + 并行 + Provider 优化；模型选小一些 |
| R8 | 跨市场（A/H/U）数据归一化复杂 | 高 | 中 | 统一 SymbolRef + Provider 内部适配；首版以 A 股为主 |
| R9 | 用户安装门槛高（Python Sidecar 依赖） | 中 | 中 | Docker 一键 + 文档清晰；提供云镜像选项 |
| R10 | 飞书 webhook 失败 | 低 | 中 | 报告仍落 Artifact；Cron 标 partial_success |

---

## 8. 任务清单（汇总）

| EP | 任务 | Phase | 优先级 | 状态 |
|----|------|-------|--------|------|
| EP-STOCKX-01 | Provider 接口骨架 | 1 | P0 | ⏳ |
| EP-STOCKX-02 | AKShare Provider | 1 | P0 | ⏳ |
| EP-STOCKX-03 | yfinance Provider | 1 | P1 | ⏳ |
| EP-STOCKX-04 | 指标计算（纯 Go） | 1 | P0 | ⏳ |
| EP-STOCKX-05 | 缓存层 | 1 | P0 | ⏳ |
| EP-STOCKX-06 | 交易日历 | 1 | P0 | ⏳ |
| EP-STOCKX-07 | Tool 注册 | 1 | P0 | ⏳ |
| EP-STOCKX-08 | stock_resolve | 1 | P0 | ⏳ |
| EP-STOCKX-09 | 单元/契约测试 | 1 | P0 | ⏳ |
| EP-STOCKX-10 | 13 个 Skill 包 | 2 | P0 | ⏳ |
| EP-STOCKX-11 | 10+ Agent 配置 | 2 | P0 | ⏳ |
| EP-STOCKX-12 | team_premarket_brief | 2 | P0 | ⏳ |
| EP-STOCKX-13 | team_stock_deep_dive (coordinator) | 2 | P0 | ⏳ |
| EP-STOCKX-14 | team_sector_rotation | 2 | P1 | ⏳ |
| EP-STOCKX-15 | team_market_recap | 2 | P1 | ⏳ |
| EP-STOCKX-16 | team_portfolio_doctor | 2 | P2 | ⏳ |
| EP-STOCKX-17 | 报告模板 + 飞书卡片 | 2 | P0 | ⏳ |
| EP-STOCKX-18 | Critic Loop | 2 | P2 | ⏳ |
| EP-STOCKX-19 | 5 张表 Schema | 3 | P0 | ⏳ |
| EP-STOCKX-20 | Stockx Usecases | 3 | P0 | ⏳ |
| EP-STOCKX-21 | stockx.proto + RPC | 3 | P0 | ⏳ |
| EP-STOCKX-22 | 前端 Watchlist 页 | 3 | P0 | ⏳ |
| EP-STOCKX-23 | 前端 Stock Detail 页 | 3 | P1 | ⏳ |
| EP-STOCKX-24 | 前端 Reports 页 | 3 | P0 | ⏳ |
| EP-STOCKX-25 | 路由 + 侧栏 + 暗色 | 3 | P0 | ⏳ |
| EP-STOCKX-26 | i18n zh-CN | 3 | P1 | ⏳ |
| EP-STOCKX-27 | 4 个 Cron 安装 | 4 | P0 | ⏳ |
| EP-STOCKX-28 | Cron 节假日跳过 | 4 | P0 | ⏳ |
| EP-STOCKX-29 | 飞书 Channel + 5 套卡片 | 4 | P0 | ⏳ |
| EP-STOCKX-30 | 邮件 SMTP Channel | 4 | P1 | ⏳ |
| EP-STOCKX-31 | Cron 告警接入 Monitor | 4 | P1 | ⏳ |
| EP-STOCKX-32 | kb_listed_companies 摄取 | 5 | P1 | ⏳ |
| EP-STOCKX-33 | kb_industry_chain | 5 | P1 | ⏳ |
| EP-STOCKX-34 | kb_index_constituents | 5 | P2 | ⏳ |
| EP-STOCKX-35 | kb_research_glossary | 5 | P2 | ⏳ |
| EP-STOCKX-36 | Grafana JSON | 5 | P1 | ⏳ |
| EP-STOCKX-37 | Prometheus 指标 | 5 | P1 | ⏳ |
| EP-STOCKX-38 | graph_stock_deep_dive | 6 | P1 | ⏳ |
| EP-STOCKX-39 | graph_research_pipeline | 6 | P2 | ⏳ |
| EP-STOCKX-40 | Graph fan_out continue | 6 | P1 | ⏳ |
| EP-STOCKX-41 | Graph 轨迹可视化 | 6 | P2 | ⏳ |
| EP-STOCKX-42 | 报告评估 LLM-as-Judge | 7 | P2 | ⏳ |
| EP-STOCKX-43 | 持仓脱敏 Plugin | 7 | P1 | ⏳ |
| EP-STOCKX-44 | API Key 加密 | 7 | P1 | ⏳ |
| EP-STOCKX-45 | 数据源限流 | 7 | P1 | ⏳ |
| EP-STOCKX-46 | README / 安装 / FAQ | 7 | P0 | ⏳ |
| EP-STOCKX-47 | Docker Compose + Sidecar | 7 | P0 | ⏳ |
| EP-STOCKX-48 | 法律 / LICENSE | 7 | P0 | ⏳ |
| EP-STOCKX-50 | 多因子模块 | 8 | P2 | ⏳ |
| EP-STOCKX-51 | Backtrader 集成 | 8 | P3 | ⏳ |
| EP-STOCKX-52 | PDF 导出 | 8 | P2 | ⏳ |
| EP-STOCKX-53 | en-US 国际化 | 8 | P2 | ⏳ |
| EP-STOCKX-54 | 微信/Slack/Discord | 8 | P3 | ⏳ |
| EP-STOCKX-55 | Watchlist 多分组 | 8 | P2 | ⏳ |
| EP-STOCKX-56 | 选股策略库 | 8 | P3 | ⏳ |
| EP-STOCKX-57 | 异常事件预警 | 8 | P3 | ⏳ |
| EP-STOCKX-58 | Marketplace 上架 | 8 | P3 | ⏳ |

> 状态图例：⏳ 待启动 | 🟡 进行中 | ✅ 完成 | ⚠️ 阻塞 | ❌ 取消

---

## 9. 验证与发布

### 9.1 验证矩阵

| 验证维度 | 目标 | 方法 |
|----------|------|------|
| 功能正确性 | 所有 P0 验收用例通过 | E2E 测试 + 手动验证 |
| 数据正确性 | 工具返回字段、指标值与权威源一致 | 黄金数据集对照 |
| 性能 | 见需求 §6.1 | 压测 / 真实场景测量 |
| 稳定性 | 连续运行 7 天 Cron 无未捕获错误 | 监控告警 |
| 报告质量 | LLM-as-Judge 分 ≥ 4/5 | M17 Evaluation |
| 文档完整性 | 新用户 1h 内启动 | 邀请外部测试者 |
| 安全 | API Key 加密、持仓脱敏 | 安全 review |

### 9.2 发布节奏

| 版本 | 内容 | 时机 |
|------|------|------|
| `v0.1.0-alpha` | M-S2 MVP（内部尝鲜） | Phase 4 完成后 |
| `v0.5.0-beta` | M-S3（接受社区反馈） | Phase 5 完成后 |
| `v0.9.0-rc` | M-S4（Graph 编排 + 评估） | Phase 6 完成后 |
| `v1.0.0` | M-S5（首个稳定开源版） | Phase 7 完成后 |
| `v1.x` | 滚动迭代（Phase 8 任务） | 持续 |

### 9.3 发布物清单

- [ ] 源码 + LICENSE（开源协议建议 Apache-2.0）
- [ ] Docker 镜像（admin / web / py-sandbox）
- [ ] `docker-compose.stockx.yml`
- [ ] 安装文档 + 5 分钟视频
- [ ] 示例配置（.env.example、watchlist.example.csv）
- [ ] Grafana 仪表盘 JSON
- [ ] FAQ + 故障排查指南
- [ ] CHANGELOG.md
- [ ] CONTRIBUTING.md
- [ ] 法律免责声明

---

## 10. 沟通与协作

| 角色 | 职责 |
|------|------|
| **场景负责人** | 把控全场景节奏、依赖协调、对外发布 |
| **后端 1（数据底座）** | Phase 1、Phase 5 KB |
| **后端 2（编排与服务）** | Phase 2、Phase 3、Phase 6 |
| **前端** | Phase 3 Web、Phase 8.x UI 增强 |
| **运维 / DevOps** | Phase 4 Channel、Phase 7 部署 |
| **金融领域顾问**（可选） | Skill 编写、报告模板审校、评估集设计 |
| **平台核心 owner** | 协调 EP-BIZ-04/08/09/10 与 KN / CB 等依赖 |

### 10.1 工作流约定

1. 每个 EP-STOCKX-XX 开 Issue + branch
2. PR 必跑：`make wire && make api && make build && make test && cd web && pnpm lint && pnpm test && pnpm build`
3. 涉及平台依赖时在 PR description 关联 `EP-*` 平台编号
4. 每周一同步状态到本文档 §2 与 §8

---

## 11. 与平台执行基线协同

| 平台 EP 状态变化 | 本场景响应 |
|------------------|------------|
| `EP-BIZ-09` Cron 调度引擎完成 | 移除本场景 ticker 兜底 |
| `EP-BIZ-10` Graph 执行引擎完成 | 启用 `graph_stock_deep_dive`，移除 coordinator 降级 |
| `EP-BIZ-08` Channel 投递完成 | 移除直连 webhook 简易实现 |
| `EP-KN-01/02` Knowledge 完成 | 启用 KB 检索，移除本地内存索引 |
| `EP-BIZ-04` CodeExecutor 完成 | 启用 mplfinance 图表，移除 SVG fallback |
| `EP-CB-01` Callback 完成 | 启用持仓脱敏 Plugin |

> 每当平台对应 EP 收敛，必须在本文档 §3「平台前置依赖」更新状态，并触发对应任务的迁移与测试。

---

## 12. 验收 Demo 脚本（M-S2 / v0.1.0-alpha）

> 用于评估 MVP 是否达成。

```
1. docker compose -f docker-compose.stockx.yml up -d
2. 打开 http://localhost:5173
3. 进入 Watchlist 页，批量导入：600519,000858,300750,002594,600036
4. 进入 Chat，选择团队「个股深度分析」，输入「分析下贵州茅台最近的走势」
5. 等待 ≤ 90s，观察：
   - 左侧成员级流式事件可见（technical / fundamental / news / sentiment）
   - 报告输出包含：TL;DR、技术面、基本面、资金面、消息面、风险评估
   - 报告底部含数据引用 + 风险提示
6. 进入 Reports 页，查看刚生成的报告
7. 进入 Cron 管理页，手动触发「盘前简报」
8. 等待 ≤ 5min，飞书群收到富文本卡片
9. 查看 Monitor 页，可见 Team Run 轨迹和 Tool Invocation 列表
```

通过该 Demo，则 M-S2 完成。

---

## 13. 参考

- [需求文档](./daily-stock-analysis.md)
- [设计文档](./daily-stock-analysis.design.md)
- [场景索引](./README.md)
- [平台执行基线](../../guides/execution-plan.md)
- [平台编码规范](../../guides/AI-DEVELOPMENT-SPECIFICATION.md)

