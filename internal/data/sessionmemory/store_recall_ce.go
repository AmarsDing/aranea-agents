package sessionmemory

import (
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
)

const ceRerankWeight = 0.15

func applyCrossEncoderRerank(query string, scores []float64, passages []string) {
	if strings.TrimSpace(query) == "" || len(scores) == 0 || len(scores) != len(passages) {
		return
	}
	ce := biz.NewCrossEncoderReranker()
	for i := range scores {
		ceScore := ce.Score(query, passages[i])
		scores[i] = (1-ceRerankWeight)*scores[i] + ceRerankWeight*ceScore
	}
}

func applyCrossEncoderRerankToScored(query string, scored []scoredEpisode, passages []string, apply func(i int, ceScore, total float64)) {
	if strings.TrimSpace(query) == "" || len(scored) == 0 || len(scored) != len(passages) {
		return
	}
	ce := biz.NewCrossEncoderReranker()
	for i := range scored {
		ceScore := ce.Score(query, passages[i])
		total := (1-ceRerankWeight)*scored[i].score + ceRerankWeight*ceScore
		apply(i, ceScore, total)
	}
}

func applyCrossEncoderRerankToFactScored(query string, scored []scoredFact, passages []string) {
	if strings.TrimSpace(query) == "" || len(scored) == 0 || len(scored) != len(passages) {
		return
	}
	ce := biz.NewCrossEncoderReranker()
	for i := range scored {
		ceScore := ce.Score(query, passages[i])
		total := (1-ceRerankWeight)*scored[i].score + ceRerankWeight*ceScore
		scored[i].breakdown.CrossEncoder = ceScore
		scored[i].breakdown.Total = total
		scored[i].score = total
	}
}

type recallScoreBreakdown struct {
	Keyword      float64 `json:"keyword"`
	Vector       float64 `json:"vector"`
	Importance   float64 `json:"importance"`
	Recency      float64 `json:"recency"`
	QualityScore float64 `json:"quality_score"`
	CrossEncoder float64 `json:"cross_encoder"`
	Total        float64 `json:"total"`
}

// RecallDebugRow is one scored recall candidate for admin debug RPC.
type RecallDebugRow struct {
	Layer     string               `json:"layer"`
	ID        string               `json:"id"`
	Title     string               `json:"title,omitempty"`
	Summary   string               `json:"summary,omitempty"`
	Statement string               `json:"statement,omitempty"`
	Scores    recallScoreBreakdown `json:"scores"`
	Raw       json.RawMessage      `json:"raw,omitempty"`
}

func episodePassage(raw []byte) string {
	var row map[string]any
	if json.Unmarshal(raw, &row) != nil {
		return ""
	}
	title, _ := row["title"].(string)
	summary, _ := row["outcome_summary"].(string)
	return strings.TrimSpace(title + " " + summary)
}

func factPassage(stmt, details string) string {
	return strings.TrimSpace(stmt + " " + details)
}
