# 生态商城设计

本文档设计 Arenea 的 **生态商城（Ecosystem Marketplace）**。商城不是简单的资源展示页，而是围绕 **Agent、Skill、Team** 三类可复用能力建立发现、评估、安装、交易、发布和治理体系。

目标是让用户可以像安装应用一样，把外部或团队内部沉淀的 Agent、Skill、Team 编排安装到当前 workspace，并且能追踪版本、权限、费用、评分和运行质量。

---

## 1. 核心目标

| 目标 | 说明 |
|------|------|
| 能力交易平台 | 支持 Agent 模板、Skill 包、Team 编排方案上架、浏览、安装和购买 |
| 降低搭建成本 | 用户可以直接复用成熟能力，而不是从零创建 Agent、Skill、Team |
| 支撑创作者生态 | 允许团队或第三方创作者发布能力，积累评分、安装量和收入 |
| 保证可信安装 | 每个商品需要权限声明、版本快照、签名校验、审核状态和风险提示 |
| 连接现有架构 | 商品安装后写入现有 agents、skills、teams 等模块，而不是另起一套运行系统 |
| 为商业化预留 | 支持免费、买断、订阅、企业授权、分账和退款等交易模型 |

---

## 2. 商品类型

商城第一期聚焦三类商品：

| 类型 | 商品内容 | 安装后落点 |
|------|----------|------------|
| Agent 模板 | Agent 角色、系统提示词、模型配置、工具策略、头像、分类 | `agents`、agent runtime settings、prompt files |
| Skill 包 | Skill 描述、版本、执行脚本/说明、参数 schema、示例、权限要求 | `skill`、skill version、skill invocation |
| Team 编排 | 多 Agent 拓扑、角色分工、编排模式、默认成员、运行策略 | `teams`、team definition、team runtime |

后续可扩展：

| 类型 | 说明 |
|------|------|
| Tool 包 | 内置工具或外部 API 工具定义 |
| Plugin 包 | 拦截器、回调点、运行增强插件 |
| MCP Server 模板 | MCP server 连接配置、工具发现规则和权限说明 |
| Channel 模板 | 飞书、Slack、Email、Webhook 等渠道接入模板 |
| Workflow 模板 | 跨 Agent / Tool / Skill 的业务流程模板 |

---

## 3. 前端页面设计

当前前端已将 `/shop` 的 `EcosystemPage.vue` 升级为商城原型。页面结构如下：

| 区域 | 内容 |
|------|------|
| Hero 区 | 商城定位、GMV、商品数、创作者数、安装数、浏览和发布入口 |
| 类型入口 | Agent 模板、Skill 包、Team 编排三类入口卡片 |
| 商品列表 | 搜索、类型筛选、免费/付费筛选、排序、商品卡片 |
| 商品卡片 | 名称、创作者、认证、价格、评分、安装量、信任分、标签、查看按钮 |
| 详情弹窗 | 商品说明、能力清单、评分、安装量、价格、安装到工作区 |
| 发布弹窗 | 选择商品类型、名称、描述，后续生成审核任务 |
| 交易治理栏 | 审核发布、可信安装、交易分账说明 |
| 榜单 | 本周高评分 / 高安装商品 |
| 发布流程 | 打包、审核、上架三个步骤 |

Quasar 组件映射：

| 功能 | 组件 |
|------|------|
| 页面 | `QPage` + `QCard` |
| 搜索 | `QInput` |
| 类型筛选 | `QBtnToggle` |
| 价格 / 排序 | `QSelect` |
| 商品展示 | `QCard` + `QChip` + `QAvatar` |
| 详情 | `QDialog` + `QExpansionItem` |
| 发布流程 | `QTimeline` |
| 状态 / 认证 | `QBadge` + `QIcon` |

主题要求：

| 模式 | 要求 |
|------|------|
| 白昼模式 | 复用全局 `app-page-cream` 奶油白视觉系统 |
| 黑夜模式 | 使用 `body.body--dark` 下深色渐变背景、深色卡片、低对比边框 |
| 响应式 | 桌面为左列表右治理栏；移动端改为单列 |

---

## 4. 商品卡字段

前端商品卡建议字段：

```ts
export type MarketplaceProductType = "agent" | "skill" | "team";

export type MarketplaceProduct = {
  id: string;
  type: MarketplaceProductType;
  name: string;
  creator_id: string;
  creator_name: string;
  description: string;
  tags: string[];
  price_cents: number;
  pricing_type: "free" | "one_time" | "subscription" | "enterprise";
  rating: number;
  install_count: number;
  trust_score: number;
  verified: boolean;
  status: "draft" | "reviewing" | "listed" | "rejected" | "delisted";
  latest_version: string;
  capabilities: string[];
  permissions_summary: string[];
  created_at: string;
  updated_at: string;
};
```

