# Model Catalog（models.dev 集成）

> **版本**：2026-06-17 | **状态**：✅ 核心可用（P0–P3 + Backlog 高/中/低 已完成）
> **外部数据源**：[anomalyco/models.dev](https://github.com/anomalyco/models.dev) · **唯一官方 API**：`https://models.dev/api.json`
> **关联**：[9 provider.md](./9%20provider.md)
> **设计**：[12-model-catalog.design.md](./12-model-catalog.design.md) · **开发计划**：[12-model-catalog.development.md](./12-model-catalog.development.md)

---

## 1. 目标

将 [models.dev](https://models.dev) 作为 **AI 模型规格的外部真相源**，Aranea 本地缓存 JSON，用于：

- Provider / 模型添加表单的**默认值**（定价 USD/1M、context、能力）
- 定期 **sync** 更新已有 `llm_provider_models` 规格与定价
- **强制迁移** 旧 provider 命名到 models.dev id
- **Agent / Session** 绑定 provider/model 随迁移更新
- System Settings 独立 Tab：策略、手动 sync、JSON 查看、更新日志、**迁移预览与一键对齐**（无用户可编辑映射表）

**边界**：models.dev **仅提供默认参数**，不参与任何运行时业务（连接、鉴权、Turn 执行仍走 trpc-agent-go + `internal/provider`）。

---

## 2. 用户故事

### 2.1 资源管理者

- 作为资源管理者，我希望从 models.dev 目录中选择 Provider 和 Model，自动填入默认定价（USD/1M）、context 限制和能力 chips，省去手动查阅文档
- 作为资源管理者，我希望能够添加自定义 Provider/模型（如 Ollama 本地部署），不被 catalog 自动覆盖
- 作为资源管理者，我希望在列表中看到 Provider Logo 和能力 chips，快速识别模型能力

### 2.2 系统管理员

- 作为系统管理员，我希望在 System Settings 中配置 sync 策略（off / scheduled）和 auto_apply 级别
- 作为系统管理员，我希望手动触发 sync（含 dry run），并看到 apply 错误反馈
- 作为系统管理员，我希望预览 Provider 迁移影响（哪些表、多少行受影响），并一键执行对齐
- 作为系统管理员，我希望查看同步日志和当前 catalog JSON

### 2.3 Agent 开发者

- 作为 Agent 开发者，我希望 Provider 命名迁移后，我的 Agent / Session / Eval 绑定自动更新，无需手动改配置
- 作为 Agent 开发者，我希望 deprecated 模型自动 disabled，避免误用

### 2.4 Usage 报表使用者

- 作为 Usage 报表使用者，我希望按 `alibaba-cn` 筛选时能看到历史 `aliyun-qwen` 的数据（alias 合并），不因迁移丢失历史统计

---

## 3. 功能需求清单

### 3.1 Catalog 同步

- **FR-1**：从 `https://models.dev/api.json` 拉取完整 catalog，本地缓存为 `current.json`
- **FR-2**：支持 ETag 条件 GET（`If-None-Match` + 304 跳过写入）
- **FR-3**：支持 scheduled 自动 sync（可配置间隔，默认 24h）
- **FR-4**：支持手动 sync（含 dry run 模式）
- **FR-5**：sync 失败时保留上次缓存，断网可用旧缓存
- **FR-6**：sync 重试（指数退避，最多 4 次，503/5xx/网络错误）
- **FR-7**：source_url 白名单仅允许 `https://models.dev/api.json`（urlguard）

### 3.2 Provider/Model 目录

- **FR-10**：目录模式选 Provider → 选 Model → 自动填 config（USD/1M 定价、context/output 限制、capability_chips）
- **FR-11**：自定义模式添加 Ollama / 本地 OpenAI 兼容部署（`catalog_source=custom`）
- **FR-12**：列表行展示 ProviderLogo + capability chips
- **FR-13**：从 modalities 推导 vision chip
- **FR-14**：展示 `catalog_env` 横幅和 `catalog_doc` 文档链接

### 3.3 自动应用（auto_apply）

- **FR-20**：`none` — 只更新本地 JSON + logos
- **FR-21**：`metadata_and_pricing`（默认）— merge 规格+定价；不覆盖密钥/HA/用户 category
- **FR-22**：`full_spec` — 含 context/limit/modality/capability_chips
- **FR-23**：`full_spec_and_runtime_overlay` — 含 catalog.api / overlay 的 base URL（不覆盖用户改过的 URL）
- **FR-24**：`catalog_managed=true` 且 `status=deprecated` → `enabled=false`
- **FR-25**：`catalog_source=custom` 或 `catalog_managed=false` → 跳过 auto_apply
- **FR-26**：apply 失败时 sync 响应回传 `apply_errors` / `apply_failed`，日志 `status=partial`

### 3.4 Provider 迁移

- **FR-30**：内置 `ProviderMigration` 规则（Go embed，随版本发布，不可编辑）
- **FR-31**：sync 的 `auto_apply` 或手动 API 触发事务迁移，更新 agents / sessions / eval / runtime / skills / knowledge_embed / web_research / llm 行
- **FR-32**：迁移预览 API 返回各表受影响行数
- **FR-33**：`migration-checkpoint.json` 记录上次成功迁移时间戳 + 版本
- **FR-34**：`migration-map.json` 已废弃（若存在则忽略）

### 3.5 Usage 定价统一

- **FR-40**：config_json 定价以 USD/1M 为单位（含 cache_read/write/reasoning/embedding）
- **FR-41**：`model_pricing_rules` 双写 USD/1M + micro/1K 列
- **FR-42**：Usage 计费内核 `ApplyTokenUsageCosts` 优先 USD/1M（micro/1K 仅 legacy fallback）
- **FR-43**：Usage 事件写入时冗余 `canonical_provider_code`
- **FR-44**：Usage 报表读侧 alias 合并 legacy provider 维度（Top / 筛选 / 事件展示）
- **FR-45**：Usage 历史 DB 不 rewrite `provider_code`

### 3.6 Provider Logo

- **FR-50**：sync 时拉取 `https://models.dev/logos/{id}.svg`，缓存至 `logos/{id}.svg`
- **FR-51**：404 回退 `default.svg`；sync 开始时主动拉取 `default.svg`
- **FR-52**：API `GET /v1/model-catalog/logos/{provider_id}` 返回 SVG

### 3.7 Runtime Overlay

- **FR-60**：`runtime_overlay.json` Go embed 运行时映射（provider_type / variant / auth_type / 中国区 URL）
- **FR-61**：前端 `web/src/config/provider_runtime_overlay.json` 须手动保持同步
- **FR-62**：`make check-overlay` + `TestRuntimeOverlayMatchesWebCopy` 校验双文件一致

### 3.8 interleaved 推理格式

- **FR-70**：`interleaved` 字段 merge（`applyInterleavedHints` → `interleaved` / `interleaved_field` / `reasoning_content_backfill`）

---

## 4. 非功能需求

- **NFR-1**：大 catalog 性能 — `offset` 分页 + `SearchCatalogRaw` 服务端 JSON 搜索；Settings Tab 不全量拉 raw
- **NFR-2**：sync 互斥 — 同一时刻只允许一个 sync 进行
- **NFR-3**：SSRF 防护 — urlguard 仅允许 `/api.json`，拒绝 `/models.json`
- **NFR-4**：OpenAPI 规范 — `PreviewMigration` POST 增加 `body: "*"`
- **NFR-5**：动态 Store root — 通过 SystemSetting `root_directory` 配置，不硬编码

---

## 5. 验收标准

- [x] System Settings 有 Model Catalog Tab，可 sync、看 JSON、看日志、配策略
- [x] 本地 `current.json` 在 sync 后更新；ETag 304 保留旧缓存；断网可用旧缓存
- [x] 目录模式选模型自动填 USD/1M 定价与 capability_chips
- [x] 自定义模式可添加 Ollama 本地模型
- [x] sync 后 deprecated 模型 auto disabled（catalog_managed）
- [x] 强制迁移更新 agents / sessions / eval 等绑定（事务）
- [x] Usage 报表 alias 合并旧 provider 维度（Top / 筛选 / 事件展示）
- [x] Provider logo 本地缓存与 API 展示
- [x] Usage **计费内核** USD/1M 优先（`ApplyTokenUsageCosts` → `usageCostMicro` 优先 USD/1M；micro/1K 仅 legacy fallback）
- [x] `interleaved` 字段 merge（`applyInterleavedHints` → `interleaved` / `interleaved_field` / `reasoning_content_backfill`）

---

## 6. 交互规格（用户视角）

### 6.1 System Settings → Model Catalog Tab

1. 进入 Tab 看到**状态概览**（last_sync_at / etag / provider_count / model_count / catalog_bytes）
2. **策略表单**：source_url（只读）/ sync_policy（off|scheduled）/ sync_interval_hours / auto_apply（none|metadata_and_pricing|full_spec|full_spec_and_runtime_overlay）
3. **手动 sync** 按钮 → 弹出 dry run 选项 → 执行后显示 apply_errors（如有）
4. **JSON 查看器**：格式化展示 current.json（支持服务端搜索）
5. **同步日志**：列表展示历史 sync 记录（status / message / details_json）
6. **Provider 迁移预览**：展示内置规则列表（legacy → catalog）+ 各表受影响行数
7. **立即对齐**按钮 → 执行事务迁移 → 显示结果

### 6.2 Resource Manager → 添加 Provider/Model

1. **目录模式**：选 Provider（带 Logo）→ 选 Model（带 capability chips）→ 自动填入 config（定价 USD/1M、limit、chips）
2. **自定义模式**：填入 provider_type / api_base_url / api_key → `catalog_source=custom`
3. 列表行展示 ProviderLogo + capability chips + env 横幅 + doc 链接

### 6.3 Usage 报表

1. 按 provider 筛选时，选择 `alibaba-cn` 自动包含历史 `aliyun-qwen` 数据
2. Top 列表合并 legacy provider 维度
3. 事件展示走 alias

---

> **架构设计、API 契约、数据模型、代码锚点**：详见 [12-model-catalog.design.md](./12-model-catalog.design.md)
>
> **开发进度、任务清单、Phase 划分、验证命令**：详见 [12-model-catalog.development.md](./12-model-catalog.development.md)
