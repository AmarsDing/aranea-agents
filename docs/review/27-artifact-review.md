# 27 Artifact Review

> **评分**：72 / 100 | **风险等级**：P2  
> **文档**：[27-artifact-development.md](../需求/27-artifact-development.md)  
> **代码锚点**：`internal/artifact/trpc/` · `internal/data/artifactfs/` · `internal/service/artifact.go` · `web/src/pages/ArtifactsPage.vue`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 15 | 20 | `ArtifactsPage` + Runner 注入 ✅；Chat 附件引用、签名下载、跨会话检索待补 |
| 架构一致性 | 20 | 25 | `internal/artifact/trpc` 适配层 ✅；`artifactfs` 文件存储独立 ✅；Runner 注入（`WithArtifactService`）✅ |
| 后端实现质量 | 16 | 20 | 基础 CRUD + 上传 + 按 Session 筛选 ✅；签名 URL 生成待补 |
| 前端实现质量 | 11 | 15 | `ArtifactsPage.vue` ✅；Chat 侧 `ChatSessionArtifactsPanel` 联动 ✅；预览局限于文本 |
| 测试与验证 | 5 | 10 | 基础存储测试；Runner 注入路径无测试 |
| 文档一致性 | 5 | 10 | `27-artifact-development.md` Runner 注入、ArtifactsPage 已对齐 |

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| Artifact CRUD | ✅ |
| 文件上传 | ✅ |
| 按 Session 筛选 | ✅ |
| Runner 注入（`WithArtifactService`）| ✅ |
| `ArtifactsPage.vue` | ✅ |
| Chat 侧制品面板 | ✅ |
| 文本/代码预览 | ✅ |
| 签名下载 URL | ❌ |
| Chat 内附件引用（消息内嵌）| ❌ |
| 跨会话制品检索 | ❌ |
| CodeExecutor → Artifact 管道 | ❌ |
| 图片/富媒体预览 | 🟡 |

---

## 主要风险

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| ART-P2-01 | 签名下载 URL 未实现；当前只能通过管理页下载，无法从 Chat 内分享 | 实现 `GeneratePresignedDownloadURL` RPC |
| ART-P2-02 | Chat 内附件引用（在消息气泡内内嵌制品链接/预览）未实现 | 规划 artifact 消息 part 类型 |
| ART-P2-03 | CodeExecutor 产出物写入 Artifact 的管道未实现 | 规划 CodeExecutor output → Artifact 写入 |
| ART-P2-04 | 跨会话制品检索（按内容/类型/Agent 查询）未规划 | 规划 Artifact 搜索 API |

---

## 建议优化路径

1. 实现签名下载 URL（P2）。
2. 规划 Chat 内附件引用（P2）。
3. 实现 CodeExecutor → Artifact 管道（P2）。
