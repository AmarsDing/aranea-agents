# 生态商城模块 — 实现设计文档

> 对应需求：`30-ecosystem.md`
> 开发计划与现状：`30-ecosystem.development.md`

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
    option (google.api.http) = { post: "/v1/ecosystem/products/{id}/install" body: "*" };
  }
  rpc UninstallProduct(UninstallProductRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/ecosystem/products/{id}/install" };
  }
}
```

`Product` 消息字段：`id / name / display_name / description / type / author_id / version / price_model / price_cents / rating / install_count / config_json / status / created_at / updated_at / installed`。

`InstallResult` 消息字段：`product_id / installed_ids / message`。

> **注意**：需求中规划的评价/评分、订单、版本快照等 RPC 尚未实现，待后续 Phase 新增。

### 2.2 附带生态 API（已实现，纯 HTTP）

```
POST   /api/v1/admin/ecosystem/preset/load    — 加载行业预设
POST   /api/v1/admin/ecosystem/preset/unload  — 卸载行业预设
GET    /api/v1/admin/ecosystem/preset/status   — 查询加载状态
```

非 gRPC，由 `EcosystemPresetService` 直接注册 HTTP 路由（见 `internal/server/http.go`）。

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
    Message      string
}

type Repo interface {
    ListProducts(ctx, query) (ListResult, error)
    GetProduct(ctx, id) (Product, error)
    CreateProduct(ctx, p Product) (Product, error)
    RecordInstall(ctx, productID, refID) error
    RemoveInstall(ctx, productID) error
    IsInstalled(ctx, productID) (bool, error)
}

type Usecase struct { repo Repo; lg loggateway.Logger }
func (uc *Usecase) List(ctx, query) (ListResult, error)
func (uc *Usecase) Get(ctx, id) (Product, error)
func (uc *Usecase) Publish(ctx, p Product) (Product, error)
func (uc *Usecase) Install(ctx, id string) (InstallResult, error)
func (uc *Usecase) Uninstall(ctx, id string) error
```

> **当前限制**：`Install` 仅调用 `RecordInstall` 记录安装关系并自增 `install_count`，未解析 `config_json` 创建实际 Agent/Team/Skill 资源。待 Phase 2 补全。

### 3.2 附带生态（已实现）

```go
// internal/biz/ecosystem_preset.go
type EcosystemLoadedStatus map[string]IndustryLoadInfo

type IndustryLoadInfo struct {
    Loaded   bool   `json:"loaded"`
    LoadedAt string `json:"loaded_at,omitempty"`
    Agents   int    `json:"agents,omitempty"`
    Teams    int    `json:"teams,omitempty"`
    OrgNodes int    `json:"org_nodes,omitempty"`
}

type EcosystemPresetRepo interface {
    GetEcosystemLoaded(ctx) (EcosystemLoadedStatus, error)
    SetEcosystemLoaded(ctx, status EcosystemLoadedStatus) error
    DeleteOrgNodesByCompany(ctx, companyKey) (int, error)
    DeleteAgentsByIndustry(ctx, industryKey) (int, error)
    DeleteTeamsByIndustry(ctx, industryKey) (deleted int, modified int, err error)
}

// PackSeeder 抽象 Pack 引擎播种操作，避免 *ent.Client 泄漏到 biz 层
type PackSeeder interface {
    SeedPackIndustry(ctx, scenarioDir, industryKey, kindOverride string) (agentsCreated int, teamsCreated int, err error)
}

type EcosystemPresetUsecase struct {
    repo        EcosystemPresetRepo
    packSeeder  PackSeeder
    lg          loggateway.Logger
    scenarioDir string
    mu          sync.Mutex  // 防止并发加载/卸载
}

func (uc *EcosystemPresetUsecase) LoadEcosystemPreset(ctx, industries, force) (*EcosystemLoadResponse, error)
func (uc *EcosystemPresetUsecase) UnloadEcosystemPreset(ctx, industries) (*EcosystemUnloadResponse, error)
func (uc *EcosystemPresetUsecase) GetEcosystemStatus(ctx) (EcosystemLoadedStatus, error)
```

