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
	// G-05-asof 时态金色探针（决策 e）：独立第二 EvalSet/语料/快照——主集与旧
	// 快照字节级不动；GateStale 键 = SetName+TopK，独立集名即独立基线档案，
	// 互不遮蔽。CI 第二 goldeval 腿另待协调窗（本轮红线：不触 workflows）。
	goldTemporalCorpusPath   = "gold-corpus-temporal.json"
	goldTemporalSetPath      = "gold-set-temporal.json"
	goldTemporalSnapshotPath = "gold-snapshot-temporal.json"
	// G-34 MOC 金色探针（第三档）：独立第三 EvalSet/语料/快照——门禁实际锚定
	// 范围（终审 F13 勘正）：全局主题问题经 seed 字面锚定 moc/* 页 +
	// explicit 成员边一跳扩散的【接线】（MOC 不出链/边物化断即 recall 跌幅）。
	// 摘要文本身不进门禁断言：fixtureSearcher 无词法打分（seed 语义即
	// 「最坏假设只命中锚点」），LLM 摘要偏离/写空不影响本档指标——摘要质量
	// 由 biz 契约钉（moc_summary/moc_summarizer 套件）担保，非金标职责。
	goldMOCCorpusPath   = "gold-corpus-moc.json"
	goldMOCSetPath      = "gold-set-moc.json"
	goldMOCSnapshotPath = "gold-snapshot-moc.json"
	// goldTopK 与 gold_recall_test.go 的 Expand TopK 同口径（种子∪邻居全集容纳得下）。
	goldTopK = 8
)

func TestGoldSnapshotRegression(t *testing.T) {
	runGoldSnapshot(t, goldCorpusPath, goldSetPath, goldSnapshotPath)
}

// TestGoldSnapshotTemporalRegression 时态探针门禁：四象限用例经同一判定链
// （ValidateGoldExpectations → Runner → CheckGoldGate），警线含 —— 时态接线
// （SetTemporalActiveLinks 被摘）或谓词回归都会被 recall 跌幅/MRR 跌幅捕获。
func TestGoldSnapshotTemporalRegression(t *testing.T) {
	runGoldSnapshot(t, goldTemporalCorpusPath, goldTemporalSetPath, goldTemporalSnapshotPath)
}

// TestGoldSnapshotMOCRegression G-34 第三档门禁：MOC 摘要锚定 + explicit 成员
// 扩散经同一判定链（ValidateGoldExpectations → Runner → CheckGoldGate）。
func TestGoldSnapshotMOCRegression(t *testing.T) {
	runGoldSnapshot(t, goldMOCCorpusPath, goldMOCSetPath, goldMOCSnapshotPath)
}

func runGoldSnapshot(t *testing.T, corpusPath, setPath, snapshotPath string) {
	t.Helper()
	corpus, err := LoadFixtureCorpus(filepath.Join("testdata", corpusPath))
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	set, err := LoadEvalSet(filepath.Join("testdata", setPath))
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

	snapPath := filepath.Join("testdata", snapshotPath)
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
