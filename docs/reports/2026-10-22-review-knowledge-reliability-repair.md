# 知识库可靠性修复复核

日期：2026-10-22（Asia/Shanghai）。对象：`F:\myproject\aranea-agents` 当前共享工作树。

本轮按 [2026-10-21 业务与用户体验评审](./2026-10-21-review-knowledge-business-and-user-experience.md) 修复 KB-01～KB-18 的已确认代码问题，并补齐图谱引用证据。当前交付是可审查的源码、契约、回归与模块文档；没有发布、提交 Git 或宣称已完成用户现场验收。工作树还包含其他任务的改动，本报告只说明本轮知识库范围。

## 1. 修复结果

| 编号 | 当前行为 | 主要代码锚点 |
|---|---|---|
| KB-01 | Retriever/Hybrid 所有分支先验证真实库及工作区；库查询失败时拒绝继续检索，Agent 指定 ID 不能增加权限 | `internal/knowledge/search_helpers.go`、`retriever.go`、`hybrid_retriever.go` |
| KB-02 | 治理查询限定工作区；人工处置要求管理员/系统身份与目标库写权；提案锁、正文处置、版本与终态同事务 | `internal/service/knowledge_governance_access.go`、`internal/data/knowledge_governance_tx.go` |
| KB-03 | 抽取保留原始 MIME；服务端和前端同时按源格式阻止历史错标二进制文件写回；保留提取内容预览 | `internal/knowledge/body_compiler.go`、`internal/biz/knowledge/vault_write.go`、`internal/service/knowledge.go` |
| KB-04 | 本地覆盖在数据库事务中删除目标身份并更新源路径、计数；数据库失败时恢复文件；缺少原子仓储能力时提前拒绝 | `internal/biz/knowledge/vault_write.go`、`internal/data/knowledge_move.go` |
| KB-05 | 用户删除本地文档先将原件移入回收站，再删数据库；失败恢复原件；外部扫描删除只删除镜像，避免无 filer 时无法同步 | `internal/biz/knowledge/vault_write.go`、`internal/knowledge/vault_sync.go` |
| KB-06 | 任一本地库参与的跨库移动均明确拒绝，前端一致提示；保留 team→team 的受支持路径及已有晋升能力 | `internal/biz/knowledge/knowledge_part2.go`、`web/src/features/knowledge/useBulkDocActions.ts` |
| KB-07 | 语义服务失败时允许无向量 chunks 进入词法降级；非空错误维度向量仍拒绝 | `internal/data/knowledge_chunks.go`、`internal/biz/knowledge/knowledge_part2.go` |
| KB-08 | 团队正文 CAS 同事务清除旧 chunks；治理后旧事实不继续作为普通检索证据；旧正文任务不能覆盖新稿索引或错误状态 | `internal/data/knowledge_team_note.go`、`internal/biz/knowledge/content_revision.go`、`internal/service/knowledge_promote.go`、`knowledge_reembed.go` |
| KB-09 | 建目录、建笔记、移动目标及治理等写入口使用 mutate 权限；共享系统库保持只读 | `internal/service/knowledge_explorer.go`、`knowledge.go`、`knowledge_governance.go` |
| KB-10 | 保存绑定请求快照，保存中继续输入仍为 dirty；草稿按用户/工作区/文档隔离；切页保护包含编辑器待刷输入；冲突保留草稿并提供确认后重载 | `web/src/features/knowledge/useKnowledgeWorkbench.ts`、`useKnowledgeDraftGuard.ts`、`web/src/components/knowledge/workbench/KnowledgeWorkbench.vue` |
| KB-11 | 全库命中按 ID 补取授权元数据，定位真实库并打开，不依赖当前分页列表 | `api/kratos/knowledge/v1/knowledge.proto`、`web/src/features/knowledge/useWorkbenchCommands.ts` |
| KB-12 | 标签持有所属库/路径；链接、侧栏和悬空笔记创建使用活动笔记上下文 | `web/src/features/knowledge/useKnowledgeWorkbench.ts`、`web/src/components/knowledge/workbench/KnowledgeWorkbench.vue` |
| KB-13 | 从可信创建身份记录 owner，数据库持久化；API 返回 ACL，前端展示可见性、权限和专项 Agent 共享名单 | `internal/biz/knowledge/knowledge_part2.go`、`internal/data/knowledge_documents.go`、`web/src/features/knowledge/useKnowledgeSharing.ts` |
| KB-14 | 普通建库默认使用托管 team 后端，服务器目录挂载置于高级入口；上传为明确主操作；团队 Markdown 支持新建和 CAS 保存 | `web/src/components/knowledge/KnowledgeCreateDialog.vue`、`web/src/features/knowledge/useKnowledgeVaultOps.ts`、`internal/biz/knowledge/team_note.go` |
| KB-15 | 时态查询停用没有历史版本契约的当前关系卡召回；保留既有“边时态 × 当前节点 × 当前权限”语义 | `internal/knowledge/graph_card_searcher.go` |
| KB-16 | 图邻居片段与卡片宿主兑现路径/排除范围；元数据筛选无法证明卡片原块时停用卡片腿，不扩大到整篇文档 | `internal/knowledge/search_scope.go`、`graph_expander.go`、`internal/data/knowledge_search.go` |
| KB-17 | reflect 增加 evaluated；未评估不再伪造充分性和满分置信；零结果固定不足；评估响应校验必需字段和区间 | `internal/tools/knowledge/tool.go`、`internal/knowledge/retrieval_evaluator.go` |
| KB-18 | 恢复持久化 embedding 维度；优先级为显式环境变量、DB、配置文件、默认值 | `internal/service/knowledge_embedder.go` |