默认行业列表：`DefaultIndustries = []string{"finance", "selfmedia", "softwaredev"}`

> **当前限制**：`PackSeeder.SeedPackIndustry` 实现位于 `internal/data/seed_pack.go`，但 `pack.ConvertCompanySpecToPack` 尚未实现，调用时返回 `BadRequest` 错误。即附带生态加载功能当前不可用。

---

## 四、Data 层

### 4.1 生态市场（已实现，原生 SQL）

**不使用 Ent ORM**，通过原生 SQL 操作以下表：

- `ecosystem_products` — 产品目录表（DDL: `internal/data/sql/ecosystem_product.sql`）
- `ecosystem_installs` — 安装记录表（DDL: 同上）

```go
// internal/data/ecosystem.go
type ecosystemRepo struct { data *Data }
func NewEcosystemRepo(data *Data) biz.EcosystemRepo
```

读写分离：读用 `r.data.RWDB().ReadDB(ctx)`，写用 `r.data.RWDB().WriteDB(ctx)`，安装记录写入使用 `r.data.ExecInTx` 事务包裹（同时插入 install 记录 + 自增 install_count）。

### 4.2 附带生态（已实现）

```go
// internal/data/ecosystem_preset.go
type EcosystemPresetRepo struct { data *Data; lg loggateway.Logger }
func NewEcosystemPresetRepo(d *Data) *EcosystemPresetRepo

func (r *EcosystemPresetRepo) GetEcosystemLoaded(ctx) (EcosystemLoadedStatus, error)
func (r *EcosystemPresetRepo) SetEcosystemLoaded(ctx, status EcosystemLoadedStatus) error
func (r *EcosystemPresetRepo) DeleteOrgNodesByCompany(ctx, companyKey) (int, error)
func (r *EcosystemPresetRepo) DeleteAgentsByIndustry(ctx, industryKey) (int, error)
func (r *EcosystemPresetRepo) DeleteTeamsByIndustry(ctx, industryKey) (deleted int, modified int, err error)
```

级联删除逻辑：taxonomy（organizations 表）→ agents → teams，跨行业 Team 保留但移除已删除 Agent 成员。

- `DeleteAgentsByIndustry` 在事务内执行软删除 + `cascadeDeleteByAgent` 清理 runtime_settings/prompt_files/sessions
- `DeleteTeamsByIndustry` 分类为"全删"与"修改成员"两组，全删组调用 `cascadeDeleteByTeam` 清理关联数据

### 4.3 数据库表结构

#### ecosystem_products（原生 SQL，V20260703）

```sql
CREATE TABLE IF NOT EXISTS ecosystem_products (
  id TEXT NOT NULL PRIMARY KEY,
  name TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL DEFAULT 'skill_pack',
  author_id TEXT NOT NULL DEFAULT 'system',
  version TEXT NOT NULL DEFAULT '1.0.0',
  price_model TEXT NOT NULL DEFAULT 'free',
  price_cents INTEGER NOT NULL DEFAULT 0,
  rating REAL NOT NULL DEFAULT 0,
  install_count INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL DEFAULT 'published',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_ecosystem_products_type ON ecosystem_products(type);
```

#### ecosystem_installs（原生 SQL，V20260703）

```sql
CREATE TABLE IF NOT EXISTS ecosystem_installs (
  id TEXT NOT NULL PRIMARY KEY,
  product_id TEXT NOT NULL,
  installed_ref_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_ecosystem_installs_product ON ecosystem_installs(product_id);
```

#### agents.kind / teams.kind / system_settings.ecosystem_loaded（Ent Schema + DDL 迁移）

- `agents.kind` — Ent Schema `internal/data/ent/schema/agent.go`，枚举 `user | system_builtin | ecosystem_preset | marketplace | certified`，默认 `user`
- `teams.kind` — Ent Schema `internal/data/ent/schema/team.go`，枚举同 Agent，默认 `user`，索引 `idx_teams_kind`
- `system_settings.ecosystem_loaded` — Ent Schema `internal/data/ent/schema/system_setting.go`，`field.Text("ecosystem_loaded").Default("{}")`