商品详情需要补充：

```ts
export type MarketplaceProductDetail = MarketplaceProduct & {
  readme_markdown: string;
  changelog_markdown: string;
  screenshots: string[];
  versions: MarketplaceProductVersion[];
  reviews: MarketplaceReview[];
  install_schema_json: string;
  manifest_json: string;
};
```

---

## 5. 数据模型建议

### 5.1 商品主表：`marketplace_products`

```sql
CREATE TABLE IF NOT EXISTS marketplace_products (
  id TEXT PRIMARY KEY,
  product_type TEXT NOT NULL,
  -- agent / skill / team

  name TEXT NOT NULL,
  summary TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  tags_json TEXT NOT NULL DEFAULT '[]',

  creator_id TEXT NOT NULL DEFAULT '',
  creator_name TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',

  pricing_type TEXT NOT NULL DEFAULT 'free',
  price_cents INTEGER NOT NULL DEFAULT 0,
  currency TEXT NOT NULL DEFAULT 'CNY',

  status TEXT NOT NULL DEFAULT 'draft',
  -- draft / reviewing / listed / rejected / delisted
  visibility TEXT NOT NULL DEFAULT 'public',
  -- public / workspace / private

  latest_version TEXT NOT NULL DEFAULT '',
  rating REAL NOT NULL DEFAULT 0,
  rating_count INTEGER NOT NULL DEFAULT 0,
  install_count INTEGER NOT NULL DEFAULT 0,
  purchase_count INTEGER NOT NULL DEFAULT 0,
  trust_score INTEGER NOT NULL DEFAULT 0,
  verified INTEGER NOT NULL DEFAULT 0,

  cover_image_url TEXT NOT NULL DEFAULT '',
  icon TEXT NOT NULL DEFAULT '',
  readme_markdown TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',

  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  listed_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT ''
);
```

推荐索引：

```sql
CREATE INDEX IF NOT EXISTS idx_marketplace_products_type_status
  ON marketplace_products(product_type, status, updated_at);

CREATE INDEX IF NOT EXISTS idx_marketplace_products_creator
  ON marketplace_products(creator_id, updated_at);

CREATE INDEX IF NOT EXISTS idx_marketplace_products_score
  ON marketplace_products(status, trust_score, rating, install_count);
```

### 5.2 商品版本：`marketplace_product_versions`

每次发布或更新商品都生成不可变版本快照。

```sql
CREATE TABLE IF NOT EXISTS marketplace_product_versions (
  id TEXT PRIMARY KEY,
  product_id TEXT NOT NULL,
  version TEXT NOT NULL,

  artifact_type TEXT NOT NULL,
  -- agent_template / skill_package / team_template
  artifact_json TEXT NOT NULL DEFAULT '{}',
  manifest_json TEXT NOT NULL DEFAULT '{}',
  permissions_json TEXT NOT NULL DEFAULT '[]',
  changelog_markdown TEXT NOT NULL DEFAULT '',

  package_hash TEXT NOT NULL DEFAULT '',
  signature TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'draft',
  -- draft / reviewing / approved / rejected / deprecated

  created_at TEXT NOT NULL,
  reviewed_at TEXT NOT NULL DEFAULT '',
  reviewer_id TEXT NOT NULL DEFAULT '',
  review_note TEXT NOT NULL DEFAULT '',

  UNIQUE(product_id, version)
);
```

### 5.3 安装记录：`marketplace_installs`

安装记录连接商城商品与当前 workspace 的实际 Agent / Skill / Team。

```sql
CREATE TABLE IF NOT EXISTS marketplace_installs (
  id TEXT PRIMARY KEY,
  product_id TEXT NOT NULL,
  version_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',

  installed_resource_type TEXT NOT NULL,
  -- agent / skill / team
  installed_resource_id TEXT NOT NULL DEFAULT '',

  status TEXT NOT NULL DEFAULT 'installed',
  -- installed / update_available / disabled / removed
  installed_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  removed_at TEXT NOT NULL DEFAULT '',

  config_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}'
);
```

### 5.4 交易订单：`marketplace_orders`

