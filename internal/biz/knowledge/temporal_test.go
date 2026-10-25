package knowledge

import (
	"testing"
	"time"
)

// ===== G-05-asof（ADR-KN-20261020-J）：TemporalSpan 双时态窗口 =====
//
// 语义矩阵（契约，四象限）：
//
//	T=0,  R=0  → current 快报（valid_to IS NULL 原文本，partial index 热路径）
//	T≠0,  R=0  → T-only：彼时有效、以当前已知信息
//	T=0,  R≠0  → R-only 知识快照：R 时已知且当时仍有效
//	T≠0,  R≠0  → 全双轴交：@>T ∧ @>R
//
// 保真边界：仅边图存在性/link_type/relation 保真；weight_f/confidence/context
// 返回当前值；co_activated 与 closed-on-arrival 空区间不回放（见 ADR-J 失真如四类）。

func TestTemporalSpan_Any(t *testing.T) {
	var zero TemporalSpan
	if zero.Any() {
		t.Fatal("zero span must be off (current fast path)")
	}
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	if got := (TemporalSpan{ValidAt: at}); !got.Any() {
		t.Fatal("T-only span must be on")
	}
	if got := (TemporalSpan{RecordedAt: at}); !got.Any() {
		t.Fatal("R-only span must be on")
	}
	if got := (TemporalSpan{ValidAt: at, RecordedAt: at}); !got.Any() {
		t.Fatal("dual-axis span must be on")
	}
}

func TestTemporalSpan_SemanticsMatrix(t *testing.T) {
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	// T 轴关闭=R-only；R 轴关闭=T-only；双开=交。谓词选择由 data 层按此零值组合分支。
	cases := []struct {
		name string
		span TemporalSpan
		wantT, wantR bool
	}{
		{"current", TemporalSpan{}, false, false},
		{"t-only", TemporalSpan{ValidAt: at}, true, false},
		{"r-only", TemporalSpan{RecordedAt: at}, false, true},
		{"dual", TemporalSpan{ValidAt: at, RecordedAt: at}, true, true},
	}
	for _, tc := range cases {
		if got := tc.span.ValidOn(); got != tc.wantT {
			t.Errorf("%s: ValidOn=%v want %v", tc.name, got, tc.wantT)
		}
		if got := tc.span.RecordedOn(); got != tc.wantR {
			t.Errorf("%s: RecordedOn=%v want %v", tc.name, got, tc.wantR)
		}
	}
}

func TestSearchQuery_CarriesTemporalSpan(t *testing.T) {
	// 载体钉（PathPrefix :1004 先例）：SearchQuery 值拷贝全链路自动携带——
	// AdaptiveRouter/GraphExpander 签名零改动的前提是字段存在于该 struct。
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	q := SearchQuery{CollectionID: "c1", Query: "q", Temporal: TemporalSpan{ValidAt: at}}
	cp := q // 值拷贝
	if !cp.Temporal.ValidOn() || !cp.Temporal.ValidAt.Equal(at) {
		t.Fatalf("value copy must carry span: %+v", cp.Temporal)
	}
}