### 4.4 数据库迁移

| 版本 | 名称 | 内容 | 实现位置 |
|------|------|------|----------|
| V20260703 | `ecosystem_schema` | 创建 `ecosystem_products` + `ecosystem_installs` 表 | `ddlEcosystemSchema` → `EnsureEcosystemSchema` |
| V20260718 | `ecosystem_preset_schema` | DDL: `system_settings.ecosystem_loaded` 列 + `teams.kind` 列；数据迁移: `agents.kind` system→system_builtin、industry_template→ecosystem_preset；`teams.kind` source=imported→ecosystem_preset | `ddlEcosystemPresetDataMigration` + `sql/migrations/20260718_ecosystem_preset_schema.sql` |

> V20260718 数据迁移在事务中执行，包含 Kind 枚举值映射与 source 字段同步逻辑。

---

## 五、Service 层

### 5.1 生态市场（已实现）

```go
// internal/service/ecosystem.go
type EcosystemService struct {
    v1.UnimplementedEcosystemServiceServer
    uc *biz.EcosystemUsecase
}
func (s *EcosystemService) ListProducts(ctx, req) (*ListProductsResponse, error)
func (s *EcosystemService) GetProduct(ctx, req) (*Product, error)
func (s *EcosystemService) PublishProduct(ctx, req) (*Product, error)
func (s *EcosystemService) InstallProduct(ctx, req) (*InstallResult, error)
func (s *EcosystemService) UninstallProduct(ctx, req) (*emptypb.Empty, error)
```

`GetProduct` 在产品不存在时返回 `apierror.NotFound("ECOSYSTEM_NOT_FOUND", ...)`。

### 5.2 附带生态（已实现）

```go
// internal/service/ecosystem_preset.go
type EcosystemPresetService struct { uc *biz.EcosystemPresetUsecase }

func (s *EcosystemPresetService) HandleLoad() func(ctx kratoshttp.Context) error
func (s *EcosystemPresetService) HandleUnload() func(ctx kratoshttp.Context) error
func (s *EcosystemPresetService) HandleStatus() func(ctx kratoshttp.Context) error
```

路由注册位于 `internal/server/http.go`：

```go
srv.Route("/").POST("/api/v1/admin/ecosystem/preset/load", ecosystemPresetSvc.HandleLoad())
srv.Route("/").POST("/api/v1/admin/ecosystem/preset/unload", ecosystemPresetSvc.HandleUnload())
srv.Route("/").GET("/api/v1/admin/ecosystem/preset/status", ecosystemPresetSvc.HandleStatus())
```

### 5.3 system_builtin 删除保护（已实现）

```go
// internal/biz/agent_usecase.go:579
return apierror.Forbidden("AGENT", "cannot delete system_builtin agent")

// internal/biz/team_usecase.go:528
return apierror.Forbidden("TEAM", "cannot delete system_builtin team")
```

---

## 六、Wire 注入（已实现）

```
data.ProviderSet  → NewEcosystemRepo + NewEcosystemPresetRepo + NewPackSeeder
biz.ProviderSet   → NewEcosystemUsecase + NewEcosystemPresetUsecase
service.ProviderSet → NewEcosystemService + NewEcosystemPresetService

wire.Bind(new(biz.EcosystemPresetRepo), new(*data.EcosystemPresetRepo))
wire.Bind(new(biz.PackSeeder), new(*data.PackSeeder))
provideEcosystemPresetScenarioDir  // 返回 biz.ScenarioDir()
```

---

## 七、Web 前端设计

### 7.1 文件结构（已实现）