```sql
CREATE TABLE IF NOT EXISTS marketplace_orders (
  id TEXT PRIMARY KEY,
  product_id TEXT NOT NULL,
  version_id TEXT NOT NULL DEFAULT '',
  buyer_workspace_id TEXT NOT NULL DEFAULT '',
  buyer_user_id TEXT NOT NULL DEFAULT '',
  seller_id TEXT NOT NULL DEFAULT '',

  pricing_type TEXT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'CNY',
  amount_cents INTEGER NOT NULL DEFAULT 0,
  platform_fee_cents INTEGER NOT NULL DEFAULT 0,
  seller_amount_cents INTEGER NOT NULL DEFAULT 0,

  status TEXT NOT NULL DEFAULT 'pending',
  -- pending / paid / failed / refunded / cancelled
  payment_provider TEXT NOT NULL DEFAULT '',
  payment_ref TEXT NOT NULL DEFAULT '',

  created_at TEXT NOT NULL,
  paid_at TEXT NOT NULL DEFAULT '',
  refunded_at TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}'
);
```

### 5.5 评分评论：`marketplace_reviews`

```sql
CREATE TABLE IF NOT EXISTS marketplace_reviews (
  id TEXT PRIMARY KEY,
  product_id TEXT NOT NULL,
  user_id TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  rating INTEGER NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'visible',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,

  UNIQUE(product_id, user_id, workspace_id)
);
```

---

## 6. API 设计

### 6.1 商品列表

```http
GET /api/v1/marketplace/products?type=agent&pricing=free&keyword=writer&sort=featured&page=1&page_size=20
```

响应：

```json
{
  "items": [],
  "total": 0,
  "page": 1,
  "page_size": 20
}
```

筛选参数：

| 参数 | 说明 |
|------|------|
| `type` | `agent` / `skill` / `team` |
| `pricing` | `free` / `paid` |
| `keyword` | 搜索名称、简介、标签、创作者 |
| `sort` | `featured` / `rating` / `install_count` / `newest` |
| `status` | 管理端可筛选 draft / reviewing / listed |

### 6.2 商品详情

```http
GET /api/v1/marketplace/products/{id}
```

返回商品主信息、版本、能力说明、权限声明、评分和安装状态。

### 6.3 发布商品

```http
POST /api/v1/marketplace/products
```

Agent 发布示例：

```json
{
  "product_type": "agent",
  "source_agent_id": "agent_xxx",
  "name": "增长文案 Agent",
  "summary": "生成多渠道增长文案",
  "pricing_type": "one_time",
  "price_cents": 9900
}
```

创建后进入 `draft` 或 `reviewing` 状态。

### 6.4 安装商品

```http
POST /api/v1/marketplace/products/{id}/install
```

安装规则：

| 商品类型 | 安装行为 |
|----------|----------|
| Agent | 根据版本快照创建新 Agent，复制 runtime settings、prompt files |
| Skill | 创建 skill 与版本，注册参数 schema 和执行配置 |
| Team | 创建 Team，复制 definition_json 和成员引用策略 |

响应：

```json
{
  "install_id": "install_xxx",
  "installed_resource_type": "agent",
  "installed_resource_id": "agent_new"
}
```

### 6.5 购买 / 订单

```http
POST /api/v1/marketplace/products/{id}/checkout
GET /api/v1/marketplace/orders/{id}
```

第一期可以先保留接口与状态模型，不接真实支付。企业内使用可先支持 `free` 和 `workspace_license`。

---

## 7. 发布与审核流程

```text
创建商品草稿
  -> 生成版本快照
  -> 生成 manifest 与 permissions
  -> 提交审核
  -> 自动安全检查
  -> 人工审核
  -> 上架
  -> 用户安装 / 购买
```

审核项：

| 审核项 | 说明 |
|--------|------|
| 权限声明 | 是否调用工具、MCP、文件、网络、命令、第三方服务 |
| 运行风险 | 是否会写文件、执行命令、发送外部请求、读取敏感数据 |
| 内容质量 | 描述、截图、README、示例是否完整 |
| 依赖完整性 | Skill / Team 是否依赖不存在的工具、Agent、模型或插件 |
| 安装可逆 | 是否支持禁用、卸载、版本回滚 |
| 安全签名 | artifact hash 和 signature 是否匹配 |

---

## 8. 权限与安全

商城安装必须显式展示权限声明。

权限类型建议：

