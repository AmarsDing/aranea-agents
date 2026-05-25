# M12: Artifact 制品 — 详细需求

> 对标 `pkg/trpc-agent-go/artifact` 包，实现制品存储和版本管理。
>
> **2026-05-21 现状对齐**：
> - ✅ Proto / Service / Biz / FS Repo / Wire / HTTP+gRPC 已注册。
> - ✅ trpc ServiceAdapter 已实现（`internal/artifact/trpc/service.go`），桥接 biz.ArtifactUsecase 到 trpc artifact.Service 接口。
> - ✅ 版本管理：FS Repo 支持同 session+name 多版本存储与按版本加载。
> - ✅ 前端 API 层 + Store 层已实现（`features/artifact/` + `stores/artifact/`）。
> - ✅ Runner 侧制品闭环：`TRPCRunnerDeps` 已注入 `ArtifactService`，Agent 运行时可通过 `artifact.ServiceKey` 访问制品。
> - ✅ CodeExecutor 产出物自动保存：`artifactSavingExecutor` + `WrapWithArtifactSave`。
> - ✅ PreviewArtifact RPC 已实现；前端 `ArtifactPreview.vue` 独立组件支持图片/PDF/代码预览。
> - ✅ 签名下载 URL 已实现（`artifact/sign.go` HMAC-SHA256 + `ServeSignedDownload` HTTP handler）。
> - ✅ 前端制品列表/预览组件已实现（`ArtifactList.vue` + `ArtifactPreview.vue` + `ChatSessionArtifactsPanel`）。
> - ❌ S3 / COS 后端**后续支持**（Phase 4），当前仅本地 FS；多租户路径隔离待 M2（EP-WS-01）。
>
> 进度以 `guides/execution-plan.md` 附录 A 为准。运维要点见设计文档 §6。

---

## 1. 用户故事

| 角色 | 故事 | 优先级 |
|------|------|--------|
| 用户 | 我希望查看 Agent 会话中产生的所有制品（文件/图片/代码），以便回顾运行结果 | P1 |
| 用户 | 我希望上传文件作为制品关联到会话，以便 Agent 后续使用 | P1 |
| 用户 | 我希望下载制品到本地，以便离线查看或二次加工 | P1 |
| 用户 | 我希望同一文件保留多版本历史，以便追踪变更和回滚 | P2 |
| 用户 | 我希望在浏览器中预览图片/PDF/代码，无需下载即可查看 | P2 |
| 用户 | 我希望下载链接有时效性签名，以保障制品访问安全 | P3 |
| 运维 | 我希望制品存储到 S3/COS 云存储，以支持多实例部署和弹性扩展 | P3 |
| Agent | 我希望在运行时通过 artifact.Service 保存/加载制品，以实现代码执行产出物自动收集 | P1 |

---

## 2. 功能规格

### 2.1 制品 CRUD

- 制品与会话（Session）绑定，每个制品属于一个会话。
- 支持上传（base64 编码）、下载（base64 编码）、列出元数据、删除。
- 单个制品大小上限 50 MB。
- 上传时自动计算 SHA-256 校验和。
- MIME 类型缺省为 `application/octet-stream`。

### 2.2 版本管理

- 同一会话下相同文件名的制品自动创建新版本，版本号递增。
- 加载制品时可指定版本号；未指定时返回最新版本。
- 列出制品元数据时默认返回每个文件名的最新版本。
- 支持查询某文件名的全部版本历史。
- 删除制品时删除该 ID 的所有版本。

### 2.3 Agent 运行时集成

- Agent 运行时可通过 trpc-agent-go 的 `artifact.Service` 接口保存/加载制品。
- 代码执行（CodeExecutor）产出物自动保存为制品。
- Runner 创建时注入 ArtifactService，使 Agent 工具链可访问制品。

### 2.4 在线预览

- 支持图片（JPEG/PNG/GIF/WebP）、PDF、代码文件（语法高亮）的浏览器内预览。
- 预览需做 XSS 防护（沙箱 iframe 或 Content-Disposition）。
- 大文件预览需流式处理，避免内存溢出。

### 2.5 签名下载 URL

- 下载链接包含 HMAC-SHA256 签名和过期时间戳。
- 签名密钥由服务端配置，不暴露给前端。
- 过期后返回 403。

### 2.6 云存储后端

- 支持 S3 和 COS 作为存储后端，替代本地文件系统。
- 可按配置选择存储后端。
- 多租户路径隔离。

---

## 3. trpc 框架参照

trpc-agent-go `artifact` 包定义了 Agent 运行时制品的核心抽象：

- **Artifact**：MIME 类型 + 二进制数据 + 可选 URL + 可选名称。
- **SessionInfo**：AppName + UserID + SessionID，标识制品归属。
- **Service 接口**：SaveArtifact / LoadArtifact / ListArtifactKeys / DeleteArtifact / ListVersions。

项目需桥接该接口到自身存储层，使 Agent 运行时可透明使用制品能力。

---

## 4. 验收标准

| # | 验收标准 | 优先级 | 状态 |
|---|----------|--------|------|
| 1 | 制品可上传/下载/列出/删除 | P1 | ✅ |
| 2 | 同一文件名可保存多个版本，可按版本加载 | P2 | ✅ |
| 3 | Agent 运行时可通过 artifact.Service 保存/加载制品 | P1 | ✅ |
| 4 | 代码执行产出物自动保存为制品 | P1 | ✅ |
| 5 | 通过 REST/gRPC API 可管理制品完整生命周期 | P1 | ✅ |
| 6 | 图片/PDF/代码可在浏览器中预览 | P2 | ✅ |
| 7 | 下载链接有时效性签名 | P3 | ✅ |
| 8 | 制品可存储到 S3/COS，按租户隔离 | P3 | 📋 后续支持 |

---

## 5. 已知限制

- 二进制存储仅使用本地文件系统。**S3/COS 后端为 Phase 4 后续支持**（可复用 `pkg/trpc-agent-go/artifact/s3|cos`）。
- 制品不在节点间复制。多实例部署需使用共享卷。
- `data_base64` 编码增加约 33% 开销；大文件（> 10 MB）应优先使用分块流式传输（计划中）。
