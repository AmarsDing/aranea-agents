package eval

import (
	"flag"
	"path/filepath"
	"strings"
	"testing"
)

// G-13 金标快照门禁：评测集 + 快照 JSON 入仓，CI 的 `go test ./...` 自动做
// before/after diff，chunk recall / chunk MRR 跌幅越过各自阈值即红。
//
// 门禁判定单一事实源在 gate.go（CheckGoldGate / ValidateGoldBaseline /
// ValidateGoldExpectations），cmd/goldeval 与这里共用，禁止另抄一份分支。
//
// 刷新快照（语料/评测集变更或有意的指标改进随代码同 PR 提交）：
//
//	go test ./internal/knowledge/eval/ -run TestGoldSnapshot -gold.update
var goldUpdate = flag.Bool("gold.update", false, "用当前运行结果重写 testdata 金标快照（必须与代码变更同 PR 提交）")

const (
	goldCorpusPath   = "gold-corpus.json"
	goldSetPath      = "gold-set.json"
	goldSnapshotPath = "gold-snapshot.json"
	// goldTopK 与 gold_recall_test.go 的 Expand TopK 同口径（种子∪邻居全集容纳得下）。
	goldTopK = 8
)

func TestGoldSnapshotRegression(t *testing.T) {
	corpus, err := LoadFixtureCorpus(filepath.Join("testdata", goldCorpusPath))
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	set, err := LoadEvalSet(filepath.Join("testdata", goldSetPath))
	if err != nil {
		t.Fatalf("load set: %v", err)
	}
	// 空期望用例 = 恒满分的免疫用例，弱化期望还会把指标“改好”——门禁路径必须拒绝。
	if err := ValidateGoldExpectations(set); err != nil {
		t.Fatalf("gold set: %v", err)
	}
	searcher, err := NewFixtureSearcher(corpus)
	if err != nil {
		t.Fatalf("build fixture searcher: %v", err)
	}
	report, err := NewRunner(searcher).Run(t.Context(), set, goldTopK)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	snapPath := filepath.Join("testdata", goldSnapshotPath)
	if *goldUpdate {
		// 带错或降级的快照一旦入库，其后所有 diff 都对着弱化参照量——拒写。
		if err := ValidateGoldBaseline(report); err != nil {
			t.Fatalf("not rewriting snapshot: %v", err)
		}
		if err := SaveRunReport(snapPath, report); err != nil {
			t.Fatalf("rewrite snapshot: %v", err)
		}
		t.Logf("gold snapshot rewritten: %d cases, chunk recall %.3f, doc recall %.3f",
			len(report.Cases), report.ChunkAgg.RecallAtK, report.DocAgg.RecallAtK)
		return
	}

	base, err := LoadRunReport(snapPath)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if violations := CheckGoldGate(base, report); len(violations) > 0 {
		var lines []string
		for _, v := range violations {
			lines = append(lines, "  "+v.String())
		}
		t.Fatalf("gold snapshot gate violations (%d):\n%s\n"+
			"若为有意变更，用 -gold.update 刷新快照并随代码同 PR 提交",
			len(violations), strings.Join(lines, "\n"))
	}
	t.Logf("gold gate ok: %d cases, chunk recall %.3f (base %.3f), doc recall %.3f (base %.3f)",
		len(report.Cases),
		report.ChunkAgg.RecallAtK, base.ChunkAgg.RecallAtK,
		report.DocAgg.RecallAtK, base.DocAgg.RecallAtK)
}
