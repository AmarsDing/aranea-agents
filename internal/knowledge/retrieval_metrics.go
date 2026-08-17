package knowledge

import (
	"math"
	"sort"

	"aranea-agents/internal/biz"
)

// RetrievalGoldCase is one deterministic information-retrieval expectation.
// RelevanceGrades is optional; absent grades default to binary relevance 1.
type RetrievalGoldCase struct {
	ID              string
	RelevantDocIDs  []string
	RelevanceGrades map[string]float64
	Abstain         bool
}

// RetrievalMetricReport contains macro-averaged document-level IR metrics.
type RetrievalMetricReport struct {
	TotalCases         int
	RankedCases        int
	AbstentionCases    int
	RecallAt           map[int]float64
	HitRateAt          map[int]float64
	NDCGAt             map[int]float64
	MRR                float64
	AbstentionAccuracy float64
}

// EvaluateRetrievalMetrics evaluates ranked chunks after deduplicating by
// document. Queries with relevant documents contribute to ranking metrics;
// Abstain cases contribute only to AbstentionAccuracy.
func EvaluateRetrievalMetrics(
	cases []RetrievalGoldCase,
	results map[string][]biz.KnowledgeChunk,
	ks []int,
) RetrievalMetricReport {
	ks = normalizedMetricKs(ks)
	report := RetrievalMetricReport{
		TotalCases: len(cases),
		RecallAt:   make(map[int]float64, len(ks)),
		HitRateAt:  make(map[int]float64, len(ks)),
		NDCGAt:     make(map[int]float64, len(ks)),
	}
	for _, gold := range cases {
		ranked := rankedDocumentIDs(results[gold.ID])
		if gold.Abstain {
			report.AbstentionCases++
			if len(ranked) == 0 {
				report.AbstentionAccuracy++
			}
			continue
		}
		relevant := relevanceByDocument(gold)
		if len(relevant) == 0 {
			continue
		}
		report.RankedCases++
		report.MRR += reciprocalRank(ranked, relevant)
		for _, k := range ks {
			report.RecallAt[k] += recallAt(ranked, relevant, k)
			report.HitRateAt[k] += hitRateAt(ranked, relevant, k)
			report.NDCGAt[k] += ndcgAt(ranked, relevant, k)
		}
	}
	if report.RankedCases > 0 {
		denom := float64(report.RankedCases)
		report.MRR /= denom
		for _, k := range ks {
			report.RecallAt[k] /= denom
			report.HitRateAt[k] /= denom
			report.NDCGAt[k] /= denom
		}
	}
	if report.AbstentionCases > 0 {
		report.AbstentionAccuracy /= float64(report.AbstentionCases)
	}
	return report
}

func normalizedMetricKs(ks []int) []int {
	seen := make(map[int]struct{}, len(ks))
	out := make([]int, 0, len(ks))
	for _, k := range ks {
		if k <= 0 {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func rankedDocumentIDs(chunks []biz.KnowledgeChunk) []string {
	seen := make(map[string]struct{}, len(chunks))
	out := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk.DocID == "" {
			continue
		}
		if _, ok := seen[chunk.DocID]; ok {
			continue
		}
		seen[chunk.DocID] = struct{}{}
		out = append(out, chunk.DocID)
	}
	return out
}

func relevanceByDocument(gold RetrievalGoldCase) map[string]float64 {
	out := make(map[string]float64, len(gold.RelevantDocIDs))
	for _, id := range gold.RelevantDocIDs {
		if id == "" {
			continue
		}
		grade := gold.RelevanceGrades[id]
		if grade <= 0 {
			grade = 1
		}
		out[id] = grade
	}
	return out
}

func recallAt(ranked []string, relevant map[string]float64, k int) float64 {
	hits := 0
	for i := 0; i < len(ranked) && i < k; i++ {
		if _, ok := relevant[ranked[i]]; ok {
			hits++
		}
	}
	return float64(hits) / float64(len(relevant))
}

func hitRateAt(ranked []string, relevant map[string]float64, k int) float64 {
	for i := 0; i < len(ranked) && i < k; i++ {
		if _, ok := relevant[ranked[i]]; ok {
			return 1
		}
	}
	return 0
}

func reciprocalRank(ranked []string, relevant map[string]float64) float64 {
	for i, id := range ranked {
		if _, ok := relevant[id]; ok {
			return 1 / float64(i+1)
		}
	}
	return 0
}

func ndcgAt(ranked []string, relevant map[string]float64, k int) float64 {
	dcg := 0.0
	for i := 0; i < len(ranked) && i < k; i++ {
		if grade := relevant[ranked[i]]; grade > 0 {
			dcg += (math.Pow(2, grade) - 1) / math.Log2(float64(i+2))
		}
	}
	ideal := make([]float64, 0, len(relevant))
	for _, grade := range relevant {
		ideal = append(ideal, grade)
	}
	sort.Slice(ideal, func(i, j int) bool { return ideal[i] > ideal[j] })
	idcg := 0.0
	for i := 0; i < len(ideal) && i < k; i++ {
		idcg += (math.Pow(2, ideal[i]) - 1) / math.Log2(float64(i+2))
	}
	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}
