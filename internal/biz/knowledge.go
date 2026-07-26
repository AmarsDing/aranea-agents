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
