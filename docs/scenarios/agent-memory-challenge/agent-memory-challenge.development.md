# Agent Memory Challenge 2026 — 开发计划

> **需求**：[agent-memory-challenge.md](./agent-memory-challenge.md) · **设计**：[agent-memory-challenge.design.md](./agent-memory-challenge.design.md)
> **提交截止**：2026-08-07 23:59（UTC+8，Cycle 1）；**2026-10 起进行 Cycle 2**（版本 `amc-2026.09`，截止日期以官网为准）

---

## 1. 模块定位

本场景不是产品新功能，而是**参赛交付**：以最小改动在现有 L0–L4 记忆体系上桥接平台 Add/Search 契约，并完成学术披露与提交。适配层代码随仓库开源，参评版本打固定 tag。

## 2. 代码锚点

| 锚点 | 路径 | 说明 |
|------|------|------|
| 评测适配层（新增） | `cmd/memoryeval/main.go`、`handler.go`、`handler_test.go` | 独立 HTTP 入口，Add/Search 契约桥接，主程序零修改 |
| 评测窄端口（新增） | `internal/biz/memory_eval.go`、`internal/data/memory_eval_store.go` | `EvalMemoryStore` → L3 UpsertFactRow / RecallL3Facts |
| Cycle 3 评测写路径增强（T19） | `internal/biz/fact_kind.go`、`memory_eval_classify.go`（SplitEvalStatements 原子化）、`memory_eval_time.go`（中英相对时间解析）、`memory_eval_query.go`（AnalyzeEvalQuery 查询模式）、`internal/data/memory_eval_store.go`/`memory_eval_store_part2.go` | 原子化入库、时序注解、治理扩展（constraint 拓宽/新槽位/否定 cue/向量矛盾失效）、L2 全量分片、证据掩码+溯源前缀、查询模式（Chronological/Range/Aggregate） |
| Cycle 3 召回结构（T19） | `internal/data/memory_l3_trgm.go`（scope 先行 MATERIALIZED CTE）、`memory_l3_fts.go`（CJK bigram 通道）、`sql/migrations/20261292_memory_facts_cjk_bigram_fts.sql`、`ddl_migration_table_late.go` | trgm 10-20 风暴治本；CJK FTS 通道（bigram_text IMMUTABLE SQL fn + generated column + GIN） |
| Cycle 3 embedding 补偿（T19） | `internal/data/memory_fact_index_sweeper.go` | stale/pending/failed 行指数退避清扫重放 SyncFactIndexFromRow（cmd/memoryeval 接线） |
| 记忆写入 | `internal/biz/memory.go`（MemoryUsecase）、`internal/biz/memory_consolidator.go` | L3 事实提取/写入 |
| 记忆召回 | `internal/biz/memory_admin_store.go`（RecallL2Episodes/RecallL3Facts）、`internal/data/memory_shim_l2.go`、`memory_shim_l3.go` | 混合评分召回 |
| Embedding | `internal/knowledge`（MultiProviderEmbedder，OpenAI 兼容端点） | 经 `singleEmbedder` 适配 `biz.EmbeddingService` |
| 部署 | `Dockerfile`（根）、`docker-compose.eval.yml` | compose 双服务（app + pgvector）唯一形态 |
| 现状梳理 | `docs/development/memory/memory.md`、`memory.design.md`、`memory-development.md` | L0–L4 完成功能真相源 |

## 3. 任务清单

