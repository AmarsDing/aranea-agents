## Context

当前种子入库系统存在 3 条独立管道：

1. **硬编码 SQL 管道**（`seed_system_admin.go`）：系统内置 Agent（精灵/管家/记忆/技能），`ON CONFLICT DO UPDATE`，启动时强制执行
2. **旧版 YAML Loader 管道**（`industry_agent_seed.go`）：行业 Agent 种子，`SeedVersionRepo` 版本门控，启动时自动执行
3. **Pack 引擎管道**（`seed_pack.go`）：行业 Pack 种子，`schema_migrations` 版本门控，Lazy 阶段延迟执行

问题：管道 2 和 3 对同一行业数据可能重复导入；Agent Kind 枚举中 `system`/`industry_template` 语义不清；行业生态自动加载不可控；前端分类页展示不合理。

## Goals / Non-Goals

**Goals:**
- 将种子数据分为 L1（系统内置，启动强制加载，不可删除）和 L2（系统附带生态，用户按需加载，可编辑可删除）两层
- 统一种子管道为 2 条：L1 启动管道 + L2 API 触发管道
- Agent Kind 枚举精简：删除 `system`/`industry_template`，新增 `ecosystem_preset`
- 前端提供一键加载附带生态入口和加载状态展示
- 行业分类页改造为树形折叠 + 岗位卡片混合布局

**Non-Goals:**
- 不做行业分类 CRUD API 重构
- 不做 Agent/Team 删除级联逻辑
- 不做附带生态增量更新（加载一次后固定，后续版本通过"重新加载"支持）
- 不做商城/认证 Agent 改动
- 不做拖拽排序后端 API 新增

## Decisions

### D1: Agent Kind 枚举精简方案

**决策**: 删除 `system` 和 `industry_template`，新增 `ecosystem_preset`。

**最终枚举**: `user | system_builtin | ecosystem_preset | marketplace | certified`

**备选方案**:
- A) 保留 `industry_template` 仅重命名为 `ecosystem_preset` → 拒绝：`system` 仍冗余
- B) 不删除旧值，仅新增 → 拒绝：枚举膨胀，语义混乱

**数据迁移**:
- `system` → `system_builtin`（当前无实际数据，但需防御性迁移）
- `industry_template` → `ecosystem_preset`（已有行业种子数据需迁移）

**Ent Schema 变更** (`internal/data/ent/schema/agent.go`):
```go
field.Enum("kind").Values(
    "user", "system_builtin", "ecosystem_preset", "marketplace", "certified",
).Default("user")
```

### D2: 附带生态加载状态存储

**决策**: 在 `system_settings` 表新增 `ecosystem_loaded` 字段（TEXT 类型，JSON 格式），记录每个行业的加载状态。

**字段结构**:
```json
{
  "finance": { "loaded": true, "loaded_at": "2026-06-05T10:00:00Z", "agents": 30, "teams": 5 },
  "selfmedia": { "loaded": true, "loaded_at": "2026-06-05T10:01:00Z", "agents": 25, "teams": 3 },
  "softwaredev": { "loaded": false }
}
```

**备选方案**:
- A) 单布尔字段 `ecosystem_loaded` → 拒绝：无法追踪部分加载失败
- B) 继续使用 `schema_migrations` 版本号 → 拒绝：语义不匹配（版本号是迁移概念，不是用户操作状态）
- C) 新建 `ecosystem_load_status` 表 → 拒绝：过度设计，system_settings 已有单例模式

**优势**: JSON 格式支持部分加载追踪、前端可直接读取展示各行业加载状态、支持"重新加载"单个行业。

### D3: 附带生态加载 API 设计

**决策**: 新增 `POST /api/v1/admin/ecosystem/preset/load`，同步执行加载。

**API 规格**:
```
POST /api/v1/admin/ecosystem/preset/load
Request Body: { "industries": ["finance", "selfmedia", "softwaredev"] }  // 可选，默认全部
Response: {
  "results": {
    "finance": { "agents_created": 30, "teams_created": 5, "taxonomy_nodes": 40 },
    "selfmedia": { "agents_created": 25, "teams_created": 3, "taxonomy_nodes": 35 },
    "softwaredev": { "agents_created": 40, "teams_created": 8, "taxonomy_nodes": 55 }
  },
  "already_loaded": []
}
```

**执行逻辑**:
1. 读取 `system_settings.ecosystem_loaded` JSON
2. 对每个请求的行业：若已 loaded=true，跳过并加入 `already_loaded`
3. 调用 `SeedPackIndustry(ctx, client, scenarioDir, industryKey, pack.KindOverride("ecosystem_preset"), lg)`
4. 更新 `ecosystem_loaded` JSON 中对应行业状态
5. 返回结果

**备选方案**:
- A) 异步加载 + WebSocket 通知 → 拒绝：行业种子数据量小（<100 Agent），同步即可，异步增加复杂度
- B) 启动时自动加载（现有 Lazy 模式）→ 拒绝：用户无法控制，违背"按需加载"需求