```
web/src/
├── pages/
│   ├── EcosystemPage.vue                # 商城浏览首页（路由 /shop）
│   ├── ShopAssetPage.vue                # 商品详情页（/shop/a/:slug）
│   ├── ShopCreatorPage.vue              # 创作者主页（/shop/u/:handle）
│   ├── ShopMePage.vue                   # 买家工作台（/shop/me）
│   ├── ShopStudioPage.vue               # 创作者中心（/shop/studio）
│   └── ShopPublishPage.vue              # 发布向导（/shop/publish）
├── features/ecosystem/
│   ├── api.ts                           # API 入口：M30 gRPC-Web（既有）+ M57 商城 mock 实现
│   ├── types.ts                         # 商城领域类型（MarketAsset/OrgBundlePreview/...）
│   ├── mock.ts                          # M57 商城 mock 数据（11 类资产、分类树、订单、工作室）
│   ├── marketUi.ts                      # UI 常量与格式化（资产类型图标/颜色、价格、安装量）
│   ├── useMarketBrowsePage.ts           # 浏览首页 composable
│   ├── useMarketAssetDetail.ts          # 详情页 composable（安装确认/评分提交）
│   └── useEcosystemPage.ts              # M30 既有页面 composable（保留兼容）
├── stores/ecosystem/index.ts            # useEcosystemStore（M30 + M57 商城状态）
├── components/ecosystem/                # 16 个商城专用组件
│   ├── AssetCard.vue                    # 资产卡片（网格单元）
│   ├── AssetTypeIcon.vue                # 资产类型图标（11 类，含 org_bundle）
│   ├── PriceTag.vue                     # 价格标签（免费/买断/订阅/企业）
│   ├── RatingStars.vue                  # 星级评分
│   ├── CategoryTree.vue                 # 分类树（域 → 多级分类，图标来自数据）
│   ├── MarketFilterBar.vue              # 过滤栏（类型 Chip 组 + 价格 + 排序 + 重置）
│   ├── MarketLeaderboard.vue            # 榜单卡片（热门/最新/高分）
│   ├── InstallConfirmDialog.vue         # 安装确认弹窗（权限 + 高风险警示）
│   ├── PermissionList.vue               # 权限清单（按风险分级着色）
│   ├── AssetScreenshots.vue             # 截图横向滚动区
│   ├── ReviewSection.vue                # 评价列表 + 写评价
│   ├── ReplyReviewDialog.vue            # 创作者回复评价弹窗
│   ├── TrendSparkline.vue               # 趋势迷你图（统计卡片用）
│   ├── PublishTypeSelect.vue            # 发布向导第一步：资产类型选择卡片组
│   ├── OrgBundleTree.vue                # 详情页：组织整包预览树（只读）
│   └── OrgBundlePicker.vue              # 发布向导：组织节点树勾选器（可交互）
├── features/system-settings/
│   ├── api.ts                           # 含 preset load/unload/status
│   ├── types.ts                         # 含 EcosystemLoadedStatus 等类型
│   └── useEcosystemPreset.ts            # 附带生态 composable
├── stores/system-settings/index.ts      # 含 ecosystemLoaded + preset actions
└── components/agents/KindBadge.vue      # Kind 徽章组件
```

### 7.2 商城前端类型与 API 契约

M57 商城领域类型（`features/ecosystem/types.ts`）：

```typescript
export type MarketAssetType =
  | 'skill' | 'mcp_server' | 'tool' | 'plugin' | 'agent' | 'team'
  | 'channel_template' | 'knowledge_pack' | 'workflow' | 'company_bundle'
  | 'org_bundle'; // 组织架构整包：部门 + 岗位 + Agent 整体打包

export type PriceModel = 'free' | 'one_time' | 'subscription' | 'enterprise';

export interface MarketAsset {
  id: string; slug: string; name: string; type: MarketAssetType;
  version: string; summary: string; readme: string; category: string;
  tags: string[]; priceModel: PriceModel; priceCents: number;
  rating: number; ratingCount: number; installCount: number;
  creator: MarketCreator; permissions: AssetPermission[];
  dependencies: AssetDependency[]; screenshots: string[];
  publishedAt: string; installed: boolean;
  orgBundle?: OrgBundlePreview; // 仅 org_bundle：部门/岗位/Agent 预览树
  reviews: AssetReview[]; versions: AssetVersion[];
}

export interface OrgBundleNode {
  id: string; kind: 'department' | 'position' | 'agent';
  name: string; children?: OrgBundleNode[];
}
```

