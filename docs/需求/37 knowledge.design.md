# Knowledge 知识库模块 — 实现设计文档

> 对应需求：`37 knowledge.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

RAG 知识库：文档导入、分块、向量化、检索增强。对标 trpc-agent-go `knowledge` 包。

---

## 二、Proto 层

### 2.1 待新增

```protobuf
service KnowledgeService {
  rpc ListKnowledgeBases(ListKnowledgeBasesRequest) returns (ListKnowledgeBasesResponse) {
    option (google.api.http) = { get: "/v1/knowledge-bases" };
  }
  rpc CreateKnowledgeBase(CreateKnowledgeBaseRequest) returns (KnowledgeBase) {
    option (google.api.http) = { post: "/v1/knowledge-bases" body: "*" };
  }
  rpc DeleteKnowledgeBase(DeleteKnowledgeBaseRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/knowledge-bases/{id}" };
  }
  rpc UploadDocument(UploadDocumentRequest) returns (Document) {
    option (google.api.http) = { post: "/v1/knowledge-bases/{id}/documents" body: "*" };
  }
  rpc ListDocuments(ListDocumentsRequest) returns (ListDocumentsResponse) {
    option (google.api.http) = { get: "/v1/knowledge-bases/{id}/documents" };
  }
  rpc DeleteDocument(DeleteDocumentRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/knowledge-bases/{kb_id}/documents/{id}" };
  }
  rpc SearchKnowledge(SearchKnowledgeRequest) returns (SearchKnowledgeResponse) {
    option (google.api.http) = { post: "/v1/knowledge-bases/{id}/search" body: "*" };
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type KnowledgeBase struct {
    ID          string
    Name        string
    Description string
    EmbeddingModel string
    ChunkStrategy string  // "fixed"/"semantic"/"sentence"
    ChunkSize   int32
    ChunkOverlap int32
    AgentID     string  // 绑定 Agent
    DocCount    int32
    Status      string
    CreatedAt   string
    UpdatedAt   string
}

type Document struct {
    ID              string
    KnowledgeBaseID string
    Name            string
    MimeType        string
    Size            int64
    ChunkCount      int32
    Status          string  // "pending"/"processing"/"ready"/"error"
    CreatedAt       string
}

type DocumentChunk struct {
    ID         string
    DocumentID string
    Content    string
    Embedding  []float64
    Metadata   map[string]string
    SortOrder  int32
}
```

### 3.2 Usecase

```go
func (uc *KnowledgeUsecase) CreateKB(ctx, kb KnowledgeBase) (KnowledgeBase, error)
func (uc *KnowledgeUsecase) UploadDocument(ctx, kbID string, doc Document, content []byte) (Document, error)
func (uc *KnowledgeUsecase) Search(ctx, kbID, query string, topK int) ([]SearchResult, error)
func (uc *KnowledgeUsecase) DeleteKB(ctx, id) error
```

### 3.3 Knowledge 接口

```go
type Knowledge interface {
    Search(ctx, query string, topK int) ([]SearchResult, error)
}

type SearchResult struct {
    Content   string
    Score     float64
    Source    string
    Metadata  map[string]string
}
```

---

## 四、Data 层

### 4.1 分块策略

```go
// internal/knowledge/chunking/fixed.go
func FixedChunk(text string, size, overlap int) []string

// internal/knowledge/chunking/semantic.go
func SemanticChunk(text string, llm model.LLM) ([]string, error)
```

### 4.2 向量化

```go
// internal/knowledge/embedding.go
func Embed(ctx, texts []string, model string) ([][]float64, error)
```

### 4.3 向量存储

```go
// internal/data/pgvector/knowledge.go
func (v *VectorStore) InsertChunks(ctx, chunks []DocumentChunk) error
func (v *VectorStore) Search(ctx, kbID string, embedding []float64, topK int) ([]SearchResult, error)
```

### 4.4 Ent Schema

- `internal/data/ent/schema/knowledge_base.go`
- `internal/data/ent/schema/knowledge_document.go`

---

## 五、Service 层

```go
func (s *KnowledgeService) CreateKnowledgeBase(ctx, req) (*KnowledgeBase, error)
func (s *KnowledgeService) UploadDocument(ctx, req) (*Document, error)
func (s *KnowledgeService) SearchKnowledge(ctx, req) (*SearchKnowledgeResponse, error)
func (s *KnowledgeService) DeleteKnowledgeBase(ctx, req) (*emptypb.Empty, error)
```

---

## 六、Wire 注入

待新增：
```
data.ProviderSet → NewKnowledgeRepo
biz.ProviderSet → NewKnowledgeUsecase
service.ProviderSet → NewKnowledgeService
```

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/features/knowledge/
├── api.ts
├── types.ts
└── components/
    ├── KnowledgeListPage.vue
    ├── KnowledgeDetailPage.vue
    ├── DocumentUploadZone.vue
    ├── DocumentList.vue
    └── KnowledgeSearchDialog.vue
```

### 7.2 组件设计

**KnowledgeDetailPage.vue**：

| 区域 | 组件 | 说明 |
|------|------|------|
| 信息 | `QCard` | 名称/描述/嵌入模型/分块策略 |
| 文档 | `DocumentList` | 上传/删除/状态 |
| 搜索 | `KnowledgeSearchDialog` | 测试检索 |
| 绑定 | `QSelect` | 绑定 Agent |

**DocumentUploadZone.vue**：拖拽上传区域，支持 PDF/TXT/MD/DOCX

### 7.3 API

```typescript
export async function listKnowledgeBases(query: KBQuery): Promise<KBListResult>
export async function createKnowledgeBase(req: CreateKBRequest): Promise<KnowledgeBase>
export async function deleteKnowledgeBase(id: string): Promise<void>
export async function uploadDocument(kbId: string, file: File): Promise<Document>
export async function listDocuments(kbId: string): Promise<Document[]>
export async function deleteDocument(kbId: string, docId: string): Promise<void>
export async function searchKnowledge(kbId: string, query: string, topK: number): Promise<SearchResult[]>
```
