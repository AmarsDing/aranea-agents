# 域 C：知识库 RAG 评测（agent-eval-20260818）

> 对应方案 [00-master-plan.md](../00-master-plan.md) §4 域 C。数据源：[sample-knowledge-qa.json](./sample-knowledge-qa.json)（30 条，5 篇文档 x 6）。

## 评测流程

1. **C-00 创建 collection**：`POST /v1/knowledge/collections` 创建 `eval-ops-kb`，记录创建延迟。
2. **C-02/03 上传并索引**：5 篇 sample-doc-*.md 上传，等待状态 pending→indexed，记录吞吐。每篇上传前 base64 编码。
3. **C05/C06/C07 检索验证**：30 条 case 逐条 `POST /v1/knowledge/search`，判定 top-5 是否命中 source_doc 对应的 document ID。
   - 命中判定：top-5 chunks 中任一 chunk 的 document_id 与 source_doc 映射匹配 → PASS
   - 判定逻辑：先查 documents 列表拿到 filename→id 映射，再逐条检索
4. **C08 chat 内知识工具**：agent 开启 knowledge_search 工具后发库内问题，记录端到端时长、是否调工具。
5. **C12 混合检索对比**：同一 query 分别用 dense、sparse、rrf 模式，对比召回率。
6. **C15 多租户隔离**：用另一 collection 或变更 workspace 尝试越界检索，验证返回空。
7. **C17 级联删除**：删除 collection，验证 documents/chunks 级联清除。
8. **汇总**：top-5 命中率、检索延迟分布、chat 内工具调用率、混合检索增益。

## 执行

```powershell
# 试跑（上传前 2 篇 + 前 6 条问答）
powershell -ExecutionPolicy Bypass -File run.ps1 -Pilot
# 全量
powershell -ExecutionPolicy Bypass -File run.ps1
```

## 清理

`DELETE /v1/knowledge/collections/{eval-ops-kb-id}`（级联删除）。