浏览过滤与买家/创作者侧类型：

```typescript
export interface BrowseFilter {
  search: string; type: MarketAssetType | ''; category: string;
  priceModel: PriceModel | ''; sort: 'hot' | 'new' | 'rating';
}
export interface MyInstall { /* 资产 + 版本 + 安装时间 + 7 日健康度 + 状态 */ }
export interface MyOrder { /* 订单号 + 金额 + 状态(paid/refunding/refunded) */ }
export interface StudioStats { /* 总安装/总收益/均分/在售数 + 趋势序列 */ }
export interface OrgPickNode { /* 发布向导可勾选组织树节点（含勾选态） */ }
```

API 层（`features/ecosystem/api.ts`）双轨：M30 既有 gRPC-Web（`listEcosystemProducts` 等）保留；M57 商城接口当前为 mock 实现（`searchAssets` / `getAsset` / `listCategories` / `installAsset` / `uninstallAsset` / `listMyInstalls` / `listMyOrders` / `getStudioStats` / `listStudioAssets` / `listStudioInbox` / `getOrgPickTree` / `submitReview`），后端 Proto 就绪后按同签名替换为真实 RPC。

### 7.3 商城页面组件设计

**EcosystemPage.vue（/shop）**：`AppPageHero`（kicker/标题/副标题 + 三入口按钮 + Hero 内嵌搜索框）→ 左侧 `CategoryTree`（col-md-3/lg-2）→ 右侧 `MarketFilterBar` + 默认视图三榜单（`MarketLeaderboard` × hot/new/top）或过滤后 `AssetCard` 网格；骨架屏 + 空态；composable `useMarketBrowsePage`。

**ShopAssetPage.vue（/shop/a/:slug）**：面包屑 + 资产头部卡（`AssetTypeIcon` 56px + 名称/类型 Chip/版本/创作者/评分安装量 + `PriceTag` + 安装/卸载 CTA）→ 左 Tab（README `v-html` 渲染 / `OrgBundleTree`（仅 org_bundle）/ 版本 / `ReviewSection`）→ 右侧栏（`PermissionList`、`AssetScreenshots`、依赖、元信息）；有权限时先弹 `InstallConfirmDialog`（含高风险警示）；composable `useMarketAssetDetail`。

**ShopCreatorPage.vue（/shop/u/:handle）**：创作者资料卡（统计四项 + 关注按钮）+ 作品 `AssetCard` 网格。

**ShopMePage.vue（/shop/me）**：已安装 Tab（`AppRegistryTable`：资产/版本/安装时间/7 日健康度/状态 + 更新/卸载）+ 订单 Tab（订单号/金额/时间/状态 Chip）。

**ShopStudioPage.vue（/shop/studio）**：统计卡片 × 4（含 `TrendSparkline`）+ 我的资产表格（`ReplyReviewDialog` 处理评价回复）+ 评价收件箱 Tab。

**ShopPublishPage.vue（/shop/publish）**：`QStepper` 四步——① `PublishTypeSelect`（11 类含 org_bundle）→ ② 基本信息（名称/slug 自动推导（支持中文 slug）/分类/定价/README 编辑预览/版本/兼容性/标签）→ ③ org_bundle 专属 `OrgBundlePicker`（组织树勾选 + 实时统计）→ ④ 确认发布（摘要表 + 勾选汇总）。

### 7.4 Pinia Store 设计