| # | 任务 | 优先级 | 依赖 | 状态 |
|---|------|--------|------|------|
| T0 | 确认仓库 public 可匿名访问（https://github.com/AmarsDing/aranea-agents） | P0 阻塞 | 用户 | ✅ 已确认 public |
| T1 | 实现 Add/Search 适配层：**独立入口 `cmd/memoryeval/`（主程序零修改）** + biz 窄接口 `EvalMemoryStore` + Bearer 鉴权；17 个契约测试通过 | P0 | 无 | ✅ 2026-08-06 |
| T2 | Docker 形态验证：~~①SQLite 单容器~~（`NewData` 硬编码 Postgres，不可行，已定论）；②compose（pgvector）双服务形态 — 配置就绪（`docker-compose.eval.yml`，镜像以 `GO_BUILD_TAGS=pgvector` 构建）；本地等价验证：全新 aranea_eval 库直跑二进制 + smoke 7 项全绿（本机无 Docker，实机构建验证留待平台 Smoke 阶段） | P0 | T1 | ✅ 本地等价验证通过 |
| T3 | 仓库 README 增加「Agent Memory Challenge」章节：构建/启动/健康检查/curl 示例/依赖与降级说明 | P0 | T2 | ✅ 2026-08-06（README §Agent Memory Challenge 2026） |
| T4 | 仓库内发布学术披露页（引用 + 方法改动，引用 design.md §3/§4 内容） | P0 | 无 | ✅（design.md §3/§4 随仓库发布，README 已链接） |
| T5 | Smoke 自测脚本：模拟平台 Add（多 user 多会话）→ Search 断言召回与隔离 → 输出报告 | P0 | T1 | ✅ 2026-08-07 本地 aranea_eval 全新库实跑 7 项全绿（含 user 隔离与幂等重试） |
| T6 | 打固定版本 tag `amc-2026.08`，全局 grep 确认无 Key 残留 | P0 | T1–T5 | ✅ 2026-08-07 tag 已推送 origin（commit `ab2468ad3`），grep 无 Key 残留 |
| T7 | 填写申请表（需求文档 §3 口径，补联系人/机构）并提交 | P0 | T0、T6 | ✅ 2026-08-07 已提交（学术榜 + 平台 Docker 部署路径；系统 Aranea-Agents Memory / 版本 amc-2026.08 / 联系人丁升；平台回执「Request submitted」，不签发 Eval Key，由平台构建评测） |
| T8 | （可选）代码记忆榜复核：确认同一适配层覆盖工程任务记忆场景，必要时补说明 | P1 | T1 | ✅ 2026-08-24 评测面 1–6：小集合全量打分、FTS 问句 OR、事件时间、L2/浅分类/supersede/PII 不改正文、Search 一跳+MMR、embed 异步 |
| T9 | （可选）社区贡献加分：3 组挑战性测试样本。**2026-10-19 官网代码实证：基准提案改为"预览+人工审核"**（`leaderboard.provide_benchmarks.disclaimer`："This preview does not transmit the form, your key, or any uploaded file"），无程序提交端点——只能浏览器填表预览后按页面向导递交（或邮件 contactus@agentmemoryleaderboard.ai）；素材已起草：`t9-challenging-samples.md`（B 多跳车队 / C 时序就医 / D 治理冲突，各 2 问）、`t9-benchmark-package.jsonl`（3 样本 9 题含 3 负例）、`t9-benchmark-readme.md`（目标/格式/schema/来源/验证划分） | P2 | T16 | 🟡 素材就绪，待用户浏览器递交 |
| T10 | 重评版本 r2：跨会话长任务记忆收尾 → 提交推送 dev → 打 tag `amc-2026.08-r2`（→ `e252ff09e`）→ 材料口径更新（需求文档 §3.5） | P0 | T7 | ✅ 2026-08-17 tag 已推送 origin；smoke 7 项自测全绿 |
| T11 | 浏览器代填「提交评测申请」表单：r2 版本 + 改动清单 + 引用首评申请编号并请求首评错误摘要（需求文档 §3.5 口径） | P0 | T10 | ✅ 2026-08-18 已提交，编号 `request_2770d1aa0f8d483799f1`（详见需求文档 §3.5「申请提交记录」） |
| T12 | Cycle 2 整改（`amc-2026.09`）：L3 召回升级 + Cycle-2 契约对齐（数值时间戳解析、Add 回显 user_id/session_id、Search 证据 `timestamp`/`created_at` 双发、embedding-v4 1024 维、pgvector 首启先建 extension、同会话 episode 幂等合并） | P0 | T11 | ✅ → commit `5e197184c` |
| T13 | Cycle 2 披露页随仓库公开：[`doc/memory-challenge.md`](../../../doc/memory-challenge.md)（架构/引用/C1–C7 改动/契约/部署/合规） | P0 | T12 | ✅ 2026-10-17 |
| T14 | 固定版本 tag `amc-2026.09`（→ `5e197184c`）打制并推送 origin；grep 确认无 Key 残留 | P0 | T12 | ✅ 已推送 origin |
| T15 | amc-2026.09 E2E 自测：干净源树（`git archive` 导出 tag）compose 构建 + 19 契约测试 + smoke 7 项全绿（复验首评疑似的平台降级路径可用性） | P0 | T14 | ✅ 2026-10-19：构建成功、19/19 契约测试、smoke 7/7、降级模式中文召回探针可用（实习发现 D 维度缺陷 → T17） |
| T17 | **D 维度治理修复**：中文『改成/换成/变为/改为』式偏好更新纳入同槽位 supersede（`chineseFavoriteRe` 终结符扩展）+ biz 回归用例 ×3 + PG 集成 `TestEvalStore_SupersedesFavoriteColorChinese`；dev 提交 commit `b7012e39d`。**注意**：首次打 r2 时发现 dev 提交态全量编译破裂（`sequencer.go` 引用未实现 `biz.DeliverySeqEvent`、`task_v2_repo.go` 缺 `MaxSeqBySession`——半成品 WIP 已部分入库），故 r2 **不从 dev 打**，改为 = `amc-2026.09`(`5e197184c`) + cherry-pick 修复（`04d2a05b3`）+ cherry-pick 披露同步（`b0e606373`）；干净导出树全量 `go build ./...` 通过、compose 构建 + smoke 7/7 全绿、颜色探针复测蓝色被 supersede 不再回流后打 tag `amc-2026.09-r2` → `b0e606373`，已推送 origin | P0 | T15 | ✅ 2026-10-19 |
| T16 | Cycle 2「Apply for Evaluation Access」提交（`/evaluation` 模态框 → `POST /evaluation-access-requests`）：系统 Aranea-Agents Memory / 版本 `amc-2026.09` / 学术榜 academic / participant_api 自部署。**本节 2026-10-19 会与事实按存档 `D:\temp\amc-cycle2-keys.txt` 订正：此为多阶段任务** | P0 | T15、T18 | 🟡 见下拆分 |
| T16a | 申请提交 + 受理回执 | P0 | T18 | ✅ 2026-09-29：request `request_cd8e1a774a864c199aa0`（new_key/academic/participant_api/pending→approved） |
| T16b | 接收 ldbd Key 并验证控制台 | P0 | T16a | ✅ 2026-10-11 Key 到手；2026-10-19 `POST /access/validate` 复核 `valid:true`、版本 `version_12232a2b392b`(amc-2026.09) active、`version_binding.allowed:true` |
| T16c | 参评版本升级与绑定：线上镜像升 amc-2026.09-r2(`b0e606373`)+`POST /access/versions` 新增 r2（版本绑定位空闲；**Full 之前必须先绑完**，Full 冷却期禁加版本） | P0 | T16b | ✅ 2026-10-19 21:30：`amc-r2-verify`(f38133) 本机+服务器双端英中 supersede 探针全绿（旧值 valid_until 置位、Search 不回流）；`POST /access/versions` 成功 → **version_5a4b31e9c7ad**（amc-2026.09-r2，active，memory_api_key_bound:true）。勘误：19:50「72c3b 错标」结论作废（二进制级复核其本就含修复；双探针坑 status↔valid_until / GBK 乱码，见提交单勘误节） |
| T16d | 评测前自测：**注意 `POST /integration-tests` 已于 Cycle 2 退役**（实测返回 LEGACY_INTEGRATION_TEST_RETIRED），正统为 `POST /api/v1/evaluations {version_id, benchmark_type:"textual", phase:"smoke", options:{max_add_concurrency,search_concurrency,top_k:100}, run_label}` | P0 | T16c | ✅ 2026-10-19 21:55：teval_d73c0817f09d22d6（r2-smoke-1）**succeeded**，46/46，add/search/answer/eval 四stage全completed；总分 0.6212；能力：B/E=1.0、A=0.8、C=0.6、D=0.533、H=0.5、streaming=0.458、G=0.3 |
| T16e | 启动 Full 评测（`POST /api/v1/evaluations`；⚠️ 每 Key 每赛道仅 2 次 Full、第二次+30天解锁=本轮实质一次机会、max_active_jobs=1）。**运行事故记录（2026-10-20 02:00–03:00）**：search 段吞吐劣化至 ~6.5/min，根因=`trgmExtraCandidates` 的 `statement %> $1 ORDER BY word_similarity` 在 eval 期间膨胀到 **379,702 行/8 GB** 的 memory_facts 上走 GIN 海量候选扫描（单条 10–16min），40 并发占满连接池致全站搜索 60s 超时、add 重试事务无法开启；处置=`ALTER DATABASE aranea SET plan_cache_mode=force_custom_plan` + 两轮全量后端连接清场（app 池自动重建），done 速率恢复 **~96/min**（15 倍），`previous_failures` 保持为空；代码容错路径（57014→warn→降级非 trgm 召回）工作正常；**10-20 晨间复核：夜间再次淤积（清场仅脉冲式恢复，~1.3/min），10:15 `statement_timeout=10s` 兜底成功下发**（ALTER DATABASE + 清场 27）→ 速率回升至 ~19/min 可持续档 | P0 | T16d | ✅ **2026-10-22 20:10:47（UTC+8）succeeded**：teval_e176051a414de4db（2026-10-19 22:05 用户点头启动，r2-full-1，add=16/search=32/top_k=100；全程 70h26m；**9602/9602 全部完成**=textual 5609+streaming 3993，add/search/answer/eval 四 stage 全 completed）。**总分 0.4717**；能力：A=0.505、B=0.412、C=0.435、D=0.570、E=0.381、G=0.460、H=0.609、streaming=0.426；子项最弱 F1=0.142(n=64)、G4=0.170、D3=0.200、E3=0.269、B3=0.297，最强 D4=0.816、streaming.intra_transfer=0.993。10-22 上午平台宕机 ~1h（本侧网络排除、Cloudflare 边缘活源站 503）+ 瞬时 status=canceled 抖动后断点续跑恢复。**唯一 previous_failure=2026-10-21 22:28(UTC+8) attempt1 EVAL_RUNTIME_ERROR「Errno 28 No space left on device」@eval 段 retryable——平台侧运行时事故**（我方 Add/Search 无磁盘写入点），平台有界重试自动续跑，无缺题无重跑。服务器冻结解除，升配窗口待用户点头 |
| T18 | **Cycle 2 公网自部署**（Cycle 2 强制前置）：阿里云 ECS `8.130.151.83:8910`（Ubuntu，2C/1.7G/40G；安全组放通 8910；compose 双容器 aranea-eval-db-1 + aranea-eval-memoryeval-1 @/opt/aranea-eval）。**2026-09-29 已部署**、09-30 安全组放通、10-11 第三次部署（向量首启降级+L2 覆盖两修复镜像）。2026-10-19 复盘核验：公网 healthz 200、无 Key 401、带存档 Key Add/Search 200 并即时召回；镜像正升 r2（T16c） | P0 | T15 | ✅ 2026-09-29（2026-10-19 重验） |
| T19 | **Cycle 3 质量升级（2026-10-22 立项，依据 `docs/reports/2026-10-22-analysis-amc-full-eval-failure-and-scoring-plan.md` §5 的 P0–P2 清单）**：T16e Full（teval_e176051a414de4db）暴露的真实质量缺口修复——全为确定性规则/检索结构、零新 LLM、契约不变。三轨并行实施（worktree 隔离）后归并：**P0** ① trgm scope 先行 MATERIALIZED CTE（10-20 搜索风暴治本，先 scope 限定 2000 行再 `%>`）+ CJK bigram FTS 通道（`bigram_text()` IMMUTABLE SQL fn + 函数式 GIN——对抗审查 #9 弃 STORED 生成列免 PG≤17 全表重写/启动写阻塞，迁移 20261292，42703/42883 优雅降级，trgm 在 CJK 通道建立后可退役）；② MemoryFactIndexSweeper（embedding_status stale/pending/failed 行 <8 次重试、SQL 侧 2^attempts 分钟退避封顶 60min、串行批 50/30s、复用 SyncFactIndexFromRow，cmd/memoryeval 接线；对抗审查 #7 补迁移 20261293 部分索引治 due 行 38 万行 seq scan）；③ 中英相对时间解析（anchor=消息时间戳，metadata.event_dates，对抗审查 #3 逐原子解析不再复用首原子结果）+ 查询模式（Chronological 按事件日期排序跳 MMR / Range 过滤 / Aggregate 追加近 5 条 L2 episodes）；**P1** ④ 治理扩展（constraint 条件句式拓宽 如果/每当/只要/规定/规则/whenever/if…then——对抗审查 #2 移出裸 若/按照，新增假设句守卫 `IsHypotheticalStatement` 绝不 supersede 在册事实；新槽位 搬到/换工作/手机号；否定 cue 不喜欢了/没感觉/改为；向量矛盾近邻失效：纠错 cue（对抗审查 #8 移出裸「改」）+ cosine≥0.82、对抗审查 #6 top50 后 scope 过滤抗多租户共享索引饿死、对抗审查 #1 同批守卫收窄至向量通道（同请求 [旧值, 纠正] 照常失效）、metadata.previous_values 留痕、对抗审查 #0/#4 UpsertFactRow eval 域 CASE 守卫+事件时序守卫——平台整请求重放不复活墓碑/不清空审计/不反噬更新事实）；⑤ L2 episode 全量分片（evalEpisodeMaxChars 4000→32768 + L1ArchiveEpisodeInsert.MaxMergeChars）；**P2** ⑥ 证据门控（AdaptiveRecallMinScore 自适应下限接入 eval Search；敏感模式掩码——身份证中 8 位/手机中 4 位/银行卡中 6 位，query 明确询问类别时放行；证据前缀 `[YYYY-MM-DD · sess-<id>]`）；⑦ 消息原子化入库（SplitEvalStatements：。！？；\n + 英文句点（对抗审查 #5 缩写词干守卫 Dr./St./etc. 前不砍句），≤8 枚/消息，metadata 记 message_index/atom_index，stored 返回值仍=消息条数）。验收：提交态带外验证（git worktree 干净树）编译绿 + 19 契约测试绿 + internal/data（61s）/internal/biz 定向套绿；三视角对抗审查（合规/正确性并发/性能 + 逐发现三票反驳）：**14 发现 → 10 定案（4 major + 6 minor，4 驳回）全部修复于 `a0cdc026c`**（12 文件 +487/-77），每项定案有定向绿测试钉死（含 PG 红线测试：重放不复活墓碑/不反噬更新事实/审计留存、同请求 supersede、逐原子日期）。分支 `amc/c3-upgrade`（基 bd377de76 + C/A/B 三轨 merge **2026-10-22 已 merge+FF 落 dev**：A11 属主会话（aranea-3c[e26bb8] 线）响应跨会话广播提交 e7cdbb23a（20261291 注册行+SQL 同 commit 入 dev）→ merge dev 最新（89 批次2/3 收官 FR-6~14 + 20261291）做 ddl_migration_table_late.go 版本序联合（**291<292<293**）→ FF dev 至 **ee29fca85**（合并态门禁绿：build + Migration 全局版本唯一守卫 + C3/CJK/EvalStore 25 条 PG 红线测）；**未推远端**（推送须用户点头）。遗留（不阻塞）：internal data/biz 5 项存量失败（SkillMerge/SkillDedup/Outbox/MCPServerRepo/EntCoverage）经基线 721a28beb 带外复现证明归属并行会话 WIP；原「previous_values 重放清空」遗留已由 #0/#4 消除 | P0 | T16e | ✅ 已落 dev（ee29fca85，未推远端） |

