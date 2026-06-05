## 1. Ent Schema 变更与数据迁移

- [x] 1.1 修改 `internal/data/ent/schema/agent.go` Kind 枚举：删除 `system`/`industry_template`，新增 `ecosystem_preset`，最终枚举为 `user | system_builtin | ecosystem_preset | marketplace | certified`
- [x] 1.2 修改 `internal/data/ent/schema/team.go` 新增 `kind` 字段（Enum，值与 Agent Kind 对齐，Default `"user"`）
- [x] 1.3 修改 `internal/data/ent/schema/system_setting.go` 新增 `ecosystem_loaded` 字段（Text 类型，Default `"{}"`）
- [x] 1.4 新增 DDL 迁移脚本：`ALTER TABLE system_settings ADD COLUMN ecosystem_loaded TEXT NOT NULL DEFAULT '{}'`；`ALTER TABLE teams ADD COLUMN kind TEXT NOT NULL DEFAULT 'user'`
- [x] 1.5 新增数据迁移：Agent Kind 值迁移（`system` → `system_builtin`，`industry_template` → `ecosystem_preset`）；Team Kind 初始化（`source = 'imported'` → `kind = 'ecosystem_preset'`）
- [x] 1.6 运行 `make api && make wire && make build` 验证编译通过

## 2. Pack 引擎 Kind 覆盖机制

- [x] 2.1 在 `internal/biz/pack/importer.go` 中新增 `importConfig` 结构体和 `ImportOption` 函数类型
- [x] 2.2 实现 `WithKindOverride(kind string) ImportOption` 选项函数
- [x] 2.3 修改 `Importer.Import()` 方法签名，接受 `opts ...ImportOption` 参数
- [x] 2.4 修改 `importAgent()` 方法，使用 `cfg.kindOverride` 覆盖 Agent Kind（优先级：kindOverride > spec.Kind > "llm"）
- [x] 2.5 修改 `importTeam()` 方法，使用 `cfg.kindOverride` 覆盖 Team Kind 和 Source
- [x] 2.6 编写 `internal/biz/pack/importer_test.go` 测试 Kind 覆盖逻辑
- [x] 2.7 运行 `go test ./internal/biz/pack/... -count=1` 验证

## 3. 清理旧版种子管道

- [x] 3.1 删除 `internal/service/industry_agent_seed.go` 文件（旧版 `SeedBuiltinIndustryAgents`）
- [x] 3.2 删除 `internal/data/data.go` 中 Lazy Seeder 行业 Pack 注册代码（`pack_finance`/`pack_selfmedia`/`pack_softwaredev`）
- [x] 3.3 清理 `internal/data/seed_versions.go`：删除 `SeedPackFinanceV1`/`SeedPackSelfmediaV1`/`SeedPackSoftwaredevV1`/`SeedPackIndustryBase` 常量和 `hashIndustryKey` 函数
- [x] 3.4 清理 `internal/biz/seed_version.go`：删除 `SeedVersionIndustryAgentsV1` 常量
- [x] 3.5 修改 `internal/data/seed_pack.go`：`SeedPackIndustry` 函数签名新增 `kindOverride string` 参数，传递给 `Importer.Import()` 的 `WithKindOverride`
- [x] 3.6 搜索并清理所有对已删除函数/常量的引用
- [x] 3.7 运行 `make build` 验证编译通过

## 4. 附带生态加载/卸载后端 API

- [x] 4.1 新增 `internal/biz/ecosystem_preset.go`：定义 `EcosystemPresetUsecase` 结构体和 `LoadEcosystemPreset`/`UnloadEcosystemPreset` 方法
- [x] 4.2 实现 `LoadEcosystemPreset`：读取 `ecosystem_loaded` JSON → 遍历行业 → 调用 `SeedPackIndustry` → 更新 JSON → 返回结果
- [x] 4.3 实现 `UnloadEcosystemPreset`：校验行业已加载 → 软删除分类节点 → 软删除 Agent → 软删除 Team（处理跨行业 Team）→ 更新 JSON → 返回删除统计
- [x] 4.4 新增 `internal/data/ecosystem_preset.go`：实现 `EcosystemPresetRepo`（读写 `system_settings.ecosystem_loaded` 字段 + 卸载删除操作）
- [x] 4.5 新增 `internal/service/ecosystem_preset.go`：定义 `EcosystemPresetService` 和 HTTP handler（load + unload + status）
- [x] 4.6 注册 HTTP 路由 `POST /api/v1/admin/ecosystem/preset/load`、`POST /api/v1/admin/ecosystem/preset/unload`、`GET /api/v1/admin/ecosystem/preset/status`
- [x] 4.7 Wire 注入：在 `biz.go`/`service.go`/`wire.go` 中注册 `EcosystemPresetUsecase` 和 `EcosystemPresetService`
- [x] 4.8 编写 `internal/biz/ecosystem_preset_test.go` 测试加载和卸载逻辑
- [x] 4.9 运行 `make wire && make build && go test ./internal/biz/... -run TestEcosystem -count=1` 验证

