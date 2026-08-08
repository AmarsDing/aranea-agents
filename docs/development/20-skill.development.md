# Skill 技能 — 开发计划

> **版本**：2026-07-16 | **状态**：🟡 运行时装配（Layer A/B）已接通；管理面 P0/P1 修复中（启用门控/发布校验/元数据与版本 UI/`skill_load_mode`）；三角色 RBAC / Skill 市场 / Catalog WS 仍待
> **需求**：[20-skill.md](./20-skill.md) · **设计**：[20-skill.design.md](./20-skill.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **变更**：[changelog/2026-05-21-Skill-DocSync-RuntimeWire.md](../changelog/2026-05-21-Skill-DocSync-RuntimeWire.md)

---

## 1. 模块定位

Skill 技能系统：管理 Agent 可用的能力包（SKILL.md + 附件），支持创建、导入、发布、启用/停用、运行时路由与 trpc-agent-go 装配、执行追踪。

**核心链路**：注册（CRUD / ZIP 导入）→ 发布 → 启用 → 运行时路由（Layer A + Layer B）→ trpc-agent-go Skill 工具链 → 执行追踪。

**代码锚点**：

| 层级 | 路径 |
|------|------|
| Proto | `api/kratos/skill/v1/skill.proto` — SkillService **22 RPC** |
| Service | `internal/service/skill.go`（19 RPC）· `skill_import.go`（4 RPC）· `skill_import_http.go`（multipart 挂载） |
| Biz | `internal/biz/skill/` — `skill.go` · `skill_import.go` · `skill_runtime.go`；`internal/biz/skill_*.go`（相似度/合并/进化/智能/去重/健康） |
| Data | `internal/data/skill*.go`（仓储 + 合并 + 去重 + 智能 + 进化 + 统一进化） |
| 桥接 | `internal/skill/trpc/` — Repository / Filter / Tools / Executor / `artifact_executor` |
| 监听 | `internal/skill/watch/` · `internal/skill/storage/` · `internal/skill/importer/` |
| 运行时路由 | `internal/tools/skillruntime/` · `internal/tools/skillrouter/` · `internal/tools/skills_butler/` · `internal/tools/skillrecommend/` |
| Agent 装配 | `internal/agent/trpc_build.go` → `buildSkillDeps` + `skillruntime.NewAgentVisibilityFilter`；`internal/agent/skill_guidance_inject.go`（Prompt 注入方式 C） |
| 前端 | `web/src/pages/SkillsPage.vue` · `SkillDetailPage.vue` · `SkillRunsPage.vue` · `pages/agent-settings/AgentSettingsSkillsTab.vue` · `components/skills/*` · `features/skills/api.ts` |

---

## 2. 现状评估（2026-06-17）

| 项 | 状态 | 证据 |
|----|------|------|
| Skill CRUD + 文件读写 | ✅ | 22 RPC；列表/详情/创建/更新/发布/启用/复制/删除/文件/运行记录/预览/版本历史/回滚/健康指标/磁盘健康 |
| ZIP 导入 + 冲突 + AI 炼化 | ✅ | proto 声明 4 端点；`POST /v1/skills/import` multipart 由 `RegisterSkillImportMultipart` |
| Layer A + B 路由 | ✅ | `ResolveSkillSlugsDetailed` + `NewAgentVisibilityFilter`；turn query 经 `RunOptionWithTurnQuery` 注入 RuntimeState |
| trpc-agent-go 桥接 | ✅ | `DBRepositoryAdapter` / `FSRepositoryAdapter` + `WithSkills` / `WithSkillFilter` |
| 文件监听与同步 | ✅ | `watch.Runner`：fsnotify + debounce + `filesystem_missing` + reconcile ticker |
| 磁盘同步 UI + 通知 | ✅ | §2.4；health API / Banner / 筛选 / Monitor 事件 |
| 存储根解析 | ✅ | `storage.ResolveRootWithPlatform` + `work_directory` |
| Agent 设置 UI | ✅ | `AgentSettingsSkillsTab`：`skill_runtime_json` + **`skill_load_mode`（含 progressive）** |
| 前端管理 | ✅ | 列表/详情/**元数据 Dialog**（新建/编辑）/文件编辑/导入/运行记录 |
| 版本历史 / 回滚 | ✅ | API + **详情页版本卡片与回滚 UI**（不可变策略） |
| 启用门控 | ✅ | 仅 `published`/`active` 可启用（biz+data+FE）；发布需正文校验（非假 pass） |
| RBAC 权限 | 🟡 | 二元 admin 门控（`requireAdminAccess`）；产品三角色（编辑者/只读）未落地 |
| Preview 选中原因 | ✅ | `ResolveSkillSlugsDetailed` 返回 `Reasons map[string]string` + `agent_id` 关联 |
| Prompt 注入方式 C | ✅ | BeforeModelHook + `BatchGetSkillGuidance` 批量获取 + 截断 + 空 guidance 防护 |
| Embedding 语义精排 | ✅ | `SkillEmbedder` + `ScoreByEmbedding` + 内存缓存 + 评分融合 + 优雅降级 |
| manifest/render 包 | ✅ | `internal/skill/manifest/` + `internal/skill/render/`；frontmatter 解析 + 变量替换 + prompt 渲染 |
| Ent 字段补齐 | ✅ | `visibility`/`default_config_json`/`file_manifest_json`/`message_id`/`parent_version_id`/`evolution_reason`/`lifecycle_status` 均已落地 |
| Repo 窄接口拆分 | ✅ | `SkillReader` + `SkillWriter`，`Repo` 组合两者；进化接口拆分为 4 个窄接口 |
| SkillFilesystem 端口 | ✅ | `SkillFilesystem` 接口下沉到 storage 层，Service 层不再直接操作 `os` 包 |
| 统一相似度引擎 | ✅ | `SkillSimilarityEngine` 4 维 Jaccard + 可选 Embedding 混合（`internal/biz/skill_similarity.go`） |
| 三阶段合并 | ✅ | `SkillMergeUsecase` 内容融合 → Gate 验证 → 事务应用（`internal/biz/skill_merge.go`） |
| 统一进化编排 | ✅ | `SkillEvolutionOrchestrator` + 3 Trigger（Pattern/Health/AgentConfig）+ 原子化检查（`internal/biz/skill_evolution_unified.go`） |
| 进化验证强化与触发扩展（P2） | ⏳ | AB 对照回放棘轮 / Gate 漂移检测 / SuccessTrigger 成功沉淀 / 触发率黄金集回归（设计 [`phase3-进化能力/08`](./phase3-进化能力/08-P2-进化验证强化与触发扩展.design.md)，2026-08-09 启动） |
| ScoreSkill 四维权重 | ✅ | SuccessRate 0.4 + Duration 0.25 + Token 0.2 + Feedback 0.15（条件启用） |
| 健康指标 | ✅ | `GetSkillHealth` RPC + `SkillHealthMetric`（7d/30d 调用统计、成功率、P95 耗时） |
| 去重缓存 | ✅ | `DetectDuplicateGroups` 10min TTL 内存缓存 + `InvalidateDedupCache()` |
| 标签字典 | ✅ | `skill_tags` 治理表 + 4 RPC（`/v1/skill-tags`）+ 改名/删除事务重写 + 孤儿标签治理 + 独立管理页 `/skills/tags` |

---

## 3. 差距与优化

### 3.1 已完成（P0–P2）

| 阶段 | 内容 | 状态 |
|------|------|------|
| **P0** | 存储根解析接通 `work_directory` | ✅ |
| **P1** | Proto CRUD/发布/预览 + Import RPC（multipart 例外） | ✅ |
| **P2** | `skillruntime` Layer A/B + Agent `buildSkillDeps` 接通 + turn query 注入 | ✅ |
| **P2′** | 文件系统监听 + debounce + `filesystem_missing` | ✅ |

### 3.2 磁盘同步优化计划（P2.5）

| 步骤 | 内容 | 状态 |
|------|------|------|
| **D1 文档** | `20-skill.md` §2.4、设计文档 §八 磁盘监听与同步 | ✅ |
| **D2 API** | proto：`filesystem_missing`、`sync_origin`、列表筛选、`GetSkillFilesystemHealth` | ✅ |
| **D3 后端** | enrichSkill 映射；watch 目录 slug 约束；EventBus + Monitor 事件；wire `NewRunnerWithBus` | ✅ |
| **D4 前端** | Banner、来源/磁盘列、筛选、`filesystem-health` 拉取 | ✅ |
| **D5 后续** | reconcile ticker；磁盘更新回退 draft；异步相似度 warn；Alert Webhook | ✅ |

### 3.3 已完成（P3–P4）

| 阶段 | 内容 | 状态 |
|------|------|------|
| **P3** | 版本历史与回滚 RPC + RBAC 接入 + Ent 字段补齐 | ✅ |
| **P4** | Prompt 注入方式 C + Embedding 精排 + Preview 选中原因 + manifest/render 包 + Repo 窄接口拆分 + SkillFilesystem 端口下沉 + N+1 优化 | ✅ |
| **P4′** | 统一相似度引擎（4 维 + Embedding）+ 三阶段合并 + 统一进化编排 + ScoreSkill 四维权重 + 健康指标 + 去重缓存 | ✅ |

### 3.4 待实现（P4+）

| # | 差距 | 优先级 | 说明 |
|---|------|--------|------|
| 1 | 自动负熵报告 | P4+ | 聚合指标 + 趋势 |
| 2 | Import multipart 完全 codegen | P4+ | 🟡 保留手动注册，已补齐 admin 校验 + 指标 |
| 3 | Budget 中间件 | P4+ | token 上限裁剪 |
| 4 | Skill 依赖 / 冲突表 | P4+ | 安装时检查 + 运行时互斥 |
| 5 | SkillBackend 多 kind 差异化 | P4+ | 按 `kind`（prompt_pack / workflow / tool_backed）差异化加载与渲染 |
| 6 | Context 目录迁移 | P4+ | `internal/skill/**` → `internal/capability/skill/**` |
| 7 | Feedback 真实接入 | P4+ | `ScoreSkill` Feedback 维度当前为启发式估算（标注 `TEMPORARY`），待接入真实用户反馈 |
| 8 | EvolutionCoordinator 清理 | P4+ | ✅ `evolution_coordinator.go` 已随 A6 物理收敛删除（含 `SetCoordinator` 委托与 fallback 逻辑） |
| 9 | `extends_skill_id` + 环检测 | P4+ | proto/biz 有字段，Ent schema 无列；需迁移后实现 |
| 10 | SkillProposal 状态机 | ✅ | `Approve/Reject/Register` 已走 `SkillProposalStateMachine`（2026-07-16） |
| 11 | `skill_catalog` WS + 聊天 Catalog | ⏳ | 69 Phase3；组件已有，事件未接 |
| 12 | Skill 市场 | ☐ | Phase3 生态 |

### 3.5 已完成（P5：标签字典）

| 阶段 | 内容 | 状态 |
|------|------|------|
| **P5** | `skill_tags` 治理表 + 实时使用计数聚合 + 改名/删除事务重写所有引用 + 孤儿标签治理 + 独立管理页 `/skills/tags` + 三处标签下拉复用字典选项源（Skill 编辑 / 列表筛选 / Agent 设置）+ 治理后 embed/去重缓存失效 | ✅ |

### 3.6 已完成（P6：管理页交互优化，2026-07-29）

| 项 | 内容 | 状态 |
|----|------|------|
| 标签分组 | 表格标签列按 `维度:值` 前缀分组显示 | ✅ |
| 列表排序 | `ListSkillsRequest.sort_by/sort_order`（proto → biz → service → data，Postgres JSONB 首个标签排序），筛选栏排序控件，默认标签升序 | ✅ |
| 操作列拆分 | `启用`（生命周期 publish）与 `发布到生态市场`（ecosystem product）双按钮分离 | ✅ |
| 统计悬浮面板 | `SkillStatsHoverChart.vue`：ECharts 趋势堆叠柱 + 成功率环形图 + 关键指标，健康数据按行懒加载 | ✅ |
| 最近调用列 | 相对时间 + 完整时间 tooltip；修复有时间却显示「未调用」的错位 | ✅ |
| MetaDialog | 加宽 + 名称/Slug/标签同行吸顶 | ✅ |
| EditorDialog | 双栏独立滚动（压过 `.app-glass-panel` overflow）+ 细滚动条 | ✅ |
| i18n | 新增 `skillsPage.*` 语言包（zh-CN / en-US），管理页新增文案全量迁移 | ✅ |

### 3.7 已完成（P7：运行时加载链路优化，2026-07-29）

| 项 | 内容 | 状态 |
|----|------|------|
| P0 运行时缓存主动失效 | `RuntimeCacheInvalidator` 端口 + `DBRepositoryAdapter.InvalidateSkillRuntimeCache`；`ToggleEnabled`/`Delete`/`RollbackVersion`/`Publish`/`UpsertSkillFromDisk`/`Patch` 成功后主动失效快照，启用/正文变更从 TTL 2min 降至秒级 | ✅ |
| P1-3 frontmatter triggers 确定性触发 | `manifest.Parse` 提取 `triggers[]` → metadata envelope 落库（`parseSkillTriggers`/`normalizeSkillTriggers`）→ `RuntimeCandidate.Triggers` → `matchTrigger`（CJK 子串 / ASCII 词边界）命中后绕过 intent 收窄与 tag 过滤、`triggerScore=2000` 置顶、占配额、Layer A deny 优先、历史排名融合保护；创建/导入/overwrite/watcher/butler/Patch 全链路刷新 triggers | ✅ |
| P1-4 发布校验触发信号 warn | `evaluatePublishValidation`：description 无触发条件 cue 且无 triggers → warn（不 block） | ✅ |
| P2-7a 发布即启用 | `Publish` 成功后自动 `enabled=true`（失败不阻断发布，返回真实状态由前端开关呈现） | ✅ |
| 验证 | `biz/skill`、`data`（PG 集成）、`skill/...`、`skillruntime`、`service(skill)` 测试全过；`go build ./cmd/... ./internal/... ./pkg/...` 干净；review 无阻断项 | ✅ |

### 3.8 已完成（P8：导入决策跨组去重修复，2026-07-29）

| 项 | 内容 | 状态 |
|----|------|------|
| bug 修复 | `groupDecisionsForSkipKeep`（`internal/pkginstall/skill_import.go`）：同一 warn candidate 出现在多个 `keep_separate` conflict group 时，原逻辑每组各发一条 `keep_separate` decision → 服务端重复插入同一 slug 违反 `skill_skill_key_key` 唯一约束（HTTP 400）；且后续 `skip_group` 会把已创建记录善后为 `deleted` 墓碑，墓碑又阻塞重装（全表唯一索引不含状态过滤）。修复：跨组先收集 kept candidate 去重，含 kept candidate 的 group 不再发 `skip_group` | ✅ |
| 测试 | 新增 `TestInstallSkillSkipKeepSeparateDeduplicatesCandidateAcrossGroups`（一 candidate 跨 3 组只发 1 条 decision）；`internal/pkginstall` 全量测试通过 | ✅ |
| 生产验证 | 阿里云运维技能批量安装（11 个 alibabacloud-*）：修复前 3 个因该 bug 反复失败并留墓碑，修复后全部 published+enabled | ✅ |

---

## 4. 开发阶段

### Phase 1：版本 + 权限 + 导入收尾（P3）— ✅ 已完成

- 版本历史与回滚 RPC（`GetSkillVersions` + `RollbackSkillVersion`）
- RBAC 接入（`requireAdminAccess` + `applySkillPermission`）
- Import multipart 收敛或 OpenAPI 聚合文档补齐
- Ent 字段补齐（`visibility` / `default_config_json` / `file_manifest_json` / `message_id` / `parent_version_id` / `evolution_reason` / `lifecycle_status`）

### Phase 2：运行时增强（P4）— ✅ 已完成

- Prompt 注入方式 C（BeforeModelHook + `BatchGetSkillGuidance`）
- Embedding 语义精排（`SkillEmbedder` + `ScoreByEmbedding` + 评分融合 + 优雅降级）
- Preview 返回选中原因（`ResolveSkillSlugsDetailed` + `Reasons map[string]string`）
- manifest/render 包（frontmatter 解析 + 变量替换 + prompt 渲染）
- Repo 窄接口拆分（`SkillReader` + `SkillWriter`）
- SkillFilesystem 端口下沉（Service 层不再直接操作 `os` 包）
- N+1 查询优化（`BatchGetSkillMarkdownBySlugs` 批量获取）

### Phase 2′：智能增强（P4′）— ✅ 已完成

- 统一相似度引擎（`SkillSimilarityEngine` 4 维 Jaccard + 可选 Embedding 混合）
- 三阶段合并（`SkillMergeUsecase` 内容融合 → Gate 验证 → 事务应用）
- 统一进化编排（`SkillEvolutionOrchestrator` + 3 Trigger + 原子化检查 + 接口拆分）
- ScoreSkill 四维权重修复（SuccessRate/Duration/Token/Feedback 条件启用）
- 健康指标（`GetSkillHealth` RPC + `SkillHealthMetric`）
- 去重缓存（`DetectDuplicateGroups` 10min TTL + `InvalidateDedupCache`）

### Phase 2″：标签字典（P5）— ✅ 已完成

- `skill_tags` Ent Schema + `TagRepo` 窄接口（`SkillTagReader`/`SkillTagWriter`）
- 实时使用计数聚合（`skillTagUsage` 扫描 `metadata_json.tags`，不落库、强一致）
- 改名/删除事务重写（`Data.ExecInTx` 内先改字典行，再重写所有 Skill 引用，返回重写条数；改名到已存在目标 = 等价合并）
- 孤儿标签治理（List 合成 `source=orphan`，收录即预建；删除清理引用）
- 4 RPC + 独立路由前缀 `/v1/skill-tags`（避免被 `/v1/skills/{id}` 吞掉）
- 独立管理页 `/skills/tags`（按维度分组 + 搜索 + 收录状态筛选 + 新建/改名/删除/收录）
- 三处标签下拉复用字典选项源（`SkillMetaDialog` / `SkillFilterBar` / `AgentSettingsSkillsTab`），仍允许输入新标签
- 治理后失效 `InvalidateEmbedCache()` + `invalidateDedupCache()`（`skillCorpusText` 含 tags，向量必须重算）

### Phase 3：生态扩展（P4+）— 待实现

- Skill 依赖声明与冲突策略
- Budget 中间件（token 上限裁剪）
- 自动负熵报告（聚合指标 + 趋势）
- SkillBackend 多 kind 差异化加载与渲染
- Skill 市场（Ecosystem 联动）
- Feedback 真实接入（替换 `TEMPORARY` 启发式估算）
- 可选：`internal/skill/**` → `internal/capability/skill/**` 目录迁移

---

## 5. 任务清单

| # | 任务 | 优先级 | 阶段 | 状态 |
|---|------|--------|------|------|
| 1 | Layer A/B 接通 `buildSkillDeps` + turn query | P2 | — | ✅ |
| 2 | 文档三件套与代码对齐 | P0 | — | ✅ |
| 3 | `GetSkillVersions` + `RollbackSkillVersion` | P3 | Phase 1 | ✅ |
| 4 | RBAC 替换硬编码权限 | P3 | Phase 1 | ✅ |
| 5 | Import multipart codegen | P3 | Phase 1 | 🟡 |
| 6 | Ent 字段补齐 | P3 | Phase 1 | ✅ |
| 7 | Prompt 注入方式 C | P4 | Phase 2 | ✅ |
| 8 | Embedding 精排 | P4 | Phase 2 | ✅ |
| 9 | Preview 选中原因 | P4 | Phase 2 | ✅ |
| 10 | manifest/render 包 | P4 | Phase 2 | ✅ |
| 11 | Repo 窄接口拆分（SkillReader + SkillWriter） | P4 | Phase 2 | ✅ |
| 12 | SkillFilesystem 端口下沉 | P4 | Phase 2 | ✅ |
| 13 | BatchGetSkillGuidance + N+1 优化 | P4 | Phase 2 | ✅ |
| 14 | 统一相似度引擎（4 维 + Embedding） | P4′ | Phase 2′ | ✅ |
| 15 | 三阶段合并（SkillMergeUsecase） | P4′ | Phase 2′ | ✅ |
| 16 | 统一进化编排（SkillEvolutionOrchestrator） | P4′ | Phase 2′ | ✅ |
| 17 | ScoreSkill 四维权重修复 | P4′ | Phase 2′ | ✅ |
| 18 | 健康指标（GetSkillHealth RPC） | P4′ | Phase 2′ | ✅ |
| 19 | 去重缓存（10min TTL + InvalidateDedupCache） | P4′ | Phase 2′ | ✅ |
| 20 | Budget 中间件 | P4+ | Phase 3 | ☐ |
| 21 | Skill 依赖 / 冲突表 | P4+ | Phase 3 | ☐ |
| 22 | 自动负熵报告 | P4+ | Phase 3 | ☐ |
| 23 | SkillBackend 多 kind 差异化 | P4+ | Phase 3 | ☐ |
| 24 | Feedback 真实接入 | P4+ | Phase 3 | ☐ |
| 25 | EvolutionCoordinator 清理 | P4+ | Phase 3 | ✅（A6） |
| 26 | R2 进化链路修复（F3 磁盘 watcher 回滚进化成果 / F6 审批草稿冻结 / F8 模板降级可观测 / F9 冷却期过滤终态 / F10 沙盒 validator 标注） | P-evo | — | ✅（2026-07-27，见 [phase3-进化能力/06 §9](./phase3-进化能力/06-P0-LLM-Curator与Reload接线.design.md#9-r2-测试修复2026-07-27)） |
| 27 | 标签字典（治理表 + 事务重写 + 孤儿治理 + 管理页 + 选项源复用） | P5 | Phase 2″ | ✅（2026-07-28） |
| 28 | P2 进化验证强化与触发扩展（AB 对照回放棘轮 / 漂移检测 / 成功沉淀触发器 / 触发率黄金集回归） | P-evo | — | ⏳（2026-08-09 启动，设计 [phase3-进化能力/08](./phase3-进化能力/08-P2-进化验证强化与触发扩展.design.md)） |

---

## 6. 验收标准

### 已达成

- [x] Skill 列表/详情/编辑/导入/运行记录页可用
- [x] Agent 设置页可配置 `skill_runtime_json`
- [x] 运行时按 policy + 用户话术收窄可见 Skill（Layer A/B）
- [x] `go test ./internal/tools/skillruntime/... ./internal/agent/...` 通过

### Phase 1（P3）— ✅ 已达成

- [x] 版本历史列表与回滚（不可变策略：新建版本 + patch 递增 + 事务保护）
- [x] 权限由 RBAC 控制（`requireAdminAccess` + 未认证零权限）
- [x] Ent 字段补齐（`visibility`/`default_config_json`/`file_manifest_json`/`message_id`/`parent_version_id`/`evolution_reason`/`lifecycle_status`）
- [ ] Import 端点在 proto/OpenAPI 完整声明（保留手动注册，已补齐 admin 校验 + 指标）

### Phase 2（P4）— ✅ 已达成

- [x] Prompt 注入方式 C（BeforeModelHook + `BatchGetSkillGuidance` 批量获取 + 截断 + 空 guidance 防护）
- [x] Preview 返回选中原因（`ResolveSkillSlugsDetailed` + `Reasons map[string]string`）
- [x] Embedding 语义精排（`SkillEmbedder` + `ScoreByEmbedding` + 内存缓存 + 评分融合 + 优雅降级）
- [x] manifest/render 包（frontmatter 解析 + 变量替换 + prompt 渲染）
- [x] Repo 窄接口拆分（`SkillReader` + `SkillWriter`）
- [x] SkillFilesystem 端口下沉（Service 层不再直接操作 `os` 包）
- [x] N+1 查询优化（`BatchGetSkillMarkdownBySlugs` 批量获取）

### Phase 2′（P4′）— ✅ 已达成

- [x] 统一相似度引擎（4 维 Jaccard + 可选 Embedding 混合，阈值常量 `similarityHighThreshold=0.8` 等）
- [x] 三阶段合并（内容融合 → Gate 验证 → 事务应用，4 步操作在单个事务内）
- [x] 统一进化编排（`SkillEvolutionOrchestrator` 线程安全 + 原子化检查 + 4 个窄接口拆分）
- [x] ScoreSkill 四维权重（SuccessRate 0.4 + Duration 0.25 + Token 0.2 + Feedback 0.15，条件启用）
- [x] 健康指标（`GetSkillHealth` RPC + 7d/30d 调用统计 + 成功率 + P95 耗时）
- [x] 去重缓存（10min TTL + `InvalidateDedupCache()` 手动失效）

### Phase 2″（P5 标签字典）— ✅ 已达成

- [x] 字典 CRUD 4 RPC（`/v1/skill-tags` 独立前缀，未被 `/v1/skills/{id}` 吞掉）
- [x] List 返回实时使用计数 + 孤儿标签合成（`source=orphan` 不落库），按 dimension + name 排序
- [x] 改名在同一事务内重写所有 Skill `metadata_json.tags`，目标已存在时删除源行等价合并，返回重写条数
- [x] 删除从事务内所有 Skill 引用中移除标签，返回重写条数
- [x] 孤儿标签一键收录（以同名预建）或删除清理
- [x] 管理页 `/skills/tags`：维度分组 + 搜索 + 收录状态筛选 + 汇总 chip（总数 / 未收录数）
- [x] Skill 编辑、列表筛选、Agent 设置三处标签下拉复用字典选项源，仍允许输入新标签
- [x] Rename/Delete 后失效 embed 路由缓存与去重缓存
- [x] `go test ./internal/data/ -run TestSkillTag` 及 biz/service 相关测试通过

### Phase 3（P4+）— 待实现

- [ ] Budget 中间件（token 上限裁剪）
- [ ] Skill 依赖 / 冲突表（安装时检查 + 运行时互斥）
- [ ] 自动负熵报告（聚合指标 + 趋势）
- [ ] SkillBackend 多 kind 差异化加载与渲染
- [ ] Feedback 真实接入（替换 `TEMPORARY` 启发式估算）
- [x] EvolutionCoordinator 清理（已随 A6 物理收敛删除 `evolution_coordinator.go` + fallback 逻辑）

---

## 7. 依赖与风险

| 依赖/风险 | 说明 |
|-----------|------|
| RBAC | ✅ 已接入 `requireAdminAccess` + 未认证零权限 |
| 向量库 | ✅ embedding 精排已通过 `SkillEmbedder` 接口接入 `knowledge.Embedder`，复用现有 Embedding 基础设施 |
| 版本回滚 | ✅ 不可变策略（新建版本 + patch 递增），事务保护 |
| Filter 缓存 | `AgentVisibilityFilter` 按 invocationID 缓存；长跑进程需关注内存（后续可加 TTL） |
| Embedding 缓存 | `Usecase.embedCache` 按 slug 缓存 embedding；Publish/ToggleEnabled/Delete/Duplicate 时失效 |
| N+1 查询 | ✅ 已通过 `BatchGetSkillMarkdownBySlugs` 批量获取解决 |
| Service 层文件 I/O | ✅ 已通过 `SkillFilesystem` 端口下沉到 storage 层，Service 层不再直接操作 `os` 包 |
| Repo 接口膨胀 | ✅ 已拆分为 `SkillReader` + `SkillWriter` 窄接口，`Repo` 组合两者；进化接口拆分为 4 个窄接口 |
| Wire 绑定 | ✅ `ProvideSkillResolveRootFn` + `storage.NewSkillFilesystem`，动态解析 root_directory |
| 进化触发器并发 | ✅ `SkillEvolutionOrchestrator` 使用 `sync.RWMutex` 保护 triggers 切片 + 快照读取；DB UNIQUE 约束兜底 |
| 进化协调器遗留 | ✅ `evolution_coordinator.go` 已随 A6 物理收敛删除（含 fallback 逻辑）；跨流水线去重统一由 `SkillEvolutionOrchestrator` + trigger 内去重 + DB 唯一索引承担 |
| ScoreSkill Feedback | ⚠️ Feedback 维度当前为 `TEMPORARY` 启发式估算，待接入真实用户反馈 |
| 去重 O(n²) 扫描 | ✅ 已通过 10min TTL 内存缓存缓解；外部可通过 `InvalidateDedupCache()` 手动失效 |
| 标签字典计数聚合 | ✅ 使用计数实时扫描 `metadata_json.tags` 不落库，强一致；改名/删除走 `ExecInTx` 事务重写，失败整体回滚；治理后 embed/去重缓存同步失效 |

---

## 8. 改动文件清单

> 以下为 Skill 模块的代码文件清单（与设计文档 §十五 Go 包布局一致）。状态标记：✅ 已实现 / 🟡 部分实现 / ☐ 待实现。

### 8.1 Proto 契约

| 文件 | 说明 | 状态 |
|------|------|------|
| `api/kratos/skill/v1/skill.proto` | SkillService 26 RPC（含标签字典 4 RPC）+ 消息定义 | ✅ |
| `api/kratos/system_setting/v1/system_setting.proto` | `work_directory` 字段（存储根解析依赖） | ✅ |

### 8.2 Service 层（薄适配）

| 文件 | 说明 | 状态 |
|------|------|------|
| `internal/service/skill.go` | 23 RPC 适配（CRUD/发布/预览/版本/回滚/健康/磁盘健康 + 标签字典 4 RPC） | ✅ |
| `internal/service/skill_import.go` | 导入 biz 桥接（4 RPC） | ✅ |
| `internal/service/skill_import_http.go` | multipart POST `/v1/skills/import` 挂载 | ✅ |
| `internal/service/skill_intelligence.go` | 智能分析服务 | ✅ |
| `internal/service/skill_evolution.go` | 进化服务 | ✅ |
| `internal/service/skill_evolution_suggestion.go` | 进化建议服务 | ✅ |
| `internal/service/skill_curator.go` | 策展服务 | ✅ |
| `internal/service/skill_dedup.go` | 去重服务 | ✅ |
| `internal/service/skill_health_metrics_adapter.go` | 健康指标适配器 | ✅ |
| `internal/service/skills_butler_adapter.go` | 管家适配器 | ✅ |

### 8.3 Biz 层（用例 + 端口）

| 文件 | 说明 | 状态 |
|------|------|------|
| `internal/biz/skill/skill.go` | 用例与端口（SkillReader/SkillWriter/Repo 接口、Usecase、DTO、SkillFilesystem、SkillEmbedder） | ✅ |
| `internal/biz/skill/skill_test.go` | 单元测试 | ✅ |
| `internal/biz/skill.go` | 类型别名 + 常量 + 构造函数 | ✅ |
| `internal/biz/skill_similarity.go` | 统一相似度引擎（4 维 + 可选 Embedding） | ✅ |
| `internal/biz/skill_merge.go` | 三阶段合并 Usecase | ✅ |
| `internal/biz/skill_merge_ai_fuser.go` | 基于规则的内容融合器 | ✅ |
| `internal/biz/skill_evolution_unified.go` | 统一进化编排器 + UnifiedEvolutionSuggestion + Reader/Writer 接口 | ✅ |
| `internal/biz/skill_evolution_triggers.go` | EvolutionTrigger 策略（Pattern/Health/AgentConfig）+ SkillScorer 窄接口 | ✅ |
| `internal/biz/skill_intelligence.go` | SkillIntelligenceUsecase（ScoreSkill 四维权重，含 Token/Feedback 条件启用；L2 视图重建 `unifiedToLegacySuggestionPtr`，A6） | ✅ |
| `internal/biz/skill_evolution.go` | SkillEvolutionUsecase + L1 视图重建 `skillProposalFromUnified`（A6） | ✅ |
| `internal/biz/skill_dedup.go` | SkillDedupUsecase（DetectDuplicateGroups 带 10min TTL 缓存）；MergeSkills Deprecated | ✅ |
| `internal/biz/skill_health.go` | SkillHealthUsecase | ✅ |
| `internal/biz/skill_scoring.go` | SkillScorer 窄接口 | ✅ |
| `internal/biz/skill_report.go` | 报告 | ✅ |
| `internal/biz/skill_load_mode.go` | 加载模式 | ✅ |
| `internal/biz/skill_invocation_stats.go` | 调用统计 | ✅ |
| `internal/biz/skill/tag.go` | 标签字典端口（`SkillTagReader`/`SkillTagWriter`/`TagRepo`、`TagInfo`、normalizeTagName、Rename/Delete 后缓存失效） | ✅ |

### 8.4 Data 层（仓储 + 聚合）

| 文件 | 说明 | 状态 |
|------|------|------|
| `internal/data/skill.go` | Ent 仓储与聚合 | ✅ |
| `internal/data/skill_merge.go` | 合并 Data 层（事务内 4 步操作） | ✅ |
| `internal/data/skill_dedup.go` | 去重 Data 层（含 SkillSimilarityEngine 集成） | ✅ |
| `internal/data/skill_intelligence.go` | 健康指标聚合（含 AvgTokenUsage/FeedbackScore） | ✅ |
| `internal/data/skill_health.go` | 健康 Data 层 | ✅ |
| `internal/data/skill_invocation_stats.go` | 调用统计 Data 层 | ✅ |
| `internal/data/skill_import_job.go` | 导入任务 Data 层 | ✅ |
| `internal/data/skill_evolution_schema.go` | legacy `skill_proposals` DDL（仅作迁移 20261111 backfill 来源，backfill 后 DROP；A6 起不再承载读写） | ✅ |
| `internal/data/unified_evolution.go` | 统一进化 Data 层（raw SQL + 读写分离；A6 起承载全部四类建议读写，legacy `skill_evolution.go` / `skill_evolution_suggestion.go` 已删除） | ✅ |
| `internal/data/unified_evolution_schema.go` | 统一进化 Schema | ✅ |
| `internal/data/skill_tag_repo.go` | 标签字典 Data 层（`skillTagUsage` 实时聚合 + `rewriteSkillTagReferences` 事务重写 + 孤儿合成） | ✅ |

### 8.5 Ent Schema（物理表）

| 文件 | 表名 | 状态 |
|------|------|------|
| `internal/data/ent/schema/platform_skill.go` | `skill` | ✅ |
| `internal/data/ent/schema/skill_version.go` | `skill_version` | ✅ |
| `internal/data/ent/schema/skill_invocation.go` | `skill_invocation` | ✅ |
| `internal/data/ent/schema/skill_import_job.go` | `skill_import_jobs` | ✅ |
| `internal/data/ent/schema/skill_tag.go` | `skill_tags`（标签字典治理表） | ✅ |

> A6：`internal/data/ent/schema/skill_evolution_suggestion.go`（`skill_evolution_suggestions`）与 `evolution_suggestion.go`（`evolution_suggestions`）Ent Schema 已删除，物理表经迁移 20261111 backfill 后 DROP，统一收敛到 `unified_evolution_suggestions`（raw SQL DDL，非 Ent）。

### 8.6 Skill 领域包（导入/监听/存储/渲染/桥接）

| 文件 | 说明 | 状态 |
|------|------|------|
| `internal/skill/importer/` | ZIP 导入引擎（engine / validate / helpers / chat / errors） | ✅ |
| `internal/skill/watch/` | Skill 根目录监听与磁盘同步（runner / reporter / reconcile） | ✅ |
| `internal/skill/storage/` | Skill 存储根解析 + SkillFilesystem 实现（root / filesystem） | ✅ |
| `internal/skill/manifest/` | frontmatter / skill.json 解析与校验 | ✅ |
| `internal/skill/render/` | prompt 块渲染、截断策略 | ✅ |
| `internal/skill/trpc/repository.go` | `FSRepositoryAdapter` — 磁盘 FS → `trpcskill.Repository` | ✅ |
| `internal/skill/trpc/db_repository.go` | `DBRepositoryAdapter` — DB + TTL 缓存 → `trpcskill.Repository` | ✅ |
| `internal/skill/trpc/filter.go` | `NewFilteredRepository(base, allowedSlugs)` → `trpcskill.ContextRepository` | ✅ |
| `internal/skill/trpc/tools.go` | `BuildSkillTools()` 产出 4 个内置 Skill 工具 | ✅ |
| `internal/skill/trpc/executor.go` | `CodeExecutor` 适配（local / docker） | ✅ |
| `internal/skill/trpc/artifact_executor.go` | 产出物 `WrapWithArtifactSave` | ✅ |
| `internal/skill/fs_registrar.go` | 文件系统登记 | ✅ |
| `internal/skill/auto_creator.go` | 自动创建 | ✅ |

### 8.7 运行时路由与工具

| 文件 | 说明 | 状态 |
|------|------|------|
| `internal/tools/skillruntime/` | 运行时装配入口（toolset / resolve / filter / runtime） | ✅ |
| `internal/tools/skillrouter/` | 意图路由与分类（detect / taxonomy） | ✅ |
| `internal/tools/skills_butler/` | Skill 管家（registry / recommend / analyze / optimize / evolve） | ✅ |
| `internal/tools/skillrecommend/` | Skill 推荐（rank / rank_feedback / health_provider） | ✅ |

### 8.8 Agent 装配

| 文件 | 说明 | 状态 |
|------|------|------|
| `internal/agent/trpc_build.go` | Agent 构建中 Skill 装配（`buildSkillDeps`） | ✅ |
| `internal/agent/skill_guidance_inject.go` | Prompt 注入方式 C（BeforeModelHook + `BatchGetSkillGuidance`） | ✅ |

### 8.9 前端

| 文件 | 说明 | 状态 |
|------|------|------|
| `web/src/pages/SkillsPage.vue` | 列表 + 上传 + 编辑 Dialog | ✅ |
| `web/src/pages/SkillDetailPage.vue` | Skill 详情页 | ✅ |
| `web/src/pages/SkillRunsPage.vue` | 运行记录页 | ✅ |
| `web/src/pages/SkillTagsPage.vue` | 标签字典管理页（维度分组 + 孤儿治理 + 新建/改名/删除/收录） | ✅ |
| `web/src/pages/agent-settings/AgentSettingsSkillsTab.vue` | `skill_runtime_json` 配置 | ✅ |
| `web/src/components/skills/SkillTable.vue` 等 | 表格/筛选/统计/编辑/上传/删除/运行记录/告警/健康卡片 | ✅ |
| `web/src/features/skills/api.ts` | 前端 API 函数清单 | ✅ |
| `web/src/features/skills/types.ts` | TypeScript 类型定义 | ✅ |
| `web/src/features/skills/useSkillsPage.ts` 等 | Composable hooks | ✅ |
| `web/src/stores/skills/index.ts` | Pinia store | ✅ |

### 8.10 测试文件

| 文件 | 说明 | 状态 |
|------|------|------|
| `internal/biz/skill/skill_test.go` | Usecase 单元测试 | ✅ |
| `internal/biz/skill_evolution_test.go` | 进化用例测试 | ✅ |
| `internal/biz/skill_evolution_loop_test.go` | 进化循环测试 | ✅ |
| `internal/biz/skill_intelligence_test.go` | 智能评分测试 | ✅ |
| `internal/biz/skill_load_mode_test.go` | 加载模式测试 | ✅ |
| `internal/service/skill_test.go` | Service 适配层测试 | ✅ |
| `internal/service/skill_intelligence_test.go` | 智能分析服务测试 | ✅ |
| `internal/service/skill_intelligence_integration_test.go` | 智能分析集成测试 | ✅ |
| `internal/service/skill_evolution_test.go` | 进化服务测试 | ✅ |
| `internal/service/skill_curator_test.go` | 策展服务测试 | ✅ |
| `internal/agent/skill_guidance_inject_test.go` | Prompt 注入测试 | ✅ |
| `internal/tools/cli_admin/skill_install_from_url_test.go` | CLI 安装测试 | ✅ |
| `internal/cli/client/skill_test.go` | CLI 客户端测试 | ✅ |
| `internal/data/skill_tag_repo_test.go` | 标签字典 Data 层测试（聚合/重写/合并/孤儿/删除语义） | ✅ |

---

## 9. 测试关注点

### 9.1 已覆盖

| 关注点 | 测试文件 | 说明 |
|--------|---------|------|
| Usecase 核心逻辑 | `internal/biz/skill/skill_test.go` | CRUD/发布/版本/回滚/磁盘同步 |
| 进化编排 | `internal/biz/skill_evolution_test.go` · `skill_evolution_loop_test.go` | Trigger 检测 + 原子化检查 + 审批/拒绝/过期 |
| 智能评分 | `internal/biz/skill_intelligence_test.go` | ScoreSkill 四维权重 + Token 归一化 |
| 加载模式 | `internal/biz/skill_load_mode_test.go` | 加载模式切换 |
| Service 适配 | `internal/service/skill_test.go` | Proto Request → Biz DTO 转换 |
| 智能分析服务 | `internal/service/skill_intelligence_test.go` · `skill_intelligence_integration_test.go` | 服务层 + 集成测试 |
| 进化服务 | `internal/service/skill_evolution_test.go` | 进化服务适配 |
| 策展服务 | `internal/service/skill_curator_test.go` | 策展服务适配 |
| Prompt 注入 | `internal/agent/skill_guidance_inject_test.go` | BeforeModelHook + 截断 + 空 guidance 防护 |
| CLI 安装 | `internal/tools/cli_admin/skill_install_from_url_test.go` | URL 安装 Skill |
| CLI 客户端 | `internal/cli/client/skill_test.go` | CLI 客户端调用 |
| 标签字典 | `internal/data/skill_tag_repo_test.go` | 使用计数聚合、改名合并/重写、删除清理、孤儿合成、其他 metadata 键保留 |

### 9.2 待补充

| 关注点 | 说明 | 优先级 |
|--------|------|--------|
| 三阶段合并 | `SkillMergeUsecase` 内容融合 → Gate 验证 → 事务应用，缺少专门测试 | P4+ |
| 统一相似度引擎 | `SkillSimilarityEngine` 4 维 Jaccard + Embedding 混合，缺少阈值边界测试 | P4+ |
| 运行时路由 | `ResolveSkillSlugsDetailed` Layer A/B + Embedding 精排，缺少端到端测试 | P4+ |
| 磁盘监听 | `watch.Runner` fsnotify + debounce + reconcile，缺少并发场景测试 | P4+ |
| ZIP 导入 | `importer.Engine` 解压 → 校验 → 相似度 → 冲突分组，缺少完整流程测试 | P4+ |
| 技能管家工具 | `skills_butler` 8 工具中 `recommend_skills` 已补单测（6 用例，2026-07-30 M6-5 ✅）；其余 7 工具（analyze_* / optimize_* / evolve_skill）仍缺单测 | P1 |

### 9.2.1 技能管家 prompt 对齐（2026-07-29，M6-5）

核查结论：`internal/tools/skills_butler/` 8 工具全部实现并经 `cli_admin_tools.go` 装配（仅 `agent_key=__skills__` 挂载）；`recommend_skills` 已实现（pending proposals + 调用统计健康度双源推荐）。唯一漂移：prompt（`internal/scenario/system/prompts/skills/skills.md`）声明了未实现的 `retire_skill`，本期移除并对齐工作流描述；补 `recommend_skills` 单测。

### 9.3 验证命令

```bash
# Biz 层测试
go test ./internal/biz/skill/... -count=1
go test ./internal/biz/ -run TestSkill -count=1

# Service 层测试
go test ./internal/service/ -run TestSkill -count=1

# Agent 层测试
go test ./internal/agent/... -count=1

# 运行时路由测试
go test ./internal/tools/skillruntime/... ./internal/tools/skillrouter/... -count=1
```

---

*文档版本：4.0 — 三件套重组：迁入设计文档的演进路线/测试关注点；新增改动文件清单；RPC 数量对齐代码现状（22 RPC）；补全 Phase 2′ 智能增强阶段（2026-06-17）。*