## 4. T1 实施要点（适配层）

1. 契约字段以官方 API Guide 最新版为准，先核对再写码（design.md §5.1 有 ⚠️ 标注）
2. Search 路径禁止任何 LLM 调用；返回库存记忆原文
3. user_id 为空一律 400；召回强制 user scope 过滤；单测必须含跨 user_id 污染用例
4. Add 同步确认 + 内部异步摄取；Embedding 不可用降级关键词索引，不阻断契约
5. 日志：适配层关键节点按 K1/K2 规范打进程日志（`loggateway`，构造注入），评测流量做计数限流
6. 遵守红线：实际实施未动 proto/wire（独立 net/http 入口）；data 层错误经 `entErrToBizErr` 翻译

## 5. 验证命令

| 改动 | 最小验证 |
|------|----------|
| 适配层 | `go build ./... && go test ./cmd/memoryeval/... -count=1 && go vet ./cmd/memoryeval/... ./internal/biz/... ./internal/data/...` |
| Docker | `docker compose -f docker-compose.eval.yml build` + up 后 smoke 脚本全绿 |
| 提交前 | 后端全量 `make build && make test && make lint` |

## 6. 验收标准

继承需求文档 §4（AC1–AC6），另加：

| # | 标准 |
|---|------|
| AC7 | T1 单测覆盖：正常 Add/Search、user_id 缺失 400、跨 user_id 零泄漏、Search 无 LLM 调用断言 |
| AC8 | Smoke 自测报告（T5）显示召回率与隔离全部通过 |

