package data

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

// TestEvalTrendCapItems covers AMC P3-c 读侧置顶：trend 模式下数值序列
// digest 行必须先于更高分的普通事实；两个分区内部各自保持分数降序；
// topK 截断在拼接之后生效。
func TestEvalTrendCapItems(t *testing.T) {
	t.Parallel()
	mk := func(id, content string, score float64) evalCollectedItem {
		return evalCollectedItem{item: biz.EvalMemoryItem{ID: id, Content: content, Score: score}, day: "2025-05-07"}
	}
	digestLo := mk("d1", "[series-digest: 步数] count=4 last=5600步 mean=4025.0步 min=2800步 max=5600步 firstValue=3200步 firstDay=2025-05-01 lastDay=2025-05-07 trend=上升约+15.6%/day", 0.05)
	factHi := mk("f1", "user: 今天步数5600步", 0.9)
	factMid := mk("f2", "user: 今天步数4500步", 0.6)
	digestHi := mk("d2", "[series-digest: 体重] count=2 last=75公斤 mean=75.5公斤 min=75公斤 max=76公斤 firstValue=76公斤 firstDay=2025-05-01 lastDay=2025-05-05 trend=下降约-3.3%/day", 0.4)
	// AMC P3 评审 C4：count=1 的单观测 digest 复述原子本身，不配置顶——
	// 即使分数最高也只能按普通事实位参与排序。
	digestSingle := mk("d9", "[series-digest: 背包] count=1 last=5公斤 mean=5.0公斤 min=5公斤 max=5公斤 firstValue=5公斤 firstDay=2025-05-02 lastDay=2025-05-02", 0.95)

	out := evalTrendCapItems([]evalCollectedItem{factHi, digestLo, factMid, digestHi, digestSingle}, 10)
	wantOrder := []string{"d2", "d1", "d9", "f1", "f2"} // pinworthy digest 分区先行、内部降序；count=1 按分数入普通分区
	if len(out) != len(wantOrder) {
		t.Fatalf("len=%d, want %d", len(out), len(wantOrder))
	}
	for i, id := range wantOrder {
		if out[i].item.ID != id {
			t.Fatalf("order[%d]=%s, want %s (%v)", i, out[i].item.ID, id, out)
		}
	}
	// topK 截断：pinworthy digest 分区优先占位。
	out = evalTrendCapItems([]evalCollectedItem{factHi, digestLo, factMid, digestHi, digestSingle}, 2)
	if len(out) != 2 || out[0].item.ID != "d2" || out[1].item.ID != "d1" {
		t.Fatalf("topK truncation wrong: %v", out)
	}
	// 空输入与无 digest 输入都不出错、不胀行。
	if got := evalTrendCapItems(nil, 5); len(got) != 0 {
		t.Fatalf("nil input must stay empty: %v", got)
	}
	out = evalTrendCapItems([]evalCollectedItem{factMid, factHi}, 10)
	if len(out) != 2 || out[0].item.ID != "f1" {
		t.Fatalf("no-digest input must keep score order: %v", out)
	}
}

// TestFilterEvalItemsByFloor_TrendPin covers AMC P3 楼层豁免（trend + 本轮
// P3-a 扩展 aggregate）：只有对应模式开启时，带正分的 rollup 行才能低于
// floor 存活；零分 hop 噪声与非 rollup 行一律照常过滤。
func TestFilterEvalItemsByFloor_TrendPin(t *testing.T) {
	t.Parallel()
	mk := func(id, content string, score float64) evalCollectedItem {
		return evalCollectedItem{item: biz.EvalMemoryItem{ID: id, Content: content, Score: score}, day: "2025-05-07"}
	}
	digest := mk("d1", "[series-digest: 步数] count=3 last=5600步 mean=4025.0步 min=2800步 max=5600步 firstValue=3200步 firstDay=2025-05-01 lastDay=2025-05-07", 0.02)
	zeroDigest := mk("d0", "[series-digest: 体重] count=1 last=75公斤 mean=75.0公斤 min=75公斤 max=75公斤 firstValue=75公斤 firstDay=2025-05-05 lastDay=2025-05-05", 0)
	// AMC P3 评审 C4：count=1 单观测 digest 即使带正分也不获 trend 豁免。
	singleDigest := mk("d9", "[series-digest: 背包] count=1 last=5公斤 mean=5.0公斤 min=5公斤 max=5公斤 firstValue=5公斤 firstDay=2025-05-02 lastDay=2025-05-02", 0.02)
	profile := mk("p1", "[profile-digest] 身份: 我叫张三 | 偏好: 我喜欢蓝色", 0.02)
	slotHist := mk("s1", "[slot-history: favorite:颜色] 我最喜欢的颜色是蓝色(2025-03-01, 已失效) → 我最喜欢的颜色是红色(2025-03-05)", 0.02)
	top := mk("f1", "user: 今天步数5600步", 10.0)
	low := mk("f2", "user: 我喜欢蓝色", 0.02)
	items := []evalCollectedItem{top, digest, zeroDigest, singleDigest, profile, slotHist, low}

	got := filterEvalItemsByFloor(items, 0.12, biz.EvalQueryAnalysis{})
	if len(got) != 1 || got[0].item.ID != "f1" {
		t.Fatalf("mode-less floor must drop sub-floor rollup rows too: %v", got)
	}
	got = filterEvalItemsByFloor(items, 0.12, biz.EvalQueryAnalysis{Trend: true})
	ids := map[string]bool{}
	for _, env := range got {
		ids[env.item.ID] = true
	}
	if !ids["f1"] || !ids["d1"] {
		t.Fatalf("trend floor must keep scored count≥2 series digest: %v", ids)
	}
	if ids["d9"] {
		t.Fatalf("trend floor must NOT exempt count=1 digest (评审 C4): %v", ids)
	}
	if ids["p1"] || ids["s1"] {
		t.Fatalf("trend mode must NOT exempt aggregate rollup rows: %v", ids)
	}
	if ids["d0"] {
		t.Fatalf("zero-score digest (hop noise) must never pass: %v", ids)
	}
	if ids["f2"] {
		t.Fatalf("sub-floor non-rollup fact must stay filtered: %v", ids)
	}
	got = filterEvalItemsByFloor(items, 0.12, biz.EvalQueryAnalysis{Aggregate: true})
	ids = map[string]bool{}
	for _, env := range got {
		ids[env.item.ID] = true
	}
	if !ids["f1"] || !ids["p1"] || !ids["s1"] {
		t.Fatalf("aggregate floor must keep scored profile digest + slot history: %v", ids)
	}
	if ids["d1"] || ids["d0"] {
		t.Fatalf("aggregate mode must NOT exempt series digests: %v", ids)
	}
	if ids["f2"] {
		t.Fatalf("sub-floor non-rollup fact must stay filtered: %v", ids)
	}
	if !strings.Contains(got[0].item.Content, "步数") && got[0].item.ID != "f1" {
		t.Fatalf("floor output order corrupted: %v", got)
	}
}
