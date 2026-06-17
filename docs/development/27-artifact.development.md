# Artifact 产出物 — 开发计划

> **版本**：2026-06-17 | **状态**：🟢 P1–P3 全部完成；Phase 4 S3/COS 生产配置**后续支持**
> **需求**：[27-artifact.md](./27-artifact.md) · **设计**：[27-artifact.design.md](./27-artifact.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-RT-08

---

## 1. 模块定位

Artifact 产出物：管理 Agent 运行时产生的文件、图片、代码等产出物，支持存储、版本管理、预览和下载。

**代码锚点**：
- `api/kratos/artifact/v1/artifact.proto` — Artifact CRUD + PreviewArtifact + SignDownloadUrl + ListArtifactVersions + DeleteArtifactVersion Proto 定义
- `internal/service/artifact.go` — ArtifactService（Kratos Service 层，含全部 RPC + ServeSignedDownload）
- `internal/biz/artifact/` — ArtifactUsecase + ArtifactReader/ArtifactWriter/ArtifactRepo 接口（子包，`internal/biz/artifact.go` 为 re-export 桥接层）
- `internal/data/artifactfs/repo.go` — FSArtifactRepo（本地文件系统实现）
- `internal/artifact/trpc/service.go` — ServiceAdapter（trpc artifact.Service 桥接）
- `internal/artifact/sign.go` — HMAC-SHA256 签名/验签（生产 fail-closed）
- `internal/artifact/storage_factory.go` — 存储后端工厂（local/s3/cos 选择）
- `internal/skill/trpc/artifact_executor.go` — artifactSavingExecutor（CodeExecutor 产出物自动保存）
- `web/src/features/artifact/ArtifactPreview.vue` — 独立预览组件（图片/PDF/代码）
- `web/src/features/artifact/ArtifactList.vue` — 制品列表组件（含预览弹窗+签名下载）
- `web/src/features/artifact/api.ts` — 前端 API（含全部 RPC 对应方法）
- `web/src/features/artifact/types.ts` — 前端类型定义
- `web/src/stores/artifact/index.ts` — 前端 Pinia Store
- `web/src/pages/ArtifactsPage.vue` — 管理页面（使用 ArtifactPreview + ArtifactsUploadDialog + ArtifactsDetailDialog）
- `web/src/components/artifact/ArtifactsUploadDialog.vue` — 上传对话框组件
- `web/src/components/artifact/ArtifactsDetailDialog.vue` — 详情对话框组件（含版本选择）
- `web/src/components/chat/ChatSessionArtifactsPanel.vue` — Chat 会话制品面板（使用 ArtifactList 组件）
- `web/src/components/chat/ChatMessageAttachments.vue` — Chat 消息气泡内嵌附件 chip + 预览

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Proto 定义 | ✅ | UploadArtifact / GetArtifact / ListArtifacts / DeleteArtifact / DeleteArtifactVersion / ListArtifactVersions / PreviewArtifact / SignDownloadUrl |
| Service 层 | ✅ | base64 编解码 + **10 MB** 上限 + 参数校验 + 全部 RPC + ServeSignedDownload |
| Biz 层 | ✅ | Artifact 模型 + ArtifactReader/ArtifactWriter/ArtifactRepo 组合接口 + Usecase（含 Preview/StorageBytes/DeleteVersion，构造注入 loggateway.Logger） |
| biz 子包辅助文件 | ✅ | `limits.go`（MaxUploadBytes 10MB）+ `filter.go`（FilterArtifacts 内存过滤）+ `turn_collector.go`（TurnCollector 产出物收集）+ `attachments_resolve.go`（附件解析+session 校验）+ `options_merge.go`（options_json 合并） |
| Data 层（FS） | ✅ | FSArtifactRepo + 版本管理 + JSON sidecar + LoadMeta/DeleteVersion/StorageBytes |
| trpc 适配器 | ✅ | ServiceAdapter 实现 trpcartifact.Service 全部 5 个方法 |
| Wire 注入 | ✅ | NewArtifactRepo → NewArtifactUsecase → NewArtifactService + provideArtifactRuntimeService + provideArtifactSigner |
| HTTP/gRPC 注册 | ✅ | http.go + grpc.go 已注册 + `/v1/artifacts/download` 签名下载路由 |
| 前端 API + Store | ✅ | features/artifact/api.ts（含全部 RPC 对应方法）+ stores/artifact/index.ts |
| 测试覆盖 | ✅ | service/artifact_test.go + biz/artifact/*_test.go + data/artifactfs/repo_test.go + artifact/trpc/service_test.go + artifact/sign_test.go + 前端 limits.spec.ts |
| Runner 注入 ArtifactService | ✅ | `TRPCRunnerDeps.ArtifactService` + `WithArtifactService` + Wire `provideArtifactRuntimeService` |
| CodeExecutor 产出物自动保存 | ✅ | `artifactSavingExecutor` + `WrapWithArtifactSave` + `NewExecutor` 自动包装 |
| 管理页面 | ✅ | `ArtifactsPage.vue`；使用 `ArtifactPreview` + `ArtifactsUploadDialog` + `ArtifactsDetailDialog` |
| PreviewArtifact RPC | ✅ | Proto + Service 层 + 前端 API + ArtifactPreview.vue 组件 |
| 签名下载 URL | ✅ | `artifact/sign.go`（生产 fail-closed）+ `SignDownloadUrl` RPC + `ServeSignedDownload` HTTP handler + 前端集成 |
| ArtifactPreview.vue 独立组件 | ✅ | 图片 `<img>` / PDF `<iframe>` / 代码 `<pre>` 渲染 + 下载按钮 |
| ArtifactList.vue 列表组件 | ✅ | MIME 图标 + 文件大小 + 版本号 + 预览弹窗 + 签名下载 |
| Chat 会话制品嵌入 | ✅ | `ChatSessionArtifactsPanel` 使用 `ArtifactList` 组件 |
| Chat 消息气泡内嵌附件 | ✅ | `options_json.attachments` + `ChatMessageAttachments.vue`（ART-01） |
| Prometheus 上传/下载/存储指标 | ✅ | `aranea_artifact_*` 于 Service + FS Repo |
| 跨会话制品列表 | ✅ | `ListArtifacts` 省略 `session_id` 时扫描全部会话 |
| ListArtifactVersions RPC | ✅ | Proto + Service + Usecase.ListVersions + 前端 api.listArtifactVersions + Store.listVersions |
| DeleteArtifactVersion RPC | ✅ | Proto + Service + Usecase.DeleteVersion + 前端 api.deleteArtifactVersion + Store.removeVersion |
| ListArtifacts 服务端检索 | ✅ | `query` + `mime_type_prefix` 参数 + Usecase 内存过滤 |
| ArtifactsPage 对话框组件拆分 | ✅ | `ArtifactsUploadDialog` + `ArtifactsDetailDialog`（含版本选择） |
| TurnCollector 产出物收集 | ✅ | `biz/artifact/turn_collector.go` + `WithTurnCollector` / `CollectorFromContext` |
| 多模态附件进 LLM | ✅ | `BuildUserMessageFromArtifacts` + `RunTRPCUserTurnMsg` |
| S3/COS 存储工厂 | ✅ | `storage_factory.go` 委托 trpc-agent-go 框架包；生产配置待 Phase 4 |
| S3/COS 生产配置 | 📋 后续支持 | 工厂已存在，生产配置与多租户路径隔离依赖 M2（EP-WS-01） |

---

## 3. 差距与优化

1. **Phase 4（后续支持）**：S3/COS 生产配置与多租户路径隔离（依赖 M2 EP-WS-01）；存储工厂已实现，委托 trpc-agent-go 框架包。
2. ~~**ART-01**：Chat 消息气泡内嵌制品预览~~ ✅
3. ~~**跨会话列表**：管理页无 session 筛选时可浏览全部制品~~ ✅
4. ~~**大文件限制**~~：单文件上限 **10 MB**（前后端统一校验）；>10 MB 提示不支持；流式传输后续支持。
5. ~~**Team 路径附件/产出物气泡**~~：TurnCollector + `ArtifactUC` on TurnDeps ✅
6. ~~**版本历史 API**~~：`ListArtifactVersions` + `DeleteArtifactVersion` + 管理页版本 chip ✅
7. ~~**Team 多模态附件**~~：`RunTRPCUserTurnMsg` + 共享 `BuildUserMessageFromArtifacts` ✅
8. ~~**管理页组件拆分**~~：上传/详情对话框独立组件 ✅
9. ~~**列表检索过滤**~~：`query` + `mime_type_prefix` 参数 ✅

---

## 4. 开发阶段

- **Phase 1**：~~Runner 注入 ArtifactService~~（✅）+ ~~CodeExecutor 产出物自动保存~~（✅）
- **Phase 2**：~~管理页列表/基础预览~~（✅）+ ~~PreviewArtifact RPC~~（✅）+ ~~ArtifactPreview.vue~~（✅）+ ~~ArtifactList.vue + Chat 嵌入~~（✅）
- **Phase 3**：~~签名下载 URL（HMAC-SHA256）~~（✅）+ ~~Chat 气泡附件（ART-01）~~（✅）+ ~~Prometheus 指标~~（✅）+ ~~ListArtifactVersions + DeleteArtifactVersion~~（✅）+ ~~列表检索过滤~~（✅）+ ~~TurnCollector + 多模态附件~~（✅）
- **Phase 4（后续支持）**：S3/COS 生产配置 + 按租户隔离

---

## 5. 任务清单

| # | 任务 | 优先级 | Phase | 涉及文件 | 状态 |
|---|------|--------|-------|----------|------|
| 1 | TRPCRunnerDeps 增加 ArtifactService 字段 + WithArtifactService 注入 | P1 | 1 | `internal/agent/trpc_runtime.go` | ✅ |
| 2 | Wire 提供 ServiceAdapter 实例 | P1 | 1 | `internal/artifact/trpc/service.go`, Wire 集成 | ✅ |
| 3 | CodeExecutor 产出物自动保存为 Artifact | P1 | 1 | `internal/skill/trpc/artifact_executor.go` | ✅ |
| 4 | ArtifactsPage 管理界面 | P2 | 2 | `web/src/pages/ArtifactsPage.vue` | ✅ |
| 5 | PreviewArtifact RPC + 在线预览 | P2 | 2 | Proto + Service + 前端 API | ✅ |
| 6 | 签名下载 URL（HMAC-SHA256） | P3 | 3 | `internal/artifact/sign.go` + Service + HTTP handler | ✅ |
| 7 | ArtifactPreview.vue 制品预览组件（图片/PDF/代码高亮） | P2 | 2 | `web/src/features/artifact/ArtifactPreview.vue` | ✅ |
| 8 | ArtifactList.vue 制品列表组件（Chat 嵌入） | P2 | 2 | `web/src/features/artifact/ArtifactList.vue` | ✅ |
| 9 | Chat 会话面板嵌入 ArtifactList | P2 | 2 | `ChatSessionArtifactsPanel.vue` | ✅ |
| 10 | S3/COS 存储工厂 | P3 | 4 | `internal/artifact/storage_factory.go` | ✅ |
| 11 | S3/COS 生产配置 + 按租户隔离 | P3 | 4 | 配置 + Wire | 📋 后续支持 |
| 12 | Chat 消息气泡内嵌附件预览（ART-01） | P3 | 3 | `ChatMessageAttachments.vue` + `options_json.attachments` | ✅ |
| 13 | 跨会话制品列表（管理页） | P2 | 3 | `artifactfs.List` 空 session_id | ✅ |
| 14 | Prometheus 制品指标 | P2 | 3 | `internal/service/artifact.go` + `artifactfs.StorageBytes` | ✅ |
| 15 | 单文件 10 MB 上限 + 超限提示 | P2 | 3 | `biz/artifact/limits.go` + 前端 `limits.ts` | ✅ |
| 16 | Assistant 产出物写入消息气泡 | P2 | 3 | `TurnCollector` + `options_json.attachments` | ✅ |
| 17 | Team 路径附件/产出物气泡 | P2 | 3 | `runner_team_trpc.go` + `Persist.ArtifactUC` | ✅ |
| 18 | ListArtifactVersions RPC | P2 | 3 | Proto + Service + ArtifactsPage 版本选择 | ✅ |
| 19 | DeleteArtifactVersion RPC | P2 | 3 | Proto + Service + 前端 api + Store | ✅ |
| 20 | ListArtifacts 服务端检索 | P2 | 3 | `query` + `mime_type_prefix` | ✅ |
| 21 | Team 多模态附件进 LLM | P2 | 3 | `BuildUserMessageFromArtifacts` + `RunTRPCUserTurnMsg` | ✅ |
| 22 | ArtifactsPage 对话框组件拆分 | P2 | 3 | `ArtifactsUploadDialog` + `ArtifactsDetailDialog` | ✅ |
| 23 | Biz 层接口拆分 Reader/Writer/Repo | P2 | 3 | `internal/biz/artifact/` 子包 | ✅ |
| 24 | 签名密钥生产 fail-closed | P3 | 3 | `internal/artifact/sign.go` | ✅ |

---

## 6. 验收标准

- [x] Agent 运行时可通过 artifact.Service 保存/加载制品（Phase 1）
- [x] CodeExecutor 产出物自动保存为 Artifact（Phase 1）
- [x] 前端可查看/上传/删除制品（ArtifactsPage，Phase 2）
- [x] PreviewArtifact RPC 可用；前端可调用预览接口（Phase 2）
- [x] 图片/PDF/代码可在浏览器中预览（ArtifactPreview.vue 独立组件）
- [x] Chat 会话内可查看关联制品列表（ArtifactList.vue + ChatSessionArtifactsPanel）
- [x] 下载链接有时效性签名（Phase 3）
- [x] Chat 消息气泡可内嵌附件 chip 并预览（ART-01）
- [x] 管理页可跨会话浏览制品（省略 session_id）
- [x] 可查询制品版本历史（ListArtifactVersions）
- [x] 可按版本删除制品（DeleteArtifactVersion）
- [x] 列表支持检索过滤（query + mime_type_prefix）
- [x] Agent 多模态附件可传入 LLM（BuildUserMessageFromArtifacts）
- [x] TurnCollector 收集单次 turn 产出物
- [x] 签名密钥生产环境 fail-closed
- [x] S3/COS 存储工厂已实现（委托 trpc-agent-go）
- [ ] S3/COS 生产配置与按租户隔离（Phase 4，**后续支持**）

---

## 7. 依赖与风险

- Phase 2 预览组件已做 XSS 防护：图片使用 data URI，PDF 使用 iframe，文本使用 `<pre>` 转义
- Phase 2 大文件预览：后端已做 512KB 文本截断；上传/下载单文件上限 **10 MB**（base64 传输）
- Phase 3 签名密钥：生产环境 fail-closed（`ErrSignKeyMissing`），开发环境回退到不安全密钥
- Phase 4 S3/COS：存储工厂已实现（委托 trpc-agent-go 框架包），**后续支持**生产配置与多租户路径隔离（依赖 M2 EP-WS-01）
