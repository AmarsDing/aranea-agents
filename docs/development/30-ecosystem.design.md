# 生态商城模块 — 实现设计文档

> 对应需求：`30 ecosystem.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

生态商城（Ecosystem Marketplace）：Agent 模板、Skill 包、Team 编排方案的发现、安装、发布、交易和治理体系。

附带生态（Ecosystem Preset）：行业预设数据的按需加载/卸载，统一种子管道和 Kind 权限分类。

---

## 二、Proto 层

### 2.1 生态市场 Proto（已实现）

```protobuf
// api/kratos/ecosystem/v1/ecosystem.proto
service EcosystemService {
  rpc ListProducts(ListProductsRequest) returns (ListProductsResponse) {
    option (google.api.http) = { get: "/v1/ecosystem/products" };
  }
  rpc GetProduct(GetProductRequest) returns (Product) {
    option (google.api.http) = { get: "/v1/ecosystem/products/{id}" };
  }
  rpc PublishProduct(PublishProductRequest) returns (Product) {
    option (google.api.http) = { post: "/v1/ecosystem/products" body: "*" };
  }
  rpc InstallProduct(InstallProductRequest) returns (InstallResult) {
    option (google.api.http) = { post: "/v1/ecosystem/products/{id}/install" };
  }
  rpc UninstallProduct(UninstallProductRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/ecosystem/products/{id}/install" };
  }
}
```

> **注意**：需求中规划的 `RateProduct` RPC 尚未实现，待 Phase 3 评价系统时新增。

### 2.2 附带生态 API（已实现，纯 HTTP）

```
POST   /api/v1/admin/ecosystem/preset/load    — 加载行业预设
POST   /api/v1/admin/ecosystem/preset/unload  — 卸载行业预设
GET    /api/v1/admin/ecosystem/preset/status   — 查询加载状态
```

非 gRPC，由 `EcosystemPresetService` 直接注册 HTTP 路由。

---

## 三、Biz 层

### 3.1 生态市场（已实现）

独立子包 `internal/biz/ecosystem/`，通过 `internal/biz/ecosystem.go` 重新导出保持向后兼容。

```go
// internal/biz/ecosystem/ecosystem.go
type Product struct {
    ID           string
    Name         string
    DisplayName  string
    Description  string
    Type         string  // "agent_template"/"skill_pack"/"team_blueprint"
    AuthorID     string
    Version      string
    PriceModel   string  // "free"/"paid"/"subscription"
    PriceCents   int64
    Rating       float64
    InstallCount int64
    ConfigJSON   string  // 产品定义
    Status       string  // "draft"/"published"/"deprecated"
    CreatedAt    string
    UpdatedAt    string
    Installed    bool    // 当前用户是否已安装
}

type InstallResult struct {
    ProductID    string
    InstalledIDs []string  // 安装后生成的 Agent/Skill/Team ID
}

type Repo interface {
    ListProducts(ctx, query) (ListResult, error)
    GetProduct(ctx, id) (Product, error)
    CreateProduct(ctx, p Product) (Product, error)
    RecordInstall(ctx, productID, userID, refID) error
    RemoveInstall(ctx, productID, userID) error
    IsInstalled(ctx, productID, userID) (bool, error)
}

type Usecase struct { repo Repo }
func (uc *Usecase) List(ctx, query) (ListResult, error)
func (uc *Usecase) Get(ctx, id) (Product, error)
func (uc *Usecase) Publish(ctx, p Product) (Product, error)
func (uc *Usecase) Install(ctx, id string) (InstallResult, error)
func (uc *Usecase) Uninstall(ctx, id string) error
```

> **当前限制**：`Install` 仅调用 `RecordInstall` 记录安装关系，未解析 `config_json` 创建实际 Agent/Team/Skill 资源。待 Phase 2 补全。

### 3.2 附带生态（已实现）

```go
// internal/biz/ecosystem_preset.go
type EcosystemPresetUsecase struct {
    repo          EcosystemPresetRepo
    seedPackFn    SeedPackFunc
    scenarioDir   string
    mu            sync.Mutex  // 防止并发加载/卸载
}

func (uc *EcosystemPresetUsecase) LoadEcosystemPreset(ctx, industries, force, client) error
func (uc *EcosystemPresetUsecase) UnloadEcosystemPreset(ctx, industries) error
func (uc *EcosystemPresetUsecase) GetEcosystemStatus(ctx) (EcosystemLoadedStatus, error)
```

默认行业列表：`["finance", "selfmedia", "softwaredev"]`

---

## 四、Data 层

### 4.1 生态市场（已实现，原生 SQL）

**不使用 Ent ORM**，通过原生 SQL 操作以下表：

- `ecosystem_products` — 产品目录表（DDL: `internal/data/sql/ecosystem_product.sql`）
- `ecosystem_installs` — 安装记录表（DDL: `internal/data/sql/ecosystem_product.sql`）

```go
// internal/data/ecosystem.go
type ecosystemRepo struct { db *ent.Client }
func NewEcosystemRepo(db *ent.Client) biz.ecosystem.Repo
```

### 4.2 附带生态（已实现）

```go
// internal/data/ecosystem_preset.go
type EcosystemPresetRepo struct { db *ent.Client }
func NewEcosystemPresetRepo(db *ent.Client) *EcosystemPresetRepo

