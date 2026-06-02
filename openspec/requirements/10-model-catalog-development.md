# Model Catalog — 开发计划

> **版本**：2026-05-25 | **状态**：✅ Phase 1–4 + 迁移 UX 优化完成  
> **需求**：[10 model-catalog.md](./10%20model-catalog.md)  
> **外部源**：[github.com/anomalyco/models.dev](https://github.com/anomalyco/models.dev)

---

## 1. 模块定位

models.dev 集成本地缓存 + sync + Provider UI 默认值 + DB 自动更新 + **内置** provider 迁移 + Usage 定价统一（USD/1M）。

**代码锚点（当前）**：

| 路径 | 职责 |
|------|------|
| `api/kratos/model_catalog/v1/model_catalog.proto` | Catalog API（apply-migration、provider-migration 只读规则） |
| `internal/modelcatalog/` | fetch、store、sync、apply、migrate、overlay、logos、checkpoint |
| `internal/modelcatalog/overlay.go` | `ProviderMigration` embed 真相源 |
| `internal/biz/model_catalog.go` | Usecase + ModelCatalogStoreProvider |
| `internal/data/model_catalog_apply.go` | ApplyBackend（事务迁移） |
| `internal/data/usage_breakdown_alias.go` | Usage 展示 alias + `canonical_provider_code` |
| `internal/service/model_catalog.go` | gRPC/HTTP |
| `web/src/features/model-catalog/` | API + applyCatalog + providerLogo |
| `web/src/pages/SystemSettingsCatalogTab.vue` | Settings Tab（迁移预览 + 立即对齐，无映射编辑） |
| `internal/modelcatalog/runtime_overlay.json` | 运行时映射（Go embed） |

---

## 2. 开发阶段

### Phase 1 — 基础设施 ✅

- [x] 需求/设计文档
- [x] `model_catalog.proto` + `make api`
- [x] `internal/modelcatalog`：types、store、fetch、sync、logs、runner、overlay
- [x] `ModelCatalogService` + wire + server 注册
- [x] System Settings **Model Catalog Tab**
- [x] 启动时 scheduled background sync runner
- [x] 动态 Store root（SystemSetting root_directory）
- [x] SSRF urlguard、sync 互斥、apply 失败 surfacing

### Phase 2 — Provider UI ✅ 基本可用

- [x] Resource Manager 目录模式（catalog 选 Provider/Model → 自动填 config）
- [x] 自定义 Provider/模型入口（Ollama / 本地）
- [x] `capability_chips` + ProviderModelRow / ProviderLogo 展示
- [x] 精简 `providerPresets.ts` → `providerRuntimeOverlay.ts` + JSON
- [ ] 目录选模从 modalities 推导 vision chip（前端未读 API modalities）
- [ ] 表单展示 `catalog_env` / `catalog_doc`

### Phase 3 — 定价统一 USD/1M ⚠️ 部分完成

- [x] `config_json.cost.*_usd_per_1m` 读写（含 cache_read/write）
- [x] `model_pricing_rules` USD/1M 列
- [x] catalog sync → UpsertModelPricing
- [ ] Usage `ApplyTokenUsageCosts` 以 USD/1M 为主路径（micro 双读保留）
- [ ] 旧 micro/1K 双读兼容 → **删除**（待稳定一个版本）

### Phase 4 — 自动同步 + 强制迁移 ✅

- [x] 内置 `ProviderMigration` + 自动/手动事务迁移
- [x] sync applier：merge llm_provider_models + metadata
- [x] agents / sessions / eval / runtime / skills / knowledge_embed / web_research 事务迁移
- [x] deprecated → enabled=false
- [x] Usage 展示层 alias
- [x] Preview Migration API
- [x] Provider logo sync + API
- [x] ETag 304 条件 GET

### Phase 5 — 迁移 UX 优化（P0–P3）✅ 2026-05-25

见需求文档 [§12](./10%20model-catalog.md#12-已完成p0p32026-05-25)。

| ID | 内容 | 状态 |
|----|------|------|
| P0-1 | 移除可编辑 migration-map UI/API | ✅ |
| P0-2 | `usage_alias` 合并进 `ProviderMigration` 单源 | ✅ |
| P1-1 | sync 响应 `apply_errors` / `apply_failed` | ✅ |
| P1-2 | `POST apply-migration` + Settings「立即对齐」 | ✅ |
| P2-1 | `migration-checkpoint.json` 替代可写映射文件 | ✅ |
| P3-1 | `canonical_provider_code` 写入 usage 事件 | ✅ |

---

## 3. 任务清单（历史）

### P0–P3 增量（2026-05-25，已完成）

| # | 任务 | 文件 |
|---|------|------|
| P0-1 | 内置迁移单源 | `overlay.go`, `migration_map.go`, `migrate_bindings.go` |
| P0-2 | 删除映射编辑 UI | `SystemSettingsCatalogTab.vue`, `api.ts` |
| P1-1 | sync apply 错误回传 | `model_catalog.proto`, `service/model_catalog.go`, `runner.go` |
| P1-2 | 手动迁移 API | `ApplyProviderMigration`, `biz/model_catalog.go` |
| P2-1 | migration checkpoint | `migration_checkpoint.go`, `store.go` |
| P3-1 | usage canonical 列 | `pricing_patch.go`, `usage_write.go`, `08_usage.sql` |

---

## 4. 待优化 Backlog

见需求文档 **[§13 待优化](./10%20model-catalog.md#13-待优化backlog)**。

| 优先级 | 项 |
|--------|-----|
| **P3** | E2E 测试（sync→apply→RM smoke） |

---

## 5. 验证

```bash
make api && make wire-admin && go build ./cmd/admin
go test ./internal/modelcatalog/... -count=1
go test ./internal/data/ -run 'MergeUsage|MigrateProvider|Alias|RecordToken' -count=1
```

手动：

1. System Settings → Model Catalog → 立即同步（观察 apply 错误提示）
2. 迁移预览 → **立即对齐**
3. Resource Manager → 目录模式选 Provider/Model
4. Usage 页按 `alibaba-cn` 筛选，确认含 legacy `aliyun-qwen` 数据

---

## 6. 风险

| 风险 | 缓解 | 状态 |
|------|------|------|
| models.dev 不可达 | 保留上次 current.json；304 不破坏缓存 | ✅ |
| 迁移误伤 custom | catalog_source=custom skip | ✅ |
| 用户误配映射 | **已移除** 可编辑映射 | ✅ |
| overlay Go/TS 漂移 | `make check-overlay` | ✅ |
