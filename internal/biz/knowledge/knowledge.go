// Package knowledge implements knowledge base collection/document/search workflows.
package knowledge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// Collection is a named vector store.
// V2：Collection 语义升级为 Vault——root_path 指向本地目录（文件系统即真相源），
// embedding_model 可选（空 = 无语义层，L0 词法 + L1 导航完整可用）。
// SP1-F：VaultBackend 区分存储后端——local=文件系统真相源 / team=PG 真相源（团队库）。
type Collection struct {
	ID             string
	Name           string
	Description    string
	EmbeddingModel string
	Dim            int
	Status         string
	DocumentCount  int
	ChunkCount     int
	Workspace      string
	// RootPath 是 vault 根目录（规范化绝对路径，唯一）；空表示历史 Collection（未迁移）或 team 库。
	RootPath string
	// SyncState 同步状态：active / paused / error / migrating。
	SyncState  string
	LastSyncAt string
	// VaultBackend 存储后端维度（SP1-F）：VaultBackendLocal / VaultBackendTeam。
	// local：文件系统即真相源，RootPath 必填；team：PG 即真相源，RootPath 必须为空。
	VaultBackend string
	CreatedAt    string
	UpdatedAt    string
}

// VaultBackend 存储后端取值（SP1-F，设计 S6）。
const (
	// VaultBackendLocal 本地库：文件系统即真相源，SyncEngine 监听 root_path。
	VaultBackendLocal = "local"
	// VaultBackendTeam 团队库：PG 即真相源（documents.content_text 承载本体），无 SyncEngine。
	VaultBackendTeam = "team"
)

// Document is one source document ingested into a collection.
type Document struct {
	ID           string
	CollectionID string
	Source       string
	MimeType     string
	SizeBytes    int64
	ChunkCount   int
	Status       string
	ErrorMessage string
	// ContentText 是提取/整理后的全文（organized=true 时为结构化 Markdown），供预览与血缘。
	ContentText string
	// Organized 标记内容是否经 LLM 整理为 Markdown。
	Organized bool
	// AssetURI 是原始文件留存路径（Phase 9 多模态血缘），文本类文档为空。
	AssetURI string
	// RelPath 是 vault 内相对路径（正斜杠）；空表示非 vault 文档。
	RelPath string
	// ContentHash 是正文 sha1（增量同步去重 + 移动识别）。
	ContentHash string
	// Summary 是 LLM 生成的摘要卡（US-17）。
	Summary string
	// SummaryHash 是被摘要内容的 hash（过期检测）。
	SummaryHash string
	// Tags 是 LLM 打标（摘要卡）。
	Tags []string
	// DocType 是自动分类（report/manual/note/faq…）。
	DocType string
	// EmbedFailCount 是 embedding 连续失败计数（SP2 #9 熔断）；0 = 熔断关闭。
	EmbedFailCount int
	// EmbedLastTried 是最近一次 embed 尝试时间（退避判定依据）；零值 = 从未尝试。
	EmbedLastTried time.Time
	CreatedAt      string
	UpdatedAt      string
}

// Chunk is one indexed text chunk with its embedding.
type Chunk struct {
	ID           string
	DocID        string
	CollectionID string
	Content      string
	Embedding    []float32
	MetadataJSON string
	ChunkIndex   int
	Score        float32
}

// SearchQuery holds search parameters.
type SearchQuery struct {
	CollectionID     string
	Query            string
	TopK             int
	MinScore         float32
	FilterJSON       string
	UseRerank        *bool
	RerankCandidates int
	// PathPrefix 搜索范围过滤（G3-B7）：vault 相对目录前缀，仅命中文档
	// rel_path 位于 "<prefix>/" 下的 chunks；空 = 全库。首尾斜杠容忍。
	PathPrefix string
	// ExcludePathPrefixes 检索排除（词条优先写回）：rel_path 以任一前缀开头
	// 的文档不参与检索（字面前缀语义，非目录边界）。用于把写回日记流水
	// （inbox/writeback-*，仅 provenance）挡在 Agent 默认检索外。
	ExcludePathPrefixes []string
}