图谱引用的补充修复：通知记录证据种类、hash、当次内容快照、库与原始 viewer；回采用当前 ACL 复核而非继承后台系统身份。引用使用当次快照，旧式缺版本通知关闭回采。合成卡片 ID 不写真实 chunk 计数；Agent 提示和聊天引用标签明确显示“派生证据”，跳转宿主文档不冒充原文行号。

主要锚点：`internal/data/knowledge_citation.go`、`internal/tools/knowledge/tool.go`、`internal/agent/knowledge_inject.go`、`web/src/features/chat/knowledgeRecall.ts`、`web/src/components/chat/KnowledgeRecallChips.vue`。

## 2. 一致性与契约

- 新增 `GetDocument`：`GET /v1/knowledge/documents/{id}`，与正文读取共用库授权和文档可见性。
- `UpdateDocumentContentResponse.base_hash` 表示实际提交版本，避免写后无条件读取最新版本而掩盖他人修改。
- `KnowledgeDocument` 增加 `visibility`、`owner_user_id`、`shared_with`。Go protobuf/HTTP/gRPC 与 TS 客户端通过 protoc 生成；聚合 OpenAPI 也已重新生成。
- 治理内的派生索引工作延后到提交后；MOC 重放在领域侧统一排队并保留创建标记，避免测试/其他适配器绕过该边界。外部重放失败不会撤销已经持久化的正文，状态保留可修复资格。
- 治理处置请求的 JSON 严格解析，拼错字段、未知字段、尾随内容不再退化成“应用全部”。文档可见性读取同样验证当前文档 ACL。
- [ADR](./2026-10-21-review-adr-knowledge-reliability.md) 与 [需求](../development/37-knowledge.md)、[设计](../development/37-knowledge.design.md)、[开发计划](../development/37-knowledge.development.md) 已同步。

## 3. 验证结论

已通过知识领域/检索/工具完整 `-race` 回归 1,208 个测试条目（含子测试），真实隔离 PostgreSQL 回归 87 个条目，另补验图谱引用和 Agent 派生证据。前端最终快照为 66 个测试文件、578 个测试通过。Go 全量编译和前端生产构建通过。

本轮扩大测试发现并修正了联邦搜索测试记录的并发竞争，以及治理延后重放与 MOC 测试适配器的衔接。没有跳过失败用例，也没有用只编译替代这组完整核心回归。

全量前端类型检查仍失败，当前日志有 119 条错误；知识库生产文件为 0 条，知识库内仍有既有图谱测试的类型断言和库夹具字段缺失问题。全局分层检查和 i18n 检查也有范围外错误；v2 契约检查通过。没有修改基线掩盖这些结果。

完整命令、原始日志与证据分层见 [验证记录](../testing/reports/report-20261022-0045.md)。早一轮前端 583 的数量来自当时工作树，本报告使用最终 578 的实际快照，不将不同快照合并计数。

## 4. 交付边界与后续验收

以下边界直接影响用户预期，当前保持明确限制：

1. 本地跨库迁移仍不提供；人工治理处置限托管库。托管库目录由笔记路径生成，暂不提供空目录。
2. 本地文件与 PostgreSQL 没有共同物理事务；本轮提供常规失败补偿和回收站。尚未证明断电/强杀发生在两个提交之间时能自动恢复，没有新增持久文件操作日志。
3. 二进制原件只读与提取内容预览已修复；独立派生稿编辑、原件/版本对照查看器仍属于产品增强。
4. 新建 owner 已记录；历史无 owner 文档保留管理员处置，不按推测补历史所有人。上传前隐私选择尚未扩展入库契约，当前可在入库后由有权用户调整。
5. 专项 Agent 共享名单不等于向该部门所有人类成员授权。新对话框使用现有组织节点语义。
6. as-of 不使用当前关系卡，元数据筛选停用缺原块锚点的卡片腿；这是保守范围限制，不代表已完成历史卡片版本化。
7. 全库联邦搜索仍受现有路由与扇出预算限制。已知的部分覆盖披露、真实中文问题质量集与模型端到端对比仍需后续验证。
8. 当前工作台已修正入口、保存、上下文及权限反馈，没有拆成四个独立产品页面；窄屏抽屉、真实用户可用性与登录后浏览器流程尚未验收。

发布前需在可运行环境执行：两个工作区与普通/管理员/专项 Agent 权限矩阵；真实 PDF/Office/图片导入和原件 hash 核对；保存冲突、切页/刷新草稿恢复；A/B 库同名和跨库命中；治理失败重试；embedding 断网恢复；真实模型引用核对及窄屏操作。现有单元、真实 SQL 与构建结果分别提供证据，不能替代这些运行验收。

仓库现有规则忽略 `docs/`、`test/` 与聚合 `openapi.yaml`；文档、验证脚本和日志已保存在工作区，尚未纳入 Git 跟踪，也未修改忽略规则。代码改动未提交。
