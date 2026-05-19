# Artifact 产出物 — 开发计划

> **版本**：2026-05-19 | **状态**：🟡 存储+Runner 注入+管理页可用；预览 RPC/Chat 附件/签名下载待做
> **需求**：[27 artifact.md](./27%20artifact.md) · **设计**：[27 artifact.design.md](./27%20artifact.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-RT-08

---

## 1. 模块定位

Artifact 产出物：管理 Agent 运行时产生的文件、图片、代码等产出物，支持存储、版本管理、预览和下载。

**代码锚点**：
- `api/kratos/artifact/v1/artifact.proto` — Artifact CRUD Proto 定义
- `internal/service/artifact.go` — ArtifactService（Kratos Service 层）
- `internal/biz/artifact.go` — ArtifactUsecase + ArtifactRepo 接口
- `internal/data/artifactfs/repo.go` — FSArtifactRepo（本地文件系统实现）
- `internal/artifact/trpc/service.go` — ServiceAdapter（trpc artifact.Service 桥接）
- `web/src/features/artifact/` — 前端 API + 类型定义
- `web/src/stores/artifact/` — 前端 Pinia Store

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Proto 定义 | ✅ | UploadArtifact / GetArtifact / ListArtifacts / DeleteArtifact |
| Service 层 | ✅ | base64 编解码 + 50MB 限制 + 参数校验 |
| Biz 层 | ✅ | Artifact 模型 + ArtifactRepo 接口 + ArtifactUsecase + ListVersions |
| Data 层（FS） | ✅ | FSArtifactRepo + 版本管理 + JSON sidecar |
| trpc 适配器 | ✅ | ServiceAdapter 实现 trpcartifact.Service 全部 5 个方法 |
| Wire 注入 | ✅ | NewArtifactRepo → NewArtifactUsecase → NewArtifactService |
| HTTP/gRPC 注册 | ✅ | http.go + grpc.go 已注册 |
| 前端 API + Store | ✅ | features/artifact/api.ts + stores/artifact/index.ts |
| 测试覆盖 | ✅ | service/artifact_test.go + data/artifactfs/repo_test.go + artifact/trpc/service_test.go |
| Runner 注入 ArtifactService | ✅ | `TRPCRunnerDeps.ArtifactService` + `WithArtifactService`（M1/EP-RT-08） |
| CodeExecutor 产出物自动保存 | ❌ | 仅有 ArtifactDir 字段，无自动持久化 |
| 管理页面 | ✅ | `ArtifactsPage.vue`；路由 `/artifacts`；列表/上传/文本预览 |
| 前端制品列表组件 | 🟡 | 合并在 `ArtifactsPage`，无独立 `ArtifactList.vue` |
| 在线预览（图片/PDF/代码） | 🟡 | 管理页内文本预览；无 PreviewArtifact RPC / PDF 渲染 |
| 签名下载 URL | ❌ | 无签名机制 |
| S3/COS 后端 | ❌ | 仅本地 FS |

---

## 3. 差距与优化

1. **P1**：CodeExecutor 产出物未自动保存为 Artifact。
2. **P2**：Chat 会话未关联制品列表/附件入口。
3. **P2**：无 PreviewArtifact RPC；图片/PDF 浏览器内预览未实现。
4. **P3**：无签名下载 URL，安全性不足。
5. **P3**：仅本地 FS 存储，多实例部署需共享卷，无法弹性扩展。

---

## 4. 开发阶段

- **Phase 1**：~~Runner 注入 ArtifactService~~（✅）+ CodeExecutor 产出物自动保存（⏳）
- **Phase 2**：~~管理页列表/基础预览~~（✅ `ArtifactsPage`）+ PreviewArtifact RPC + 图片/PDF/代码高亮（⏳）
- **Phase 3**：签名下载 URL（HMAC-SHA256）
- **Phase 4**：S3/COS 云存储后端

---

## 5. 任务清单

| # | 任务 | 优先级 | Phase | 涉及文件 | EP |
|---|------|--------|-------|----------|-----|
| 1 | TRPCRunnerDeps 增加 ArtifactService 字段 + WithArtifactService 注入 | P1 | 1 | `internal/agent/trpc_runtime.go` | EP-RT-08 ✅ |
| 2 | Wire 提供 ServiceAdapter 实例 | P1 | 1 | `internal/artifact/trpc/service.go`, Wire 集成 | EP-RT-08 ✅ |
| 3 | CodeExecutor 产出物自动保存为 Artifact | P1 | 1 | `internal/agent/codeexecutor/executor.go` | EP-RT-08 |
| 4 | ArtifactsPage 管理界面 | P2 | 2 | `web/src/pages/ArtifactsPage.vue` | — ✅ |
| 5 | PreviewArtifact RPC + 在线预览（图片/PDF/代码） | P2 | 2 | Proto + Service + Biz + 前端 | — |
| 6 | ArtifactPreview.vue 制品预览组件 | P2 | 2 | `web/src/features/artifact/` | — |
| 7 | 签名下载 URL（HMAC-SHA256） | P3 | 3 | Proto + Service + Biz | — |
| 8 | S3 存储后端 | P3 | 4 | `internal/artifact/s3/` | — |
| 9 | COS 存储后端 | P3 | 4 | `internal/artifact/cos/` | — |
| 10 | 存储后端配置选择 + 按租户隔离 | P3 | 4 | 配置 + Wire | — |

---

## 6. 验收标准

- [x] Agent 运行时可通过 artifact.Service 保存/加载制品（Phase 1）
- [ ] CodeExecutor 产出物自动保存为 Artifact（Phase 1）
- [x] 前端可查看/上传/删除制品（ArtifactsPage，Phase 2）
- [ ] 图片/PDF/代码可在浏览器中预览（Phase 2，需 Preview RPC）
- [ ] 下载链接有时效性签名（Phase 3）
- [ ] 制品可存储到 S3/COS，按租户隔离（Phase 4）

---

## 7. 依赖与风险

- Phase 1 依赖 trpcrunner.WithArtifactService API 可用性
- Phase 2 预览功能需考虑 XSS 防护（沙箱 iframe 或 Content-Disposition）
- Phase 2 大文件预览需流式处理，避免内存溢出
- Phase 4 S3/COS 后端需配置凭证管理，多租户路径隔离依赖 M2（EP-WS-01）
