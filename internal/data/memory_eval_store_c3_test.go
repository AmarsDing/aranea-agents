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

// evalStaticEmbedder keeps the pre-P3 deterministic vector regression fixture.
type evalStaticEmbedder struct{}

func (evalStaticEmbedder) Embed(context.Context, string) ([]float32, error) {
	return []float32{1, 0, 0, 0}, nil
}

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

// Atom count assertions exclude derived projections and lifecycle controls.
func evalC3AtomFacts(t *testing.T, r *l3FactRepo, userID string) []map[string]any {
	t.Helper()
	facts := evalC3AllFacts(t, r, userID)
	out := make([]map[string]any, 0, len(facts))
	for _, f := range facts {
		if stmt, _ := f["statement"].(string); biz.IsEvalRollupStatement(stmt) || f["source_kind"] == evalControlSource {
			continue
		}
		out = append(out, f)
	}
	return out
}

func evalC3FindFact(t *testing.T, facts []map[string]any, contains string) map[string]any {
	t.Helper()
	for _, f := range facts {
		if stmt, _ := f["statement"].(string); strings.Contains(stmt, contains) && !biz.IsEvalRollupStatement(stmt) {
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
	facts := evalC3AtomFacts(t, r, "u-atom")
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
	if got := len(evalC3AtomFacts(t, r, "u-atom")); got != 3 {
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
		{Role: "user", Content: "我最喜欢的旅行城市是杭州", MessageID: "m1", Timestamp: "2025-05-01T09:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddMessages(ctx, "u-rd", "s-rd", []biz.EvalMessage{
		{Role: "user", Content: "我上个月把最喜欢的旅行城市改成上海了", MessageID: "m2", Timestamp: "2025-05-07T09:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	facts := evalC3AtomFacts(t, r, "u-rd")
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
	wantDay := "2025-04-01"
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
// AMC P3 评审 #0/#6：模式经 VIR 路由，测试按新契约接确定性嵌入 double
// （查询句本身是 chronological pos 锚点，锚表直接供向量）。
func TestEvalStore_ChronologicalOrder_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	emb := newEvalIntentTestEmbedder(nil)
	store := NewEvalMemoryStore(r.data, emb, loggateway.NewNoop()).(*evalMemoryStore)
	store.syncer = nil
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
// retains both sessions while avoiding transcripts already covered by L3.
func TestEvalStore_AggregateAppendsEpisodes_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	emb := newEvalIntentTestEmbedder(map[string][]float32{
		"总结一下我最喜欢的水果和饮料": evalIntentTestAxis(evalIntentAxesTable[biz.EvalIntentAggregate], 1),
	})
	store := NewEvalMemoryStore(r.data, emb, loggateway.NewNoop()).(*evalMemoryStore)
	store.syncer = nil
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
	if l2Count > 2 {
		t.Fatalf("duplicate L2 episodes attached=%d: %v", l2Count, evalContents(items))
	}
	joined := evalContents(items)
	if !strings.Contains(joined, "芒果") || !strings.Contains(joined, "红茶") || !strings.Contains(joined, "sess-s-agg-a]") || !strings.Contains(joined, "sess-s-agg-b]") {
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
//
//	(a) 已墓碑的旧行不得被指纹 upsert 起死（valid_until 保留）；
//	(b) 重放的旧语句不得经同槽通道再次失效事件时间更晚的新行（时序守卫）；
//	(c) 新行 metadata.previous_values 审计留痕不得被重放抹掉。
//
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
	facts := evalC3AtomFacts(t, r, "u-sbr")
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
	facts := evalC3AtomFacts(t, r, "u-pad")
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
	if strings.Contains(joined, "alice@example.com") || !strings.Contains(joined, "[email redacted]") {
		t.Fatalf("unrequested email must be masked: %s", joined)
	}

	items, err = store.SearchMemories(ctx, "u-mask", "用户的手机号是多少", 10)
	if err != nil {
		t.Fatal(err)
	}
	if joined := evalContents(items); !strings.Contains(joined, "13812345678") {
		t.Fatalf("asking about phone should unmask it: %s", joined)
	}
}

// AMC P3 VIR: the former evalStaticEmbedder (uniform {1,0,0,0}) has been
// replaced by newEvalIntentTestEmbedder (memory_eval_intent_test.go), a
// router-aware double putting each intent's anchors on its own basis axis so
// biz.EvalIntentRouter.Match routes statements deterministically. Statement
// overrides below pin each PG scenario's intent routing.
//
// TestEvalStore_VectorCorrectionSupersedes_PG covers A4/Q7: with an embedder
// wired, a correction-intent statement invalidates a semantically-near older
// fact (cosine >= 0.82) even across different preference slots; a sub-threshold
// hit must invalidate nothing.
func TestEvalStore_VectorCorrectionSupersedes_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	fvs := &fakeVectorStore{}
	emb := newEvalIntentTestEmbedder(map[string][]float32{
		// 评审 #1：意图路由比对 stripEvalRolePrefix 后的裸文本，键必须去前缀。
		"Actually, I hate blue now": evalIntentTestAxis(evalIntentAxesTable[biz.EvalIntentCorrection], 1),
	})
	store := NewEvalMemoryStore(r.data, emb, loggateway.NewNoop()).(*evalMemoryStore)
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

// TestEvalStore_ForgetCueVectorSupersede_PG covers AMC P3 / D3：遗忘请求经
// 向量近邻通道失效旧事实；假设式遗忘（如果我忘了…就…）与强化式说法
// （don't forget）绝不能触发失效。AMC P3 VIR：遗忘意图由路由器判定，强化
// 句由反向锚点压制（don't forget → forget 轴 -1 方向），假设句仍由
// biz.IsHypotheticalStatement 形状守卫拦截。
func TestEvalStore_ForgetCueVectorSupersede_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	fvs := &fakeVectorStore{}
	emb := newEvalIntentTestEmbedder(map[string][]float32{
		// 评审 #1：意图路由比对 stripEvalRolePrefix 后的裸文本，键必须去前缀。
		"please forget my old phone number": evalIntentTestAxis(evalIntentAxesTable[biz.EvalIntentForget], 1),
		// 假设式遗忘：同样命中 forget 意图——只剩假设守卫能拦截（守卫回归面）。
		"如果我把零食忘了就提醒我": evalIntentTestAxis(evalIntentAxesTable[biz.EvalIntentForget], 1),
		// 强化式说法：与 neg 锚点同向，路由器必须判否。
		"please don't forget my birthday": evalIntentTestAxis(evalIntentAxesTable[biz.EvalIntentForget], -1),
	})
	store := NewEvalMemoryStore(r.data, emb, loggateway.NewNoop()).(*evalMemoryStore)
	store.facts = newL3FactRepo(r.data, fvs)
	store.syncer = nil
	ctx := context.Background()

	// 场景 A：显式遗忘走持久化控制通道，历史查询也不得恢复目标。
	if _, err := store.AddMessages(ctx, "u-forget", "s-forget", []biz.EvalMessage{
		{Role: "user", Content: "My old phone number is 13800001111", MessageID: "m1"},
	}); err != nil {
		t.Fatal(err)
	}
	factsA := evalC3AllFacts(t, r, "u-forget")
	oldA := evalC3FindFact(t, factsA, "old phone number is 138")
	oldAID, _ := oldA["id"].(string)
	fvs.hits = []vector.VectorHit{{ID: oldAID, Score: 0.9}}
	if _, err := store.AddMessages(ctx, "u-forget", "s-forget", []biz.EvalMessage{
		{Role: "user", Content: "please forget my old phone number", MessageID: "m2"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"What is my old phone number?", "What was my old phone number previously?", "Summarize all conversations"} {
		items, err := store.SearchMemories(ctx, "u-forget", q, 100)
		if err != nil || strings.Contains(evalContents(items), "13800001111") || strings.Contains(evalContents(items), "forget my old phone") {
			t.Fatalf("forgotten source or control command leaked: %s %v", evalContents(items), err)
		}
	}
	policies, err := store.evalMemoryPolicies(ctx, "u-forget")
	if err != nil || len(policies) != 1 || policies[0].Mode != biz.EvalControlForget {
		t.Fatalf("missing durable forget control: %+v %v", policies, err)
	}

	// 场景 B：假设式遗忘（如果…就…）绝不失效旧事实。
	if _, err := store.AddMessages(ctx, "u-forget-hyp", "s-forget-hyp", []biz.EvalMessage{
		{Role: "user", Content: "我最喜欢的零食是薯片", MessageID: "m1"},
	}); err != nil {
		t.Fatal(err)
	}
	factsB := evalC3AllFacts(t, r, "u-forget-hyp")
	oldB := evalC3FindFact(t, factsB, "零食是薯片")
	oldBID, _ := oldB["id"].(string)
	fvs.hits = []vector.VectorHit{{ID: oldBID, Score: 0.9}}
	if _, err := store.AddMessages(ctx, "u-forget-hyp", "s-forget-hyp", []biz.EvalMessage{
		{Role: "user", Content: "如果我把零食忘了就提醒我", MessageID: "m2"},
	}); err != nil {
		t.Fatal(err)
	}
	factsB = evalC3AllFacts(t, r, "u-forget-hyp")
	oldB = evalC3FindFact(t, factsB, "零食是薯片")
	if vu, _ := oldB["valid_until"].(string); vu != "" {
		t.Fatalf("hypothetical forget must not invalidate: %v", oldB)
	}

	// 场景 C：强化式说法（don't forget）不是遗忘请求，旧事实必须存活。
	if _, err := store.AddMessages(ctx, "u-forget-keep", "s-forget-keep", []biz.EvalMessage{
		{Role: "user", Content: "my birthday is May 1", MessageID: "m1"},
	}); err != nil {
		t.Fatal(err)
	}
	factsC := evalC3AllFacts(t, r, "u-forget-keep")
	oldC := evalC3FindFact(t, factsC, "birthday is May 1")
	oldCID, _ := oldC["id"].(string)
	fvs.hits = []vector.VectorHit{{ID: oldCID, Score: 0.9}}
	if _, err := store.AddMessages(ctx, "u-forget-keep", "s-forget-keep", []biz.EvalMessage{
		{Role: "user", Content: "please don't forget my birthday", MessageID: "m2"},
	}); err != nil {
		t.Fatal(err)
	}
	factsC = evalC3AllFacts(t, r, "u-forget-keep")
	oldC = evalC3FindFact(t, factsC, "birthday is May 1")
	if vu, _ := oldC["valid_until"].(string); vu != "" {
		t.Fatalf("reinforcement phrasing must not invalidate: %v", oldC)
	}
}

// TestEvalStore_NumericDigest_PG covers AMC P3-c / G4：同键主题数值序列滚动
// digest 的端到端生命周期——
//
//	(a) 多天同主题观测聚成一条 active digest（历史轮转行带 valid_until）；
//	(b) 计数/极值/首末日/趋势由写侧滚动维护且 canonical 行可解析回放；
//	(c) 平台回放过期观测时 counters 不倒卷（时序守卫，C3 #0/#4 同思路）；
//	(d) trend 模式查询把 digest 行置顶返回（置顶 + floor 豁免同验证）。
func TestEvalStore_NumericDigest_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	emb := newEvalIntentTestEmbedder(map[string][]float32{
		"按这个趋势我下周的步数大概是多少": evalIntentTestAxis(evalIntentAxesTable[biz.EvalIntentTrend], 1),
	})
	store := NewEvalMemoryStore(r.data, emb, loggateway.NewNoop()).(*evalMemoryStore)
	store.syncer = nil
	ctx := context.Background()

	add := func(content, ts, msgID string) {
		t.Helper()
		if _, err := store.AddMessages(ctx, "u-num", "s-num", []biz.EvalMessage{
			{Role: "user", Content: content, MessageID: msgID, Timestamp: ts},
		}); err != nil {
			t.Fatal(err)
		}
	}
	add("今天步数3200步", "2025-05-01T08:00:00Z", "m1")
	add("今天步数4500步", "2025-05-03T08:00:00Z", "m2")
	add("今天步数2800步", "2025-05-05T08:00:00Z", "m3")
	add("今天步数5600步", "2025-05-07T08:00:00Z", "m4")

	// (a)(b) 恰好一条 active digest，counter 全对，canonical 行 round-trip。
	activeDigest := func() biz.EvalNumSeries {
		t.Helper()
		var actives []biz.EvalNumSeries
		rotations := 0
		for _, f := range evalC3AllFacts(t, r, "u-num") {
			stmt, _ := f["statement"].(string)
			if !strings.HasPrefix(stmt, "[series-digest: 步数]") {
				continue
			}
			if vu, _ := f["valid_until"].(string); vu != "" {
				rotations++
				continue
			}
			series, ok := biz.ParseEvalSeriesDigest(stmt)
			if !ok {
				t.Fatalf("active digest row does not round-trip: %q", stmt)
			}
			actives = append(actives, series)
		}
		if len(actives) != 1 {
			t.Fatalf("active digest rows=%d, want exactly 1", len(actives))
		}
		if rotations == 0 {
			t.Fatal("expected rotated (invalidated) digest rows from rolling updates")
		}
		return actives[0]
	}
	s := activeDigest()
	if s.Count != 4 || s.Last != 5600 || s.FirstValue != 3200 || s.Min != 2800 || s.Max != 5600 {
		t.Fatalf("digest counters wrong: %+v", s)
	}
	if s.FirstDay != "2025-05-01" || s.LastDay != "2025-05-07" {
		t.Fatalf("digest day window wrong: %+v", s)
	}

	// (c) 回放过期观测（day1）：counters/窗口一律不动。
	add("今天步数3200步", "2025-05-01T08:00:00Z", "m1-replay")
	s = activeDigest()
	if s.Count != 4 || s.Last != 5600 || s.LastDay != "2025-05-07" {
		t.Fatalf("stale replay rewound the digest: %+v", s)
	}

	// (d) trend 查询：digest 行置顶。
	items, err := store.SearchMemories(ctx, "u-num", "按这个趋势我下周的步数大概是多少", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("trend query returned no evidence")
	}
	if !strings.Contains(items[0].Content, "[series-digest: 步数]") {
		t.Fatalf("trend query must pin the digest row first, got: %q", items[0].Content)
	}
	if !strings.Contains(items[0].Content, "count=4") {
		t.Fatalf("pinned digest missing counters: %q", items[0].Content)
	}
}

// TestEvalStore_ProfileDigestRollup_PG covers AMC P3-a（F1 摘要/长历史全局
// 综合）：确定性 rollup 写侧+读侧端到端。
//
//	(a) profile digest 恰一条 active 行、分区正确、不回收已失效旧值；
//	(b) 改口后 slot-history 行成链（旧值在前且带"已失效"标注）；
//	(c) aggregate 查询经 VIR 路由后 rollup 行置顶（两种 rollup 行占据前二，
//	    普通事实一律排后）。
func TestEvalStore_ProfileDigestRollup_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	// 查询句本身是 aggregate 正向锚点（锚表直接供向量）；改口走词级 cue
	// 通道（同总成 supersede），无需向量近邻。
	emb := newEvalIntentTestEmbedder(nil)
	store := NewEvalMemoryStore(r.data, emb, loggateway.NewNoop()).(*evalMemoryStore)
	store.syncer = nil
	ctx := context.Background()

	add := func(content, ts, msgID string) {
		t.Helper()
		if _, err := store.AddMessages(ctx, "u-prof", "s-prof", []biz.EvalMessage{
			{Role: "user", Content: content, MessageID: msgID, Timestamp: ts},
		}); err != nil {
			t.Fatal(err)
		}
	}
	add("我叫张三", "2025-03-01T08:00:00Z", "m1")
	add("我最喜欢的旅行城市是杭州", "2025-04-01T08:00:00Z", "m2")
	add("我上个月把最喜欢的旅行城市改成上海了", "2025-05-01T08:00:00Z", "m3")

	activeByPrefix := func(prefix string) []string {
		t.Helper()
		var stmts []string
		for _, f := range evalC3AllFacts(t, r, "u-prof") {
			stmt, _ := f["statement"].(string)
			if !strings.HasPrefix(stmt, prefix) {
				continue
			}
			if vu, _ := f["valid_until"].(string); vu != "" {
				continue
			}
			stmts = append(stmts, stmt)
		}
		return stmts
	}

	// (a) profile digest：恰一条 active、分区齐、旧值（杭州）已失效不入 digest。
	digests := activeByPrefix("[profile-digest]")
	if len(digests) != 1 {
		t.Fatalf("active profile digests=%d, want exactly 1: %v", len(digests), digests)
	}
	d := digests[0]
	if !strings.Contains(d, "身份: 我叫张三") {
		t.Fatalf("digest missing identity section: %q", d)
	}
	if !strings.Contains(d, "偏好: ") || !strings.Contains(d, "上海") {
		t.Fatalf("digest missing preference section with current value: %q", d)
	}
	if strings.Contains(d, "杭州") {
		t.Fatalf("superseded value leaked into profile digest: %q", d)
	}

	// (b) slot-history：恰一条 active、链序正确、旧值带"已失效"标注。
	histories := activeByPrefix("[slot-history: favorite:旅行城市]")
	if len(histories) != 1 {
		t.Fatalf("active slot histories=%d, want exactly 1: %v", len(histories), histories)
	}
	h := histories[0]
	iOld, iNew := strings.Index(h, "杭州"), strings.Index(h, "上海")
	if iOld < 0 || iNew < 0 || iOld > iNew {
		t.Fatalf("chain must order old value before new: %q", h)
	}
	if !strings.Contains(h, "已失效") || !strings.Contains(h, " → ") {
		t.Fatalf("chain markers wrong: %q", h)
	}
	// 身份/name 槽单值不成链。
	if got := activeByPrefix("[slot-history: name]"); len(got) != 0 {
		t.Fatalf("single-value slot must not render a history row: %v", got)
	}

	// (c) aggregate 查询（VIR 路由，锚点原句）：两条 rollup 行占据前二，
	// 普通事实一律排后。
	items, err := store.SearchMemories(ctx, "u-prof", "总结一下我们聊了什么", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 2 {
		t.Fatalf("aggregate query returned %d items, want >=2", len(items))
	}
	p0, p1 := "", ""
	if len(items) > 0 {
		p0 = items[0].Content
	}
	if len(items) > 1 {
		p1 = items[1].Content
	}
	isRollup := map[string]bool{}
	for i, c := range []string{p0, p1} {
		isRollup[c] = biz.IsEvalProfileDigestStatement(c) || biz.IsEvalSlotHistoryStatement(c)
		if !isRollup[c] {
			t.Fatalf("items[%d] must be a rollup row, got: %q", i, c)
		}
	}
	if p0 == p1 {
		t.Fatalf("two distinct rollup rows must lead: %q vs %q", p0, p1)
	}
}

// TestEvalStore_RelationalChain_PG covers AMC P3-d（B1/B3 证据链）读侧：
// Relational 查询（VIR 路由，锚点原句）命中集复用 Q6 时序语义按事件日升序
// 成链输出。判别点：晚事件「合租室友」与查询词面重叠最高（"我和老王"+关
// 系词），纯分数排序会把它置顶；链式排序必须仍让早期事件先行。
func TestEvalStore_RelationalChain_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	emb := newEvalIntentTestEmbedder(nil)
	store := NewEvalMemoryStore(r.data, emb, loggateway.NewNoop()).(*evalMemoryStore)
	store.syncer = nil
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "u-rel", "s-rel", []biz.EvalMessage{
		{Role: "user", Content: "老王介绍我去他们公司上班", MessageID: "m1", Timestamp: "2025-04-01T08:00:00Z"},
		{Role: "user", Content: "老王邀我参加了他的婚礼", MessageID: "m2", Timestamp: "2025-04-03T08:00:00Z"},
		{Role: "user", Content: "我和老王现在合租在一起也是多年好友", MessageID: "m3", Timestamp: "2025-04-05T08:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	items, err := store.SearchMemories(ctx, "u-rel", "我和老王是什么关系", 10)
	if err != nil {
		t.Fatal(err)
	}
	idxJob, idxWedding, idxMate := -1, -1, -1
	for i, it := range items {
		if strings.Contains(it.Content, "上班") && idxJob < 0 {
			idxJob = i
		}
		if strings.Contains(it.Content, "婚礼") && idxWedding < 0 {
			idxWedding = i
		}
		if strings.Contains(it.Content, "合租") && idxMate < 0 {
			idxMate = i
		}
	}
	if idxJob < 0 || idxWedding < 0 || idxMate < 0 {
		t.Fatalf("chain evidence incomplete, job=%d wedding=%d mate=%d: %s",
			idxJob, idxWedding, idxMate, evalContents(items))
	}
	if !(idxJob < idxWedding && idxWedding < idxMate) {
		t.Fatalf("relational chain must order evidence by day asc, job@%d wedding@%d mate@%d: %s",
			idxJob, idxWedding, idxMate, evalContents(items))
	}
}

// TestEvalStore_DigestSurvivesHotUserWindow_PG covers AMC P3 评审 C1 (high)：
// 数值 digest 的查找必须按系列键前缀定向（per-key prefix lookup），而不是
// 共享的 40 行 updated_at 窗口。热点用户连续写入 >40 条其它事实后，旧的
// 步数 digest 会被挤出窗口——若 found=false 则 counters 清零重滚、同一
// 序列出现两条 active digest。本用例先写一条步数事实，再写 45 条无数字
// 填充事实把 digest 挤出旧窗口边界，然后写第二条步数事实：必须命中既有
// digest 滚动成 count=2 且全库恰好一条 active digest 行。
func TestEvalStore_DigestSurvivesHotUserWindow_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "u-hot", "s-hot", []biz.EvalMessage{
		{Role: "user", Content: "今天步数3200步", MessageID: "m-walk-1", Timestamp: "2025-05-01T08:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	// 45 条无数字填充事实：把 digest 挤出 ORDER BY updated_at DESC LIMIT 40
	// 的共享窗口（评审 C1 复现条件）。
	letters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	fillers := make([]biz.EvalMessage, 0, 45)
	for i := 0; i < 45; i++ {
		code := string(letters[i/26]) + string(letters[i%26])
		fillers = append(fillers, biz.EvalMessage{
			Role: "user", Content: "我喜欢风格" + code,
			MessageID: "m-fill-" + code, Timestamp: "2025-05-02T08:00:00Z",
		})
	}
	if _, err := store.AddMessages(ctx, "u-hot", "s-hot", fillers); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddMessages(ctx, "u-hot", "s-hot", []biz.EvalMessage{
		{Role: "user", Content: "今天步数4500步", MessageID: "m-walk-2", Timestamp: "2025-05-03T08:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	// 断言：恰好一条 active 步数 digest，且 counters 是滚动累加（count=2）
	// 而非清零重滚。
	facts := evalC3AllFacts(t, r, "u-hot")
	activeDigests := 0
	for _, f := range facts {
		stmt, _ := f["statement"].(string)
		if !biz.IsEvalSeriesDigestStatement(stmt) || !strings.Contains(stmt, "[series-digest: 步数]") {
			continue
		}
		if u, _ := f["valid_until"].(string); u != "" {
			continue
		}
		activeDigests++
		if !strings.Contains(stmt, "count=2") {
			t.Fatalf("digest must roll forward to count=2 (counters reset = C1 starvation), got %q", stmt)
		}
		if !strings.Contains(stmt, "mean=3850.0") {
			t.Fatalf("digest mean must reconcile both observations, got %q", stmt)
		}
	}
	if activeDigests != 1 {
		t.Fatalf("exactly one active 步数 digest expected, got %d", activeDigests)
	}
}