| 权限 | 示例 |
|------|------|
| `model:invoke` | 调用模型 |
| `tool:run` | 调用内置工具 |
| `mcp:call` | 调用 MCP server |
| `file:read` | 读取用户上传文件 |
| `file:write` | 写入产物文件 |
| `network:request` | 访问外部 URL |
| `command:execute` | 执行本地命令，高风险 |
| `notion:write` | 写入第三方工作区 |

安装确认页需要展示：

| 内容 | 说明 |
|------|------|
| 商品来源 | 创作者、认证状态、版本 |
| 权限清单 | 明确列出高风险能力 |
| 数据流向 | 是否会发送外部服务 |
| 费用影响 | 是否会产生模型调用、工具调用或订阅费用 |
| 回滚方式 | 如何禁用、卸载、恢复旧版本 |

---

## 9. 与现有系统的关系

商城只负责 **发现、交易、发布、安装**，运行时仍复用现有模块。

| 现有模块 | 商城关系 |
|----------|----------|
| Agent 管理 | 安装 Agent 商品后创建 Agent |
| Skill 管理 | 安装 Skill 商品后创建 Skill 与版本 |
| Team 管理 | 安装 Team 商品后创建 Team definition |
| Plugin / Tool / MCP | 作为商品依赖或未来商品类型 |
| Session 历史 | 安装后的运行行为仍进入 session / message / usage 体系 |
| Usage 统计 | 商城商品可按 installed_resource_id 统计运行成本和质量 |
| Monitor | 商品安装、审核、运行异常可写入 monitor/audit |

---

## 10. 前端状态与类型

建议新增独立 store：

```ts
export const useMarketplaceStore = defineStore("marketplace", {
  state: () => ({
    query: {} as MarketplaceProductQuery,
    items: [] as MarketplaceProduct[],
    total: 0,
    selected: null as MarketplaceProductDetail | null,
    loading: false
  }),
  actions: {
    async search() {},
    async loadDetail(id: string) {},
    async install(id: string) {},
    async publish(payload: MarketplacePublishInput) {}
  }
});
```

当前前端原型暂时使用页面内静态数据，后续接后端时迁移到 store。

---

## 11. 落地阶段

### Phase 1：前端商城原型

| 工作 | 说明 |
|------|------|
| 页面 | `/shop` 建立商城首页 |
| 商品卡 | Agent / Skill / Team 三类卡片 |
| 交互 | 搜索、筛选、排序、详情弹窗、发布弹窗 |
| 主题 | 适配白昼奶油白与黑夜模式 |

当前已完成。

### Phase 2：商品与安装 API

| 工作 | 说明 |
|------|------|
| 数据库 | `marketplace_products`、`marketplace_product_versions`、`marketplace_installs` |
| API | 商品列表、详情、发布、安装 |
| 安装器 | Agent / Skill / Team 从 artifact_json 写入现有模块 |
| 前端 | 静态数据改为 API 数据 |

### Phase 3：审核与可信治理

| 工作 | 说明 |
|------|------|
| 审核流 | draft / reviewing / listed / rejected |
| 权限声明 | manifest + permissions 展示 |
| 签名校验 | package hash、signature |
| 审计 | 安装、审核、上架、卸载写 audit log |

### Phase 4：交易商业化

| 工作 | 说明 |
|------|------|
| 订单 | `marketplace_orders` |
| 支付 | checkout、支付回调、退款 |
| 授权 | workspace license、订阅状态 |
| 分账 | 平台费用、创作者收入 |

### Phase 5：质量与推荐

| 工作 | 说明 |
|------|------|
| 评分评论 | `marketplace_reviews` |
| 榜单 | 安装榜、评分榜、趋势榜 |
| 质量指标 | 运行成功率、平均成本、平均延迟 |
| 推荐 | 根据当前 workspace 的 Agent / Skill 使用情况推荐商品 |

---

## 12. 关键设计原则

1. **商品是版本化能力包**：安装的是某个版本快照，不是实时引用创作者当前配置。
2. **运行复用现有模块**：商城不直接执行 Agent / Skill / Team，只负责安装和治理。
3. **权限先于安装**：任何可能访问文件、网络、MCP、命令或第三方服务的能力都必须明确声明。
4. **审核与审计不可省略**：发布、上架、安装、卸载、购买都应可追踪。
5. **免费与付费共存**：内部生态可先免费流转，外部生态再接支付和分账。
6. **信任分可解释**：信任分应来自认证、审核、安装量、评分、运行成功率、安全事件等可解释指标。
7. **Team 是一等商品**：Team 编排不是多个 Agent 的简单组合，应保存拓扑、角色、执行策略和依赖声明。