```typescript
// stores/ecosystem/index.ts（setup store；M30 状态保留兼容，M57 商城状态新增）
export const useEcosystemStore = defineStore('ecosystem', () => {
  // M30 既有：products / loading / load / install / publish

  // M57 浏览页
  const categories = ref<CategoryNode[]>([]);
  const filter = reactive<BrowseFilter>({ search: '', type: '', category: '', priceModel: '', sort: 'hot' });
  const assets = ref<MarketAsset[]>([]);
  const browseLoading = ref(false);
  async function browse()            // searchAssets({ ...filter })
  function resetFilter()             // 重置 filter 并重新 browse

  // M57 详情 / 创作者
  const assetDetail = ref<MarketAsset | null>(null);
  const creatorDetail = ref<{ creator: MarketCreator | null; assets: MarketAsset[] }>(...);
  async function loadAsset(slug: string)
  async function loadCreator(handle: string)

  // M57 买家 / 创作者中心 / 发布向导
  const myInstalls = ref<MyInstall[]>([]);
  const myOrders = ref<MyOrder[]>([]);
  const studioStats = ref<StudioStats | null>(null);
  const studioAssets = ref<StudioAsset[]>([]);
  const studioInbox = ref<StudioInboxItem[]>([]);
  const orgPickTree = ref<OrgPickNode[]>([]);
  async function installAssetById(id: string)   // 安装后同步 assets/assetDetail/myInstalls 状态
  async function uninstallAssetById(id: string)
  async function submitReview(assetId: string, rating: number, content: string)
});
```

> 注意：`filter` 为 `reactive` 对象。composable 中通过 `storeToRefs` 解构会被包成 `Ref`，脚本内访问需用 `store.filter` 直接引用以保持响应性（`useMarketBrowsePage` 即此模式）；模板中 v-model 绑定不受影响。

### 7.5 附带生态前端类型与 API

```typescript
// features/system-settings/types.ts
export interface IndustryLoadInfo {
  loaded: boolean;
  loaded_at?: string;
  agents?: number;
  teams?: number;
  taxonomy_nodes?: number;
}
export type EcosystemLoadedStatus = Record<string, IndustryLoadInfo>;

export interface EcosystemLoadResult {
  agents_created: number;
  teams_created: number;
  taxonomy_nodes: number;
}
export interface EcosystemLoadResponse {
  results: Record<string, EcosystemLoadResult>;
  already_loaded?: string[];
  errors?: Record<string, string>;
}

export interface EcosystemUnloadResult {
  agents_deleted: number;
  teams_deleted: number;
  taxonomy_nodes_deleted: number;
  teams_modified?: number;
}
export interface EcosystemUnloadResponse {
  results: Record<string, EcosystemUnloadResult>;
  errors?: Record<string, string>;
}

// features/system-settings/api.ts
export async function loadEcosystemPreset(industries?: string[], force?: boolean): Promise<EcosystemLoadResponse>
export async function unloadEcosystemPreset(industries: string[]): Promise<EcosystemUnloadResponse>
export async function getEcosystemPresetStatus(): Promise<EcosystemLoadedStatus>
```

### 7.6 KindBadge 组件（已实现）

`web/src/components/agents/KindBadge.vue` — 根据 kind 显示不同样式徽章。

| Kind | 徽章文字 | 颜色 |
|------|----------|------|
| `system_builtin` | 内置 | `var(--color-accent)` |
| `ecosystem_preset` | 预设 | `var(--color-positive)` |
| `marketplace` | 商城 | `var(--color-accent-indigo, #4F46E5)` |
| `certified` | 认证 | `var(--color-warning, #F09B54)` |

### 7.7 system_builtin 前端保护（已实现）

- `web/src/components/teams/TeamCard.vue` — `v-if="team.kind !== 'system_builtin'"` 隐藏复制与删除按钮
- `web/src/components/chat/ChatEntitySidebar.vue` — 过滤 `system_builtin` Agent 不展示在聊天侧栏

---

## 八、子模块 B：种子数据分层与管道设计

### 8.1 种子数据分层模型

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

### 8.2 种子管道架构

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

L1 管道使用 Pack 引擎 + `WithKindOverride("ecosystem_preset")`（注：builtin-templates Pack 当前使用 `ecosystem_preset` 而非 `system_builtin`，详见开发计划）。

L2 管道通过 `PackSeeder.SeedPackIndustry` 调用，当前实现存在阻塞（见 §8.3）。

