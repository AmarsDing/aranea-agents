# Knowledge 知识库模块优化

> 日期：2026-05-29 | 模块：`internal/knowledge` + `internal/biz/knowledge` + `internal/data/knowledge` + `internal/service/knowledge`

---

## 第一轮：安全 + 可靠性修复（已完成）

### 问题发现

对知识库模块（Phase 5: Advanced RAG）进行全面审查，发现 27 个问题，其中高危 6 个、中危 14 个、低危 7 个。

### 高危问题

| ID | 文件 | 问题 |
|----|------|------|
| KB-01 | `internal/data/knowledge.go:300` | SQL 注入风险：MinScore 通过 fmt.Sprintf 拼接进 SQL |
| KB-02 | `internal/service/knowledge.go:149-173` | 5 处 `_ =` 吞掉异步 ingest 状态更新错误 |
| KB-03 | `internal/knowledge/retrieval_evaluator.go:70-73` | LLM 评估失败返回 Sufficient:true 假阳性 |
| KB-04 | `internal/knowledge/embedder.go:175,292,325` | 使用 http.DefaultClient 无超时，可致 goroutine 阻塞 |
| KB-05 | `internal/service/knowledge.go:246-257` | Service 层包含评估+补充检索业务逻辑（红线 #9） |
| KB-RI | `internal/biz/knowledge/knowledge.go:66-82` | Repo 接口 11 个方法，违反红线 #15 |

### 修复实施

| 修复项 | 说明 |
|--------|------|
| KB-01: SQL 注入修复 | MinScore 参数化查询 |
| KB-02: 错误吞没修复 | 5 处 `_ =` 异步 ingest + 查询重写错误 |
| KB-03: LLM 评估假阳性修复 | Sufficient:true → false（安全降级） |
| KB-04: HTTP 客户端超时 | http.DefaultClient → 60s 超时客户端 |
| KB-05: Service 层业务逻辑下移 | 提取到 `search_helpers.go`，定义 `ChunkSearcher`/`ChunkAssessor` 窄接口 |
| KB-06: resolveModel 代码重复消除 | 提取到 `llm_resolver.go`，定义 `RefineLLMSettingsGetter`/`LLMCatalogLister` 窄接口 |
| KB-RI: Repo 接口拆分 | 11 方法 → CollectionRepo(5)/DocumentRepo(5)/ChunkRepo(3) |
| 常量提取 | DefaultChunkSize=512 / DefaultChunkOverlap=64 |

---

## 第二轮：错误处理规范化（已完成）

### 问题发现

第一轮审查后遗留的 P1 问题：`internal/knowledge` 包中约 37 处 `fmt.Errorf` 需替换为 `kerrors`（红线 #14），以及 P2 的 `adaptive_router.go` 子查询全失败静默返回空。

### 修复实施

#### KB-FMT: 全量 fmt.Errorf → kerrors 替换

对 `internal/knowledge/` 下 9 个文件共 27 处 `fmt.Errorf` 进行精确分类替换：

| 文件 | 替换数 | 分类 |
|------|--------|------|
| `embedder.go` | 16 | 2 BadRequest + 14 InternalServer |
| `ingest.go` | 4 | 2 BadRequest + 2 InternalServer |
| `retriever.go` | 3 | 2 ServiceUnavailable + 1 InternalServer |
| `hybrid_retriever.go` | 5 | 3 ServiceUnavailable + 2 InternalServer |
| `adaptive_router.go` | 1 | 1 ServiceUnavailable |
| `chunk_strategy.go` | 1 | 1 BadRequest |
| `document_extract.go` | 3 | 2 BadRequest + 1 InternalServer |
| `ocr.go` | 2 | 1 ServiceUnavailable + 1 InternalServer |
| `reranker_factory.go` | 1 | 1 InternalServer |