## 5. Agent/Team 删除权限控制

- [x] 5.1 修改 Agent 删除 usecase 方法：当 Agent Source（映射自 DB kind）为 `system_builtin` 时返回 403 错误（已存在）
- [x] 5.2 修改 Team 删除 usecase 方法：当 Team Kind 为 `system_builtin` 时返回 403 错误（从 Source 改为 Kind）
- [x] 5.3 Team biz 类型新增 Kind 字段 + data 层映射 + 测试更新
- [x] 5.4 运行 `go test ./internal/biz/... -run TestTeam -count=1` 验证

## 6. 前端 - 系统设置附带生态区块

- [x] 6.1 在 `web/src/features/system-settings/api.ts` 新增 `loadEcosystemPreset`/`unloadEcosystemPreset`/`getEcosystemPresetStatus` API 调用
- [x] 6.2 在 `web/src/features/system-settings/types.ts` 新增 `IndustryLoadInfo`/`EcosystemLoadedStatus`/`EcosystemLoadResult`/`EcosystemLoadResponse`/`EcosystemUnloadResult`/`EcosystemUnloadResponse` 类型
- [x] 6.3 修改 `web/src/stores/system-settings/index.ts`：新增 `ecosystemLoaded` 状态、`fetchEcosystemStatus`/`loadEcosystemPreset`/`unloadEcosystemPreset` action
- [x] 6.4 修改 `web/src/pages/SystemSettingsPage.vue`：在常规 Tab 底部新增"附带生态"区块，展示各行业加载状态、加载/卸载按钮
- [x] 6.5 实现卸载确认对话框（显示将删除的 Agent/Team/分类节点数量 + 不可撤销警告）
- [x] 6.6 运行 `cd web && pnpm lint && pnpm build` 验证

## 7. 前端 - Agent/Team Kind 徽章与删除控制

- [x] 7.1 新增 `web/src/components/agents/KindBadge.vue`：根据 Kind 显示不同颜色徽章（内置/预设/商城/认证）
- [x] 7.2 修改 `web/src/components/agents/AgentCard.vue`：集成 KindBadge，`system_builtin` 时隐藏删除按钮
- [x] 7.3 修改 `web/src/components/teams/TeamCard.vue`：集成 KindBadge（基于 Team kind 字段），`system_builtin` 时隐藏删除按钮
- [x] 7.4 修改 `web/src/components/agents/AgentSettingsHeader.vue`：设置页头部显示 KindBadge（AgentSettingsPage 无删除按钮，仅添加徽章）
- [x] 7.5 运行 `cd web && pnpm lint && pnpm build` 验证

## 8. 前端 - 行业分类树形布局改造

- [x] 8.1 新增 `web/src/components/agents/TaxonomyDepartmentNode.vue`：部门级折叠节点组件（QExpansionItem + vuedraggable）
- [x] 8.2 修改 `web/src/components/agents/TaxonomyTree.vue`：使用 TaxonomyDepartmentNode 实现三层树形结构（行业→部门→岗位）
- [x] 8.3 修改 `web/src/components/agents/TaxonomyPositionCard.vue`：新增 agentCount prop 和 variantTags 计算属性
- [x] 8.4 修改 `web/src/pages/TaxonomyPage.vue`：使用改造后的 TaxonomyTree 组件
- [x] 8.5 修改 `web/src/features/platform/useTaxonomyPage.ts`：新增 onReorderPositions 函数
- [x] 8.6 运行 `cd web && pnpm lint && pnpm build` 验证

## 9. 全量验证

- [x] 9.1 后端全量验证：`go build ./cmd/admin` 通过 + 关键测试通过
- [x] 9.2 前端全量验证：`pnpm lint && pnpm build` 通过
- [ ] 9.3 手动验证：启动系统 → 确认 L1 内置 Agent/Team 正常加载 → 确认行业 Pack 不自动加载 → 加载附带生态 → 确认 Agent/Team Kind 为 ecosystem_preset → 确认分类树展示正常 → 确认内置 Agent/Team 不可删除 → 确认预设 Agent/Team 可编辑可删除 → 卸载某行业 → 确认该行业 Agent/Team/分类节点已删除 → 重新加载该行业 → 确认数据恢复
