# 生态商城模块 — 实现设计文档

> 对应需求：`30 ecosystem.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

生态商城（Ecosystem Marketplace）：Agent 模板、Skill 包、Team 编排方案的发现、安装、发布、交易和治理体系。

---

## 二、Proto 层

### 2.1 待新增

```protobuf
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
  rpc RateProduct(RateProductRequest) returns (ProductRating) {
    option (google.api.http) = { post: "/v1/ecosystem/products/{id}/ratings" body: "*" };
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type Product struct {
    ID          string
    Name        string
    DisplayName string
    Description string
    Type        string  // "agent_template"/"skill_pack"/"team_blueprint"
    AuthorID    string
    Version     string
    PriceModel  string  // "free"/"paid"/"subscription"
    PriceCents  int64
    Rating      float64
    InstallCount int64
    ConfigJSON  string  // 产品定义
    Status      string  // "draft"/"published"/"deprecated"
    CreatedAt   string
    UpdatedAt   string
}

type ProductRating struct {
    ID        string
    ProductID string
    UserID    string
    Score     int32
    Comment   string
    CreatedAt string
}

type InstallResult struct {
    ProductID   string
    InstalledIDs []string  // 安装后生成的 Agent/Skill/Team ID
    Conflicts   []Conflict
}
```

### 3.2 Usecase

```go
func (uc *EcosystemUsecase) ListProducts(ctx, query) (ProductListResult, error)
func (uc *EcosystemUsecase) GetProduct(ctx, id) (Product, error)
func (uc *EcosystemUsecase) PublishProduct(ctx, p Product) (Product, error)
func (uc *EcosystemUsecase) InstallProduct(ctx, id string) (InstallResult, error)
func (uc *EcosystemUsecase) RateProduct(ctx, productID string, rating ProductRating) (ProductRating, error)
```

---

## 四、Data 层

### 4.1 Ent Schema

- `internal/data/ent/schema/ecosystem_product.go` — 产品表
- `internal/data/ent/schema/ecosystem_rating.go` — 评分表
- `internal/data/ent/schema/ecosystem_install.go` — 安装记录表

---

## 五、Service 层

```go
func (s *EcosystemService) ListProducts(ctx, req) (*ListProductsResponse, error)
func (s *EcosystemService) GetProduct(ctx, req) (*Product, error)
func (s *EcosystemService) PublishProduct(ctx, req) (*Product, error)
func (s *EcosystemService) InstallProduct(ctx, req) (*InstallResult, error)
func (s *EcosystemService) RateProduct(ctx, req) (*ProductRating, error)
```

---

## 六、Wire 注入

待新增：
```
data.ProviderSet → NewEcosystemRepo
biz.ProviderSet → NewEcosystemUsecase
service.ProviderSet → NewEcosystemService
```

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/features/ecosystem/
├── api.ts
├── types.ts
└── components/
    ├── MarketplacePage.vue
    ├── ProductCard.vue
    ├── ProductDetailPage.vue
    ├── ProductInstallDialog.vue
    ├── ProductPublishDialog.vue
    └── ProductRatingForm.vue
```

### 7.2 组件设计

**MarketplacePage.vue**：

| 区域 | 组件 | 说明 |
|------|------|------|
| 搜索 | `QInput` | 关键词搜索 |
| 分类 | `QBtnToggle` | Agent/Skill/Team |
| 排序 | `QSelect` | 评分/安装量/最新 |
| 列表 | `ProductCard` 网格 | 产品卡片 |

**ProductCard.vue**：名称/描述/评分/安装量/价格/安装按钮

**ProductDetailPage.vue**：详情 + 评分 + 安装/卸载

### 7.3 API

```typescript
export async function listProducts(query: ProductQuery): Promise<ProductListResult>
export async function getProduct(id: string): Promise<Product>
export async function publishProduct(req: PublishProductRequest): Promise<Product>
export async function installProduct(id: string): Promise<InstallResult>
export async function rateProduct(id: string, req: RateRequest): Promise<ProductRating>
```
