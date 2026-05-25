# 27 Artifact Review

> **评分**：**89 / 100** | **风险等级**：**P2（低）**  
> **文档**：[27-artifact-development.md](../需求/27-artifact-development.md)  
> **代码锚点**：`internal/biz/artifact/` · `internal/agent/attachments.go` · `internal/service/artifact.go` · `web/src/features/artifact/`  
> **审查时间**：2026-05-25（ART-01 多轮优化 + review 跟进修复后）

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 19 | 20 | ART-01 / Team 多模态 / 10 MB / 跨会话 / 版本 / 指标均已落地 |
| 架构一致性 | 22 | 25 | `ResolveAttachmentRefs` 统一校验；`TurnCollector` 模式清晰；List 过滤下沉 Biz |
| 后端实现质量 | 18 | 20 | 持久化与 LLM 路径校验已对齐；跨 session 全量扫描仍为 MVP |
| 前端实现质量 | 14 | 15 | 对话框拆分 + 服务端分页 + 共享 `readFileAsBase64` |
| 测试与验证 | 8 | 10 | Biz/Service/Agent 单测；Chat/Team Turn 仍缺 E2E |
| 文档一致性 | 8 | 10 | development.md 已同步；本 review 已更新 |

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| Artifact CRUD + 10 MB 上限 | ✅ |
| Chat/Team 消息气泡 attachments | ✅ |
| Team Vision 多模态 | ✅ |
| Turn 产出物 TurnCollector | ✅ |
| 跨会话列表 + 服务端检索 | ✅ |
| ListArtifactVersions | ✅ |
| Prometheus 指标 | ✅ |
| 附件 ID 统一校验（ResolveAttachmentRefs） | ✅ |
| S3/COS | 📋 后续支持 |

---

## 已修复（2026-05-25 review 跟进）

| ID | 问题 | 修复 |
|----|------|------|
| ART-P2-01 | 持久化 vs LLM 附件校验双标准 | `ResolveAttachmentRefs` 统一入口 |
| ART-P3-01 | `options_attachments.go` 重复死代码 | 已删除，合并逻辑仅保留 biz |
| ART-P3-02 | List 过滤在 Service 全量拉取 | 过滤逻辑下沉 `biz/artifact` |
| ART-P3-03 | `AttachmentsCount` 不准确 | 使用 resolved refs 长度 |
| ART-P3-04 | 管理页 limit 200 硬编码 | 服务端分页 15/30/50 |
| ART-P4-01 | base64 读取重复 | `fileBase64.ts` 共享 |
| ART-P4-02 | `filteredRows` 误导命名 | 已移除 |

---

## 剩余风险（P3/P4）

| ID | 问题 | 建议 |
|----|------|------|
| ART-P3-05 | 跨 session + filter 仍扫描全 FS | Phase 4 或索引化 |
| ART-P3-06 | 多附件 multimodal 全量 Load 内存 | 流式 / 上限 N 个附件 |
| ART-P2-04 | 缺 Chat/Team 附件 E2E | 补 1 条集成测或 Playwright |

---

## 建议验证

```powershell
go test ./internal/biz/artifact/... ./internal/agent/... ./internal/service/... ./internal/team/... -count=1
go build ./cmd/admin
cd web; pnpm exec vitest run src/features/artifact/__tests__/
```

手动：Chat/Team 带无效 attachment ID（应 Turn 失败且不落库）；管理页翻页 + MIME 筛选。
