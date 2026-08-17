# Agent Memory Challenge 2026 — 参赛需求

> **场景索引**：[README.md](./README.md) · **设计**：[agent-memory-challenge.design.md](./agent-memory-challenge.design.md) · **计划**：[agent-memory-challenge.development.md](./agent-memory-challenge.development.md)
> **规则原文**：https://agentmemories.ai/rules · https://agentmemories.ai/competition/

---

## 1. 赛事规则摘要（事实层，来自官网）

### 1.1 基本事实

| 项 | 内容 |
|----|------|
| 赛事名称 | Agent Memory Challenge 2026（首届 Agent 记忆挑战赛） |
| 主办 | Agent Memory Leaderboard 团队；牛津大学、南洋理工大学、清华大学、东南大学等 20 余所高校与研究机构联合主办 |
| 费用 | 免费；参赛方承担自身 API、数据库、带宽和计算成本，平台承担统一 Answer、Eval 与评测编排成本 |
| 报名开放 | 2026-07-29 |
| 提交截止 | **2026-08-07 23:59（UTC+8）** |
| 首次放榜 | 2026 年 8 月中旬 |
| 责任边界 | 参赛系统只负责 **Add（写入记忆）** 与 **Search（检索证据）**；平台统一 **Answer（生成答案）** 与 **Eval（评分复核）** |

### 1.2 评测类型与数据规模

| 类型 | 评测内容 | 数据规模 |
|------|----------|----------|
| 文本记忆 | 事实召回、多跳整合、时序理解、记忆治理、个性化、规则执行、安全与隐私 | 1,559 公开子集对话、≈150M 字符长程记忆、≈5K 多维能力问题 |
| 代码记忆 | 从历史工程任务中检索、筛选并复用调试经验、开发经验和项目上下文 | 344 个真实 Base Task、12 个真实 GitHub 仓库、2,987 个标注相关任务（强相关 555 / 弱相关 514 / 无关 1,918） |

评测维度（文本榜）：**A** 显式事实召回 · **B** 关系与多跳组合 · **C** 时间与事件序列 · **D** 记忆治理（更新/冲突消解/删除/遗忘） · **E** 个性化与关怀 · **G** 规则与流程执行 · **H** 认识论安全与隐私。

### 1.3 参赛路径（三选一）

| 路径 | 要求 | 凭证 |
|------|------|------|
| 学术·API（自行部署） | 公开仓库 + 固定版本 + 公网 Add/Search 地址 + 鉴权 + 运行说明 | 审核后发 Eval Key |
| **学术·代码（平台部署）✅ 我方选择** | 公开仓库 + Docker 启动方式 + Add/Search 封装 + 完整运行说明 | 不签发 Eval Key，平台构建 |
| 商业·API | 固定产品版本 + Add/Search API + 鉴权 + 容量说明；接口 ≥30 天稳定 | 审核后发 Eval Key |

### 1.4 流程与版本纪律

1. 提交评测申请（选择类型、组别、提交方式 + 系统版本 + 完整材料）
2. 完成接入（平台按 Docker 说明构建）
3. Smoke（固定公开小子集预检，可按错误信息修正重验）→ Full（**一个 Key 当期仅一次 Full，受理后版本冻结**）
4. 复核（版本、结果、合规）→ 公榜

排名综合公开与私有评测结果；私有题目和答案不向参赛方开放。

### 1.5 参赛基本要求（红线）

1. **只返回记忆证据**：Search 不得直接生成最终答案，也不得把答案伪装为记忆记录
2. **保持样本隔离**：不得跨 user_id、任务、样本或团队共享和检索评测记忆
3. **披露来源与改动**：复用论文、仓库或代码时，必须注明原作者、技术报告和全部方法改动
4. **不得操纵评测**：严禁硬编码、数据泄漏、提示词注入、人工实时答题、结果操纵和恶意刷榜

### 1.6 奖励

