package eval

import (
	"context"
	"errors"
	"fmt"
)

// Searcher abstracts the retrieval system under evaluation. Implementations
// must return hits ordered by descending relevance.
type Searcher interface {
	Search(ctx context.Context, query string, topK int) ([]Hit, error)
}

// Hit is one ranked retrieval result.
type Hit struct {
	ChunkID string  `json:"chunk_id"`
	DocID   string  `json:"doc_id"`
	Score   float64 `json:"score"`
}

// Runner evaluates an EvalSet against a Searcher.
type Runner struct {
	searcher Searcher
}

// NewRunner builds a Runner around the given Searcher.
func NewRunner(s Searcher) *Runner {
	return &Runner{searcher: s}
}

// CaseMetrics holds the per-case metric scores for one basis (chunk or doc).
type CaseMetrics struct {
	RecallAtK    float64 `json:"recall_at_k"`
	MRR          float64 `json:"mrr"`
	Hit          float64 `json:"hit"`
	FirstHitRank int     `json:"first_hit_rank"` // 1-based; 0 means no hit
}

// CaseResult is the per-case evaluation detail. Err carries the search error
// when the case could not be retrieved; scores stay zero (or 1.0 under the
// vacuous-success convention) and the run continues with the next case.
type CaseResult struct {
	CaseID         string      `json:"case_id"`
	Query          string      `json:"query"`
	RankedChunkIDs []string    `json:"ranked_chunk_ids,omitempty"`
	RankedDocIDs   []string    `json:"ranked_doc_ids,omitempty"`
	Chunk          CaseMetrics `json:"chunk"`
	Doc            CaseMetrics `json:"doc"`
	Err            string      `json:"err,omitempty"`
}

// Aggregates holds macro-averaged metrics over every case of a run.
type Aggregates struct {
	CaseCount int     `json:"case_count"`
	RecallAtK float64 `json:"recall_at_k"`
	MRR       float64 `json:"mrr"`
	HitRate   float64 `json:"hit_rate"`
}

// RunReport is the outcome of one Runner.Run: per-case detail plus
// chunk-level and doc-level aggregates.
type RunReport struct {
	SetName  string       `json:"set_name"`
	TopK     int          `json:"top_k"`
	Cases    []CaseResult `json:"cases"`
	ChunkAgg Aggregates   `json:"chunk_agg"`
	DocAgg   Aggregates   `json:"doc_agg"`
}

// Run searches every case of set at the given topK and scores results on two
// bases: chunk-id hits and doc-id hits (doc ids deduplicated in rank order).
// Per-case search errors are recorded in CaseResult.Err without aborting the
// run; context cancellation aborts and returns the partial report.
func (r *Runner) Run(ctx context.Context, set EvalSet, topK int) (RunReport, error) {
	if r == nil || r.searcher == nil {
		return RunReport{}, errors.New("eval: runner requires a Searcher")
	}
	if topK <= 0 {
		return RunReport{}, fmt.Errorf("eval: topK must be positive, got %d", topK)
	}
	report := RunReport{
		SetName: set.Name,
		TopK:    topK,
		Cases:   make([]CaseResult, 0, len(set.Cases)),
	}
	for _, gc := range set.Cases {
		if err := ctx.Err(); err != nil {
			return report, fmt.Errorf("eval: run aborted after %d of %d cases: %w", len(report.Cases), len(set.Cases), err)
		}
		report.Cases = append(report.Cases, r.evalCase(ctx, gc, topK))
	}
	report.ChunkAgg = aggregate(report.Cases, func(c CaseResult) CaseMetrics { return c.Chunk })
	report.DocAgg = aggregate(report.Cases, func(c CaseResult) CaseMetrics { return c.Doc })
	return report, nil
}

func (r *Runner) evalCase(ctx context.Context, gc GoldenCase, topK int) CaseResult {
	res := CaseResult{CaseID: gc.ID, Query: gc.Query}
	hits, err := r.searcher.Search(ctx, gc.Query, topK)
	if err != nil {
		res.Err = err.Error()
	}
	res.RankedChunkIDs, res.RankedDocIDs = rankLists(hits)
	res.Chunk = caseMetrics(res.RankedChunkIDs, gc.ExpectedChunkIDs, topK)
	res.Doc = caseMetrics(res.RankedDocIDs, gc.ExpectedDocIDs, topK)
	return res
}

// rankLists projects hits into chunk-id and doc-id rank lists; doc ids are
// deduplicated so one document's many chunks cannot crowd out the window.
func rankLists(hits []Hit) (chunks, docs []string) {
	seenDoc := make(map[string]struct{}, len(hits))
	for _, h := range hits {
		if h.ChunkID != "" {
			chunks = append(chunks, h.ChunkID)
		}
		if h.DocID != "" {
			if _, ok := seenDoc[h.DocID]; !ok {
				seenDoc[h.DocID] = struct{}{}
				docs = append(docs, h.DocID)
			}
		}
	}
	return chunks, docs
}

func caseMetrics(ranked, expected []string, k int) CaseMetrics {
	return CaseMetrics{
		RecallAtK:    RecallAtK(ranked, expected, k),
		MRR:          MRR(ranked, expected),
		Hit:          HitAtK(ranked, expected, k),
		FirstHitRank: FirstHitRank(ranked, expected),
	}
}

func aggregate(cases []CaseResult, pick func(CaseResult) CaseMetrics) Aggregates {
	agg := Aggregates{CaseCount: len(cases)}
	if len(cases) == 0 {
		return agg
	}
	for _, c := range cases {
		m := pick(c)
		agg.RecallAtK += m.RecallAtK
		agg.MRR += m.MRR
		agg.HitRate += m.Hit
	}
	denom := float64(len(cases))
	agg.RecallAtK /= denom
	agg.MRR /= denom
	agg.HitRate /= denom
	return agg
}
