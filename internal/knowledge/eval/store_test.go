package eval

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadEvalSetSample(t *testing.T) {
	set, err := LoadEvalSet(filepath.Join("testdata", "golden-sample.json"))
	if err != nil {
		t.Fatalf("LoadEvalSet error: %v", err)
	}
	if set.Name != "golden-sample" {
		t.Fatalf("set name = %q, want golden-sample", set.Name)
	}
	if len(set.Cases) != 3 {
		t.Fatalf("case count = %d, want 3", len(set.Cases))
	}
	first := set.Cases[0]
	if first.ID == "" || first.Query == "" || len(first.ExpectedChunkIDs) == 0 || len(first.ExpectedDocIDs) == 0 {
		t.Fatalf("first case underpopulated: %+v", first)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	set := EvalSet{
		Name: "round-trip",
		Cases: []GoldenCase{
			{ID: "a", Query: "巡检周期", ExpectedChunkIDs: []string{"c1", "c2"}, ExpectedDocIDs: []string{"d1"}, Notes: "n"},
			{ID: "b", Query: "TACACS+ 端口", ExpectedDocIDs: []string{"d2"}},
			{ID: "c", Query: "无期望结果"},
		},
	}
	path := filepath.Join(t.TempDir(), "set.json")
	if err := SaveEvalSet(path, set); err != nil {
		t.Fatalf("SaveEvalSet error: %v", err)
	}
	got, err := LoadEvalSet(path)
	if err != nil {
		t.Fatalf("LoadEvalSet error: %v", err)
	}
	if !reflect.DeepEqual(got, set) {
		t.Fatalf("round trip mismatch:\ngot  %+v\nwant %+v", got, set)
	}
}

func TestLoadEvalSetErrors(t *testing.T) {
	if _, err := LoadEvalSet(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected error for missing file")
	}

	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEvalSet(bad); err == nil {
		t.Fatal("expected error for malformed json")
	}

	dup := filepath.Join(dir, "dup.json")
	if err := os.WriteFile(dup, []byte(`{"name":"x","cases":[{"id":"a","query":"q1"},{"id":"a","query":"q2"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEvalSet(dup); err == nil {
		t.Fatal("expected error for duplicate case id")
	}

	// 字段拼写错误（expected_chunk_id 少个 s）必须报出来——宽松解析会把期望
	// 静默吞掉，用例退化成恒满分的免疫壳。
	typo := filepath.Join(dir, "typo.json")
	if err := os.WriteFile(typo, []byte(`{"name":"x","cases":[{"id":"a","query":"q","expected_chunk_id":["c1"]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEvalSet(typo); err == nil {
		t.Fatal("expected error for unknown json field")
	}
}

func TestSaveEvalSetValidates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "set.json")
	if err := SaveEvalSet(path, EvalSet{}); err == nil {
		t.Fatal("expected error for empty name")
	}
	bad := EvalSet{Name: "x", Cases: []GoldenCase{{ID: "", Query: "q"}}}
	if err := SaveEvalSet(path, bad); err == nil {
		t.Fatal("expected error for empty case id")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatal("invalid set must not be written")
	}
}

// writeTestFile 写小文本 fixture（fixture_search_test.go 复用）。
func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestRunReportSaveLoadRoundTrip(t *testing.T) {
	report := RunReport{
		SetName: "rt",
		TopK:    8,
		Cases: []CaseResult{
			{
				CaseID:         "a",
				Query:          "巡检周期",
				RankedChunkIDs: []string{"c1", "c2"},
				RankedDocIDs:   []string{"d1"},
				Chunk:          CaseMetrics{RecallAtK: 1, MRR: 1, Hit: 1, FirstHitRank: 1},
				Doc:            CaseMetrics{RecallAtK: 1, MRR: 1, Hit: 1, FirstHitRank: 1},
			},
			{CaseID: "b", Query: "TACACS+ 端口", Err: "boom"},
		},
		ChunkAgg: Aggregates{CaseCount: 2, RecallAtK: 0.5, MRR: 0.5, HitRate: 0.5},
		DocAgg:   Aggregates{CaseCount: 2, RecallAtK: 0.5, MRR: 0.5, HitRate: 0.5},
	}
	path := filepath.Join(t.TempDir(), "report.json")
	if err := SaveRunReport(path, report); err != nil {
		t.Fatalf("SaveRunReport error: %v", err)
	}
	got, err := LoadRunReport(path)
	if err != nil {
		t.Fatalf("LoadRunReport error: %v", err)
	}
	if !reflect.DeepEqual(got, report) {
		t.Fatalf("round trip mismatch:\ngot  %+v\nwant %+v", got, report)
	}
}

func TestLoadRunReportErrors(t *testing.T) {
	if _, err := LoadRunReport(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected error for missing file")
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRunReport(bad); err == nil {
		t.Fatal("expected error for malformed json")
	}
}