func (r *EcosystemPresetRepo) GetEcosystemLoaded(ctx) (EcosystemLoadedStatus, error)
func (r *EcosystemPresetRepo) SetEcosystemLoaded(ctx, status EcosystemLoadedStatus) error
func (r *EcosystemPresetRepo) DeleteTaxonomyNodesByIndustry(ctx, industry string) (int, error)
func (r *EcosystemPresetRepo) DeleteAgentsByIndustry(ctx, industry string) (int, error)
func (r *EcosystemPresetRepo) DeleteTeamsByIndustry(ctx, industry string) (deleted int, modified int, error)
```

级联删除逻辑：taxonomy → agents → teams，跨行业 Team 保留但移除已删除 Agent 成员。

### 4.3 数据库迁移

| 版本 | 名称 | 内容 |
|------|------|------|
| V20260703 | `ecosystem_schema` | 创建 `ecosystem_products` + `ecosystem_installs` 表 |
| V20260718 | `ecosystem_preset_schema` | 添加 `system_settings.ecosystem_loaded` 列 + `teams.kind` 列 + Kind 数据迁移 |

---

## 五、Service 层

### 5.1 生态市场（已实现）

```go
// internal/service/ecosystem.go
type EcosystemService struct { uc *biz.ecosystem.Usecase }
func (s *EcosystemService) ListProducts(ctx, req) (*ListProductsResponse, error)
func (s *EcosystemService) GetProduct(ctx, req) (*Product, error)
func (s *EcosystemService) PublishProduct(ctx, req) (*Product, error)
func (s *EcosystemService) InstallProduct(ctx, req) (*InstallResult, error)
func (s *EcosystemService) UninstallProduct(ctx, req) (*emptypb.Empty, error)
```

### 5.2 附带生态（已实现）

```go
// internal/service/ecosystem_preset.go
type EcosystemPresetService struct { uc *biz.EcosystemPresetUsecase }
func (s *EcosystemPresetService) RegisterRoutes(r *mux.Router)
// POST /api/v1/admin/ecosystem/preset/load
// POST /api/v1/admin/ecosystem/preset/unload
// GET  /api/v1/admin/ecosystem/preset/status
```

---

## 六、Wire 注入（已实现）

```
data.ProviderSet  → NewEcosystemRepo + NewEcosystemPresetRepo
biz.ProviderSet   → NewEcosystemUsecase + NewEcosystemPresetUsecase
service.ProviderSet → NewEcosystemService + NewEcosystemPresetService

wire.Bind(new(biz.EcosystemPresetRepo), new(*data.EcosystemPresetRepo))
provideEcosystemPresetSeedPackFn
provideEcosystemPresetScenarioDir
provideEcosystemPresetClientProvider
```

---

## 七、Web 前端设计

### 7.1 文件结构（已实现）

```
web/src/
├── pages/EcosystemPage.vue              # 商城页面（路由 /shop）
├── features/ecosystem/
│   ├── api.ts                           # gRPC-Web 客户端
│   ├── types.ts                         # EcosystemProduct 类型
│   └── useEcosystemPage.ts              # 页面 composable
├── stores/ecosystem/index.ts            # useEcosystemStore
├── features/system-settings/
│   ├── api.ts                           # 含 preset load/unload/status
│   └── types.ts                         # 含 EcosystemLoadedStatus 等类型
├── stores/system-settings/index.ts      # 含 ecosystemLoaded + preset actions
└── components/agents/KindBadge.vue      # Kind 徽章组件
```

### 7.2 组件设计

**EcosystemPage.vue**：商城首页，技术预览状态

| 区域 | 组件 | 说明 |
|------|------|------|
| 搜索 | `QInput` | 关键词搜索 |
| 类型筛选 | `QBtnToggle` | Agent/Skill/Team |
| 排序 | `QSelect` | 评分/安装量/最新 |
| 列表 | 产品卡片 | 商品展示 |
| 安装 | API 调用 | 安装/卸载 |
| 发布 | `QDialog` | 发布弹窗 |

**KindBadge.vue**（已实现）：

| Kind | 徽章文字 | 颜色 |
|------|----------|------|
| `system_builtin` | 内置 | 蓝色 |
| `ecosystem_preset` | 预设 | 绿色 |
| `marketplace` | 商城 | 紫色 |
| `certified` | 认证 | 橙色 |

### 7.3 API

```typescript
// features/ecosystem/api.ts — 生态市场
export async function listProducts(query): Promise<ProductListResult>
export async function getProduct(id: string): Promise<Product>
export async function publishProduct(req): Promise<Product>
export async function installProduct(id: string): Promise<InstallResult>
export async function uninstallProduct(id: string): Promise<void>

