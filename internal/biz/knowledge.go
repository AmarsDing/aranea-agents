package biz

import "aranea-agents/internal/biz/knowledge"

type (
	KnowledgeCollection    = knowledge.Collection
	KnowledgeDocument      = knowledge.Document
	KnowledgeChunk         = knowledge.Chunk
	KnowledgeSearchQuery   = knowledge.SearchQuery
	KnowledgeRepo          = knowledge.Repo
	KnowledgeUsecase       = knowledge.Usecase
	KnowledgeEmbedSetting  = knowledge.EmbedSetting
)

var (
	NewKnowledgeUsecase     = knowledge.NewUsecase
	ErrKnowledgeUnavailable = knowledge.ErrUnavailable
	KnowledgeEmbedConfigured  = knowledge.EmbedConfigured
	ApplyKnowledgeEmbedPatch  = knowledge.ApplyEmbedPatch
)