// Repo is the persistence interface for knowledge base operations.
// Stability:evolving
type CollectionRepo interface {
	CreateCollection(ctx context.Context, c Collection) (Collection, error)
	GetCollection(ctx context.Context, id string) (Collection, error)
	ListCollections(ctx context.Context, workspace string, limit, offset int) ([]Collection, int, error)
	DeleteCollection(ctx context.Context, id string) error
	UpdateCollectionCounts(ctx context.Context, id string, docDelta, chunkDelta int) error
	// UpdateCollectionSyncState 回写 vault 同步状态与最近一次同步完成时间（P1-3 轮询）。
	UpdateCollectionSyncState(ctx context.Context, id, state string, lastSyncAt time.Time) error
	// EnableCollectionSemantic 空语义层单向启用（B2）：守卫式 UPDATE，仅当集合
	// embedding_model 仍为空时绑定 model/dim；返回 bool=是否生效（false=并发已绑定/不存在）。
	EnableCollectionSemantic(ctx context.Context, id, model string, dim int) (bool, error)
}

// DocumentSyncMeta 同步镜像元数据（Vault 同步 modified 事件回写，P1-3）。
type DocumentSyncMeta struct {
	ContentHash string
	Summary     string
	SummaryHash string
	Tags        []string
	DocType     string
}

// Stability:evolving
type DocumentRepo interface {
	CreateDocument(ctx context.Context, d Document) (Document, error)
	GetDocument(ctx context.Context, id string) (Document, error)
	// GetDocumentByRelPath 按 vault 相对路径寻址文档（Vault 同步用）。
	GetDocumentByRelPath(ctx context.Context, collectionID, relPath string) (Document, error)
	// UpdateDocumentRelPath 文件移动/重命名时更新镜像路径（保留文档身份与索引）。
	UpdateDocumentRelPath(ctx context.Context, id, newRelPath string) error
	// UpdateDocumentSyncMeta 文件内容变更时回写同步元数据（hash/摘要卡字段）。
	UpdateDocumentSyncMeta(ctx context.Context, id string, meta DocumentSyncMeta) error
	UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error
	// UpdateDocumentContent 回写文档正文与整理标记（Phase 9 图片异步提取完成后调用）。
	UpdateDocumentContent(ctx context.Context, id, contentText string, organized bool) error
	ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]Document, int, error)
	// ListDocumentsPendingReembed 列出待重嵌入文档（B1）：有正文、非 indexing、
	// 且（chunks embedding IS NULL 或无任何 chunks）。按 created_at ASC（先入队先处理）。
	ListDocumentsPendingReembed(ctx context.Context, collectionID string) ([]Document, error)
	DeleteDocument(ctx context.Context, id string) error
	// MoveDocument 文档连同 chunks 移至目标 Collection（US-14）。
	// Repo 实现必须在单事务内完成 documents/chunks 的 collection_id 更新与两侧计数校正。
	MoveDocument(ctx context.Context, id, targetCollectionID string) (Document, error)
}

// Stability:evolving
type ChunkRepo interface {
	InsertChunks(ctx context.Context, chunks []Chunk) error
	DeleteChunksByDocument(ctx context.Context, docID string) error
	SearchChunks(ctx context.Context, q SearchQuery, queryEmbedding []float32) ([]Chunk, error)
}

// Stability:evolving
type Repo interface {
	CollectionRepo
	DocumentRepo
	ChunkRepo
}

// SparseSearcher is the interface for BM25/full-text search over knowledge chunks.
// Stability:evolving
type SparseSearcher interface {
	SearchChunksBM25(ctx context.Context, q SearchQuery) ([]Chunk, error)
}

// KnowledgeEmbedder generates text embeddings using a remote API and exposes
// runtime configuration for the admin UI. The concrete HTTP implementation
// lives in the data layer; biz depends only on this interface.
// Stability:evolving
type KnowledgeEmbedder interface {
	// Embed returns a single embedding vector for the input text.
	Embed(ctx context.Context, text string) ([]float32, error)
	// EmbedWithTaskType returns a single embedding with a task type hint (e.g. "RETRIEVAL_QUERY").
	EmbedWithTaskType(ctx context.Context, text string, taskType string) ([]float32, error)
	// EmbedBatch returns embeddings for a slice of texts using provider batch APIs when available.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	// EmbedBatchWithTaskType returns embeddings with an optional task type hint.
	EmbedBatchWithTaskType(ctx context.Context, texts []string, taskType string) ([][]float32, error)
	// Config returns a redacted view of embedder settings.
	Config() (provider, baseURL, model string, dim int, configured bool, hasAPIKey bool)
	// Update applies runtime embedder settings from admin UI.
	Update(provider, baseURL, apiKey, model string, dim int)
}