## 7. 风险与应对

| 风险 | 等级 | 应对 |
|------|------|------|
| 仓库非 public / 开源合规未就绪 | 高 | T0 今天确认；若受阻则改走商业·API 路径（需公网稳定部署，成本更高）或放弃本期 |
| 平台 Docker 环境无法访问外部 Embedding API | 中 | 设计已内建关键词降级；T2 compose 形态实测验证 |
| ~~SQLite 模式检索质量不足~~ | — | 已排除：`NewData` 硬编码 Postgres，compose（pgvector）为唯一形态 |
| 契约字段与官方最新版偏差 | 中 | T1 动手前先复核官方 API Guide |
| 截止前时间不足 | 高 | 严格按 T0→T7 顺序，P0 之外全部可砍；材料（M1–M4）已完成不阻塞 |

## 8. 改动文件清单（T1 实际）

新增：
- `internal/biz/memory_eval.go` — `EvalMessage` / `EvalMemoryItem` / `EvalMemoryStore` 窄端口（Stability:internal）
- `internal/data/memory_eval_store.go` — 委托现有 `l3FactRepo`（UpsertFactRow 幂等写入 + RecallL3Facts 混合召回 + PII gate）与 `MemoryFactIndexSyncer`（向量索引，可降级）
- `cmd/memoryeval/main.go`、`handler.go` — 独立 HTTP 入口（`POST /v1/memory/add`、`POST /v1/memory/search`、`GET /healthz`）
- `cmd/memoryeval/handler_test.go` — 17 个契约测试
- `docker-compose.eval.yml`、`test/agent-memory-challenge/smoke.sh`（T2/T5）