// features/system-settings/api.ts — 附带生态
export async function loadEcosystemPreset(industries?, force?): Promise<EcosystemLoadResponse>
export async function unloadEcosystemPreset(industries: string[]): Promise<EcosystemUnloadResponse>
export async function getEcosystemPresetStatus(): Promise<EcosystemLoadedStatus>
```

---

## 子模块：Ecosystem Preset Seed Refactor 设计

## 1. 系统架构

### 1.1 种子数据分层模型

```
┌─────────────────────────────────────────────────────────┐
│                    种子数据分层                           │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  L1 系统内置层 (system_builtin)                          │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐  │
│  │ 精灵助手  │ │ 系统管家  │ │ 记忆管家  │ │ 技能管家  │  │
│  │ readonly  │ │ readonly  │ │ readonly  │ │ readonly  │  │
│  │ 不可删除  │ │ 不可删除  │ │ 不可删除  │ │ 不可删除  │  │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘  │
│  加载时机：P1 启动阶段，ON CONFLICT DO UPDATE            │
│                                                         │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  L2 附带生态层 (ecosystem_preset)                        │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐               │
│  │ 金融行业  │ │ 自媒体    │ │ 软件开发  │               │
│  │ 可编辑    │ │ 可编辑    │ │ 可编辑    │               │
│  │ 可删除    │ │ 可删除    │ │ 可删除    │               │
│  └──────────┘ └──────────┘ └──────────┘               │
│  加载时机：用户在系统设置页按需触发                        │
│  卸载：按行业卸载，弹框确认                               │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### 1.2 种子管道架构（已实现）

```
改造后（2 条管道，职责清晰）：
┌──────────────────────┐  ┌──────────────────────┐
│ L1 启动管道           │  │ L2 API 触发管道       │
│ P1 阶段强制执行       │  │ 用户按需触发          │
│                      │  │                      │
│ SeedSystemAdminAgent │  │ POST /ecosystem/     │
│ SeedSpiritAgent      │  │   preset/load        │
│ SeedMemoryAgent      │  │                      │
│ SeedSkillsAgent      │  │ SeedPackIndustry     │
│ SeedBuiltinCLI...    │  │   (finance)          │
│ SeedPackBuiltin...   │  │ SeedPackIndustry     │
│ SeedSpiritPrompt...  │  │   (selfmedia)        │
│ SeedButlerPrompt...  │  │ SeedPackIndustry     │
│ SeedCronTasks        │  │   (softwaredev)      │
│                      │  │                      │
│ kind=system_builtin  │  │ kind=ecosystem_preset│
│ readonly=1           │  │ 可编辑可删除          │
│ 不可删除             │  │                      │
└──────────────────────┘  └──────────────────────┘
```

## 2. 数据模型

### 2.1 Agent Kind 枚举（已实现）

```
user | system_builtin | ecosystem_preset | marketplace | certified
```

| Kind | 含义 | 可编辑 | 可删除 | 徽章 |
|------|------|--------|--------|------|
| `user` | 用户自建 | 是 | 是 | 无 |
| `system_builtin` | 系统内置 | 是 | 否 | 内置(蓝) |
| `ecosystem_preset` | 附带生态 | 是 | 是 | 预设(绿) |
| `marketplace` | 商城导入 | 是 | 是 | 商城(紫) |
| `certified` | 认证 | 是 | 是 | 认证(橙) |

### 2.2 Team Kind 字段（已实现）

```go
// internal/data/ent/schema/team.go
field.Enum("kind").Values(
    "user", "system_builtin", "ecosystem_preset", "marketplace", "certified",
).Default("user").Comment("team kind: aligned with agent.kind for unified permission model")
```

**kind vs source 职责划分**：
- `kind`：权限分类（决定可编辑性/可删除性/徽章显示）
- `source`：来源追踪（`imported`/`system`/`user`，用于审计和统计）

### 2.3 ecosystem_loaded 状态存储（已实现）

```json
// system_settings.ecosystem_loaded 字段（TEXT, JSON 格式）
{
  "finance": {
    "loaded": true,
    "loaded_at": "2026-06-05T10:00:00Z",
    "agents": 30,
    "teams": 5,
    "taxonomy_nodes": 40
  },
  "selfmedia": {
    "loaded": false
  },
  "softwaredev": {
    "loaded": false
  }
}
```

## 3. API 设计（已实现）

### 3.1 加载附带生态

