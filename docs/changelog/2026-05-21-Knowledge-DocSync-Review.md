# Knowledge 文档同步 + Embedder 持久化 Review

**日期**：2026-05-21  
**模块**：Knowledge (37) · SystemSetting

## 摘要

- 四份 Knowledge 文档 + `frontend-pages.md` 对齐：P2 分块/解析/EmbedBatch、`system_settings.knowledge_embed_*` 持久化与配置优先级。
- Review 修复：`PersistKnowledgeEmbed` 改经 `SystemSettingRepo` + `ApplyKnowledgeEmbedPatch`（移除 service 内临时 Usecase）；持久化失败 FlowLog 警告。

## Review 结论

| 维度 | 结论 |
|------|------|
| 架构 | ✅ env → DB → runtime 分层清晰；biz 负责 patch 合并，data 负责 Ent/SQLite patch |
| 单一职责 | ✅ 修复 `PersistKnowledgeEmbed` 绕过 Usecase 临时构造 |
| 业务逻辑 | ✅ API Key 留空不覆盖；`Get` 不回显 key；Gemini/HF 配置校验在 `KnowledgeEmbedConfigured` |
| 待办 | 系统设置页 UI 未暴露 `knowledge_embed` 表单（可经 API / Knowledge 页保存） |

## 代码

| 文件 | 变更 |
|------|------|
| `internal/service/knowledge_embedder.go` | `PersistKnowledgeEmbed` 走 repo + patch |
| `internal/biz/knowledge_embed_setting.go` | 简化 baseURL patch |
| `internal/biz/knowledge_embed_setting_test.go` | patch / configured 单测 |
| `internal/service/knowledge.go` | 持久化失败 SysLog |

## 文档

- `docs/需求/37 knowledge.md`
- `docs/需求/37 knowledge.design.md`
- `docs/需求/37-knowledge-development.md`
- `docs/需求/frontend-pages.md`

## 验证

```bash
go test ./internal/biz/... ./internal/knowledge/... -count=1
go build ./...
```
