package data

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/vector"
	"aranea-agents/pkg/loggateway"
)

// evalC3Facts returns every fact row (including invalidated) for the user.
func evalC3AllFacts(t *testing.T, r *l3FactRepo, userID string) []map[string]any {
	t.Helper()
	rows, err := r.ListFactRowsForUserAll(context.Background(), "user", userID, userID, "", 200, 0)
	if err != nil {
		t.Fatalf("ListFactRowsForUserAll: %v", err)
	}
	out := make([]map[string]any, 0, len(rows))
	for _, raw := range rows {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal fact row: %v", err)
		}
		out = append(out, m)
	}
	return out
}

func evalC3FindFact(t *testing.T, facts []map[string]any, contains string) map[string]any {
	t.Helper()
	for _, f := range facts {
		if stmt, _ := f["statement"].(string); strings.Contains(stmt, contains) {
			return f
		}
	}
	t.Fatalf("no fact containing %q in %v", contains, facts)
	return nil
}

// TestEvalStore_AtomizesMultiSentenceChinese_PG covers A1/Q4: one Chinese
// message with three sentences yields three fact rows (stored still counts
// the message), carries message_index/atom_index metadata, and a full
// request replay must not add rows (fingerprint upsert idempotency).
func TestEvalStore_AtomizesMultiSentenceChinese_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	msgs := []biz.EvalMessage{
		{Role: "user", Content: "我喜欢咖啡。我住在杭州。我的工号是9527。", MessageID: "m1"},
	}
	stored, err := store.AddMessages(ctx, "u-atom", "s-atom", msgs)
	if err != nil {
		t.Fatal(err)
	}
	if stored != 1 {
		t.Fatalf("stored=%d, want 1 (messages, not atoms)", stored)
	}
	facts := evalC3AllFacts(t, r, "u-atom")
	if len(facts) != 3 {
		t.Fatalf("facts=%d, want 3 atoms", len(facts))
	}
	seenAtomIdx := map[float64]bool{}
	for _, f := range facts {
		var meta map[string]any
		if err := json.Unmarshal([]byte(func() string { s, _ := f["metadata_json"].(string); return s }()), &meta); err != nil {
			t.Fatalf("metadata_json unmarshal: %v", err)
		}
		if v, ok := meta["message_index"].(float64); !ok || v != 0 {
			t.Fatalf("message_index=%v, want 0", meta["message_index"])
		}
		idx, ok := meta["atom_index"].(float64)
		if !ok {
			t.Fatalf("atom_index missing: %v", meta)
		}
		seenAtomIdx[idx] = true
	}
	if len(seenAtomIdx) != 3 {
		t.Fatalf("distinct atom_index values=%v, want 3", seenAtomIdx)
	}
	// Replay the identical request: stored counts again, rows must not double.
	stored, err = store.AddMessages(ctx, "u-atom", "s-atom", msgs)
	if err != nil {
		t.Fatal(err)
	}
	if stored != 1 {
		t.Fatalf("replay stored=%d, want 1", stored)
	}
	if got := len(evalC3AllFacts(t, r, "u-atom")); got != 3 {
		t.Fatalf("replay facts=%d, want 3 (fingerprint upsert idempotency)", got)
	}
}

