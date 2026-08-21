# 域 C 知识库 RAG 评测结果（2026-08-18）

Collection：eval-ops-kb（team backend，embedding_model=bge-m3，dim=1024）
埋入文档：5 篇 sample-doc-*.md → 29 chunks，索引全部 indexed
执行脚本：[run.ps1](run.ps1)；明细：[evidence/results.md](evidence/results.md)；每条检索证据：evidence/kb-*-search.json

## 检索准确率（top-5 命中出处文档）

| 文档组 | 命中 | 准确率 |
|------|------|------|
| ins（巡检） | 6/6 | 100% |
| chg（变更） | 6/6 | 100% |
| emg（应急） | 6/6 | 100% |
| dty（值班） | 6/6 | 100% |
| sec（安全） | 6/6 | 100% |
| **合计（hybrid=auto）** | **30/30** | **100%** |

## C-12 混合检索对比（30 条 x 3 模式）

| 模式 | top-5 命中 | 准确率 | 平均延迟 |
|------|------|------|------|
| dense（纯向量） | 30/30 | 100% | 1468ms |
| rrf（混合） | 30/30 | 100% | 1444ms |
| sparse（纯词法 tsvector/trigram） | **12/30** | **40%** | 643ms |

**结论**：语义层（bge-m3）是中文 RAG 召回的绝对主力；纯词法对中文查询命中率仅 40%（tsvector 'simple' 无中文分词，trigram 部分补偿）。与域 B 记忆检索的中文分词短板互为印证——**词法路径的中文能力是系统级短板**。

## 检索延迟（hybrid=auto，n=30）

- min=1173ms max=2492ms P95=2222ms
- 对比：sparse 643ms（无 embedding 调用）；dense/rrf 含 ollama bge-m3 网络往返（容器→宿主机 11434），占 ~60-70% 延迟
- 优化方向：embedder 容器化同网络部署 / query embedding 缓存 / rrf 与 dense 延迟接近可默认 rrf

## 生命周期验证

| 用例 | 结果 | 说明 |
|------|------|------|
| C-00 建库 | PASS | 32ms |
| C-02 上传 x5 | PASS | 25~29ms/篇 |
| C-03 索引 | PASS | 5/5 indexed，29 chunks，秒级完成 |
| C-15 越界隔离 | PASS | 不存在 collection 返回 404，无数据泄露 |
| C-17 级联删除 | PASS | 删除后文档列表 404，docsLeft=0 |

## 发现的缺陷

### BUG-C-01：CreateCollectionRequest 无 dim 参数，与运行时 embedder 脱节（P1）
- **现象**：API 创建 collection 恒走默认 dim=1536（[knowledge.go L347-349](file:///f:/myproject/aranea-agents/internal/biz/knowledge/knowledge.go#L347-L349)），运行时 embedder 配置为 bge-m3（dim=1024）→ ingest 时 `InsertChunks` 报 `embedding dimension mismatch`，文档永久 error
- **影响**：任何通过 API 创建的 semantic collection 在非 1536 维 embedder 下必然索引失败；仅 UI/DB 直改路径（如存量"UX验证库" dim=1024）可用
- **workaround**：本评测 DB 直改 `UPDATE knowledge_collections SET dim=1024`（脚本 C-00b 步骤）
- **修复建议**：CreateCollection 时取 embedder 配置的 dim 作为默认，或 API 增加 dim 字段

### 词法检索中文短板（已知，归入 C 阶段统一处理）
- sparse 40% vs dense 100%；与域 B `tokenizeQuery` 中文分词问题同源（tsvector 'simple' + trigram 对 CJK 无词边界）

## 未执行项

- **C-08 chat 内知识工具**（agent 挂 knowledge_search 走 chat 链路）：依赖 C 阶段 R1/R2 修复后的镜像重建，合并到修复后回归一起验证

## 结论

域 C 语义检索链路（ingest → chunking → embedding → dense/rrf 检索）**功能完好，准确率 100%**；主要风险在 (a) BUG-C-01 集成缺陷阻断 API 建库路径，(b) 纯词法模式中文不可用，(c) embedding 网络往返推高延迟至秒级。
