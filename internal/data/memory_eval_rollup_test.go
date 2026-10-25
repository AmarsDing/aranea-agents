package data

import (
	"testing"

	"aranea-agents/internal/biz"
)

// TestEvalAggregateCapItems covers AMC P3-a 读侧置顶：aggregate 模式下
// rollup 行（profile digest / slot-history）必须先于更高分的普通事实与
// episode；剩余条目走常规 MMR（剩余预算）；topK 截断在拼接之后生效，
// rollup 占据预算时尾部 MMR 不再补位。
func TestEvalAggregateCapItems(t *testing.T) {
	t.Parallel()
	mk := func(id, content string, score float64) evalCollectedItem {
		return evalCollectedItem{item: biz.EvalMemoryItem{ID: id, Content: content, Score: score}, day: "2025-05-07"}
	}
	profile := mk("p1", "[profile-digest] 身份: 我叫张三 | 偏好: 我喜欢蓝色", 0.5)
	slotHist := mk("s1", "[slot-history: favorite:颜色] 我最喜欢的颜色是蓝色(2025-03-01, 已失效) → 我最喜欢的颜色是红色(2025-03-05)", 0.4)
	series := mk("d1", "[series-digest: 步数] count=4 last=5600步 mean=4025.0步 min=2800步 max=5600步 firstValue=3200步 firstDay=2025-05-01 lastDay=2025-05-07 trend=上升", 0.3)
	factHi := mk("f1", "user: 我叫张三", 0.9)
	factLo := mk("f2", "user: 我喜欢蓝色", 0.2)

	// rollup（profile+slot）先行且内部按分数降序；series digest 不是 P3-a
	// 置顶对象（那是 trend 模式的分区），落回 MMR 尾部。
	out := evalAggregateCapItems([]evalCollectedItem{factHi, series, slotHist, profile, factLo}, 10)
	if len(out) != 5 {
		t.Fatalf("len=%d, want 5", len(out))
	}
	if out[0].item.ID != "p1" || out[1].item.ID != "s1" {
		t.Fatalf("rollup rows must lead in score order: %v", out)
	}
	for _, env := range out[2:] {
		if env.item.ID == "p1" || env.item.ID == "s1" {
			t.Fatalf("rollup row leaked into MMR tail: %v", out)
		}
	}

	// topK 截断：rollup 先占预算，剩余交给 MMR；预算不够放 rollup 时尾部
	// MMR 完全不补位。
	out = evalAggregateCapItems([]evalCollectedItem{factHi, slotHist, profile, factLo}, 3)
	if len(out) != 3 || out[0].item.ID != "p1" || out[1].item.ID != "s1" {
		t.Fatalf("topK=3 must keep rollup first: %v", out)
	}
	out = evalAggregateCapItems([]evalCollectedItem{factHi, slotHist, profile, factLo}, 2)
	if len(out) != 2 || out[0].item.ID != "p1" || out[1].item.ID != "s1" {
		t.Fatalf("topK=2 must be rollup-only: %v", out)
	}

	// 空输入与无 rollup 输入都不出错；无 rollup 时行为退化为纯 MMR。
	if got := evalAggregateCapItems(nil, 5); len(got) != 0 {
		t.Fatalf("nil input must stay empty: %v", got)
	}
	out = evalAggregateCapItems([]evalCollectedItem{factLo, factHi}, 10)
	if len(out) != 2 || out[0].item.ID != "f1" {
		t.Fatalf("no-rollup input must keep MMR score order: %v", out)
	}
}