// TestEvalStore_RelativeDateSupersede_PG covers A2/A4 (改期矛盾): a new
// statement with a relative date and an update cue invalidates the old
// same-slot value (valid_until set, no recall leakage) and records the audit
// trail in metadata.previous_values plus event_dates metadata.
func TestEvalStore_RelativeDateSupersede_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "u-rd", "s-rd", []biz.EvalMessage{
		{Role: "user", Content: "我最喜欢的旅行城市是杭州", MessageID: "m1"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddMessages(ctx, "u-rd", "s-rd", []biz.EvalMessage{
		{Role: "user", Content: "我上个月把最喜欢的旅行城市改成上海了", MessageID: "m2"},
	}); err != nil {
		t.Fatal(err)
	}

	facts := evalC3AllFacts(t, r, "u-rd")
	if len(facts) != 2 {
		t.Fatalf("facts=%d, want 2", len(facts))
	}
	oldFact := evalC3FindFact(t, facts, "我最喜欢的旅行城市是杭州")
	if vu, _ := oldFact["valid_until"].(string); vu == "" {
		t.Fatalf("old fact valid_until empty, want set: %v", oldFact)
	}
	newFact := evalC3FindFact(t, facts, "改成上海")
	if vu, _ := newFact["valid_until"].(string); vu != "" {
		t.Fatalf("new fact must stay active: %v", newFact)
	}

	// previous_values audit trail on the new fact.
	newFactMeta, _ := newFact["metadata_json"].(string)
	var meta map[string]any
	if err := json.Unmarshal([]byte(newFactMeta), &meta); err != nil {
		t.Fatal(err)
	}
	prevs, _ := meta["previous_values"].([]any)
	if len(prevs) == 0 {
		t.Fatalf("previous_values missing on new fact: %v", meta)
	}
	found := false
	for _, p := range prevs {
		pm, _ := p.(map[string]any)
		if s, _ := pm["statement"].(string); strings.Contains(s, "我最喜欢的旅行城市是杭州") {
			if vu, _ := pm["valid_until"].(string); vu == "" {
				t.Fatalf("previous_values entry missing valid_until: %v", pm)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("previous_values lacks superseded statement: %v", prevs)
	}

	// event_dates resolves 上个月 → first day of the previous month.
	nowUTC := time.Now().UTC()
	y, mo, _ := nowUTC.Date()
	wantDay := time.Date(y, mo-1, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	dates, _ := meta["event_dates"].([]any)
	if len(dates) != 1 || dates[0] != wantDay {
		t.Fatalf("event_dates=%v, want [%s]", meta["event_dates"], wantDay)
	}

	// Recall never surfaces the superseded value.
	items, err := store.SearchMemories(ctx, "u-rd", "用户最喜欢的旅行城市", 10)
	if err != nil {
		t.Fatal(err)
	}
	joined := evalContents(items)
	if !strings.Contains(joined, "上海") {
		t.Fatalf("missing updated city: %s", joined)
	}
	if strings.Contains(joined, "杭州") {
		t.Fatalf("stale city leaked into recall: %s", joined)
	}
}

// TestEvalStore_ChronologicalOrder_PG covers A3/Q6 chronological mode: a
// 顺序/先后 query orders evidence by the resolved evidence day ascending
// (event_dates win over valid_from) and the Q9 evidence prefix is attached.
func TestEvalStore_ChronologicalOrder_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "u-chron", "s-chron", []biz.EvalMessage{
		{Role: "user", Content: "昨天我搬家到了上海", MessageID: "m1", Timestamp: "2025-03-05T08:00:00Z"},
		{Role: "user", Content: "今天我入职了新公司", MessageID: "m2", Timestamp: "2025-06-08T08:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := store.SearchMemories(ctx, "u-chron", "我搬家和入职的先后顺序", 5)
	if err != nil {
		t.Fatal(err)
	}
	idxMove, idxJob := -1, -1
	for i, it := range items {
		if strings.Contains(it.Content, "搬家") && idxMove < 0 {
			idxMove = i
		}
		if strings.Contains(it.Content, "入职") && idxJob < 0 {
			idxJob = i
		}
	}
	if idxMove < 0 || idxJob < 0 {
		t.Fatalf("missing evidence, move=%d job=%d items=%s", idxMove, idxJob, evalContents(items))
	}
	if idxMove > idxJob {
		t.Fatalf("chronological order wrong: move@%d after job@%d: %s",
			idxMove, idxJob, evalContents(items))
	}
	// m1 anchor 2025-03-05, 昨天 → evidence day 2025-03-04, prefix before role.
	if first := items[idxMove].Content; !strings.HasPrefix(first, "[2025-03-04 · sess-s-chron] ") {
		t.Fatalf("evidence prefix wrong: %q", first)
	}
}

// TestEvalStore_DateRangeFilter_PG covers A3/Q6 range mode: an absolute date
// pins the evidence day so out-of-range facts are filtered out even though
// they would otherwise match.
func TestEvalStore_DateRangeFilter_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "u-range", "s-range", []biz.EvalMessage{
		{Role: "user", Content: "我那天体检了", MessageID: "m1", Timestamp: "2025-03-04T08:00:00Z"},
		{Role: "user", Content: "我那天出差去了深圳", MessageID: "m2", Timestamp: "2025-06-08T08:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := store.SearchMemories(ctx, "u-range", "2025年3月4日我做了什么", 10)
	if err != nil {
		t.Fatal(err)
	}
	joined := evalContents(items)
	if !strings.Contains(joined, "体检") {
		t.Fatalf("in-range fact missing: %s", joined)
	}
	if strings.Contains(joined, "出差") {
		t.Fatalf("out-of-range fact leaked: %s", joined)
	}
}

// TestEvalStore_AggregateAppendsEpisodes_PG covers A5/Q8a: aggregate mode
// attaches the user's recent L2 episodes as evidence (deduped by id).
func TestEvalStore_AggregateAppendsEpisodes_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "u-agg", "s-agg-a", []biz.EvalMessage{
		{Role: "user", Content: "我最喜欢的水果是芒果", MessageID: "m1"},
		{Role: "assistant", Content: "记住了你的芒果偏好", MessageID: "m2"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddMessages(ctx, "u-agg", "s-agg-b", []biz.EvalMessage{
		{Role: "user", Content: "我最喜欢的饮料是红茶", MessageID: "m3"},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := store.SearchMemories(ctx, "u-agg", "总结一下我最喜欢的水果和饮料", 10)
	if err != nil {
		t.Fatal(err)
	}
	l2Count := 0
	for _, it := range items {
		if strings.HasPrefix(it.ID, "l2:") {
			l2Count++
		}
	}
	if l2Count != 2 {
		t.Fatalf("l2 episodes attached=%d, want 2 (one per session): %v", l2Count, evalContents(items))
	}
	joined := evalContents(items)
	if !strings.Contains(joined, "芒果") || !strings.Contains(joined, "红茶") {
		t.Fatalf("aggregate evidence incomplete: %s", joined)
	}
}

// TestEvalStore_AggregateEpisodeDedup drives appendAggregateEpisodes directly
// to prove id-dedup against already-seen episodes.
func TestEvalStore_AggregateEpisodeDedup(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop()).(*evalMemoryStore)
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "u-agg2", "s-agg2", []biz.EvalMessage{
		{Role: "user", Content: "我最喜欢的零食是薯片", MessageID: "m1"},
	}); err != nil {
		t.Fatal(err)
	}
	eps, err := store.episodes.ListEpisodeRowsForRecall(ctx, "u-agg2", "", 10)
	if err != nil || len(eps) == 0 {
		t.Fatalf("episodes=%d err=%v", len(eps), err)
	}
	var epRow map[string]any
	if err := json.Unmarshal(eps[0], &epRow); err != nil {
		t.Fatal(err)
	}
	epID, _ := epRow["id"].(string)
	seen := map[string]struct{}{"l2:" + epID: {}}
	var items []evalCollectedItem
	store.appendAggregateEpisodes(ctx, "u-agg2", "总结", time.Now().UTC(), seen, &items)
	for _, env := range items {
		if env.item.ID == "l2:"+epID {
			t.Fatalf("aggregate re-attached an already-seen episode")
		}
	}
	// Without the pre-seeded seen map the episode is attached.
	items = nil
	store.appendAggregateEpisodes(ctx, "u-agg2", "总结", time.Now().UTC(), map[string]struct{}{}, &items)
	if len(items) == 0 {
		t.Fatalf("aggregate attached no episodes")
	}
}

// TestEvalStore_ReplayDoesNotResurrectSuperseded_PG covers AMC C3 修复
// #0/#4（红线「被 supersede 的旧值永不回流」）：平台重试重放旧请求时——
//   (a) 已墓碑的旧行不得被指纹 upsert 起死（valid_until 保留）；
//   (b) 重放的旧语句不得经同槽通道再次失效事件时间更晚的新行（时序守卫）；
//   (c) 新行 metadata.previous_values 审计留痕不得被重放抹掉。
// 该测试在修复前于 (a)/(b) 均失败。
func TestEvalStore_ReplayDoesNotResurrectSuperseded_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	msgs1 := []biz.EvalMessage{
		{Role: "user", Content: "我最喜欢的旅行城市是杭州", MessageID: "m1", Timestamp: "2025-03-05T08:00:00Z"},
	}
	msgs2 := []biz.EvalMessage{
		{Role: "user", Content: "我上个月把最喜欢的旅行城市改成上海了", MessageID: "m2", Timestamp: "2025-03-06T08:00:00Z"},
	}
	if _, err := store.AddMessages(ctx, "u-rp", "s-rp", msgs1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddMessages(ctx, "u-rp", "s-rp", msgs2); err != nil {
		t.Fatal(err)
	}

	oldFact := evalC3FindFact(t, evalC3AllFacts(t, r, "u-rp"), "是杭州")
	if vu, _ := oldFact["valid_until"].(string); vu == "" {
		t.Fatalf("baseline: old fact must be superseded: %v", oldFact)
	}
	newFact := evalC3FindFact(t, evalC3AllFacts(t, r, "u-rp"), "改成上海")

	// (a)(b) 重放第一条（较旧）请求：旧行不得起死，新行不得被回失效。
	if _, err := store.AddMessages(ctx, "u-rp", "s-rp", msgs1); err != nil {
		t.Fatal(err)
	}
	facts := evalC3AllFacts(t, r, "u-rp")
	oldFact = evalC3FindFact(t, facts, "是杭州")
	if vu, _ := oldFact["valid_until"].(string); vu == "" {
		t.Fatalf("replay of older request resurrected the superseded fact: %v", oldFact)
	}
	newFact = evalC3FindFact(t, facts, "改成上海")
	if vu, _ := newFact["valid_until"].(string); vu != "" {
		t.Fatalf("replay of older request re-invalidated the newer fact (time guard broken): %v", newFact)
	}
	prevBefore, _ := newFact["metadata_json"].(string)
	if !strings.Contains(prevBefore, "previous_values") || !strings.Contains(prevBefore, "是杭州") {
		t.Fatalf("(c) prev audit missing before second replay: %s", prevBefore)
	}

	// (c) 重放第二条请求本身：previous_values 不得被 ON CONFLICT 抹掉。
	if _, err := store.AddMessages(ctx, "u-rp", "s-rp", msgs2); err != nil {
		t.Fatal(err)
	}
	newFact = evalC3FindFact(t, evalC3AllFacts(t, r, "u-rp"), "改成上海")
	metaStr, _ := newFact["metadata_json"].(string)
	if !strings.Contains(metaStr, "previous_values") || !strings.Contains(metaStr, "是杭州") {
		t.Fatalf("replay of the same request wiped previous_values audit: %s", metaStr)
	}

	// 事实通道（非 L2 转录）绝不回浮旧值。
	items, err := store.SearchMemories(ctx, "u-rp", "我最喜欢的旅行城市是哪里", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if strings.HasPrefix(it.ID, "l2:") {
			continue
		}
		if strings.Contains(it.Content, "是杭州") {
			t.Fatalf("superseded fact resurfaced in fact channel after replay: %q", it.Content)
		}
	}
}

// TestEvalStore_SameRequestSupersedesEarlierMessage_PG covers AMC C3 修复
// #1：同一请求内的 [旧值, 改口] 消息对必须生效 supersede——batchIDs 早退
// 只作用于向量通道（防同批 sibling 被近邻误判），不得挡住同槽时序通道。
// 修复前 m1 行保持 active（有效回退）。
func TestEvalStore_SameRequestSupersedesEarlierMessage_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "u-sbr", "s-sbr", []biz.EvalMessage{
		{Role: "user", Content: "我最喜欢的颜色是蓝色", MessageID: "m1"},
		{Role: "user", Content: "不对，我现在把最喜欢的颜色改成红色了", MessageID: "m2"},
	}); err != nil {
		t.Fatal(err)
	}
	facts := evalC3AllFacts(t, r, "u-sbr")
	if len(facts) != 2 {
		t.Fatalf("facts=%d, want 2", len(facts))
	}
	oldFact := evalC3FindFact(t, facts, "是蓝色")
	if vu, _ := oldFact["valid_until"].(string); vu == "" {
		t.Fatalf("in-request earlier fact must be superseded by the later correction: %v", oldFact)
	}
	newFact := evalC3FindFact(t, facts, "改成红色")
	if vu, _ := newFact["valid_until"].(string); vu != "" {
		t.Fatalf("correction fact must stay active: %v", newFact)
	}
	items, err := store.SearchMemories(ctx, "u-sbr", "我最喜欢的颜色", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if strings.HasPrefix(it.ID, "l2:") {
			continue
		}
		if strings.Contains(it.Content, "是蓝色") {
			t.Fatalf("stale in-request value leaked into recall: %q", it.Content)
		}
	}
}

// TestEvalStore_PerAtomEventDates_PG covers AMC C3 修复 #3：相对时间须按
// 原子解析——同一消息内第二枚原子的「前天」锚的消息时戳，不得复用第一枚
// 原子「今天」的 event_dates。
func TestEvalStore_PerAtomEventDates_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "u-pad", "s-pad", []biz.EvalMessage{
		{Role: "user", Content: "我今天去医院体检了。前天我开始吃药调理。", MessageID: "m1",
			Timestamp: "2025-05-07T08:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	facts := evalC3AllFacts(t, r, "u-pad")
	if len(facts) != 2 {
		t.Fatalf("facts=%d, want 2 atoms", len(facts))
	}
	wantDates := map[string]string{"体检": "2025-05-07", "吃药": "2025-05-05"}
	for kw, want := range wantDates {
		f := evalC3FindFact(t, facts, kw)
		metaStr, _ := f["metadata_json"].(string)
		var meta map[string]any
		if err := json.Unmarshal([]byte(metaStr), &meta); err != nil {
			t.Fatal(err)
		}
		dates, _ := meta["event_dates"].([]any)
		if len(dates) != 1 || dates[0] != want {
			t.Fatalf("atom %q event_dates=%v, want [%s]", kw, meta["event_dates"], want)
		}
	}
}

// TestHasEvalCorrectionCue_Tightened covers AMC C3 修复 #8：裸「改」收紧为
// 明确改口句式——改变/改天/改善 不再触发向量矛盾扫描。
func TestHasEvalCorrectionCue_Tightened(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stmt string
		want bool
	}{
		{"我把城市改成上海了", true},
		{"不对，我没去过北京", true},
		{"其实我记错了", true},
		{"不是杭州而是上海", true},
		{"Actually I have already moved", true},
		{"天气突然改变了", false},
		{"我们改天再约", false},
		{"最近伙食改善了", false},
		{"routine check passed", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := HasEvalCorrectionCue(tc.stmt); got != tc.want {
			t.Errorf("HasEvalCorrectionCue(%q) = %v, want %v", tc.stmt, got, tc.want)
		}
	}
}

// TestEvalStore_MaskEvidence_PG covers A6/Q9: identifiers in recalled
// evidence are masked unless the query asks about that category; emails stay
// readable.
func TestEvalStore_MaskEvidence_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "u-mask", "s-mask", []biz.EvalMessage{
		{Role: "user", Content: "我的手机号是13812345678", MessageID: "m1"},
		{Role: "user", Content: "我的身份证号是110101199001011234", MessageID: "m2"},
		{Role: "user", Content: "我的邮箱是alice@example.com", MessageID: "m3"},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := store.SearchMemories(ctx, "u-mask", "介绍一下用户的信息", 10)
	if err != nil {
		t.Fatal(err)
	}
	joined := evalContents(items)
	if !strings.Contains(joined, "138****5678") {
		t.Fatalf("phone not masked: %s", joined)
	}
	if strings.Contains(joined, "13812345678") {
		t.Fatalf("phone leaked: %s", joined)
	}
	if !strings.Contains(joined, "110101********1234") || strings.Contains(joined, "110101199001011234") {
		t.Fatalf("id number not masked: %s", joined)
	}
	if !strings.Contains(joined, "alice@example.com") {
		t.Fatalf("email must never be masked: %s", joined)
	}

	items, err = store.SearchMemories(ctx, "u-mask", "用户的手机号是多少", 10)
	if err != nil {
		t.Fatal(err)
	}
	if joined := evalContents(items); !strings.Contains(joined, "13812345678") {
		t.Fatalf("asking about phone should unmask it: %s", joined)
	}
}

// evalStaticEmbedder is a deterministic biz.EmbeddingService double for the
// vector-correction path (the canned vector hits come from fakeVectorStore).
type evalStaticEmbedder struct{}

func (evalStaticEmbedder) Embed(context.Context, string) ([]float32, error) {
	return []float32{1, 0, 0, 0}, nil
}

// TestEvalStore_VectorCorrectionSupersedes_PG covers A4/Q7: with an embedder
// wired, a correction-cued statement invalidates a semantically-near older
// fact (cosine >= 0.82) even across different preference slots; a sub-threshold
// hit must invalidate nothing.
func TestEvalStore_VectorCorrectionSupersedes_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	fvs := &fakeVectorStore{}
	store := NewEvalMemoryStore(r.data, evalStaticEmbedder{}, loggateway.NewNoop()).(*evalMemoryStore)
	store.facts = newL3FactRepo(r.data, fvs)
	store.syncer = nil
	ctx := context.Background()

	// u-vec-hi: canned hit score 0.9 (>= 0.82) → old fact invalidated.
	if _, err := store.AddMessages(ctx, "u-vec-hi", "s-vec-hi", []biz.EvalMessage{
		{Role: "user", Content: "My favorite color is blue", MessageID: "m1"},
	}); err != nil {
		t.Fatal(err)
	}
	hiFacts := evalC3AllFacts(t, r, "u-vec-hi")
	oldHi := evalC3FindFact(t, hiFacts, "favorite color is blue")
	oldHiID, _ := oldHi["id"].(string)
	fvs.hits = []vector.VectorHit{{ID: oldHiID, Score: 0.9}}
	if _, err := store.AddMessages(ctx, "u-vec-hi", "s-vec-hi", []biz.EvalMessage{
		{Role: "user", Content: "Actually, I hate blue now", MessageID: "m2"},
	}); err != nil {
		t.Fatal(err)
	}
	hiFacts = evalC3AllFacts(t, r, "u-vec-hi")
	oldHi = evalC3FindFact(t, hiFacts, "favorite color is blue")
	if vu, _ := oldHi["valid_until"].(string); vu == "" {
		t.Fatalf("vector correction did not invalidate old fact: %v", oldHi)
	}
	newHi := evalC3FindFact(t, hiFacts, "hate blue")
	newHiMeta, _ := newHi["metadata_json"].(string)
	var meta map[string]any
	if err := json.Unmarshal([]byte(newHiMeta), &meta); err != nil {
		t.Fatal(err)
	}
	prevs, _ := meta["previous_values"].([]any)
	if len(prevs) == 0 {
		t.Fatalf("vector supersede missing previous_values audit: %v", meta)
	}

	// u-vec-lo: canned hit score 0.5 (< 0.82) → old fact survives.
	if _, err := store.AddMessages(ctx, "u-vec-lo", "s-vec-lo", []biz.EvalMessage{
		{Role: "user", Content: "My favorite color is blue", MessageID: "m1"},
	}); err != nil {
		t.Fatal(err)
	}
	loFacts := evalC3AllFacts(t, r, "u-vec-lo")
	oldLo := evalC3FindFact(t, loFacts, "favorite color is blue")
	oldLoID, _ := oldLo["id"].(string)
	fvs.hits = []vector.VectorHit{{ID: oldLoID, Score: 0.5}}
	if _, err := store.AddMessages(ctx, "u-vec-lo", "s-vec-lo", []biz.EvalMessage{
		{Role: "user", Content: "Actually, I hate blue now", MessageID: "m2"},
	}); err != nil {
		t.Fatal(err)
	}
	loFacts = evalC3AllFacts(t, r, "u-vec-lo")
	oldLo = evalC3FindFact(t, loFacts, "favorite color is blue")
	if vu, _ := oldLo["valid_until"].(string); vu != "" {
		t.Fatalf("sub-threshold vector hit invalidated old fact: %v", oldLo)
	}
}