主程序修改：**评测零侵入**（api proto、cmd/admin、internal/service 未触碰；`make build` 自动产出 `bin/memoryeval`）。仅两处共享构建/迁移设施的必要修复：
- `internal/data/plugin_run_schema.go`（`splitDDLStatements` 先剥离 `--` 注释再切分）— 修复全新 Postgres 部署时 `20261128_memory_facts_fts_index` 迁移因注释含 `;` 被切碎而失败（P1 启动阻塞，平台干净构建必经路径）；回归测试 `internal/data/split_ddl_statements_test.go`
- `Makefile` / `Dockerfile` — 新增可选 `GO_BUILD_TAGS`（默认空，主程序构建行为不变）；评测 compose 传 `pgvector` 并设 `DAO_VECTOR_PGVECTOR=true` 启用向量召回

## 文本评分增量实施（2026-10-22）

范围：`cmd/memoryeval/handler*.go`、`internal/biz/memory_eval*.go`、`internal/data/memory_eval*.go` 及本场景三件套；不部署或改绑正式版本，不发起平台评测。

| 任务 | 状态 | 验收 |
|---|---|---|
| 连续英文分句、Unicode 与短样例无损 | ✅ | 连续三句、混合中文与 A=1 等短样例回归通过 |
| 选项检索、规则及长历史证据覆盖 | ✅ | HTTP 契约、隔离 PG 和 Linux 实际接口冒烟通过 |
| 来源时间、日历范围、历史旧值检索 | ✅ | 来源锚定、闰月、多日期与历史有效区间通过 |
| 同主体纠错、重放与事务回滚 | ✅ | 相似非矛盾、跨说话人、失败回滚、乱序与并发用例通过；4 项 race 通过 |
| 隐私脱敏与真实相关性门控 | ✅ | 标识符按需披露、国际电话/邮箱/住址/SSN/凭据、零词法与零语义噪声拒收通过 |
| 验证归档 | ✅ | 分析报告及本地验收报告已写入；未部署、未提交正式评测 |