```
POST /api/v1/admin/ecosystem/preset/load

Request:
{
  "industries": ["finance", "selfmedia", "softwaredev"],  // 可选，默认全部
  "force": false  // 可选，true 时重新加载已加载行业
}

Response 200:
{
  "results": {
    "finance": { "agents_created": 30, "teams_created": 5, "taxonomy_nodes": 40 },
    "selfmedia": { "agents_created": 25, "teams_created": 3, "taxonomy_nodes": 35 }
  },
  "already_loaded": ["softwaredev"],
  "errors": {}
}
```

### 3.2 卸载附带生态

```
POST /api/v1/admin/ecosystem/preset/unload

Request:
{
  "industries": ["finance"]
}

Response 200:
{
  "results": {
    "finance": {
      "agents_deleted": 30,
      "teams_deleted": 5,
      "taxonomy_nodes_deleted": 40,
      "teams_modified": 2
    }
  }
}
```

### 3.3 查询加载状态

```
GET /api/v1/admin/ecosystem/preset/status

Response 200:
{
  "finance": { "loaded": true, "loaded_at": "...", "agents": 30, "teams": 5, "taxonomy_nodes": 40 },
  "selfmedia": { "loaded": false },
  "softwaredev": { "loaded": false }
}
```

## 4. 前端设计

### 4.1 系统设置 — 附带生态区块（已实现）

集成在 `web/src/stores/system-settings/` 和 `web/src/features/system-settings/` 中。

### 4.2 行业分类树形布局（未实现）

当前仍为扁平卡片布局，待改造为树形折叠 + 岗位卡片混合布局：

```
┌──────────────────────────────────────────────────────┐
│  行业分类管理                    [搜索] [仅看自建]      │
├──────────────────────────────────────────────────────┤
│                                                      │
│  ▼ 金融                              [编辑][+部门]   │
│    ▼ 量化交易                        [编辑][+岗位]   │
│      ┌─────────┐ ┌─────────┐ ┌─────────┐           │
│      │量化研究员│ │算法交易  │ │量化开发  │ ← 可拖拽  │
│      └─────────┘ └─────────┘ └─────────┘           │
│    ▶ 风控合规                                        │
│    ▶ 投资研究                                        │
│                                                      │
│  ▶ 自媒体                                            │
│  ▶ 软件开发                                          │
│                                                      │
└──────────────────────────────────────────────────────┘
```

组件结构：
```
TaxonomyPage.vue
  └── TaxonomyTree.vue（改造）
        ├── TaxonomyIndustryNode.vue（行业节点，QExpansionItem）
        │     └── TaxonomyDepartmentNode.vue（部门节点，QExpansionItem）
        │           └── TaxonomyPositionCard.vue（岗位卡片，QCard + vuedraggable）
        └── 操作按钮（新增/编辑/删除/启停）
```

### 4.3 Kind 徽章组件（已实现）

`web/src/components/agents/KindBadge.vue` — 根据 kind 显示不同样式徽章。

## 5. 数据迁移（已执行）

### 5.1 DDL 迁移

```sql
-- system_settings 新增 ecosystem_loaded 字段
ALTER TABLE system_settings ADD COLUMN ecosystem_loaded TEXT NOT NULL DEFAULT '{}';

-- teams 新增 kind 字段
ALTER TABLE teams ADD COLUMN kind TEXT NOT NULL DEFAULT 'user';
```

### 5.2 数据迁移

```sql
-- Agent Kind 迁移
UPDATE agents SET kind = 'system_builtin' WHERE kind = 'system';
UPDATE agents SET kind = 'ecosystem_preset' WHERE kind = 'industry_template';

-- Team Kind 初始化
UPDATE teams SET kind = 'ecosystem_preset' WHERE source = 'imported';
```

### 5.3 回滚策略

- DDL 迁移仅新增列（无破坏性），可安全回滚
- Kind 枚举回滚需反向数据迁移

## 6. 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| Kind 枚举变更兼容性 | 旧代码引用已删除枚举值 | 数据迁移脚本 + 全量编译验证 ✅ |
| 部分加载失败 | 某行业加载失败影响其他行业 | 按行业独立记录状态，互不影响 ✅ |
| 卸载操作误操作 | 用户误删大量数据 | 确认对话框 + 软删除 + 重新加载能力 ✅ |
| 卸载时跨行业 Team | Team 成员分属多个行业 | 保留跨行业 Team，仅移除被卸载行业的 Agent 成员 ✅ |
| Pack 引擎 Kind 覆盖影响现有调用 | 现有 Pack 导入行为变更 | `WithKindOverride` 为可选参数，不传时行为不变 ✅ |
| 行业 Pack 数据不完整 | selfmedia/softwaredev 无数据，finance 严重缺失 | 需业务方补充行业知识内容 ❌ |