- 学术方法榜 Top 10：1–3 名 ChatGPT Pro 月度会员；4–10 名 ChatGPT Plus 月度会员（赛事方独立采购发放）
- 社区贡献计划（满足任一：前 50 位有效提交 / 邀请 3 位有效参赛 / 提交 3 组挑战性样本并通过审核）：≥ ¥50 Kimi Token 额度

---

## 2. 我方参评需求

### 2.1 目标

| ID | 需求 | 优先级 |
|----|------|--------|
| AMC-N1 | 2026-08-07 23:59（UTC+8）前完成评测申请提交，材料完整可核验 | P0 |
| AMC-N2 | 参评文本记忆榜（学术方法榜，代码提交/平台 Docker 部署路径） | P0 |
| AMC-N3 | 参评代码记忆榜（同组别同路径，复用同一 Add/Search 封装） | P1 |
| AMC-N4 | Add/Search 适配层满足平台契约，Smoke 自测通过 | P0 |
| AMC-N5 | 完整学术披露：原始工作引用 + 方法改动清单 + 运行复现说明 | P0 |
| AMC-N6 | 全程合规：样本隔离、Search 不生成答案、版本固定可追溯 | P0 |

### 2.2 参评载体

- **系统**：Aranea-Agents Memory（Aranea-Agents 平台的记忆子系统独立参评）
- **核心能力**：L0–L4 五层记忆体系（上下文窗口 / 工作记忆 / 情景记忆 / 语义事实 / 图谱与进化）
- **仓库**：https://github.com/AmarsDing/aranea-agents （public ✅ 已确认）
- **固定版本**：重评版本 tag `amc-2026.08-r2`（commit `e252ff09e`，跨会话长任务记忆收尾 + 契约自测全绿；Full 受理后冻结）。首评版本 tag `amc-2026.08`（commit `ab2468ad3`）为历史快照，保留不删（平台冻结纪律）

---

## 3. 评测申请表材料（M1）

> 以下内容为提交申请表时的填写口径。标 ⚠️ 的字段需用户确认后填入。

### 3.1 共同材料（所有参赛者必填）

| 字段 | 填写内容 |
|------|----------|
| 系统名称 | Aranea-Agents Memory |
| 系统版本 | `amc-2026.08-r2`（git tag，指向 commit `e252ff09e`；Full 受理后冻结；首评版本 `amc-2026.08` → `ab2468ad3`） |
| 联系人 | 丁升 / dingsheng88888888@126.com / 13521757871 |
| 机构或团队 | 个人开发者（Independent Developer） |
| 拟参评类型 | 文本记忆（Textual Memory）+ 代码记忆（Coding Memory） |
| 参赛组别 | 学术方法榜（Academic Methods） |
| 提交方式 | 代码提交，由平台 Docker 部署（Academic · Code） |
| 方法或产品说明 | 见 §3.2 摘要（完整版见仓库参赛文档） |
| 允许公开展示的信息 | 系统名称、版本、方法描述、架构图、评测成绩、GitHub 仓库地址 |
| 完整提交说明 | 见 §3.3 |

### 3.2 方法说明摘要（申请表用，≤500 字）

> Aranea-Agents Memory 是一个面向 LLM Agent 的五层长期记忆系统，构建于开源 Agent 运行时 trpc-agent-go 之上。系统将认知科学记忆分层模型产品化为 L0–L4 五层：L0 感觉层（上下文窗口组装与压缩）、L1 工作记忆（任务状态板）、L2 情景记忆（会话事件时间线 + 向量召回）、L3 语义记忆（事实/偏好/规则，向量 + 关键词混合评分召回）、L4 持久层（实体关系图谱与自我进化）。
>
> 针对评测维度的核心机制：事实召回（A）由 L3 混合评分召回承担（向量余弦 + Postgres 全文检索 + 时间衰减 + 置信度强化）；多跳组合（B）由 L4 图谱路径与跨层融合承担；时序理解（C）由 L2 情景时间线与 turn 索引承担；记忆治理（D）由冲突检测、反驳/确认、版本回滚、级联删除与全局衰减模型承担；个性化（E）由 user 作用域偏好事实与业务化置信度模型承担；安全隐私（H）由 PII 扫描脱敏、scope 五级作用域隔离与全量审计承担。Search 接口仅返回记忆条目证据，不做任何答案生成；user_id 为唯一检索隔离边界。
>
> 系统以 docker-compose（Postgres + pgvector + 评测适配层）运行，Add/Search 契约适配层以独立 HTTP 端口暴露；未配置 Embedding API 时自动降级为关键词混合召回。