**错误分类标准**：
- `BadRequest`：用户输入校验失败（空文本、无效 JSON、不支持的格式）
- `InternalServer`：内部处理错误（解析失败、嵌入为空、HTTP 请求失败、环境变量无效）
- `ServiceUnavailable`：服务未就绪（nil 组件、未配置的 provider）

#### KB-AR: adaptive_router 子查询全失败日志

当所有子查询均检索失败时，新增 `event.SysLogWarn` 日志记录，包含原始查询和子查询数量。返回空结果不变（降级策略合理），但不再静默。

#### KB-SH: search_helpers 静默降级日志

`SearchWithEvaluation` 中评估失败和补充检索失败两个降级路径，新增 `event.SysLogWarn` 日志记录，包含错误信息和 collection_id。

#### KB-SVC: Service 层错误映射修复

1. 搜索和评估两处 `kerrors.InternalServer("KNOWLEDGE", err.Error())` 改为 `kerrors.FromError(err)`，保留下游 kerrors 的原始错误类型（如 ServiceUnavailable/BadRequest），避免强制覆盖为 500。
2. 修正 `if rewriteErr != nil` 块的缩进问题。

### Review 报告

使用 `aranea-review` 技能进行系统性审查：

#### 后端架构合规

| 检查项 | 结果 |
|--------|------|
| BA2: biz 不 import trpc-agent-go | ✅ |
| BA3: biz 不 import api proto | ✅ |
| BA4: Service 层无业务逻辑 | ✅ |
| BA6: 跨模块通过窄接口 | ✅ |

#### 后端错误处理

| 检查项 | 结果 |
|--------|------|
| BE1: 业务错误用 kerrors | ✅（`fmt.Errorf` 已清零） |
| BE2: 错误不丢失上下文 | ✅（FromError 保留原始类型） |
| BE4: 不吞掉错误 | ✅（所有降级路径有日志） |
| BLG1: 不使用 log/slog | ✅ |

#### 后端 OOP 合规

| 检查项 | 结果 |
|--------|------|
| BI1: 接口方法 ≤ 5 | ✅ |
| BI6: Repository 接口方法 ≤ 5 | ✅（子接口拆分） |

#### 验证结果

- `go build ./internal/knowledge/...` ✅
- `go build ./internal/biz/knowledge/...` ✅
- `go test ./internal/knowledge/... -count=1` ✅

---

## 第三轮：OOP 优化 + 降级策略注释（已完成）

### 修复实施

| 修复项 | 说明 |
|--------|------|
| RetrievalEvaluator 降级策略注释 | 三种降级策略（no LLM → 保守、resolveModel 失败 → 保守、LLM Call 失败 → 安全）均添加注释说明设计意图 |
| Usecase 子接口字段拆分 | `Usecase.repo` 拆分为 `collections`/`documents`/`chunks` 三个子接口字段，11 处方法调用精确路由到对应子接口 |
| `requireRepo()` 更新 | 检查三个子接口字段而非单一 `repo` 字段 |
| data/knowledge.go 编译期接口检查 | 新增 `var _ biz.KnowledgeRepo = (*knowledgeRepo)(nil)` 确保完整接口实现 |
| KnowledgeService options struct（回滚） | 尝试将 8 参数构造函数改为 options struct 模式，但 Wire 不支持自动解析 struct 字段注入，回滚为原始签名 |

### Review 报告

使用 `aranea-review` 技能进行第三轮审查：

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 |
|------|---------|---------|---------|
| 后端 — 架构合规 | 0 | 0 | 0 |
| 后端 — 分层合规 | 0 | 1 | 0 |
| 后端 — OOP | 0 | 0 | 1 |
| 后端 — 错误处理 | 0 | 0 | 0 |
| 后端 — 依赖注入 | 0 | 1 | 0 |

**建议项**：
- S1: `KnowledgeService` 构造函数 8 参数过多，但 Wire 不支持 options struct 自动注入，需等待 Wire 升级或手动添加 `wire.FieldsOf` provider
- S2: `IngestDocument` 中 `chunkSize`/`chunkOverlap` 默认值设置可考虑下移到 biz 层