// Domain errors for knowledge biz layer — the Service layer maps these to apierror.
var (
	ErrUnavailable            = apierror.Internal("KNOWLEDGE", "knowledge base requires PostgreSQL with pgvector; configure data.postgres.source")
	ErrNameRequired           = apierror.BadRequest("KNOWLEDGE", "name is required")
	ErrEmbeddingModelRequired = apierror.BadRequest("KNOWLEDGE", "embedding_model is required")
	ErrIDRequired             = apierror.BadRequest("KNOWLEDGE", "id is required")
	ErrCollectionIDRequired   = apierror.BadRequest("KNOWLEDGE", "collection_id is required")
	ErrSourceRequired         = apierror.BadRequest("KNOWLEDGE", "source is required")
	ErrQueryRequired          = apierror.BadRequest("KNOWLEDGE", "query is required")
	ErrDimensionMismatch      = apierror.BadRequest("KNOWLEDGE", "embedding dimension mismatch")
	ErrEmbeddingEmpty         = apierror.BadRequest("KNOWLEDGE", "embedding is empty")
	// ErrMoveDimensionMismatch 目标库与源库 dim 不一致，禁止跨库移动（向量维度不兼容）。
	ErrMoveDimensionMismatch = apierror.Conflict("KNOWLEDGE", "target collection embedding dimension differs; re-ingest the document instead of moving")
	// ErrCollectionSemanticConflict 语义层已被并发请求绑定（B2 守卫式 UPDATE 未生效）。
	ErrCollectionSemanticConflict = apierror.Conflict("KNOWLEDGE", "semantic layer already enabled")
)

// DefaultCollectionName 是 US-14「上传免预选」的兜底知识库名称（懒创建，按 name 复用）。
const DefaultCollectionName = "默认知识库"

// Usecase implements collection/document/search operations.
type Usecase struct {
	collections CollectionRepo
	documents   DocumentRepo
	chunks      ChunkRepo
	// links/entities 为可选关联能力（P2-4），经 SetLinkRepos 接线；nil 时关联方法降级 no-op。
	links    LinkRepo
	entities EntityRepo
	// blockIndex/resolveIndex 为块级双链派生索引（SP1），经 SetBlockIndexRepos 接线；
	// blockIndex nil 时 RebuildBlockIndex 降级 no-op。
	blockIndex   BlockIndexRepo
	resolveIndex ResolveIndex
	// linkIndex/graphPub 为统一链接索引与图谱增量事件出口（SP1-D），经 SetLinkIndex 接线；
	// linkIndex nil 时 RebuildBlockIndex 跳过内存图 apply 与 WS 增量（降级安全）。
	linkIndex *LinkIndex
	graphPub  GraphDeltaPublisher
	// blockLinks/docNames 为块级反链读端口（SP1-E），经 SetBacklinkRepos 接线；
	// blockLinks 为启动窗口 DB 兜底，docNames 为源文档名解析（失败留空降级）。
	blockLinks BlockLinkReader
	docNames   DocNameReader
	// paths/resolvedLinks 为资源管理器能力（P3），经 SetExplorerRepos 接线；
	// paths nil 时 ListVaultTree 显式报错，resolvedLinks nil 时关联查询降级为空。
	paths         DocumentPathReader
	resolvedLinks ResolvedLinkReader
	// graphLinks 为库级关联读取（G4-B8），经 SetGraphRepo 接线；
	// nil 时 ListCollectionGraph 降级为仅节点无边。
	graphLinks CollectionLinkReader
	// mentionSearch 为 unlinked mentions 内容扫描端口（P2-7），经 SetMentionSearcher 接线；
	// nil 时 ListUnlinkedMentions 降级为空。
	mentionSearch DocContentSearcher
	// filer 为 vault 文件系统边界（G1-B1），经 SetVaultFiler 接线；
	// nil 时 ListVaultTree 目录退化为纯索引聚合。
	filer *VaultFiler
	// applier 为单文档立即应用端口（G1-B2），经 SetVaultApplier 接线；
	// nil 时 CreateVaultDocument 跳过立即索引（同步轮询兜底）。
	applier VaultDocApplier
	// promoteReader/promoteWriter 为晋升端口（SP1-G），经 SetPromoteRepos 接线；
	// nil 时 PromoteBlocks 返回 ErrUnavailable。
	promoteReader PromoteBlockReader
	promoteWriter PromoteLineageWriter
	// embedCircuit 为 embedding 熔断端口（SP2 #9），经 SetEmbedCircuitRepo 接线；
	// nil 时熔断读写降级 no-op（embed 失败仍降级词法索引，仅失去熔断记忆）。
	embedCircuit EmbedCircuitRepo
	// linkUsage 为 wikilink 落链 recency 端口（B4 #8），经 SetLinkUsageRepo 接线；
	// nil 时 RecordLinkUse/ListRecentLinkUses 降级 no-op（recency 非正确性依赖）。
	linkUsage LinkUsageRepo
	// writeBackReplay 为写回 chunk 重放钩子（2026-08-15），经 SetWriteBackReplay
	// 接线（生产由 KnowledgeService 注入 replayPromotedDocChunks 同逻辑）；
	// nil 时写回只落 documents 表不重建 chunks（降级——检索不可见，ReembedDocuments
	// 手动自愈）。放 biz 层收口：knowledge_write 工具直调 Usecase（不经 service
	// 包装），重放挂 service 层时该路径绕过，entries/* 永久 pending。
	writeBackReplay WriteBackReplayFunc
	// lg 为域日志器（SP1-H 起：回填等 best-effort 副作用的失败 Warn 出口）；
	// 构造默认 Noop，生产经 SetLogger 接线。
	lg loggateway.Logger
}

