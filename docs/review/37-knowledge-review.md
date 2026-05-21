# 37 Knowledge / RAG Review

> **评分**：76 / 100 | **风险等级**：P1  
> **文档**：[37-knowledge-development.md](../需求/37-knowledge-development.md)  
> **代码锚点**：`internal/knowledge/` · `internal/biz/knowledge.go` · `internal/service/knowledge*.go` · `web/src/features/knowledge/`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 15 | 20 | 管理页 + Embedder UI + 摄取 WS + Rerank ✅；OCR/多租户待规划 |
| 架构一致性 | 21 | 25 | `internal/knowledge` 独立适配层 ✅；Knowledge search tool 经 `internal/tools/knowledge/tool.go` 挂载 ✅ |
| 后端实现质量 | 17 | 20 | 分块/解析/EmbedBatch ✅；OCR（`internal/knowledge/ocr.go`）✅ |
| OCR pipeline | ✅ |
| 前端实现质量 | 12 | 15 | 集合列表 + 文档面板 + 语义检索测试 ✅；Embedder 配置面板 ✅；Rerank 选项 ✅ |
| 测试与验证 | 5 | 10 | 基础功能有测试；分块/嵌入路径无专项测试 |
| 文档一致性 | 6 | 10 | `37-knowledge-development.md` 管理页 G0 对齐良好 |

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| 知识库集合 CRUD | ✅ |
| 文档上传与分块（Phase 2） | ✅ |
| Embedder（`KRATOS_KNOWLEDGE_EMBED_*` + 系统设置）| ✅ |
| EmbedBatch | ✅ Phase 2 |
| WS 摄取进度（`useKnowledgeIngestWs`）| ✅ |
| Rerank（trpc reranker，KN-01）| ✅ |
| Knowledge search tool（Agent 工具链）| ✅ |
| 语义检索测试面板 | ✅ |
| OCR pipeline | ✅ `internal/knowledge/ocr.go` |
| 多租户 pgvector | 🟡 |
| AgenticFilter | ❌ |
| 跨集合检索 | ❌ |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| KN-P1-01 | OCR pipeline | ✅ 已实现基础 OCR 适配 |
| KN-P1-02 | 分块/嵌入路径（chunker → embedder → pgvector）无专项测试 | 补嵌入路径集成测试 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| KN-P2-01 | pgvector 多租户隔离（多 Agent/多 Knowledge 集合）稳定性待验证 | 补多租户场景测试 |
| KN-P2-02 | 嵌入维度配置（`dim`）出错时错误信息不清晰 | 摄取时校验 dim 与模型维度一致性，返回清晰错误 |
| KN-P2-03 | `AgenticFilter`（Agent 决定是否检索相关文档）未实现 | 规划 AgenticFilter 接入 |

---

## 系统设置集成

Knowledge Embedder 默认值通过系统设置管理：
```
system_settings.knowledge_embed_* ← SystemSettingsPage 保存
env KRATOS_KNOWLEDGE_EMBED_* 优先（高于系统设置）
```

**注意**：API Key 仅存库不回显（正确实现）。

---

## 建议优化路径

1. 规划并实现 OCR pipeline（P2）。
2. 补分块/嵌入路径集成测试。
3. 多租户 pgvector 稳定性测试。
4. 规划 AgenticFilter（P3）。
