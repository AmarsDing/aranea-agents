package agent

import (
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTokenEstimateFormula(t *testing.T) {

	isCJK := func(r rune) bool {
		return (r >= 0x4E00 && r <= 0x9FFF) ||
			(r >= 0x3040 && r <= 0x309F) ||
			(r >= 0x30A0 && r <= 0x30FF) ||
			(r >= 0xAC00 && r <= 0xD7AF)
	}
	estimate := func(combined string) int {
		var cjkCount, otherCount int
		for _, r := range combined {
			if isCJK(r) {
				cjkCount++
			} else {
				otherCount++
			}
		}
		est := cjkCount + otherCount/4
		if est == 0 && combined != "" {
			est = 1
		}
		return est
	}

	tests := []struct {
		name     string
		combined string
		want     int
	}{
		{name: "empty string", combined: "", want: 0},
		{name: "hello (5 non-CJK runes)", combined: "hello", want: 1},
		{name: "Chinese 4 runes (CJK)", combined: "你好世界", want: 4},
		{name: "single ASCII char", combined: "a", want: 1},
		{name: "two ASCII chars", combined: "ab", want: 1},
		{name: "three ASCII chars", combined: "abc", want: 1},
		{name: "four ASCII chars", combined: "abcd", want: 1},
		{name: "eight ASCII chars", combined: "abcdefgh", want: 2},
		{name: "mixed CJK+ASCII: 你好ab", combined: "你好ab", want: 2},
		{name: "single CJK char", combined: "中", want: 1},
		{name: "CJK + lots of ASCII", combined: "你好hello world", want: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimate(tt.combined)
			if got != tt.want {
				t.Errorf("estimate(%q) = %d, want %d", tt.combined, got, tt.want)
			}
		})
	}
}

type stubL1CueReader struct {
	sessionRows [][]byte
	latest      []byte
	latestAgent string
	exclude     string
}

func (s *stubL1CueReader) ListL1TaskRows(context.Context, string, string, string, string) ([][]byte, error) {
	return s.sessionRows, nil
}
func (s *stubL1CueReader) ListL1FieldRows(context.Context, string, bool, ...string) ([][]byte, error) {
	return nil, nil
}
func (s *stubL1CueReader) GetL1TaskRow(context.Context, string, string) ([]byte, error) {
	return nil, nil
}
func (s *stubL1CueReader) GetL1FieldRow(context.Context, string, string) ([]byte, error) {
	return nil, nil
}
func (s *stubL1CueReader) LatestL1TaskBoard(_ context.Context, agentID, exclude string) ([]byte, error) {
	s.latestAgent = agentID
	s.exclude = exclude
	return s.latest, nil
}

func TestL1MemoryCue_CrossSessionBoardWhenCurrentEmpty(t *testing.T) {
	row, _ := json.Marshal(map[string]any{
		"id":            "t-old",
		"task_title":    "机房巡检",
		"task_goal":     "完成机柜清点",
		"status":        "paused",
		"metadata_json": `{"task_board":{"status":"取证完成","done":["拍照"],"next":"执行清除","blockers":["缺钥匙"]}}`,
	})
	stub := &stubL1CueReader{latest: row}
	got := L1MemoryCue(context.Background(), stub, biz.Agent{ID: "ag-1"},
		biz.MemoryRuntimePolicy{InjectL1: true}, "sess-new", loggateway.NewNoop())
	if got == nil {
		t.Fatal("expected cross-session cue")
	}
	if stub.latestAgent != "ag-1" || stub.exclude != "sess-new" {
		t.Fatalf("lookup agent=%q exclude=%q", stub.latestAgent, stub.exclude)
	}
	if !strings.Contains(got.Cue, "Task status (previous session)") {
		t.Fatalf("missing cross-session header: %s", got.Cue)
	}
	if !strings.Contains(got.Cue, "机房巡检") || !strings.Contains(got.Cue, "执行清除") {
		t.Fatalf("missing board fields: %s", got.Cue)
	}
}

func TestL1MemoryCue_NoCrossSessionWithoutBoarder(t *testing.T) {
	reader := sessionOnlyReader{}
	got := L1MemoryCue(context.Background(), reader, biz.Agent{ID: "ag-1"},
		biz.MemoryRuntimePolicy{InjectL1: true}, "sess-new", loggateway.NewNoop())
	if got != nil {
		t.Fatalf("expected nil without L1LatestTaskBoardReader, got %+v", got)
	}
}

type sessionOnlyReader struct{}

func (sessionOnlyReader) ListL1TaskRows(context.Context, string, string, string, string) ([][]byte, error) {
	return nil, nil
}
func (sessionOnlyReader) ListL1FieldRows(context.Context, string, bool, ...string) ([][]byte, error) {
	return nil, nil
}
func (sessionOnlyReader) GetL1TaskRow(context.Context, string, string) ([]byte, error) {
	return nil, nil
}
func (sessionOnlyReader) GetL1FieldRow(context.Context, string, string) ([]byte, error) {
	return nil, nil
}