### D4: Pack 引擎 Kind 覆盖机制

**决策**: 在 `pack.Importer.Import()` 方法中新增 `WithKindOverride(kind string) ImportOption` 选项。

**实现**:
```go
// internal/biz/pack/importer.go
type ImportOption func(*importConfig)

type importConfig struct {
    kindOverride string
}

func WithKindOverride(kind string) ImportOption {
    return func(c *importConfig) { c.kindOverride = kind }
}

func (imp *Importer) Import(ctx context.Context, p *Pack, strategy ConflictStrategy, opts ...ImportOption) (*ImportResult, error) {
    cfg := &importConfig{}
    for _, o := range opts { o(cfg) }
    // importAgent 中：kind = firstNonEmpty(cfg.kindOverride, spec.Kind, "llm")
}
```

**备选方案**:
- A) 在 Pack Spec 中硬编码 Kind → 拒绝：同一 Pack 可能以不同 Kind 导入（如内置模板 vs 附带生态）
- B) 在 `AgentPackSpec` 中新增 Kind 字段 → 可选但不够灵活，Option 模式更通用

### D5: 种子管道统一

**决策**: 删除旧版管道，保留 2 条管道。

**管道 1 — L1 启动管道**（P1 阶段，不变）:
```
seedP1Data():
  ensureChannelPlatformAvatars
  ensureAgentAvatars
  SeedSystemAdminAgent      (kind=system_builtin, readonly=1)
  SeedSpiritAgent           (kind=system_builtin, readonly=1)
  SeedMemoryAgent           (kind=system_builtin, readonly=1)
  SeedSkillsAgent           (kind=system_builtin, readonly=1)
  SeedBuiltinCLIAdminTools
  SeedPackBuiltinTemplates  (内置模板 Pack)
  SeedSpiritPromptFiles
  SeedButlerPromptFiles
  SeedCronTasks
```

**管道 2 — L2 API 触发管道**（新增）:
```
POST /api/v1/admin/ecosystem/preset/load:
  SeedPackIndustry(finance, kind=ecosystem_preset)
  SeedPackIndustry(selfmedia, kind=ecosystem_preset)
  SeedPackIndustry(softwaredev, kind=ecosystem_preset)
  更新 ecosystem_loaded JSON
```

**删除的代码**:
- `internal/service/industry_agent_seed.go` — `SeedBuiltinIndustryAgents`
- `internal/data/data.go` 中 Lazy Seeder 行业 Pack 注册
- `internal/data/seed_versions.go` 中 `SeedPackFinanceV1`/`SeedPackSelfmediaV1`/`SeedPackSoftwaredevV1`/`SeedPackIndustryBase`
- `internal/biz/seed_version.go` 中 `SeedVersionIndustryAgentsV1`

### D6: 前端行业分类树形布局

**决策**: 改造 `TaxonomyPage.vue` 为树形折叠 + 岗位卡片混合布局。

**组件结构**:
```
TaxonomyPage.vue
  └── TaxonomyTree.vue（改造）
        ├── TaxonomyIndustryNode.vue（行业节点，QExpansionItem，可折叠）
        │     └── TaxonomyDepartmentNode.vue（部门节点，QExpansionItem，可折叠）
        │           └── TaxonomyPositionCard.vue（岗位卡片，QCard，可拖拽排序）
        └── 操作按钮（新增子级/编辑/删除/启停）
```

**交互规则**:
- 行业层、部门层：`QExpansionItem` 折叠节点，默认收起
- 岗位层：`QCard` 卡片网格，支持 `vuedraggable` 拖拽排序
- 部门卡片也可拖拽排序（同级部门间）
- 搜索/仅看自建筛选复用现有逻辑

### D7: 前端系统设置"加载生态"区块

**决策**: 在 `SystemSettingsPage.vue` 常规 Tab 底部新增"附带生态"区块。

**UI 结构**:
```
附带生态
  说明文字
  各行业加载状态列表（已加载/未加载 + Agent/Team 数量）
  [加载附带生态] 按钮（未加载时显示）/ [重新加载] 按钮（已加载时显示）
```

**API 调用**: `features/system-settings/api.ts` 新增 `loadEcosystemPreset(industries?)`

### D8: Kind 徽章与删除控制

**决策**: Agent/Team 列表和详情页根据 Kind 显示徽章，控制删除按钮。

| Kind | 徽章 | 删除按钮 |
|------|------|----------|
| `system_builtin` | "内置" 蓝色 | 隐藏 |
| `ecosystem_preset` | "预设" 绿色 | 显示 |
| `user` | 无 | 显示 |
| `marketplace` | "商城" 紫色 | 显示 |
| `certified` | "认证" 橙色 | 显示 |

**实现**: `AgentCard.vue` / `TeamCard.vue` 中新增 `KindBadge` 子组件，根据 `agent.kind` / `team.kind` 渲染徽章；删除按钮增加 `v-if="agent.kind !== 'system_builtin'"` 条件。

