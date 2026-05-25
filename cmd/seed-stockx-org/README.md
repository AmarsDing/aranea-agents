# Daily Stock Analysis — 组织 / Agent / Team 种子数据

> 场景代号：`daily_stock_analysis` / 命名空间：`stockx`  
> 对应文档：[docs/scenarios/daily_stock_analysis/README.md](../../docs/scenarios/daily_stock_analysis/README.md)

本 CLI 将 **Daily Stock Analysis** 场景的组织架构、Agent 员工与 Team 编排一次性写入 SQLite，供 Web UI 直接使用（组织树、Agent 设置、Teams 页面）。

---

## 前置条件

1. **数据库路径**：默认使用仓库根目录 `data/arenea.sqlite`（与 `configs/config.yaml` 一致）；若不存在则尝试 `cmd/data/arenea.sqlite`。
2. **停止占用 SQLite 的进程**：Windows 上请先停止 `go run ./cmd/admin`，否则可能因文件锁导致写入失败。
3. **认证环境变量**（本地 CLI 必需）：

```powershell
$env:KRATOS_HTTP_AUTH_DISABLED = "1"
$env:DEPLOY_ENV = "dev"
```

4. **（可选）指定数据库**：

```powershell
$env:ARANEA_SQLITE_PATH = "data/arenea.sqlite"
```

5. **至少有一个已启用的 LLM Provider/Model**（用于 Agent 的 `provider` / `model`）；未设置环境变量时，脚本会从 `llm_provider_models` 表自动选取。

---

## 快速开始

在仓库根目录执行：

```powershell
# 完整种子：行业 + 部门 + 岗位 + 13 Agent + 7 Team
go run ./cmd/seed-stockx-org

# 预览计划（不写库）
go run ./cmd/seed-stockx-org --dry-run

# Agent / Team 已存在时，刷新配置
go run ./cmd/seed-stockx-org --update

# 仅创建/更新 Team（要求 Agent 已存在）
go run ./cmd/seed-stockx-org --teams-only
go run ./cmd/seed-stockx-org --teams-only --update
```

Linux / macOS 将 `go run` 前的 PowerShell 环境变量改为：

```bash
export KRATOS_HTTP_AUTH_DISABLED=1
export DEPLOY_ENV=dev
export ARANEA_SQLITE_PATH=data/arenea.sqlite
go run ./cmd/seed-stockx-org
```

---

## 命令行参数

| 参数 | 说明 |
|------|------|
| `--dry-run` | 打印将创建的组织 / Agent / Team 计划，不写入数据库 |
| `--update` | 已存在的 Agent、Team、岗位角色配置会被更新（默认跳过已存在项） |
| `--teams-only` | 只 seed Team；跳过组织树与 Agent（Agent 必须已入库） |

---

## 环境变量

| 变量 | 说明 |
|------|------|
| `ARANEA_SQLITE_PATH` | SQLite 文件路径（可带 `file:` 前缀） |
| `KRATOS_HTTP_AUTH_DISABLED` | 本地 CLI 设为 `1` 跳过 JWT 校验 |
| `DEPLOY_ENV` | 与上项配合，设为 `dev` |
| `STOCKX_SEED_PROVIDER` | 覆盖默认 fast 档 Agent 的 provider |
| `STOCKX_SEED_MODEL` | 覆盖默认 fast 档 Agent 的 model |
| `STOCKX_SEED_STRONG_PROVIDER` | 覆盖主控/报告等 strong 档 provider |
| `STOCKX_SEED_STRONG_MODEL` | 覆盖 strong 档 model |

---

## 写入内容一览

### 1. 组织架构（`agent_category_nodes`）

| 层级 | Key | 名称 |
|------|-----|------|
| 行业 | `stockx-company` | Stockx AI 投研 |
| 部门 | `stockx-dept-coordination` | 调度管理部 |
| 部门 | `stockx-dept-data` | 数据采集部 |
| 部门 | `stockx-dept-research` | 多维分析部 |
| 部门 | `stockx-dept-output` | 报告输出部 |
| 岗位 ×13 | `stockx-pos-*` | 主控、评审、采集、各分析师、图表、报告等 |

岗位节点的 `config_json` / `metadata_json` 含角色元数据：`role_key`、`expected_agent_key`、`tools_allow`、`skills_allow` 等。

### 2. Agent 员工（13 个）