// NewUsecase constructs a KnowledgeUsecase from individual sub-interfaces.
func NewUsecase(collections CollectionRepo, documents DocumentRepo, chunks ChunkRepo) *Usecase {
	return &Usecase{collections: collections, documents: documents, chunks: chunks, lg: loggateway.NewNoop()}
}

// NewUsecaseFromRepo constructs a KnowledgeUsecase from the combined Repo interface.
func NewUsecaseFromRepo(repo Repo) *Usecase {
	return &Usecase{collections: repo, documents: repo, chunks: repo, lg: loggateway.NewNoop()}
}

// SetLogger 接线域日志器（生产装配调用；nil 保持 Noop）。
func (u *Usecase) SetLogger(lg loggateway.Logger) {
	if lg != nil {
		u.lg = lg.With(loggateway.Domain("knowledge"))
	}
}

func (u *Usecase) requireRepo() error {
	if u == nil || u.collections == nil || u.documents == nil || u.chunks == nil {
		return ErrUnavailable
	}
	return nil
}

// IsUnavailable reports whether the knowledge backend (Postgres/pgvector) is not configured.
// When true, knowledge_search/knowledge_reflect tools should not be registered for agents.
func (u *Usecase) IsUnavailable() bool {
	return u == nil || u.collections == nil || u.documents == nil || u.chunks == nil
}

func newKnowledgeID() string {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "kn-fallback"
	}
	return hex.EncodeToString(buf)
}

// CreateCollection validates and persists a new collection.
func (u *Usecase) CreateCollection(ctx context.Context, in Collection) (Collection, error) {
	if err := u.requireRepo(); err != nil {
		return Collection{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.EmbeddingModel = strings.TrimSpace(in.EmbeddingModel)
	if in.Name == "" {
		return Collection{}, ErrNameRequired
	}
	if in.EmbeddingModel == "" {
		return Collection{}, ErrEmbeddingModelRequired
	}
	if in.Dim <= 0 {
		in.Dim = 1536
	}
	if in.ID == "" {
		in.ID = newKnowledgeID()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	return u.collections.CreateCollection(ctx, in)
}

// GetCollection returns a single collection.
func (u *Usecase) GetCollection(ctx context.Context, id string) (Collection, error) {
	if err := u.requireRepo(); err != nil {
		return Collection{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Collection{}, ErrIDRequired
	}
	return u.collections.GetCollection(ctx, id)
}

// EnableCollectionSemantic 空语义层单向启用（B2）：透传守卫式 UPDATE；
// 未生效（并发已绑定）→ ErrCollectionSemanticConflict。
func (u *Usecase) EnableCollectionSemantic(ctx context.Context, id, model string, dim int) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return ErrIDRequired
	}
	ok, err := u.collections.EnableCollectionSemantic(ctx, id, model, dim)
	if err != nil {
		return err
	}
	if !ok {
		return ErrCollectionSemanticConflict
	}
	return nil
}

// ListCollections returns all collections visible in the workspace.
func (u *Usecase) ListCollections(ctx context.Context, workspace string, limit, offset int) ([]Collection, int, error) {
	if err := u.requireRepo(); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	return u.collections.ListCollections(ctx, workspace, limit, offset)
}

// DeleteCollection removes a collection and all its documents/chunks.
// SP1-G：删除成功后同步内存图（源边消失、外部入边转 dangling）并发 WS 增量。
func (u *Usecase) DeleteCollection(ctx context.Context, id string) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return ErrIDRequired
	}
	if err := u.collections.DeleteCollection(ctx, id); err != nil {
		return err
	}
	u.removeLinkIndexCollection(ctx, id)
	return nil
}

