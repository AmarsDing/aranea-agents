package biz

import (
	"context"
	"errors"
	"strings"
	"time"
)

// AgentMemory carries one stored memory atom (relational fields + optional embedding).
type AgentMemory struct {
	ID        int64
	AgentID   string
	UserID    string // optional scope under agent; empty = default namespace
	Content   string
	Embedding []float32 // present on insert / when Search returns vectors (optional per implementation)
	CreatedAt time.Time
}

// MemoryRepo abstracts durable agent vector memory backed by Postgres pgvector.
type MemoryRepo interface {
	Insert(ctx context.Context, m *AgentMemory) error
	FindSimilar(ctx context.Context, agentID string, query []float32, topK int) ([]*AgentMemory, error)
	// Optional scoping alongside agent (default "").
	FindSimilarWithUser(ctx context.Context, agentID, userID string, query []float32, topK int) ([]*AgentMemory, error)
	// UpsertFactVector replaces the pgvector row keyed by fact_id (read index for memory_facts).
	UpsertFactVector(ctx context.Context, agentID, userID, factID, statement string, embedding []float32) error
}

// EmbeddingService turns text into a dense vector aligned with data.postgres.vector_dim.
type EmbeddingService interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// ErrMemoryUnavailable when postgres vector backing is disabled or unreachable.
var ErrMemoryUnavailable = errors.New("memory: postgres vector store not available")

// MemoryUsecase coordinates embedding + vector repository.
type MemoryUsecase struct {
	repo     MemoryRepo
	embedder EmbeddingService
}

// NewMemoryUsecase wires repository and embedding service.
func NewMemoryUsecase(repo MemoryRepo, embedder EmbeddingService) *MemoryUsecase {
	return &MemoryUsecase{repo: repo, embedder: embedder}
}

// Remember embeds text and persists it for agent (single default partition; no UserID scope).
// Prefer UpsertFactRow on SessionAdminStore + SyncFactIndex; Remember remains for legacy callers.
func (uc *MemoryUsecase) Remember(ctx context.Context, agentID, text string) error {
	return uc.RememberWithUser(ctx, agentID, "", text)
}

// RememberWithUser scopes storage under agent + user partition (userID "" matches default PG rows).
func (uc *MemoryUsecase) RememberWithUser(ctx context.Context, agentID, userID, text string) error {
	if uc == nil || uc.embedder == nil || uc.repo == nil {
		return ErrMemoryUnavailable
	}
	vec, err := uc.embedder.Embed(ctx, text)
	if err != nil {
		return err
	}
	return uc.repo.Insert(ctx, &AgentMemory{
		AgentID:   agentID,
		UserID:    userID,
		Content:   text,
		Embedding: vec,
	})
}

// Embed implements biz.EmbeddingService for L2 recall query embedding at the store layer.
func (uc *MemoryUsecase) Embed(ctx context.Context, text string) ([]float32, error) {
	return uc.EmbedText(ctx, text)
}

// EmbedText returns an embedding vector for arbitrary recall/index text.
func (uc *MemoryUsecase) EmbedText(ctx context.Context, text string) ([]float32, error) {
	if uc == nil || uc.embedder == nil {
		return nil, ErrMemoryUnavailable
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	return uc.embedder.Embed(ctx, text)
}

// Recall embeds query text and returns topK hits for agent (default user scope "").
func (uc *MemoryUsecase) Recall(ctx context.Context, agentID, query string, topK int) ([]*AgentMemory, error) {
	return uc.RecallWithUser(ctx, agentID, "", query, topK)
}

// RecallWithUser embeddings query vector and retrieves nearest memories for agent+user partition.
func (uc *MemoryUsecase) RecallWithUser(ctx context.Context, agentID, userID, query string, topK int) ([]*AgentMemory, error) {
	if uc == nil || uc.embedder == nil || uc.repo == nil {
		return nil, ErrMemoryUnavailable
	}
	vec, err := uc.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	return uc.repo.FindSimilarWithUser(ctx, agentID, userID, vec, topK)
}
