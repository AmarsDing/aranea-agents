# Skill 技能 — 开发计划

> **版本**：2026-05-18 | **状态**：✅ 端到端可用（P0–P2′ 已完成）
> **需求**：[20 skill.md](./20%20skill.md) · **设计**：[20 skill.design.md](./20%20skill.design.md) · **架构**：[20 skill struct design.md](./20%20skill%20struct%20design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Skill 技能系统：管理 Agent 可用的能力包（SKILL.md + 附件文件），支持技能的创建、导入、发布、启用/停用、运行时路由与装配、执行追踪。

**核心链路**：注册（CRUD / ZIP 导入）→ 发布 → 启用 → 运行时路由（Layer A + Layer B）→ ADK Toolset 装配 → 执行追踪。

**代码锚点**：
- `api/kratos/skill/v1/skill.proto` — SkillService 14 RPC
- `internal/service/skill.go` — SkillService（薄适配）
- `internal/biz/skill.go` — SkillUsecase + SkillRepo 接口
- `internal/biz/skill_runtime.go` — SkillRuntimePolicy + SkillRuntimeCandidate
- `internal/biz/skill_import.go` — 导入 DTO
- `internal/data/skill.go` — SkillRepo 实现（Ent 仓储）
- `internal/skill/trpc/` — trpc-agent-go Skill 桥接（Repository / DBRepository / Filter / Tools / Executor）
- `internal/skill/watch/` — 文件系统监听与磁盘同步
- `internal/skill/storage/` — Skill 存储根解析
- `internal/skill/importer/` — ZIP 导入引擎
- `internal/tools/skillruntime/` — 运行时装配入口（Layer A + Layer B 路由）
- `internal/tools/skillrouter/` — 意图路由与分类
- `internal/agent/trpc_build.go` — Skill 装配集成（buildSkillDeps）
- `internal/server/skill_import_http.go` — ZIP 导入 HTTP 路由

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Skill CRUD（14 RPC） | ✅ | List/Get/Create/Update/Publish/ToggleEnabled/Duplicate/Delete + 文件读写删 + 运行时预览 + 运行记录 |
| ZIP 导入 + 冲突检测 + AI 炼化 | ✅ | 4 个 HTTP 端点（import / status / apply / refine） |
| 运行时路由（Layer A + Layer B） | ✅ | `skillruntime.resolveSkillSlugs`：allow/deny + 意图路径 + 标签过滤 + 评分排序 |
| trpc-agent-go 桥接 | ✅ | `FSRepositoryAdapter` + `DBRepositoryAdapter` + `FilteredRepository` + `BuildSkillTools` |
| 文件系统监听与同步 | ✅ | `watch.Runner`：fsnotify + debounce 2s + eventBus + filesystem_missing 标记 |
| 存储根解析 | ✅ | `storage.ResolveRootWithPlatform`：env + work_directory + OS 默认 |
| CodeExecutor | ✅ | local / docker，按 `CODE_EXECUTOR_BACKEND` 选择 |
| 版本管理（基础） | ✅ | `skill_version` 表 + `PublishSkill` 生成版本；版本回滚 API 待实现 |
| 前端管理 | ✅ | Skill 列表 / 编辑 / 导入 / 运行记录页 |

---

## 3. 差距与优化

### 3.1 已完成（原 P0–P2′）

| 阶段 | 内容 | 状态 |
|------|------|------|
| **P0** | 存储根解析接通 `work_directory` | ✅ |
| **P1** | 补齐 proto：Create/Update/Publish/Get/DeleteFile/PreviewRuntime | ✅ |
| **P2** | 运行时路由 + DBRepositoryAdapter + Agent 集成 | ✅ |
| **P2′** | 文件系统监听 + debounce + eventBus + filesystem_missing | ✅ |

### 3.2 待实现（P3–P4）

| # | 差距 | 优先级 | 说明 |
|---|------|--------|------|
| 1 | 版本历史 / 回滚 API | P3 | `GetSkillVersions` + `RollbackSkillVersion` RPC；本期只展示当前版本号 |
| 2 | 权限接入 RBAC | P3 | Proto 已定义 `SkillPermissions` / `SkillInvocationPermissions`；当前 data 层硬编码 `CanEdit=true` |
| 3 | 自动负熵报告 | P3 | 本期只展示已有聚合指标（invoke/success/failure/avg_duration） |
| 4 | Prompt 注入（方式 C） | P4 | Assembler 产出 `## Available Skills` 文本块写入 system/developer message |
| 5 | embedding 语义精排 | P4 | 候选筛选增加向量相似度匹配，替换或增强 `scoreCandidates` |
| 6 | Budget 中间件 | P4 | 注入 token 上限裁剪，按 Skill 优先级与 token 预算动态调整 |
| 7 | Preview API 增强 | P4 | 返回每个 Skill 的选中原因（`Reasons map[string]string`） |
| 8 | Skill 依赖声明 | P4 | `required_skill` / `optional_skill` / `tool_capability` / `runtime_feature` |
| 9 | Skill 冲突表扩展 | P4 | 运行时互斥策略；分级 info/warn/block |
| 10 | ZIP 导入路由收敛进 proto | P3 | 4 个手写 HTTP 端点建议逐步 code-gen 或补 proto |
| 11 | 未落地 Ent 字段 | P3 | `skill.current_version_id`、`skill.visibility`、`skill.default_config_json`、`skill_version.file_manifest_json`、`skill_invocation.message_id` |
| 12 | 规划新包 | P4 | `internal/skill/manifest/`（frontmatter 解析）、`internal/skill/render/`（prompt 渲染） |

---

## 4. 开发阶段

### Phase 1：版本管理 + 权限 + 导入收敛（P3）

- 实现版本历史列表与回滚 API
- 接入 RBAC 替换硬编码权限
- ZIP 导入 4 端点收敛进 proto 或 OpenAPI 聚合文档
- 补齐未落地 Ent 字段（`visibility`、`default_config_json` 等）

### Phase 2：运行时增强（P4）

- Prompt 注入方式 C 实现
- embedding 语义精排替换/增强关键词路由
- Budget 中间件（token 上限裁剪）
- Preview API 增强返回选中原因

### Phase 3：生态扩展（P4+）

- Skill 依赖声明与安装时检查
- Skill 冲突表扩展（运行时互斥策略）
- Skill 市场/分享机制（需与 Ecosystem 模块联动）
- Context 目录迁移（`internal/skill/**` → `internal/capability/skill/**`）

---

## 5. 任务清单

| # | 任务 | 优先级 | 阶段 | 依赖 |
|---|------|--------|------|------|
| 1 | `GetSkillVersions` RPC + biz/data 实现 | P3 | Phase 1 | — |
| 2 | `RollbackSkillVersion` RPC + biz/data 实现 | P3 | Phase 1 | #1 |
| 3 | RBAC 权限接入（替换硬编码 `CanEdit` 等） | P3 | Phase 1 | — |
| 4 | ZIP 导入端点收敛进 proto | P3 | Phase 1 | — |
| 5 | 补齐 `skill.visibility` / `default_config_json` 等字段 | P3 | Phase 1 | — |
| 6 | 自动负熵报告（聚合指标 + 趋势） | P3 | Phase 1 | — |
| 7 | Prompt 注入方式 C（Assembler → system message） | P4 | Phase 2 | — |
| 8 | embedding 语义精排（向量相似度匹配） | P4 | Phase 2 | — |
| 9 | Budget 中间件（token 上限裁剪） | P4 | Phase 2 | #7 |
| 10 | Preview API 增强（选中原因） | P4 | Phase 2 | — |
| 11 | `internal/skill/manifest/` 包（frontmatter 解析） | P4 | Phase 2 | — |
| 12 | `internal/skill/render/` 包（prompt 渲染） | P4 | Phase 2 | #7 |
| 13 | Skill 依赖声明 schema + 安装时检查 | P4 | Phase 3 | — |
| 14 | Skill 冲突表扩展（运行时互斥策略） | P4 | Phase 3 | #13 |
| 15 | Skill 市场/分享机制 | P4+ | Phase 3 | Ecosystem 模块 |

---

## 6. 验收标准

### Phase 1

- [ ] Skill 可查看版本历史列表
- [ ] Skill 可回滚到指定版本
- [ ] 权限由 RBAC 控制，不再硬编码
- [ ] ZIP 导入端点在 proto 或 OpenAPI 文档中有声明
- [ ] `visibility` / `default_config_json` 等字段已落地

### Phase 2

- [ ] 运行时可选择 Prompt 注入方式（方式 C）
- [ ] 候选 Skill 筛选支持向量相似度匹配
- [ ] Skill 装配受 token 预算限制
- [ ] Preview API 返回每个 Skill 的选中原因

### Phase 3

- [ ] Skill 可声明依赖，安装时自动检查
- [ ] 运行时互斥策略生效
- [ ] 用户可浏览和安装共享 Skill

---

## 7. 依赖与风险

| 依赖/风险 | 说明 |
|-----------|------|
| RBAC 模块 | 权限接入依赖 RBAC 基础设施就绪 |
| Ecosystem 模块 | Skill 市场需与 Ecosystem 模块联动 |
| 向量数据库 | embedding 语义精排依赖向量存储与检索基础设施 |
| 版本兼容性 | 版本回滚需考虑与 Agent 绑定、运行时策略的兼容性 |
| ZIP 导入收敛 | 手写路由收敛进 proto 可能影响前端 API 调用方式 |
| Context 目录迁移 | `internal/skill/**` → `internal/capability/skill/**` 需同步更新所有 import |
