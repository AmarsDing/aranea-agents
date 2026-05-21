# Artifact 产出物 — 开发计划

> **版本**：2026-05-21 | **状态**：🟢 P1–P3 全量完成；P4 S3/COS 云存储待做
> **需求**：[27 artifact.md](./27%20artifact.md) · **设计**：[27 artifact.design.md](./27%20artifact.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-RT-08

---

## 1. 模块定位

Artifact 产出物：管理 Agent 运行时产生的文件、图片、代码等产出物，支持存储、版本管理、预览和下载。

**代码锚点**：
- `api/kratos/artifact/v1/artifact.proto` — Artifact CRUD + PreviewArtifact + SignDownloadUrl Proto 定义
- `internal/service/artifact.go` — ArtifactService（Kratos Service 层，含 PreviewArtifact + SignDownloadUrl + ServeSignedDownload）
- `internal/biz/artifact.go` — ArtifactUsecase + ArtifactRepo 接口
- `internal/data/artifactfs/repo.go` — FSArtifactRepo（本地文件系统实现）
- `internal/artifact/trpc/service.go` — ServiceAdapter（trpc artifact.Service 桥接）
- `internal/artifact/sign.go` — HMAC-SHA256 签名/验签
- `internal/skill/trpc/artifact_executor.go` — artifactSavingExecutor（CodeExecutor 产出物自动保存）
- `web/src/features/artifact/ArtifactPreview.vue` — 独立预览组件（图片/PDF/代码）
- `web/src/features/artifact/ArtifactList.vue` — 制品列表组件（含预览弹窗+签名下载）
- `web/src/features/artifact/api.ts` — 前端 API（含 previewArtifact + signDownloadUrl）
- `web/src/features/artifact/types.ts` — 前端类型定义
- `web/src/stores/artifact/index.ts` — 前端 Pinia Store
- `web/src/pages/ArtifactsPage.vue` — 管理页面（使用 ArtifactPreview 组件）
- `web/src/components/chat/ChatSessionArtifactsPanel.vue` — Chat 会话制品面板（使用 ArtifactList 组件）

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Proto 定义 | ✅ | UploadArtifact / GetArtifact / ListArtifacts / DeleteArtifact / PreviewArtifact / SignDownloadUrl |
| Service 层 | ✅ | base64 编解码 + 50MB 限制 + 参数校验 + PreviewArtifact + SignDownloadUrl + ServeSignedDownload |
| Biz 层 | ✅ | Artifact 模型 + ArtifactRepo 接口 + ArtifactUsecase + ListVersions |
| Data 层（FS） | ✅ | FSArtifactRepo + 版本管理 + JSON sidecar |
| trpc 适配器 | ✅ | ServiceAdapter 实现 trpcartifact.Service 全部 5 个方法 |
| Wire 注入 | ✅ | NewArtifactRepo → NewArtifactUsecase → NewArtifactService + provideArtifactRuntimeService |
| HTTP/gRPC 注册 | ✅ | http.go + grpc.go 已注册 + `/v1/artifacts/download` 签名下载路由 |
| 前端 API + Store | ✅ | features/artifact/api.ts（含 previewArtifact + signDownloadUrl）+ stores/artifact/index.ts |
| 测试覆盖 | ✅ | service/artifact_test.go + data/artifactfs/repo_test.go + artifact/trpc/service_test.go + artifact/sign_test.go |
| Runner 注入 ArtifactService | ✅ | `TRPCRunnerDeps.ArtifactService` + `WithArtifactService` + Wire `provideArtifactRuntimeService` |
| CodeExecutor 产出物自动保存 | ✅ | `artifactSavingExecutor` + `WrapWithArtifactSave` + `NewExecutor` 自动包装 |
| 管理页面 | ✅ | `ArtifactsPage.vue`；使用 `ArtifactPreview` 组件渲染图片/PDF/代码 |
| PreviewArtifact RPC | ✅ | Proto + Service 层 + 前端 API + ArtifactPreview.vue 组件 |
| 签名下载 URL | ✅ | `artifact/sign.go` + `SignDownloadUrl` RPC + `ServeSignedDownload` HTTP handler + 前端集成 |
| ArtifactPreview.vue 独立组件 | ✅ | 图片 `<img>` / PDF `<iframe>` / 代码 `<pre>` 渲染 + 下载按钮 |
| ArtifactList.vue 列表组件 | ✅ | MIME 图标 + 文件大小 + 版本号 + 预览弹窗 + 签名下载 |
| Chat 会话制品嵌入 | ✅ | `ChatSessionArtifactsPanel` 使用 `ArtifactList` 组件 |
| S3/COS 后端 | ❌ | 仅本地 FS |

---

## 3. 差距与优化

1. **P3**：仅本地 FS 存储，多实例部署需共享卷，无法弹性扩展。

---

## 4. 开发阶段

- **Phase 1**：~~Runner 注入 ArtifactService~~（✅）+ ~~CodeExecutor 产出物自动保存~~（✅）
- **Phase 2**：~~管理页列表/基础预览~~（✅）+ ~~PreviewArtifact RPC~~（✅）+ ~~ArtifactPreview.vue~~（✅）+ ~~ArtifactList.vue + Chat 嵌入~~（✅）
- **Phase 3**：~~签名下载 URL（HMAC-SHA256）~~（✅）
- **Phase 4**：S3/COS 云存储后端

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
| 10 | S3 存储后端 | P3 | 4 | `internal/artifact/s3/` | — |
| 11 | COS 存储后端 | P3 | 4 | `internal/artifact/cos/` | — |
| 12 | 存储后端配置选择 + 按租户隔离 | P3 | 4 | 配置 + Wire | — |

---

## 6. 验收标准

- [x] Agent 运行时可通过 artifact.Service 保存/加载制品（Phase 1）
- [x] CodeExecutor 产出物自动保存为 Artifact（Phase 1）
- [x] 前端可查看/上传/删除制品（ArtifactsPage，Phase 2）
- [x] PreviewArtifact RPC 可用；前端可调用预览接口（Phase 2）
- [x] 图片/PDF/代码可在浏览器中预览（ArtifactPreview.vue 独立组件）
- [x] Chat 会话内可查看关联制品列表（ArtifactList.vue + ChatSessionArtifactsPanel）
- [x] 下载链接有时效性签名（Phase 3）
- [ ] 制品可存储到 S3/COS，按租户隔离（Phase 4）

---

## 7. 依赖与风险

- Phase 2 预览组件已做 XSS 防护：图片使用 data URI，PDF 使用 iframe，文本使用 `<pre>` 转义
- Phase 2 大文件预览：后端已做 512KB 文本截断，图片/PDF 通过 base64 传输受 50MB 上限约束
- Phase 4 S3/COS 后端需配置凭证管理，多租户路径隔离依赖 M2（EP-WS-01）
