# Skill 技能 — 开发计划

> **版本**：2026-05-29 | **状态**：✅ 管理面 + 运行时装配（Layer A/B）+ Phase 1 + Phase 2 + 架构优化已接通
> **需求**：[20 skill.md](./20%20skill.md) · **设计**：[20 skill.design.md](./20%20skill.design.md) · **架构**：[20 skill struct design.md](./20%20skill%20struct%20design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **变更**：[changelog/2026-05-21-Skill-DocSync-RuntimeWire.md](../changelog/2026-05-21-Skill-DocSync-RuntimeWire.md)

---

## 1. 模块定位

Skill 技能系统：管理 Agent 可用的能力包（SKILL.md + 附件），支持创建、导入、发布、启用/停用、运行时路由与 trpc-agent-go 装配、执行追踪。

**核心链路**：注册（CRUD / ZIP 导入）→ 发布 → 启用 → 运行时路由（Layer A + Layer B）→ trpc-agent-go Skill 工具链 → 执行追踪。

**代码锚点**：

| 层级 | 路径 |
|------|------|
| Proto | `api/kratos/skill/v1/skill.proto` — SkillService **18 RPC** |
| Service | `internal/service/skill.go` · `skill_import.go` · `skill_import_http.go` |
| Biz | `internal/biz/skill.go` · `skill_import.go` · `skill_runtime.go` |
| Data | `internal/data/skill.go` |
| 桥接 | `internal/skill/trpc/` — Repository / Filter / Tools / Executor / `artifact_executor` |
| 监听 | `internal/skill/watch/` · `internal/skill/storage/` · `internal/skill/importer/` |
| 运行时路由 | `internal/tools/skillruntime/` · `internal/tools/skillrouter/` |
| Agent 装配 | `internal/agent/trpc_build.go` → `buildSkillDeps` + `skillruntime.NewAgentVisibilityFilter` |
| 前端 | `web/src/pages/SkillsPage.vue` · `SkillRunsPage.vue` · `components/skills/*` · `features/skills/api.ts` · `AgentSettingsSkillsTab.vue` |

---

## 2. 现状评估（2026-05-21）

| 项 | 状态 | 证据 |
|----|------|------|
| Skill CRUD + 文件读写 | ✅ | 18 RPC；列表/详情/创建/更新/发布/启用/复制/删除/文件/运行记录/预览 |
| ZIP 导入 + 冲突 + AI 炼化 | ✅ | proto 声明 3/4 端点；`POST /v1/skills/import` multipart 由 `RegisterSkillImportMultipart` |
| Layer A + B 路由 | ✅ | `ResolveSkillSlugs` + `NewAgentVisibilityFilter`；turn query 经 `RunOptionWithTurnQuery` 注入 RuntimeState |
| trpc-agent-go 桥接 | ✅ | `DBRepositoryAdapter` / `FSRepositoryAdapter` + `WithSkills` / `WithSkillFilter` |
| 文件监听与同步 | ✅ | `watch.Runner`：fsnotify + debounce + `filesystem_missing` |
| 磁盘同步 UI + 通知 | ✅ | §2.4；health API / Banner / 筛选 / Monitor 事件 |
| 存储根解析 | ✅ | `storage.ResolveRootWithPlatform` + `work_directory` |
| Agent 设置 UI | ✅ | `AgentSettingsSkillsTab`：`skill_runtime_json` 白名单/标签/意图收窄 |
| 前端管理 | ✅ | 列表/编辑 Dialog/导入/运行记录 |
| 版本历史 / 回滚 API | ✅ | `GetSkillVersions` + `RollbackSkillVersion`；不可变策略（新建版本 + patch 递增） |
| RBAC 权限 | ✅ | `requireAdminAccess` + `applySkillPermission`；未认证返回零权限 |
| Preview 选中原因 | ✅ | `ResolveSkillSlugsDetailed` 返回 `Reasons map[string]string` + `agent_id` 关联 |

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
| **D1 文档** | `20 skill.md` §2.4、struct design §2.6 通知链路 | ✅ |
| **D2 API** | proto：`filesystem_missing`、`sync_origin`、列表筛选、`GetSkillFilesystemHealth` | ✅ |
| **D3 后端** | enrichSkill 映射；watch 目录 slug 约束；EventBus + Monitor 事件；wire `NewRunnerWithBus` | ✅ |
| **D4 前端** | Banner、来源/磁盘列、筛选、`filesystem-health` 拉取 | ✅ |
| **D5 后续** | reconcile ticker；磁盘更新回退 draft；异步相似度 warn；Alert Webhook | ✅ |

### 3.3 待实现（P3–P4）

| # | 差距 | 优先级 | 说明 |
|---|------|--------|------|
| 1 | 版本历史 / 回滚 API | P3 | ✅ | `GetSkillVersions` + `RollbackSkillVersion`；事务保护 |
| 2 | RBAC 权限 | P3 | ✅ | `requireAdminAccess` + 未认证零权限 |
| 3 | Import multipart 完全 codegen | P3 | 🟡 | 保留手动注册，已补齐 admin 校验 + 指标 |
| 4 | 未落地 Ent 字段 | P3 | ✅ | `visibility`/`default_config_json`/`file_manifest_json`/`message_id` |
| 5 | 自动负熵报告 | P3 | ❌ | 聚合指标 + 趋势 |
| 6 | Prompt 注入（方式 C） | P4 | ✅ | BeforeModelHook + `BatchGetSkillGuidance` 批量获取 |
| 7 | embedding 语义精排 | P4 | ✅ | `SkillEmbedder` + `ScoreByEmbedding` + 评分融合 |
| 8 | Budget 中间件 | P4 | ❌ | token 上限裁剪 |
| 9 | Preview API 增强 | P4 | ✅ | `Reasons map[string]string` + `agent_id` |
| 10 | Skill 依赖 / 冲突表 | P4 | ❌ | 安装时检查 + 运行时互斥 |
| 11 | `internal/skill/manifest/` · `render/` | P4 | ✅ | frontmatter 解析与 prompt 渲染 |

---

## 4. 开发阶段

### Phase 1：版本 + 权限 + 导入收尾（P3）

- 版本历史与回滚 RPC
- RBAC 接入
- Import multipart 收敛或 OpenAPI 聚合文档补齐
- Ent 字段补齐（`visibility` 等）

### Phase 2：运行时增强（P4）

- Prompt 注入方式 C
- embedding 精排、Budget 中间件
- Preview 返回选中原因

### Phase 3：生态扩展（P4+）

- Skill 依赖声明与冲突策略
- Skill 市场（Ecosystem 联动）
- 可选：`internal/skill/**` → `internal/capability/skill/**` 目录迁移

---

## 5. 任务清单

| # | 任务 | 优先级 | 阶段 | 状态 |
|---|------|--------|------|------|
| 1 | Layer A/B 接通 `buildSkillDeps` + turn query | P2 | — | ✅ |
| 2 | 文档四件套与代码对齐 | P0 | — | ✅ |
| 3 | `GetSkillVersions` + `RollbackSkillVersion` | P3 | Phase 1 | ✅ |
| 4 | RBAC 替换硬编码权限 | P3 | Phase 1 | ✅ |
| 5 | Import multipart codegen | P3 | Phase 1 | 🟡 |
| 6 | Ent 字段补齐 | P3 | Phase 1 | ✅ |
| 7 | Prompt 注入方式 C | P4 | Phase 2 | ✅ |
| 8 | embedding 精排 | P4 | Phase 2 | ✅ |
| 9 | Preview 选中原因 | P4 | Phase 2 | ✅ |
| 10 | manifest/render 包 | P4 | Phase 2 | ✅ |

---

## 6. 验收标准

### 已达成

- [x] Skill 列表/编辑/导入/运行记录页可用
- [x] Agent 设置页可配置 `skill_runtime_json`
- [x] 运行时按 policy + 用户话术收窄可见 Skill（Layer A/B）
- [x] `go test ./internal/tools/skillruntime/... ./internal/agent/...` 通过

### Phase 1（P3）

- [x] 版本历史列表与回滚（不可变策略：新建版本 + patch 递增 + 事务保护）
- [x] 权限由 RBAC 控制（`requireAdminAccess` + 未认证零权限）
- [x] Ent 字段补齐（`visibility`/`default_config_json`/`file_manifest_json`/`message_id`）
- [ ] Import 端点在 proto/OpenAPI 完整声明（保留手动注册，已补齐 admin 校验 + 指标）

### Phase 2（P4）

- [x] Prompt 注入方式 C（BeforeModelHook + `BatchGetSkillGuidance` 批量获取 + 截断 + 空 guidance 防护）
- [x] Preview 返回选中原因（`ResolveSkillSlugsDetailed` + `Reasons map[string]string`）
- [x] Embedding 语义精排（`SkillEmbedder` + `ScoreByEmbedding` + 内存缓存 + 评分融合 + 优雅降级）
- [x] manifest/render 包（frontmatter 解析 + 变量替换 + prompt 渲染）

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
| Repo 接口膨胀 | ✅ 已拆分为 `SkillReader` + `SkillWriter` 窄接口，`Repo` 组合两者 |
| Wire 绑定 | ✅ `ProvideSkillResolveRootFn` + `storage.NewSkillFilesystem`，动态解析 root_directory |
