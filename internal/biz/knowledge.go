package biz

import "aranea-agents/internal/biz/knowledge"

type (
	KnowledgeCollection       = knowledge.Collection
	KnowledgeDocument         = knowledge.Document
	KnowledgeChunk            = knowledge.Chunk
	KnowledgeSearchQuery      = knowledge.SearchQuery
	KnowledgeCollectionRepo   = knowledge.CollectionRepo
	KnowledgeDocumentRepo     = knowledge.DocumentRepo
	KnowledgeChunkRepo        = knowledge.ChunkRepo
	KnowledgeRepo             = knowledge.Repo
	KnowledgeUsecase          = knowledge.Usecase
	KnowledgeEmbedSetting     = knowledge.EmbedSetting
	KnowledgeSparseSearcher   = knowledge.SparseSearcher
	KnowledgeDocumentSyncMeta = knowledge.DocumentSyncMeta
	// P3 资源管理器（Vault explorer）：树构建与已解析关联。
	KnowledgeDocumentPath       = knowledge.DocumentPath
	KnowledgeVaultTreeNode      = knowledge.VaultTreeNode
	KnowledgeResolvedLink       = knowledge.ResolvedLink
	KnowledgeDocumentPathReader = knowledge.DocumentPathReader
	KnowledgeResolvedLinkReader = knowledge.ResolvedLinkReader
)

var (
	NewKnowledgeUsecase                = knowledge.NewUsecase
	NewKnowledgeUsecaseFromRepo        = knowledge.NewUsecaseFromRepo
	ErrKnowledgeUnavailable            = knowledge.ErrUnavailable
	ErrKnowledgeNameRequired           = knowledge.ErrNameRequired
	ErrKnowledgeEmbeddingModelRequired = knowledge.ErrEmbeddingModelRequired
	ErrKnowledgeIDRequired             = knowledge.ErrIDRequired
	ErrKnowledgeCollectionIDRequired   = knowledge.ErrCollectionIDRequired
	ErrKnowledgeSourceRequired         = knowledge.ErrSourceRequired
	ErrKnowledgeQueryRequired          = knowledge.ErrQueryRequired
	ErrKnowledgeDimensionMismatch      = knowledge.ErrDimensionMismatch
	ErrKnowledgeEmbeddingEmpty         = knowledge.ErrEmbeddingEmpty
	KnowledgeEmbedConfigured           = knowledge.EmbedConfigured
	ApplyKnowledgeEmbedPatch           = knowledge.ApplyEmbedPatch
)

// ProvideKnowledgeUsecase 是生产 Wire provider：在 NewUsecaseFromRepo 之上
// 按接口断言自动装配可选能力——P2-4 双轨关联（LinkRepo/EntityRepo）与
// P3 资源管理器（DocumentPathReader/ResolvedLinkReader）。repo 未实现对应
// 接口时保持未接线降级语义（关联 no-op / 关联查询为空 / 树查询显式报错）。
// filer 为共享 VaultFiler 实例（G1-B1 树目录 FS 扫描 + G1-B2 树内写文件）：
// 与 vault 同步链同一实例，自写标记统一登记防 watcher 回环。
func ProvideKnowledgeUsecase(repo KnowledgeRepo, filer *knowledge.VaultFiler) *KnowledgeUsecase {
	uc := knowledge.NewUsecaseFromRepo(repo)
	uc.SetVaultFiler(filer)
	if repo == nil {
		return uc
	}
	links, lok := repo.(knowledge.LinkRepo)
	entities, eok := repo.(knowledge.EntityRepo)
	if lok && eok {
		uc.SetLinkRepos(links, entities)
	}
	paths, pok := repo.(knowledge.DocumentPathReader)
	resolved, rok := repo.(knowledge.ResolvedLinkReader)
	if pok || rok {
		uc.SetExplorerRepos(paths, resolved)
	}
	return uc
}
