package eval

import (
	"fmt"
	"strconv"
	"strings"
)

// RegressionThreshold is the chunk-level Recall@K drop that marks a case as
// regressed when diffing two run reports.
const RegressionThreshold = 0.02

// RenderMarkdown renders a RunReport as a Markdown document: a per-case
// table (case, recall, MRR, first-hit rank at chunk basis) followed by
// aggregate rows for both chunk and doc bases.
func RenderMarkdown(r RunReport) string {
	var b strings.Builder
	name := r.SetName
	if name == "" {
		name = "(unnamed set)"
	}
	fmt.Fprintf(&b, "# Eval Report: %s\n\n", name)
	fmt.Fprintf(&b, "- topK: %d\n- cases: %d\n\n", r.TopK, len(r.Cases))

	b.WriteString("| Case | Recall | MRR | First Hit | Note |\n")
	b.WriteString("| --- | ---: | ---: | ---: | --- |\n")
	for _, c := range r.Cases {
		firstHit := "-"
		if c.Chunk.FirstHitRank > 0 {
			firstHit = strconv.Itoa(c.Chunk.FirstHitRank)
		}
		fmt.Fprintf(&b, "| %s | %.3f | %.3f | %s | %s |\n",
			escapePipe(c.CaseID), c.Chunk.RecallAtK, c.Chunk.MRR, firstHit, escapePipe(c.Err))
	}

	b.WriteString("\n## Aggregates\n\n")
	b.WriteString("| Basis | Cases | Recall | MRR | Hit Rate |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: |\n")
	writeAggRow(&b, "chunk", r.ChunkAgg)
	writeAggRow(&b, "doc", r.DocAgg)
	return b.String()
}

func writeAggRow(b *strings.Builder, basis string, agg Aggregates) {
	fmt.Fprintf(b, "| %s | %d | %.3f | %.3f | %.3f |\n",
		basis, agg.CaseCount, agg.RecallAtK, agg.MRR, agg.HitRate)
}

