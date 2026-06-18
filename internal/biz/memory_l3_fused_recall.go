package biz

import (
	"context"
	"sort"
	"strings"

	"aranea-agents/pkg/loggateway"
)

const (
	// decayFuseBase is the retained fraction of Total when Decay=0 (no fusion).
	// decayFuseWeight is the additional fraction restored as Decay approaches 1.0.
	// Final Total = Total * (decayFuseBase + decayFuseWeight*Decay).
	// When Decay=1.0 (no forgetting), Total is unchanged (0.7+0.3=1.0).
	// When Decay→0+ (fully forgotten), Total retains 70% of its value.
	decayFuseBase   = 0.7
	decayFuseWeight = 0.3
)

// RecallScoreBreakdown is the component-wise recall ranking for one hit.
type RecallScoreBreakdown struct {
	Keyword      float64
	Vector       float64
	Importance   float64
	Recency      float64
	QualityScore float64
	SessionBoost float64
	CrossEncoder float64
	Decay        float64 // Ebbinghaus reachability R_t in (0,1]; 0 means not computed
	Total        float64
}

// RecallHit is one typed recall result (progressive replacement for raw JSON rows).
type RecallHit struct {
	Layer     string
	ID        string
	Title     string
	Summary   string
	Statement string
	Raw       []byte
	Scores    RecallScoreBreakdown
}

// L3RecallQuery parameters for semantic fact retrieval.
type L3RecallQuery struct {
	ScopeType string
	ScopeID   string
	UserID    string
	Query     string
	Limit     int32
	MinScore  float64
}

// SessionL3RecallStore is the persistence port for L3 recall (implemented by sessionmemory.Store).
type SessionL3RecallStore interface {
	RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error)
}

// SessionL3ScoredRecallStore returns scored L3 hits for fused multi-scope recall.
type SessionL3ScoredRecallStore interface {
	RecallL3Hits(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32) ([]RecallHit, error)
}

// L3FusedRecallQuery parameters for cross-scope fused L3 recall.
type L3FusedRecallQuery struct {
	Runtime           MemoryRuntimeContext
	Scopes            []string
	Query             string
	Limit             int32
	MinScoreQuery     float64
	MinScorePassive   float64
}

// MemoryL3Recaller performs query-aware L3 fact recall with optional fused multi-scope ranking.
type MemoryL3Recaller interface {
	RecallFacts(ctx context.Context, q L3RecallQuery) ([][]byte, error)
	RecallFactsFused(ctx context.Context, q L3FusedRecallQuery) ([][]byte, error)
}

// MemoryL3RecallUsecase embeds optional query vectors and delegates rerank to the store.
type MemoryL3RecallUsecase struct {
	store    SessionL3RecallStore
	scored   SessionL3ScoredRecallStore
	embedder EmbeddingService
	lg       loggateway.Logger
}

func NewMemoryL3RecallUsecase(store SessionL3RecallStore, scored SessionL3ScoredRecallStore, embedder EmbeddingService, lg loggateway.Logger) *MemoryL3RecallUsecase {
	if store == nil {
		return nil
	}
	return &MemoryL3RecallUsecase{store: store, scored: scored, embedder: embedder, lg: lg}
}

func (uc *MemoryL3RecallUsecase) RecallFacts(ctx context.Context, q L3RecallQuery) ([][]byte, error) {
	if uc == nil || uc.store == nil {
		return nil, nil
	}
	query := strings.TrimSpace(q.Query)
	minScore := q.MinScore
	if query == "" {
		minScore = 0
	}
	var qvec []float32
	if query != "" && uc.embedder != nil {
		if vec, err := uc.embedder.Embed(ctx, query); err == nil {
			qvec = vec
		} else {
			uc.lg.Warn("L3 recall embed failed, degrading to non-vector search",
				loggateway.StepID("memory.l3_embed_fail"),
				loggateway.Err(err))
		}
	}
	lim := q.Limit
	if lim <= 0 {
		lim = 12
	}
	return uc.store.RecallL3Facts(ctx, strings.TrimSpace(q.ScopeType), strings.TrimSpace(q.ScopeID), strings.TrimSpace(q.UserID), query, qvec, lim, minScore)
}

func (uc *MemoryL3RecallUsecase) RecallFactsFused(ctx context.Context, q L3FusedRecallQuery) ([][]byte, error) {
	if uc == nil {
		return nil, nil
	}
	if uc.scored == nil {
		return uc.RecallFacts(ctx, L3RecallQuery{
			ScopeType: "agent",
			ScopeID:   strings.TrimSpace(q.Runtime.AgentID),
			UserID:    strings.TrimSpace(q.Runtime.UserID),
			Query:     q.Query,
			Limit:     q.Limit,
			MinScore:  EffectiveL3MinScore(MemoryRuntimePolicy{L3MinScoreQuery: q.MinScoreQuery, L3MinScorePassive: q.MinScorePassive}, q.Query),
		})
	}
	query := strings.TrimSpace(q.Query)
	minScore := q.MinScorePassive
	if query != "" {
		minScore = q.MinScoreQuery
	}
	var qvec []float32
	if query != "" && uc.embedder != nil {
		if vec, err := uc.embedder.Embed(ctx, query); err == nil {
			qvec = vec
		} else {
			uc.lg.Warn("L3 fused recall embed failed, degrading to non-vector search",
				loggateway.StepID("memory.l3_fused_embed_fail"),
				loggateway.Err(err))
		}
	}
	lim := int(q.Limit)
	if lim <= 0 {
		lim = 12
	}
	if lim > maxL3RecallLimit {
		lim = maxL3RecallLimit
	}
	perScope := int32(lim * 2)
	if perScope < 12 {
		perScope = 12
	}

	var merged []RecallHit
	for _, target := range L3ScopeTargets(q.Runtime, q.Scopes) {
		hits, err := uc.scored.RecallL3Hits(ctx, target.ScopeType, target.ScopeID, strings.TrimSpace(q.Runtime.UserID), query, qvec, perScope)
		if err != nil {
			return nil, err
		}
		merged = append(merged, hits...)
	}
	// Fuse Ebbinghaus decay factor into Total before ranking. When Decay > 0,
	// Total is scaled by (0.7 + 0.3*Decay); when Decay == 0 (not computed),
	// Total is unaffected. This down-weights low-reachability (forgotten)
	// memories so they rank lower in fused recall.
	for i := range merged {
		if merged[i].Scores.Decay > 0 {
			merged[i].Scores.Total *= decayFuseBase + decayFuseWeight*merged[i].Scores.Decay
		}
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Scores.Total > merged[j].Scores.Total
	})

	seen := make(map[string]struct{})
	out := make([][]byte, 0, lim)
	for _, hit := range merged {
		if minScore > 0 && hit.Scores.Total < minScore {
			continue
		}
		key := strings.TrimSpace(hit.ID)
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(hit.Statement))
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if len(hit.Raw) > 0 {
			out = append(out, append([]byte(nil), hit.Raw...))
		}
		if len(out) >= lim {
			break
		}
	}
	return out, nil
}