// CreateDocument records a document and returns it (status=pending).
func (u *Usecase) CreateDocument(ctx context.Context, d Document) (Document, error) {
	if err := u.requireRepo(); err != nil {
		return Document{}, err
	}
	d.Source = strings.TrimSpace(d.Source)
	d.CollectionID = strings.TrimSpace(d.CollectionID)
	if d.CollectionID == "" {
		return Document{}, ErrCollectionIDRequired
	}
	if d.Source == "" {
		return Document{}, ErrSourceRequired
	}
	if d.ID == "" {
		d.ID = newKnowledgeID()
	}
	if d.Status == "" {
		d.Status = "pending"
	}
	return u.documents.CreateDocument(ctx, d)
}

// GetDocument returns a single document by ID, including its extracted/organized content.
func (u *Usecase) GetDocument(ctx context.Context, id string) (Document, error) {
	if err := u.requireRepo(); err != nil {
		return Document{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Document{}, ErrIDRequired
	}
	return u.documents.GetDocument(ctx, id)
}

// ListDocuments returns documents for a collection.
func (u *Usecase) ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]Document, int, error) {
	if err := u.requireRepo(); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	return u.documents.ListDocuments(ctx, collectionID, limit, offset)
}

// ListDocumentsPendingReembed 列出集合内待重嵌入文档（B1：维度对账向量置 NULL 后，
// UI 上传文档无 vault_sync 自愈循环，由本方法筛出喂给重嵌入管线）。
func (u *Usecase) ListDocumentsPendingReembed(ctx context.Context, collectionID string) ([]Document, error) {
	if err := u.requireRepo(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(collectionID) == "" {
		return nil, ErrCollectionIDRequired
	}
	return u.documents.ListDocumentsPendingReembed(ctx, collectionID)
}

// DeleteDocument removes a document and its chunks. Repo implementations MUST
// keep `knowledge_collections.document_count / chunk_count` in sync atomically.
// DAT-02 / KB-04: prior repo implementations only deleted the document row
// (relying on FK cascade for chunks) but never decremented the collection tally.
// SP1-D：删除成功后同步内存图（出边清除、入边转 dangling）并发 WS 增量。
func (u *Usecase) DeleteDocument(ctx context.Context, id string) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return ErrIDRequired
	}
	if err := u.documents.DeleteDocument(ctx, id); err != nil {
		return err
	}
	u.removeLinkIndexDoc(ctx, id)
	return nil
}

// InsertChunks stores indexed chunks for a document.
func (u *Usecase) InsertChunks(ctx context.Context, chunks []Chunk) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	if len(chunks) == 0 {
		return nil
	}
	if err := u.chunks.InsertChunks(ctx, chunks); err != nil {
		if strings.Contains(err.Error(), "dimension mismatch") {
			return ErrDimensionMismatch
		}
		return apierror.Wrap(err, apierror.CodeInternal, apierror.DomainKnowledge)
	}
	return nil
}

// Search performs a vector similarity search.
func (u *Usecase) Search(ctx context.Context, q SearchQuery, queryEmbedding []float32) ([]Chunk, error) {
	if err := u.requireRepo(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(q.CollectionID) == "" {
		return nil, ErrCollectionIDRequired
	}
	if strings.TrimSpace(q.Query) == "" {
		return nil, ErrQueryRequired
	}
	if q.TopK <= 0 {
		q.TopK = 5
	}
	chunks, err := u.chunks.SearchChunks(ctx, q, queryEmbedding)
	if err != nil {
		if strings.Contains(err.Error(), "embedding is empty") {
			return nil, ErrEmbeddingEmpty
		}
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainKnowledge)
	}
	return chunks, nil
}

// UpdateDocumentStatus marks a document's indexing state.
func (u *Usecase) UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	return u.documents.UpdateDocumentStatus(ctx, id, status, errMsg, chunkCount)
}