func escapePipe(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

// MetricDelta is the per-metric movement from base to cand.
type MetricDelta struct {
	Base  float64 `json:"base"`
	Cand  float64 `json:"cand"`
	Delta float64 `json:"delta"` // Cand - Base
}

// AggDiff holds the per-metric deltas of one basis (chunk or doc).
type AggDiff struct {
	RecallAtK MetricDelta `json:"recall_at_k"`
	MRR       MetricDelta `json:"mrr"`
	HitRate   MetricDelta `json:"hit_rate"`
}

// CaseDiff is the per-case metric movement between two runs, at both bases.
// Regression is set when chunk-level RecallAtK drops by more than
// RegressionThreshold.
type CaseDiff struct {
	CaseID      string  `json:"case_id"`
	ChunkRecall float64 `json:"chunk_recall_delta"`
	ChunkMRR    float64 `json:"chunk_mrr_delta"`
	DocRecall   float64 `json:"doc_recall_delta"`
	DocMRR      float64 `json:"doc_mrr_delta"`
	Regression  bool    `json:"regression"`
}

// DiffReport compares a candidate run against a baseline run of the same
// EvalSet. Cases present on only one side are listed as added/removed.
type DiffReport struct {
	SetName         string     `json:"set_name"`
	TopK            int        `json:"top_k"`
	BaseCases       int        `json:"base_cases"`
	CandCases       int        `json:"cand_cases"`
	Chunk           AggDiff    `json:"chunk"`
	Doc             AggDiff    `json:"doc"`
	Cases           []CaseDiff `json:"cases,omitempty"`
	AddedCaseIDs    []string   `json:"added_case_ids,omitempty"`
	RemovedCaseIDs  []string   `json:"removed_case_ids,omitempty"`
	RegressionCount int        `json:"regression_count"`
}

// DiffReports diffs cand against base. Cases are matched by CaseID; per-case
// deltas follow base order, and aggregate deltas are computed per metric for
// both chunk and doc bases.
func DiffReports(base, cand RunReport) DiffReport {
	report := DiffReport{
		SetName:   candSetName(base, cand),
		TopK:      cand.TopK,
		BaseCases: len(base.Cases),
		CandCases: len(cand.Cases),
		Chunk:     diffAgg(base.ChunkAgg, cand.ChunkAgg),
		Doc:       diffAgg(base.DocAgg, cand.DocAgg),
	}
	candByID := make(map[string]CaseResult, len(cand.Cases))
	for _, c := range cand.Cases {
		candByID[c.CaseID] = c
	}
	baseByID := make(map[string]CaseResult, len(base.Cases))
	for _, b := range base.Cases {
		baseByID[b.CaseID] = b
	}
	for _, bc := range base.Cases {
		cc, ok := candByID[bc.CaseID]
		if !ok {
			report.RemovedCaseIDs = append(report.RemovedCaseIDs, bc.CaseID)
			continue
		}
		d := CaseDiff{
			CaseID:      bc.CaseID,
			ChunkRecall: cc.Chunk.RecallAtK - bc.Chunk.RecallAtK,
			ChunkMRR:    cc.Chunk.MRR - bc.Chunk.MRR,
			DocRecall:   cc.Doc.RecallAtK - bc.Doc.RecallAtK,
			DocMRR:      cc.Doc.MRR - bc.Doc.MRR,
		}
		if d.ChunkRecall < -(RegressionThreshold + 1e-9) {
			d.Regression = true
			report.RegressionCount++
		}
		report.Cases = append(report.Cases, d)
	}
	for _, cc := range cand.Cases {
		if _, ok := baseByID[cc.CaseID]; !ok {
			report.AddedCaseIDs = append(report.AddedCaseIDs, cc.CaseID)
		}
	}
	return report
}

func diffAgg(base, cand Aggregates) AggDiff {
	return AggDiff{
		RecallAtK: MetricDelta{Base: base.RecallAtK, Cand: cand.RecallAtK, Delta: cand.RecallAtK - base.RecallAtK},
		MRR:       MetricDelta{Base: base.MRR, Cand: cand.MRR, Delta: cand.MRR - base.MRR},
		HitRate:   MetricDelta{Base: base.HitRate, Cand: cand.HitRate, Delta: cand.HitRate - base.HitRate},
	}
}

func candSetName(base, cand RunReport) string {
	if cand.SetName != "" {
		return cand.SetName
	}
	return base.SetName
}

// RenderDiffMarkdown renders a DiffReport as Markdown for CI job summaries and
// review comments: aggregate deltas at both bases, added/removed case drift,
// and the per-case table with regressions flagged.
func RenderDiffMarkdown(d DiffReport) string {
	var b strings.Builder
	name := d.SetName
	if name == "" {
		name = "(unnamed set)"
	}
	fmt.Fprintf(&b, "# Eval Diff: %s\n\n", name)
	fmt.Fprintf(&b, "- topK: %d\n- baseline cases: %d, candidate cases: %d\n", d.TopK, d.BaseCases, d.CandCases)
	fmt.Fprintf(&b, "- regressions: %d (chunk recall drop > %.3f)\n", d.RegressionCount, RegressionThreshold)
	if len(d.AddedCaseIDs) > 0 {
		fmt.Fprintf(&b, "- added cases: %s\n", strings.Join(d.AddedCaseIDs, ", "))
	}
	if len(d.RemovedCaseIDs) > 0 {
		fmt.Fprintf(&b, "- removed cases: %s\n", strings.Join(d.RemovedCaseIDs, ", "))
	}

	b.WriteString("\n## Aggregates\n\n")
	b.WriteString("| Basis | Metric | Base | Cand | Δ |\n")
	b.WriteString("| --- | --- | ---: | ---: | ---: |\n")
	writeDiffRows(&b, "chunk", d.Chunk)
	writeDiffRows(&b, "doc", d.Doc)

	if len(d.Cases) > 0 {
		b.WriteString("\n## Cases\n\n")
		b.WriteString("| Case | Chunk Recall Δ | Chunk MRR Δ | Doc Recall Δ | Doc MRR Δ | Regression |\n")
		b.WriteString("| --- | ---: | ---: | ---: | ---: | --- |\n")
		for _, c := range d.Cases {
			flag := ""
			if c.Regression {
				flag = "⚠️ regression"
			}
			fmt.Fprintf(&b, "| %s | %+.3f | %+.3f | %+.3f | %+.3f | %s |\n",
				escapePipe(c.CaseID), c.ChunkRecall, c.ChunkMRR, c.DocRecall, c.DocMRR, flag)
		}
	}
	return b.String()
}

func writeDiffRows(b *strings.Builder, basis string, agg AggDiff) {
	for _, row := range []struct {
		name string
		d    MetricDelta
	}{
		{"recall", agg.RecallAtK},
		{"mrr", agg.MRR},
		{"hit rate", agg.HitRate},
	} {
		fmt.Fprintf(b, "| %s | %s | %.3f | %.3f | %+.3f |\n",
			basis, row.name, row.d.Base, row.d.Cand, row.d.Delta)
	}
}