| Agent Key | 岗位 | 说明 |
|-----------|------|------|
| `agent-coordinator` | 主控调度员 | 任务拆分与成员调度 |
| `agent-critic` | 评审员 | 报告质量评审 |
| `agent-data-collector` | 数据采集员 | 行情/财务/资金/新闻归一化 |
| `agent-technical-analyst` | 技术分析师 | K 线、指标、趋势 |
| `agent-fundamental-analyst` | 基本面分析师 | 财报与估值 |
| `agent-money-flow-analyst` | 资金面分析师 | 北向、龙虎榜、主力 |
| `agent-news-analyst` | 消息面分析师 | 公告、研报、政策 |
| `agent-sentiment-analyst` | 情绪面分析师 | 雪球/股吧舆情 |
| `agent-industry-analyst` | 行业分析师 | 产业链、板块轮动 |
| `agent-risk-assessor` | 风险评估师 | 波动、回撤、集中度 |
| `agent-quant-factor` | 因子计算员 | 多因子计算 |
| `agent-chart-builder` | 图表构建员 | K 线/财务/热力图 |
| `agent-report-writer` | 报告撰写员 | Markdown / 飞书卡片汇总 |

每个 Agent 包含完整配置：

- **Prompt 文件**（约 10 个）：`IDENTITY.md`、`SOUL.md`、`CAPABILITIES.md`、`AGENTS_CORE.md`、`RULE.md` 等
- **Runtime Settings**：tools allow/deny、skills 白名单、`tools_profile`、memory 等
- **岗位绑定**：`category_position_id` 指向对应 `stockx-pos-*`

> Tools / Skills 名称已在配置中预置；实际工具与 Skill 包需按场景开发计划后续注册。

### 3. Team 编排（7 个）

| Team Key | 名称 | 模式 |
|----------|------|------|
| `team-premarket-brief` | 盘前简报团队 | coordinator |
| `team-stock-deep-dive` | 个股深度分析团队 | coordinator |
| `team-sector-rotation` | 板块扫描团队 | sequential |
| `team-portfolio-doctor` | 持仓诊断团队 | parallel |
| `team-market-recap` | 盘后复盘团队 | sequential |
| `team-research-pipeline` | 自定义研究流水线 | sequential |
| `team-deep-dive-critic` | 深度分析·评审精修 | critic_loop |

`definition_json` 为 v2 编排规格（`mode`、`members`、`synthesizer_agent_id`、嵌入式 Graph 等），默认 `runtime_engine: graph`。

---

## 验证

1. 重启或刷新 Web 前端。
2. **组织 / Agent 类型**：查看行业树 `Stockx AI 投研` 及下属部门、岗位与员工。
3. **Agents 页**：确认 13 个 `agent-*` 存在且文件 Tab 有内容。
4. **Teams 页**：确认 7 个 `team-*` 存在，成员与模式正确。
5. **Chat**：选择 `team-stock-deep-dive`，发送「分析贵州茅台」做联调（需 LLM 与后续 Tool 就绪）。

---

## 幂等与更新策略

- **组织 / 岗位**：按 `category_key` 去重，已存在则跳过（`--update` 时更新岗位 `role_config`）。
- **Agent**：按 `agent_key` 去重；`--update` 时重写 settings、prompt files、`config_json`。
- **Team**：按 `team_key` 去重；`--update` 时重写 `definition_json`。

重复执行 `go run ./cmd/seed-stockx-org`（无 `--update`）不会重复创建，只会跳过已存在记录。

---

## 源码结构

```
cmd/seed-stockx-org/
├── main.go          # CLI 入口、组织与 Agent 入库
├── agents_spec.go   # Agent / 岗位 / 行业定义与 runtime 配置
├── prompts.go       # 各角色 Prompt 文件内容
├── teams_spec.go    # 7 个 Team 编排定义
├── teams_seed.go    # Team 入库
└── README.md        # 本文档
```

---

## 常见问题

**Q: `database is locked`**  
A: 停止 `go run ./cmd/admin` 或其他占用同一 SQLite 的进程后重试。

**Q: `KRATOS_AUTH_SECRET is not set`**  
A: 设置 `KRATOS_HTTP_AUTH_DISABLED=1` 与 `DEPLOY_ENV=dev`。

**Q: `missing agent "agent-xxx"`（`--teams-only`）**  
A: 先执行完整 seed 或确保对应 Agent 已存在。

**Q: UI 看不到新数据**  
A: 确认 `ARANEA_SQLITE_PATH` 与 admin 服务使用的是同一库文件；刷新页面或重启 admin。

---

## 后续（未包含在本 CLI）

- Cron 定时任务（盘前/盘后 Cron）
- 27 个 stock 数据源 Tool 注册
- 13 个 Skill 包导入
- Graph 资产 `graph_stock_deep_dive` 独立持久化

以上按 [开发计划](../../docs/scenarios/daily_stock_analysis/daily-stock-analysis-development.md) 分 Phase 实施。