验证入口：`test/agent-memory-challenge/run-score-regressions.ps1`（自动创建并清理独立 Postgres；可传专用 `aranea_test` DSN；`-WithRace` 可选）及 `live-score-smoke.ps1`（只允许回环地址）。详细证据见 [验收记录](../../testing/reports/acceptance-2026-10-22-amc-score-optimization.md)。

### 第二轮业务逻辑优化

依据分析报告 §6 的 Q1–Q12：

| 工作包 | 内容 | 状态 |
|---|---|---|
| 历史与治理 | 中间状态区间、治理键、过去状态、单值属性 | ✅ 领域与 PG 业务回归通过 |
| 时间证据 | 日历粒度、日期标题、范围候选、未知时间与来源偏移 | ✅ 含跨午夜、候选截断和标题重放用例 |
| 证据组织 | 规则原文卡、函数示例、跨来源覆盖、去重与摘录 | ✅ 规则 top_k=1、跨 session 及函数迁移通过 |
| 隐私与验收 | 细地址和凭据最小披露、回归与实际接口 | ✅ 97 项目标回归、8 项 race、两组 46 条实际 HTTP 断言通过 |

第二轮产物为 `bin/memoryeval-amc-quality-20261022.exe` 与 `bin/memoryeval-amc-quality-20261022-linux-amd64`，哈希见验收记录。另经用户授权完成一处 `approval_policy_repo.go` 查询端口兼容修复，以恢复本地构建；未改审批策略行为。实际复验脚本为 `live-quality-smoke.ps1`；记录使用 `quality-` 前缀，与第一轮分开保存。