### 8.3 SeedPackIndustry 实现现状

```go
// internal/data/seed_pack.go
func SeedPackIndustry(ctx, client, scenarioDir, industryKey, kindOverride, lg) (int, int, error) {
    spec, loadErr := loader.LoadCompanySpec(scenarioDir, industryKey)
    if loadErr != nil { return 0, 0, entErrToBizErr(loadErr, "SEED") }

    // TODO(debt): pack.ConvertCompanySpecToPack is not yet implemented.
    _ = spec
    return 0, 0, apierror.BadRequest("SEED",
        fmt.Sprintf("pack.ConvertCompanySpecToPack not yet implemented for industry %s", industryKey))
}
```

> **阻塞点**：`pack.ConvertCompanySpecToPack` 未实现，L2 管道当前无法实际加载行业数据。调用 `POST /api/v1/admin/ecosystem/preset/load` 会返回 400 错误。

### 8.4 ecosystem_loaded 状态存储

```json
// system_settings.ecosystem_loaded 字段（TEXT, JSON 格式）
{
  "finance": {
    "loaded": true,
    "loaded_at": "2026-06-05T10:00:00Z",
    "agents": 30,
    "teams": 5,
    "org_nodes": 40
  },
  "selfmedia": {
    "loaded": false
  },
  "softwaredev": {
    "loaded": false
  }
}
```

### 8.5 API 响应契约

#### 加载附带生态

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
    "finance": { "agents_created": 30, "teams_created": 5, "taxonomy_nodes": 40 }
  },
  "already_loaded": ["softwaredev"],
  "errors": {}
}
```

#### 卸载附带生态

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

#### 查询加载状态

```
GET /api/v1/admin/ecosystem/preset/status

Response 200:
{
  "finance": { "loaded": true, "loaded_at": "...", "agents": 30, "teams": 5, "taxonomy_nodes": 40 },
  "selfmedia": { "loaded": false },
  "softwaredev": { "loaded": false }
}
```

### 8.6 行业分类树形布局（未实现）

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

组件结构（规划）：

```
TaxonomyPage.vue
  └── TaxonomyTree.vue（改造）
        ├── TaxonomyIndustryNode.vue（行业节点，QExpansionItem）
        │     └── TaxonomyDepartmentNode.vue（部门节点，QExpansionItem）
        │           └── TaxonomyPositionCard.vue（岗位卡片，QCard + vuedraggable）
        └── 操作按钮（新增/编辑/删除/启停）
```

### 8.7 数据迁移回滚策略

- DDL 迁移仅新增列（无破坏性），可安全回滚
- Kind 枚举回滚需反向数据迁移（`system_builtin` → `system`、`ecosystem_preset` → `industry_template`）

---

## 九、风险与缓解

| 风险 | 影响 | 缓解措施 | 状态 |
|------|------|----------|------|
| Kind 枚举变更兼容性 | 旧代码引用已删除枚举值 | 数据迁移脚本 + 全量编译验证 | ✅ 已缓解 |
| 部分加载失败 | 某行业加载失败影响其他行业 | 按行业独立记录状态，互不影响 | ✅ 已缓解 |
| 卸载操作误操作 | 用户误删大量数据 | 确认对话框 + 软删除 + 重新加载能力 | ✅ 已缓解 |
| 卸载时跨行业 Team | Team 成员分属多个行业 | 保留跨行业 Team，仅移除被卸载行业的 Agent 成员 | ✅ 已缓解 |
| Pack 引擎 Kind 覆盖影响现有调用 | 现有 Pack 导入行为变更 | `WithKindOverride` 为可选参数，不传时行为不变 | ✅ 已缓解 |
| `pack.ConvertCompanySpecToPack` 未实现 | L2 行业 Pack 加载管道不可用 | 需实现 CompanySpec → Pack 转换函数 | ❌ 阻塞中 |
| builtin-templates Pack Kind 标记 | 当前使用 `ecosystem_preset` 而非 `system_builtin` | 需确认是否应为 `system_builtin` | ⚠️ 待确认 |
