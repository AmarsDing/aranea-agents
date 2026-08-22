package knowledge

import "context"

// Wave 3：领域门面。Usecase 仍是 Wire 装配根与 nil-safe 兼容面；新调用方按域取门面。
// Stability:evolving

type vaultFields struct {
	collections   CollectionRepo
	documents     DocumentRepo
	chunks        ChunkRepo
	paths         DocumentPathReader
	resolvedLinks ResolvedLinkReader
	filer         *VaultFiler
	applier       VaultDocApplier
}

type graphFields struct {
	links         LinkRepo
	entities      EntityRepo
	blockIndex    BlockIndexRepo
	resolveIndex  ResolveIndex
	linkIndex     *LinkIndex
	graphPub      GraphDeltaPublisher
	blockLinks    BlockLinkReader
	docNames      DocNameReader
	graphLinks    CollectionLinkReader
	mentionSearch DocContentSearcher
	linkUsage     LinkUsageRepo
	promoteReader PromoteBlockReader
	promoteWriter PromoteLineageWriter
}

type writeBackFields struct {
	writeBackReplay WriteBackReplayFunc
	writeBackGraph  WriteBackGraphFunc
	docACL          DocumentACLStore
	factVersions    FactVersionRepo
	proposals       GovernanceProposalRepo
	arbiter         WriteBackArbiter
}

type curateFields struct {
	embedCircuit EmbedCircuitRepo
	curate       KnowledgeCurateRepo
	hotDocs      HotDocumentLister
	distill      DistillFactWriter
}

// Vault is the collection / document / filesystem surface.
// Stability:evolving
type Vault struct{ u *Usecase }

// Retrieve is the chunk search surface (Advanced RAG stays in internal/knowledge).
// Stability:evolving
type Retrieve struct{ u *Usecase }

// Graph is the block-link / promote / memory-graph surface.
// Stability:evolving
type Graph struct{ u *Usecase }

// WriteBack is the Memory→Knowledge projection surface.
// Stability:evolving
type WriteBack struct{ u *Usecase }

// Curate is the self-governance / distill surface.
// Stability:evolving
type Curate struct{ u *Usecase }

func (u *Usecase) Vault() *Vault {
	if u == nil {
		return nil
	}
	return &Vault{u: u}
}

func (u *Usecase) Retrieve() *Retrieve {
	if u == nil {
		return nil
	}
	return &Retrieve{u: u}
}

func (u *Usecase) Graph() *Graph {
	if u == nil {
		return nil
	}
	return &Graph{u: u}
}

func (u *Usecase) WriteBack() *WriteBack {
	if u == nil {
		return nil
	}
	return &WriteBack{u: u}
}

func (u *Usecase) Curate() *Curate {
	if u == nil {
		return nil
	}
	return &Curate{u: u}
}

func (v *Vault) CreateCollection(ctx context.Context, in Collection) (Collection, error) {
	if v == nil || v.u == nil {
		return Collection{}, ErrUnavailable
	}
	return v.u.CreateCollection(ctx, in)
}

func (v *Vault) GetCollection(ctx context.Context, id string) (Collection, error) {
	if v == nil || v.u == nil {
		return Collection{}, ErrUnavailable
	}
	return v.u.GetCollection(ctx, id)
}

func (v *Vault) EnsureDefaultCollection(ctx context.Context, embeddingModel string, dim int, ws string) (Collection, error) {
	if v == nil || v.u == nil {
		return Collection{}, ErrUnavailable
	}
	return v.u.EnsureDefaultCollection(ctx, embeddingModel, dim, ws)
}

func (v *Vault) GetDocument(ctx context.Context, id string) (Document, error) {
	if v == nil || v.u == nil {
		return Document{}, ErrUnavailable
	}
	return v.u.GetDocument(ctx, id)
}

func (v *Vault) GetDocumentByContentHash(ctx context.Context, collectionID, contentHash string) (Document, error) {
	if v == nil || v.u == nil {
		return Document{}, ErrUnavailable
	}
	return v.u.GetDocumentByContentHash(ctx, collectionID, contentHash)
}

func (v *Vault) UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error {
	if v == nil || v.u == nil {
		return ErrUnavailable
	}
	return v.u.UpdateDocumentStatus(ctx, id, status, errMsg, chunkCount)
}

func (v *Vault) CommitIndexedDocument(ctx context.Context, collectionID, docID string, chunks []Chunk, docDelta int) error {
	if v == nil || v.u == nil {
		return ErrUnavailable
	}
	return v.u.CommitIndexedDocument(ctx, collectionID, docID, chunks, docDelta)
}

func (v *Vault) MoveDocument(ctx context.Context, id, targetCollectionID string) (Document, error) {
	if v == nil || v.u == nil {
		return Document{}, ErrUnavailable
	}
	return v.u.MoveDocument(ctx, id, targetCollectionID)
}

func (r *Retrieve) Search(ctx context.Context, q SearchQuery, queryEmbedding []float32) ([]Chunk, error) {
	if r == nil || r.u == nil {
		return nil, ErrUnavailable
	}
	return r.u.Search(ctx, q, queryEmbedding)
}

func (g *Graph) SetLinkIndex(idx *LinkIndex, pub GraphDeltaPublisher) {
	if g == nil || g.u == nil {
		return
	}
	g.u.SetLinkIndex(idx, pub)
}

func (g *Graph) RebuildCollectionBlockIndex(ctx context.Context, collectionID string, onProgress func(done, total, failed int)) (RebuildIndexResult, error) {
	if g == nil || g.u == nil {
		return RebuildIndexResult{}, ErrUnavailable
	}
	return g.u.RebuildCollectionBlockIndex(ctx, collectionID, onProgress)
}

func (g *Graph) PromoteDocuments(ctx context.Context, docIDs []string, targetCollectionID string) (PromoteResult, error) {
	if g == nil || g.u == nil {
		return PromoteResult{}, ErrUnavailable
	}
	return g.u.PromoteDocuments(ctx, docIDs, targetCollectionID)
}

func (w *WriteBack) WriteBackSessionFacts(ctx context.Context, in WriteBackInput) (WriteBackResult, error) {
	if w == nil || w.u == nil {
		return WriteBackResult{}, ErrUnavailable
	}
	return w.u.WriteBackSessionFacts(ctx, in)
}

func (w *WriteBack) SetReplay(fn WriteBackReplayFunc) {
	if w == nil || w.u == nil {
		return
	}
	w.u.SetWriteBackReplay(fn)
}

func (w *WriteBack) HasReplay() bool {
	return w != nil && w.u != nil && w.u.HasWriteBackReplay()
}

func (c *Curate) CurateKnowledge(ctx context.Context, opts CurateOptions) (CurateReport, error) {
	if c == nil || c.u == nil {
		return CurateReport{}, ErrUnavailable
	}
	return c.u.CurateKnowledge(ctx, opts)
}