### 第三轮：r2 Full 基线补齐

依据分析报告 §8，来源为用户确认的 r2-full-1；前两轮未提交该参评版本。

| 工作包 | 状态 |
|---|---|
| R1 删除、持续抑制、恢复与转录一致性 | ✅ 当前/历史/L2、重放、回滚、引文和用户隔离通过 |
| R2 结构化原文与枚举 | ✅ JSON top_k=1、表格/列表/步骤结构回归通过 |
| R3 有界实体中间证据 | ✅ 跨三会话两跳、删除桥接、无种子与完整父链通过 |
| R4 相关个人背景线索 | ✅ 饮食背景归属和无关请求负例通过 |
| R5 范围化指令、步骤与格式 | ✅ 项目间不互相覆盖，L2/L3 范围和原文步骤通过 |
| R6 历史时间区间与轨迹 | ✅ 本地午夜、半开区间和同日状态排序通过 |
| R7 综合证据类型覆盖与验收 | ✅ 120 项目标回归、9 项 race、96 条实际 HTTP 断言通过 |

本轮新增领域测试 `internal/biz/memory_eval_full_test.go`（5 项）与数据测试 `internal/data/memory_eval_full_test.go`（18 项）。关键代码为 `memory_eval_control.go`（biz/data）、`memory_eval_context.go`（biz）、`memory_eval_relations.go`、`memory_eval_applicability.go`、`memory_eval_validity.go`（data）；并同步分类、范围 SQL、召回和证据组织。测试脚本为 `live-fullfocus-smoke.ps1`，完整回归入口已纳入新领域用例。

构建产物为 `bin/memoryeval-amc-fullfocus-20261022.exe` 与 `bin/memoryeval-amc-fullfocus-20261022-linux-amd64`；本轮专用 PG/运行容器和网络已清理。产物哈希、源码清单、命令及证据见验收报告第三轮。未接真实 embedding、未做 Full 容量/P95 测试、未部署和提交评测，不将本地验收标记为比赛能力完成。

### P3 主线整合（2026-10-23）

范围：合并 `amc/p3-upgrade` 的四个独有提交，兼容前三轮行为；不变更参评绑定。

| 工作包 | 状态 |
|---|---|
| 意图路由、纯领域摘要/数值解析及隐私类别扩展 | ✅ 已接入源码，保留离线模式 |
| 事务写链、派生来源、遗忘后重建与关系隔离 | ✅ 已整合；专项用例见 `internal/data/memory_eval_merge_test.go` |
| 合并态构建、领域/隔离 PG/race/实际 HTTP 验证 | ✅ C10=`c53042230`（R7–R12）+ C10+=`93e4f90b3`（补并 R13/R14）；FactKind 定向绿 + OOT build 绿 |
| 正式比赛版本部署、真实嵌入与 Full 容量评测 | 📋 待用户点头：推送 `amc/c10-merge`→dev → tag `amc-2026.10` → 新机 `8.160.178.131` 双赛道部署 |

### C10+ 补并（2026-10-28）

Claude Code 会话在 `amc/c10-merge` worktree 已 `merge --no-commit 1e0fbe4fb`（无冲突）后因上游 502 中断；续跑完成提交 **`93e4f90b3`**（父：`c53042230` + `1e0fbe4fb`；差集仅 `fact_kind.go` / `fact_kind_eval_ext_test.go`）。P3 fact-kind 对抗链 R13/R14 已入部署根候选；**未推远端、未打 tag**。

关联源码：`internal/biz/memory_eval_{intent,numdigest,rollup}.go`、`internal/data/memory_eval_derived.go`、既有 `memory_eval_{store,ingest,control,search,relations}.go`；ADR 见设计文档 §10.3。保留合并前的离线日期/模式测试，并新增路由兼容测试；原 P3 遗忘失效审计断言按既有持久化删除契约验证当前、历史与摘要不回流。
