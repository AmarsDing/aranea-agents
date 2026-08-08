package biz

import (
	"aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

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
	// G4-B8 库级图谱：节点/边/图谱与关联读取端口。
	KnowledgeCollectionGraphNode  = knowledge.CollectionGraphNode
	KnowledgeCollectionGraphEdge  = knowledge.CollectionGraphEdge
	KnowledgeCollectionGraph      = knowledge.CollectionGraph
	KnowledgeCollectionLinkReader = knowledge.CollectionLinkReader
)

var (
	NewKnowledgeUsecase                = knowledge.NewUsecase
	NewKnowledgeUsecaseFromRepo        = knowledge.NewUsecaseFromRepo
	NewKnowledgeVaultFiler             = knowledge.NewVaultFiler
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
	KnowledgeHashContent               = knowledge.HashContent
)

// ProvideKnowledgeUsecase 是生产 Wire provider：在 NewUsecaseFromRepo 之上
// 按接口断言自动装配可选能力——P2-4 双轨关联（LinkRepo/EntityRepo）与
// P3 资源管理器（DocumentPathReader/ResolvedLinkReader）。repo 未实现对应
// 接口时保持未接线降级语义（关联 no-op / 关联查询为空 / 树查询显式报错）。
// filer 为共享 VaultFiler 实例（G1-B1 树目录 FS 扫描 + G1-B2 树内写文件）：
// 与 vault 同步链同一实例，自写标记统一登记防 watcher 回环。
// lg 为域日志器（SP1-H 回填等 best-effort 副作用的失败 Warn 出口）。
func ProvideKnowledgeUsecase(repo KnowledgeRepo, filer *knowledge.VaultFiler, blockIndex knowledge.BlockIndexRepo, lg loggateway.Logger) *KnowledgeUsecase {
	uc := knowledge.NewUsecaseFromRepo(repo)
	uc.SetVaultFiler(filer)
	uc.SetLogger(lg)
	// SP1-C 块级双链索引：BlockIndexRepo 与 ResolveIndex 由同一 data repo 实现，
	// 经类型断言接线；未提供时 RebuildBlockIndex 降级 no-op。
	if blockIndex != nil {
		var idx knowledge.ResolveIndex
		if ri, ok := blockIndex.(knowledge.ResolveIndex); ok {
			idx = ri
		}
		uc.SetBlockIndexRepos(blockIndex, idx)
		// SP1-E：反链落库兜底端口由同一块索引 repo 实现（断言失败保持未接线降级）。
		if blr, ok := blockIndex.(knowledge.BlockLinkReader); ok {
			uc.SetBacklinkRepos(blr, nil)
		}
		// SP1-G：晋升端口由同一块索引 repo 实现（reader + lineage writer 双断言）。
		if pr, ok := blockIndex.(knowledge.PromoteBlockReader); ok {
			if pw, ok2 := blockIndex.(knowledge.PromoteLineageWriter); ok2 {
				uc.SetPromoteRepos(pr, pw)
			}
		}
	}
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
	// G4-B8：库级图谱关联读取（repo 已实现 CollectionLinkReader 时接线；
	// 未接线时 ListCollectionGraph 降级为仅节点无边）。
	if graphLinks, gok := repo.(knowledge.CollectionLinkReader); gok {
		uc.SetGraphRepo(graphLinks)
	}
	// SP1-E：源文档名解析（DocNameReader）补接进反链端口（SetBacklinkRepos
	// 可能已由 blockIndex 断言接线兜底 reader，此处仅覆盖 names）。
	if names, nok := repo.(knowledge.DocNameReader); nok {
		uc.SetBacklinkNames(names)
	}
	return uc
}