### D9: 行业卸载机制

**决策**: 新增 `POST /api/v1/admin/ecosystem/preset/unload` API，卸载指定行业的附带生态数据。

**API 规格**:
```
POST /api/v1/admin/ecosystem/preset/unload
Request Body: { "industries": ["finance"] }  // 必填，指定要卸载的行业
Response: {
  "results": {
    "finance": { "agents_deleted": 30, "teams_deleted": 5, "taxonomy_nodes_deleted": 40 }
  }
}
```

**执行逻辑**:
1. 校验指定行业在 `ecosystem_loaded` 中为 `loaded: true`，否则返回错误
2. 查找该行业下的所有分类节点（industry → departments → positions），软删除
3. 查找 `kind = 'ecosystem_preset'` 且 `taxonomy_position_id` 属于该行业分类节点的 Agent，软删除
4. 查找 `kind = 'ecosystem_preset'` 且成员 Agent 全部属于该行业的 Team，软删除
5. 更新 `ecosystem_loaded` JSON 中对应行业状态为 `loaded: false`
6. 返回删除统计

**前端交互**:
- 系统设置页每个已加载行业旁显示"卸载"按钮
- 点击"卸载"→ 弹出确认对话框，明确提示："卸载将删除该行业下所有 Agent（XX 个）、Team（XX 个）和分类节点（XX 个），此操作不可撤销。确定要卸载吗？"
- 确认后调用卸载 API

**备选方案**:
- A) 不提供卸载，仅支持单个手动删除 → 拒绝：行业数据量大（30-50 Agent），逐个删除体验差
- B) 卸载后数据可恢复（回收站）→ 拒绝：增加复杂度，当前软删除已提供一定保护

### D10: Team Kind 字段

**决策**: Team 表新增 `kind` 字段，枚举值与 Agent Kind 完全对齐。

**字段命名**: 使用 `kind` 而非 `team_kind`。理由：
- 在 Team 表上下文中 `kind` 无歧义（同一张表内不会与 Agent.kind 混淆）
- 与 Agent.kind 保持命名一致，降低认知负担
- 前端/后端代码中通过 `agent.kind` / `team.kind` 访问，上下文清晰

**Ent Schema 变更** (`internal/data/ent/schema/team.go`):
```go
field.Enum("kind").Values(
    "user", "system_builtin", "ecosystem_preset", "marketplace", "certified",
).Default("user").Comment("team kind: aligned with agent.kind for unified permission model")
```

**数据迁移**:
- 现有 Team 默认 `kind = 'user'`
- 系统内置 Team（如精灵组建的 Team）需迁移为 `system_builtin`
- 旧版行业种子 Team 需迁移为 `ecosystem_preset`

**与 `source` 字段的关系**:
- `kind`：权限分类（决定可编辑性/可删除性/徽章显示）
- `source`：来源追踪（`imported`/`system`/`user` 等，用于审计和统计）
- 两者职责不同，保留 `source` 字段不删除

## Risks / Trade-offs

- **[Kind 枚举变更兼容性]** → 数据迁移脚本将旧值映射到新值；Ent schema 变更后需 `make api && make wire && make build` 全量验证
- **[部分加载失败]** → `ecosystem_loaded` JSON 按行业独立记录状态，失败行业不影响已成功行业；前端展示各行业独立状态
- **[Pack 引擎 Kind 覆盖影响现有调用]** → `WithKindOverride` 是可选参数，不传时行为不变（向后兼容）
- **[删除附带生态后无法恢复]** → 提供"重新加载"按钮，重置 `ecosystem_loaded` 对应行业状态后重新加载
- **[卸载操作不可逆]** → 使用软删除（`deleted_at` 字段），数据库层面可恢复；前端明确提示"不可撤销"避免误操作
- **[卸载时 Team 成员跨行业]** → 仅删除成员 Agent 全部属于该行业的 Team；跨行业 Team 保留但移除已删除 Agent 成员
- **[TaxonomyPage 改造范围大]** → 分阶段实施：先完成种子管道和 API，再改造前端布局

## Migration Plan

1. **DDL 迁移**: `system_settings` 新增 `ecosystem_loaded` 列（TEXT, DEFAULT '{}'）；`teams` 新增 `kind` 列（TEXT, DEFAULT 'user'）
2. **数据迁移**: `UPDATE agents SET kind = 'system_builtin' WHERE kind = 'system'`；`UPDATE agents SET kind = 'ecosystem_preset' WHERE kind = 'industry_template'`；`UPDATE teams SET kind = 'ecosystem_preset' WHERE source = 'imported'`
3. **代码部署**: Ent schema 变更 → `make api && make wire && make build` → 部署
4. **回滚策略**: DDL 迁移仅新增列（无破坏性），Kind 枚举回滚需反向数据迁移

## Open Questions

（已全部解决，无遗留问题）
