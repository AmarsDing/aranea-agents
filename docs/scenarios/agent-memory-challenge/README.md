# Agent Memory Challenge 2026 — 参赛材料

> **赛事**：首届 Agent 记忆挑战赛（Agent Memory Leaderboard 首期公开评测）
> **官网**：https://agentmemories.ai/competition/ · 规则：https://agentmemories.ai/rules
> **提交截止**：2026-08-07 23:59（UTC+8） · **首次放榜**：2026 年 8 月中旬
> **参评系统**：Aranea-Agents Memory（基于 trpc-agent-go 的 L0–L4 五层记忆体系）

---

## 1. 参评决策（已确认）

| 维度 | 选择 | 说明 |
|------|------|------|
| 评测类型 | **文本记忆 + 代码记忆**（双榜） | 两个独立榜单，同一套 Add/Search 契约 |
| 参赛组别 | **学术方法榜** | 要求公开 GitHub 仓库 + 披露原始工作与方法改动 |
| 参赛路径 | **学术·代码（平台 Docker 部署）** | 提交公开仓库 + Docker 启动方式 + Add/Search 封装 + 运行说明；平台构建评测，**不签发 Eval Key** |
| 参评仓库 | https://github.com/AmarsDing/aranea-agents | ⏳ 待确认 public 状态与固定版本 tag |

## 2. 材料清单与状态

| # | 材料 | 对应文件 | 状态 |
|---|------|----------|------|
| M1 | 评测申请表材料（系统名称/版本/联系人/参评类型/方法说明/公开信息/提交说明） | [agent-memory-challenge.md](./agent-memory-challenge.md) §3 | ✅ 初稿（联系人等待填） |
| M2 | 方法/系统说明（学术披露：架构、原始工作引用、方法改动、维度映射） | [agent-memory-challenge.design.md](./agent-memory-challenge.design.md) §1–§4 | ✅ 初稿 |
| M3 | Add/Search API 封装方案（契约映射、样本隔离、鉴权、Docker 部署要求） | [agent-memory-challenge.design.md](./agent-memory-challenge.design.md) §5–§8 | ✅ 初稿 |
| M4 | 开发计划与提交 Checklist | [agent-memory-challenge.development.md](./agent-memory-challenge.development.md) | ✅ 初稿 |
| M5 | 仓库参赛 README 章节（运行说明、Docker 命令、API 入口） | 待写入仓库 README / `docs/` | ⏳ 待实施（T3） |
| M6 | Add/Search 适配层代码 | 待实施 | ⏳ 待实施（T1） |
| M7 | 固定版本 tag（如 `amc-2026.08`） | git tag | ⏳ 待实施（T6 前） |

## 3. 有效提交五条件（平台复核标准）

1. **材料完整**：参赛身份、系统版本和所需材料完整、真实、可核验
2. **Smoke 通过**：Add/Search、鉴权和端到端链路符合接入协议
3. **Full 完成**：正式评测任务全部成功完成，无缺失或重复结果
4. **版本一致**：正式评测使用的代码/镜像与申报版本一致（Full 受理后版本冻结）
5. **复核通过**：版本、结果与合规状态通过主办方审核

## 4. 参赛红线（违反即取消）

| # | 红线 | 我们的合规设计 |
|---|------|---------------|
| R1 | Search 只返回记忆证据，不得生成最终答案 | Search 适配层只返回记忆条目 `{id, content, score, timestamp}`，无 LLM 生成环节 |
| R2 | 样本隔离：禁止跨 user_id/任务/样本共享检索 | user_id 直接映射内部 `scope=user` 隔离边界，检索强制带 user_id 谓词 |
| R3 | 披露来源与改动：复用论文/仓库/代码必须注明 | design.md §3 列出 trpc-agent-go、mem0、pgvector 等全部引用与改动 |
| R4 | 禁止硬编码/数据泄漏/提示词注入/人工答题/刷榜 | 无私有数据预置；评测数据全部经 Add 接口写入 |

## 5. 关键开放问题（阻塞项置顶）

| # | 问题 | 影响 | 状态 |
|---|------|------|------|
| Q1 | ~~仓库是否已 public？~~ | 已确认 public ✅ | ✅ |
| Q2 | ~~联系人 / 机构团队信息~~ | 丁升 / dingsheng88888888@126.com / 个人开发者 | ✅ |
| Q3 | 平台 Docker 环境能否访问外部 Embedding API（OpenAI 兼容端点） | 决定语义检索是否可用；降级方案为关键词检索 | 📋 已在运行说明中按"可配置"设计，Smoke 阶段实测 |
| Q4 | 数据库形态：单容器 SQLite 降级 vs compose Postgres+pgvector | 影响 Docker 运行说明 | 📋 方案已双写，T2 验证后定稿 |

## 6. 时间线

| 节点 | 时间 | 动作 |
|------|------|------|
| 提交截止 | **2026-08-07 23:59（UTC+8）** | 提交评测申请 + 完整材料（仓库、Docker、API 封装、运行说明） |
| 审核/构建 | 主办方排期 | 平台按 Docker 说明构建 |
| Smoke | 主办方排期 | 公开小子集接口预检（可修复后重验） |
| Full | 主办方排期 | 当期一次正式评测，受理后版本冻结 |
| 放榜 | 2026 年 8 月中旬 | 学术方法榜 / 商业产品榜分别发布 |