**提示项**：
- T1: `NewUsecase(repo Repo)` 仍接收 `Repo` 组合接口（向后兼容），未来可考虑直接接收三个子接口参数

### 验证结果

- `go build ./internal/knowledge/...` ✅
- `go build ./internal/biz/knowledge/...` ✅
- `go test ./internal/knowledge/... -count=1` ✅

---

## 剩余工作

### 仍需修复的中低优先级问题

| 优先级 | 问题 | 文件 | 说明 |
|--------|------|------|------|
| P2 | `KnowledgeService` 构造函数参数过多（8 个） | service/knowledge.go | Wire 不支持 options struct 自动注入，需添加 `wire.FieldsOf` provider 或等待 Wire 升级 |
| P2 | `NewUsecase` 仍接收 `Repo` 组合接口 | biz/knowledge/knowledge.go | 内部已拆分为三个子接口字段，但构造函数签名保持 `Repo` 向后兼容；未来可改为直接接收三个子接口 |
| P2 | `IngestDocument` 中默认值逻辑可下移 | service/knowledge.go | chunkSize/chunkOverlap 默认值设置和 strategy 解析可考虑下移到 biz 层 |
| P3 | `internal/agent/knowledge_inject.go` 编译错误 | agent/knowledge_inject.go | `deps.KnowledgeUsecase` 字段不存在（预存问题，非本次引入） |

---

## 全部修改文件清单

| 文件 | 变更类型 | 轮次 |
|------|----------|------|
| `internal/data/knowledge.go` | Bug 修复 + 安全修复 | 第一轮 |
| `internal/service/knowledge.go` | 错误处理修复 + 业务逻辑下移 + 错误映射修复 | 第一轮+第二轮 |
| `internal/knowledge/retrieval_evaluator.go` | 安全降级修复 + 代码重构 | 第一轮 |
| `internal/knowledge/embedder.go` | 超时修复 + fmt.Errorf→kerrors | 第一轮+第二轮 |
| `internal/knowledge/search_helpers.go` | 新增（评估+补充检索逻辑提取 + 降级日志） | 第一轮+第二轮 |
| `internal/knowledge/llm_resolver.go` | 新增（共享 LLM 解析函数） | 第一轮 |
| `internal/knowledge/query_rewriter.go` | 代码重构（消除重复） | 第一轮 |
| `internal/knowledge/ingest.go` | 常量提取 + fmt.Errorf→kerrors | 第一轮+第二轮 |
| `internal/knowledge/chunk_strategy.go` | 使用常量 + fmt.Errorf→kerrors | 第一轮+第二轮 |
| `internal/knowledge/retriever.go` | fmt.Errorf→kerrors | 第二轮 |
| `internal/knowledge/hybrid_retriever.go` | fmt.Errorf→kerrors | 第二轮 |
| `internal/knowledge/adaptive_router.go` | fmt.Errorf→kerrors + 子查询全失败日志 | 第二轮 |
| `internal/knowledge/document_extract.go` | fmt.Errorf→kerrors | 第二轮 |
| `internal/knowledge/ocr.go` | fmt.Errorf→kerrors | 第二轮 |
| `internal/knowledge/reranker_factory.go` | fmt.Errorf→kerrors | 第二轮 |
| `internal/biz/knowledge/knowledge.go` | 接口拆分 + repo 字段引用修复 + 子接口字段拆分 | 第一轮+第三轮 |
| `internal/biz/knowledge.go` | 类型别名更新 | 第一轮 |
| `internal/knowledge/retrieval_evaluator.go` | 安全降级修复 + 代码重构 + 降级策略注释 | 第一轮+第三轮 |
| `internal/data/knowledge.go` | Bug 修复 + 安全修复 + 编译期接口检查 | 第一轮+第三轮 |