// UpdateDocumentContent 回写文档正文与整理标记（Phase 9 图片异步提取完成后调用）。
func (u *Usecase) UpdateDocumentContent(ctx context.Context, id, contentText string, organized bool) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	return u.documents.UpdateDocumentContent(ctx, id, contentText, organized)
}

// UpdateCollectionCounts adjusts document/chunk tallies on a collection.
func (u *Usecase) UpdateCollectionCounts(ctx context.Context, id string, docDelta, chunkDelta int) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	return u.collections.UpdateCollectionCounts(ctx, id, docDelta, chunkDelta)
}

// EnsureDefaultCollection 返回「默认知识库」，不存在则按当前 Embedder 配置懒创建（US-14 上传免预选兜底）。
// 按 name + workspace 精确匹配复用——不引入 is_default 标记列，避免多默认库歧义。
// ws 为调用方 workspace（system 调用传 "" 表示共享库）；创建时盖章到 Collection，
// 与 service 层 CreateCollection 的 C-01 租户盖章行为一致——否则懒创建的默认库
// workspace="" 会被 assertCollectionMutateAccess 视为共享只读，导致首次入库 404。
func (u *Usecase) EnsureDefaultCollection(ctx context.Context, embeddingModel string, dim int, ws string) (Collection, error) {
	if err := u.requireRepo(); err != nil {
		return Collection{}, err
	}
	cols, _, err := u.collections.ListCollections(ctx, ws, 1000, 0)
	if err != nil {
		return Collection{}, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainKnowledge)
	}
	for _, c := range cols {
		if c.Name == DefaultCollectionName && c.Workspace == ws {
			return c, nil
		}
	}
	return u.CreateCollection(ctx, Collection{
		Name:           DefaultCollectionName,
		Description:    "未指定知识库的文档自动归入此处",
		EmbeddingModel: embeddingModel,
		Dim:            dim,
		Workspace:      ws,
	})
}

// MoveDocument 文档跨库移动（US-14 整理归档：默认库收件箱 → 分类库）。
// 校验目标库存在且与源库 dim 兼容后，委托 Repo 在单事务内完成 documents/chunks 随迁与计数校正。
func (u *Usecase) MoveDocument(ctx context.Context, id, targetCollectionID string) (Document, error) {
	if err := u.requireRepo(); err != nil {
		return Document{}, err
	}
	id = strings.TrimSpace(id)
	targetCollectionID = strings.TrimSpace(targetCollectionID)
	if id == "" {
		return Document{}, ErrIDRequired
	}
	if targetCollectionID == "" {
		return Document{}, ErrCollectionIDRequired
	}
	doc, err := u.documents.GetDocument(ctx, id)
	if err != nil {
		return Document{}, err
	}
	if doc.CollectionID == targetCollectionID {
		return doc, nil // 同库 no-op
	}
	src, err := u.collections.GetCollection(ctx, doc.CollectionID)
	if err != nil {
		return Document{}, err
	}
	dst, err := u.collections.GetCollection(ctx, targetCollectionID)
	if err != nil {
		return Document{}, err
	}
	if src.Dim != dst.Dim {
		return Document{}, ErrMoveDimensionMismatch
	}
	return u.documents.MoveDocument(ctx, id, targetCollectionID)
}

// ── Embed settings ────────────────────────────────────────────────────────────

// EmbedSetting is persisted platform default for the knowledge embedder.
type EmbedSetting struct {
	Provider  string
	BaseURL   string
	Model     string
	Dim       int
	APIKey    string
	HasAPIKey bool
}

// EmbedConfigured reports whether stored settings are sufficient for the provider.
func EmbedConfigured(s EmbedSetting) bool {
	p := strings.TrimSpace(s.Provider)
	if p == "" {
		return false
	}
	switch p {
	case "ollama":
		return true
	case "huggingface":
		return strings.TrimSpace(s.BaseURL) != ""
	default:
		return s.HasAPIKey
	}
}

// ApplyEmbedPatch merges an update onto current settings.
func ApplyEmbedPatch(cur EmbedSetting, provider, baseURL, apiKey, model string, dim int, updateAPIKey bool) EmbedSetting {
	out := cur
	if p := strings.TrimSpace(provider); p != "" {
		out.Provider = p
	}
	out.BaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	out.Model = strings.TrimSpace(model)
	if dim > 0 {
		out.Dim = dim
	}
	if updateAPIKey && strings.TrimSpace(apiKey) != "" {
		out.APIKey = strings.TrimSpace(apiKey)
		out.HasAPIKey = true
	}
	return out
}