### 3.3 提交说明（Submission Notes）

1. **仓库**：https://github.com/AmarsDing/aranea-agents ，固定 tag `amc-2026.08-r2`（首评 tag `amc-2026.08` 保留不删）
2. **构建**：`docker compose -f docker-compose.eval.yml build`（根 Dockerfile，Go 1.23 多阶段构建，自动包含 `memoryeval` 评测二进制）
3. **运行**：`docker compose -f docker-compose.eval.yml up -d`（app + pgvector 两服务）；评测端点 `http://<host>:8910`，健康检查 `GET /healthz`
4. **Add/Search 封装**：`cmd/memoryeval/` 独立 HTTP 适配层（主程序零修改），契约细节见 [design.md §5](./agent-memory-challenge.design.md#5-addsearch-api-封装方案)
5. **外部依赖**：语义向量检索依赖 OpenAI 兼容 Embedding API（环境变量配置 base_url/api_key/model/dim）；未配置时自动降级为关键词混合召回，Add/Search 契约保持可用
6. **数据库**：Postgres + pgvector（compose 内置 `pgvector/pgvector:pg16` 服务，首次启动自动完成 schema 迁移）
7. **鉴权**：适配层启用 Bearer Token（Memory System Key，环境变量 `EVAL_MEMORY_TOKEN` 注入），Key 通过申请表安全渠道提交，不在仓库出现
8. **容量与限制**：compose 默认配置支持评测规模（≈150M 字符写入）；Add 为同步确认（含幂等 upsert，平台重试安全），Search 同步返回；详见 design.md §7

### 3.4 学术方法补充材料（学术榜必填）

| 字段 | 内容 |
|------|------|
| 公开仓库 | https://github.com/AmarsDing/aranea-agents |
| README | 仓库根 README.md + 参赛章节（T3 任务补充） |
| Docker 命令 | 见 §3.3-2/3 |
| API 入口 | 适配层 `POST /v1/memory/add`、`POST /v1/memory/search`（见 design.md §5） |
| 依赖配置 | Go 1.23（Docker 多阶段构建内固定）；运行时外部依赖仅 Embedding API（可选降级） |
| 原始工作引用 | trpc-agent-go（Tencent，Agent 运行时内核）；mem0（记忆管理范式参考）；pgvector（向量检索）；认知分层记忆理论（见 design.md §3） |
| 方法改动 | 见 design.md §4（五层模型、混合评分召回、治理机制等全部改动清单） |
| 运行步骤 | 见仓库 README 参赛章节（T3） |

### 3.5 重评版本（r2）申请口径

> 背景：首评（`amc-2026.08` / `ab2468ad3`，申请编号 `request_c2ed746fa6484454b1b9`）已放榜，综合 24.56（49/50），各维度远低于 SQLite-FTS 基线，疑似平台侧构建/集成故障（如无外网 Embedding 致召回为空），非算法差距。平台规则：一个 Key 当期仅一次 Full、Full 配额每 3 个月 1 次、受理后版本冻结；已提交仓库/tag/commit 不得删除替换，**新增 tag 属安全增量**。故以新增 tag `amc-2026.08-r2` 走「提交评测申请」表单申请新版本评测。

**版本信息**

| 字段 | 内容 |
|------|------|
| 系统版本 | `amc-2026.08-r2`（git tag → commit `e252ff09e`，已推送 origin） |
| 相对首评增量 | 跨会话长任务记忆增强 + 稳定性优化（下表清单） |
| 接口契约 | 与首评完全一致（Add/Search 适配层零行为变更，Smoke 7 项自测全绿） |
| Docker 形态 | 不变（`docker-compose.eval.yml`，app + pgvector 双服务） |

**r2 相对首评（ab2468ad3 → e252ff09e）改动清单**

1. **跨会话长任务记忆**：TaskBoard 任务状态表注入 L1 工作记忆（`memory_l1_tasks.metadata_json["task_board"]` 契约：status/done/next/blockers 四要素，全空不渲染）
2. **压缩契约 v4**：会话压缩摘要双段化——叙事段 + 结构化 `task_state` JSON 段（`ExtractTaskState` 拆段、`session_summaries.task_state_json` 持久化、快照注入时结构化状态先于叙事渲染、压缩产出回写 L1 task_board 闭环）
3. **deferred 工具热替换**：不可变 `deferredView` 经 atomic.Pointer 整体换面（vendored 框架零改动）
4. **稳定性/兼容性修复**：twin openapi 新增 `PUT /api/v1/graphs/{id}` 原位更新；工具别名与确认校验、SSRF 防护、事件与连接鉴权等修复（见 tag 附注与各 commit message）
5. **验证**：`go build ./cmd/... ./internal/...` 通过；受影响包测试全绿（含 PG 集成）；`test/agent-memory-challenge/smoke.sh` 7 项全绿（healthz/401/400/双用户隔离/幂等重放/空 scope 返回 `[]`）

**提交说明（申请表"提交说明"栏口径）**

1. 本次为首评 `request_c2ed746fa6484454b1b9` 的**新版本评测申请**（同一系统、同一仓库、同一接口契约），参评版本从 `amc-2026.08`（`ab2468ad3`）更新至 `amc-2026.08-r2`（`e252ff09e`），首评版本 tag/commit 全部保留未动
2. 首评结果（综合 24.56，A17.3/B17.6/C8.9/D18.5/E51.5/G29.0/H39.4）各维度远低于仓库内置关键词降级路径的本地自测表现，怀疑首评评测环境存在构建或集成故障（例如平台构建环境无外网 Embedding 且降级路径未被触发，导致召回为空）。**恳请随受理回执提供首评的错误摘要/构建日志**，以便核对是否为部署侧问题
3. 运行说明与首评一致：`docker compose -f docker-compose.eval.yml build && up -d`，评测端点 `:8910`；Embedding API 可选（未配置时自动降级为关键词混合召回，契约保持可用）
4. r2 版本内存域能力增量为「跨会话长任务记忆」（结构化任务状态注入工作记忆 + 压缩摘要双段化），对评测维度 B（多跳组合）/C（时序理解）/G（规则流程执行）有正向影响

---

## 4. 验收标准

| # | 标准 | 验证方式 |
|---|------|----------|
| AC1 | 申请表全部字段可填写、无占位符遗留 | 提交前对照 §3 检查 |
| AC2 | 仓库 public 可访问，tag `amc-2026.08-r2` 指向固定 commit（首评 tag `amc-2026.08` 保留不删） | 匿名窗口访问 + `git ls-remote` |
| AC3 | Docker 从干净环境按 README 一条命令构建成功 | T2 验证（新机器/清理缓存构建） |
| AC4 | 容器启动后 Add/Search smoke 自测通过（本目录 test 脚本） | T5 自测脚本全绿 |
| AC5 | 学术披露完整：引用与改动清单无遗漏 | 对照 design.md §3/§4 与代码依赖清单 |
| AC6 | 无任何 Key/凭证出现在仓库与材料中 | 全局 grep 检查 |
