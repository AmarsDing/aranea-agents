package biz

import (
	"context"
	"testing"
)

// intentTestEmbedder is a deterministic EmbeddingService+EmbeddingBatchService
// double: every registered text maps to a handcrafted vector; unknown texts map
// to a far-off-axis neutral vector (no intent match). The anchor vectors are
// generated from the live evalIntentSpecs registry, so spec edits flow into
// every test automatically.
type intentTestEmbedder struct {
	vectors map[string][]float32
}

func intentAxis(axis int, sign float32) []float32 {
	v := make([]float32, 8)
	v[axis] = sign
	return v
}

var intentAxes = map[EvalIntent]int{
	EvalIntentChronological: 0,
	EvalIntentAggregate:     1,
	EvalIntentEnumerative:   2,
	EvalIntentRelational:    3,
	EvalIntentCorrection:    4,
	EvalIntentForget:        5,
	EvalIntentTrend:         6,
}

// newIntentTestEmbedder places each intent's pos anchors on their own basis
// axis and neg anchors on the opposite (-1) direction of that axis, so a pos
// anchor scores cosine 1.0 and strictly beats every neg (-1.0).
func newIntentTestEmbedder(extra map[string][]float32) *intentTestEmbedder {
	vectors := map[string][]float32{}
	for intent, spec := range evalIntentSpecs {
		for _, t := range spec.pos {
			vectors[t] = intentAxis(intentAxes[intent], 1)
		}
		for _, t := range spec.neg {
			vectors[t] = intentAxis(intentAxes[intent], -1)
		}
	}
	for k, v := range extra {
		vectors[k] = v
	}
	return &intentTestEmbedder{vectors: vectors}
}

func (e *intentTestEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if v, ok := e.vectors[text]; ok {
		return v, nil
	}
	return intentAxis(7, 1), nil // neutral off-axis: never matches any intent
}

func (e *intentTestEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v, err := e.Embed(ctx, t)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func embedText(t *testing.T, e *intentTestEmbedder, text string) []float32 {
	t.Helper()
	v, err := e.Embed(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// TestEvalIntentRouter_QueryModes covers AMC P3 VIR：读侧四类模式经锚点向量
// 路由命中各自意图、互不串列；中性/反向句一律不命中。
func TestEvalIntentRouter_QueryModes(t *testing.T) {
	t.Parallel()
	emb := newIntentTestEmbedder(map[string][]float32{
		"按时间顺序列出我的就医记录": intentAxis(0, 1),
		"总结一下我过去聊过的内容":   intentAxis(1, 1),
		"列出我提过的所有城市":      intentAxis(2, 1),
		"我和老王是什么关系":       intentAxis(3, 1),
		"模拟一下我下周的体重走势":    intentAxis(6, 1),
		"我上次测的体重是多少":      intentAxis(7, 1),
		"今天天气怎么样":          intentAxis(7, 1),
		"我的手机号是多少":         intentAxis(7, -1),
	})
	r, err := NewEvalIntentRouter(context.Background(), emb)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		text string
		want EvalIntent // "" means no intent should match
		not  []EvalIntent
	}{
		{"按时间顺序列出我的就医记录", EvalIntentChronological, []EvalIntent{EvalIntentEnumerative, EvalIntentAggregate, EvalIntentRelational}},
		{"总结一下我过去聊过的内容", EvalIntentAggregate, []EvalIntent{EvalIntentChronological, EvalIntentEnumerative}},
		{"列出我提过的所有城市", EvalIntentEnumerative, []EvalIntent{EvalIntentAggregate, EvalIntentChronological}},
		{"我和老王是什么关系", EvalIntentRelational, []EvalIntent{EvalIntentChronological}},
		{"模拟一下我下周的体重走势", EvalIntentTrend, []EvalIntent{EvalIntentChronological, EvalIntentAggregate}},
		{"我上次测的体重是多少", "", []EvalIntent{EvalIntentTrend}},
		{"今天天气怎么样", "", []EvalIntent{EvalIntentChronological, EvalIntentAggregate, EvalIntentEnumerative, EvalIntentRelational, EvalIntentTrend}},
		{"我的手机号是多少", "", []EvalIntent{EvalIntentEnumerative}},
	}
	for _, tc := range cases {
		if tc.want != "" && !r.Match(embedText(t, emb, tc.text), tc.want) {
			t.Errorf("Match(%q, %s) = false, want true", tc.text, tc.want)
		}
		for _, n := range tc.not {
			if r.Match(embedText(t, emb, tc.text), n) {
				t.Errorf("Match(%q, %s) = true, want false", tc.text, n)
			}
		}
	}
}

// TestEvalIntentRouter_WriteGates covers AMC P3 VIR 写侧：遗忘/改口请求命中
// 各自意图；强化记忆句（难忘/don't forget）被反向锚点压制 —— 这正是词表时代
// 需要手工 reinforcement guard 的场景，现由语义路由完成。
func TestEvalIntentRouter_WriteGates(t *testing.T) {
	t.Parallel()
	emb := newIntentTestEmbedder(map[string][]float32{
		"user: 请忘掉我之前给你的电话号码":         intentAxis(5, 1),
		"user: please forget my old address":  intentAxis(5, 1),
		"user: 其实我住在上海不是杭州":             intentAxis(4, 1),
		"user: I am no longer using Gmail":    intentAxis(4, 1),
		// 强化记忆句：挂在 forget 轴 -1 方向（与 neg 锚点同向）。
		"user: 我永远也不会忘记这个夏天":            intentAxis(5, -1),
		"user: don't forget my birthday":      intentAxis(5, -1),
		"user: 我住在杭州":                      intentAxis(7, 1),
		"user: 今天天气真不错":                    intentAxis(7, 1),
	})
	r, err := NewEvalIntentRouter(context.Background(), emb)
	if err != nil {
		t.Fatal(err)
	}
	wantMatch := map[string]EvalIntent{
		"user: 请忘掉我之前给你的电话号码":        EvalIntentForget,
		"user: please forget my old address": EvalIntentForget,
		"user: 其实我住在上海不是杭州":            EvalIntentCorrection,
		"user: I am no longer using Gmail":   EvalIntentCorrection,
	}
	for text, intent := range wantMatch {
		if !r.Match(embedText(t, emb, text), intent) {
			t.Errorf("Match(%q, %s) = false, want true", text, intent)
		}
	}
	// Anti-cases: reinforcement and neutral statements must hit NEITHER gate.
	for _, text := range []string{
		"user: 我永远也不会忘记这个夏天",
		"user: don't forget my birthday",
		"user: 我住在杭州",
		"user: 今天天气真不错",
	} {
		if r.Match(embedText(t, emb, text), EvalIntentForget) {
			t.Errorf("Match(%q, forget) = true, want false (reinforcement/neutral must not invalidate)", text)
		}
		if r.Match(embedText(t, emb, text), EvalIntentCorrection) {
			t.Errorf("Match(%q, correction) = true, want false", text)
		}
	}
}

// TestEvalIntentRouter_CrossIntentMargin pins the maxCos(pos) > maxCos(neg)
// rule plus threshold check: an embedding on the NEG direction of the forget
// axis scores cos=-0.9 vs pos anchors and cos=+0.9 vs neg anchors — it must
// not match even though magnitudes exceed the threshold.
func TestEvalIntentRouter_CrossIntentMargin(t *testing.T) {
	t.Parallel()
	emb := newIntentTestEmbedder(map[string][]float32{
		"neg-dominant": intentAxis(5, -1),
	})
	r, err := NewEvalIntentRouter(context.Background(), emb)
	if err != nil {
		t.Fatal(err)
	}
	if r.Match(embedText(t, emb, "neg-dominant"), EvalIntentForget) {
		t.Error("neg-dominant embedding must not match forget")
	}
}

// TestEvalIntentRouter_NearDupFactoidsRejected（评审 #2 回归钉）：单点
// 事实/偏好问句与近邻 pos 锚点只差一两个词元（"what do I like" vs
// summarize-family；"when did I move" vs sequence-family），此前会被
// 误路由进 aggregate/chronological。修复后这些形态落在 neg 锚方向上，
// 任何模式都不得命中。
func TestEvalIntentRouter_NearDupFactoidsRejected(t *testing.T) {
	t.Parallel()
	emb := newIntentTestEmbedder(nil) // neg 锚点文本由注册表自动映射到各轴 -1
	r, err := NewEvalIntentRouter(context.Background(), emb)
	if err != nil {
		t.Fatal(err)
	}
	neutralFactoids := []string{
		"what do I like", "我喜欢什么", "do I like coffee",
		"when did I move", "我什么时候搬的家", "我第一次搬家是哪天",
	}
	for _, text := range neutralFactoids {
		v := embedText(t, emb, text)
		for _, intent := range []EvalIntent{
			EvalIntentChronological, EvalIntentAggregate,
			EvalIntentEnumerative, EvalIntentRelational, EvalIntentTrend,
		} {
			if r.Match(v, intent) {
				t.Errorf("near-dup factoid %q wrongly routed to %s", text, intent)
			}
		}
	}
	// 对照：改写后的判别式 pos 锚点仍须命中各自意图。
	positive := map[string]EvalIntent{
		"overall, summarize what kinds of things I like": EvalIntentAggregate,
		"what was the sequence of my moves":              EvalIntentChronological,
	}
	for text, intent := range positive {
		if !r.Match(embedText(t, emb, text), intent) {
			t.Errorf("discriminating anchor %q must still route to %s", text, intent)
		}
	}
}

// TestEvalIntentRouter_NilAndEmpty guards degrade paths: nil router / empty
// embedding / unknown intent never route anywhere; zero vector has undefined
// direction and must not route either.
func TestEvalIntentRouter_NilAndEmpty(t *testing.T) {
	t.Parallel()
	var r *EvalIntentRouter
	if r.Match([]float32{1, 0}, EvalIntentForget) {
		t.Error("nil router must not match")
	}
	emb := newIntentTestEmbedder(nil)
	r2, err := NewEvalIntentRouter(context.Background(), emb)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Match(nil, EvalIntentForget) {
		t.Error("empty embedding must not match")
	}
	if r2.Match([]float32{0, 0, 0, 0}, EvalIntentForget) {
		t.Error("zero vector must not match")
	}
	if r2.Match([]float32{1, 0}, EvalIntent("nonexistent")) {
		t.Error("unknown intent must not match")
	}
}

// TestEvalIntentRouter_AnchorRegistryIsCovered ensures the exported anchor
// specs mirror evalIntentSpecs exactly — a spec edit forgetting the export
// breaks here.
func TestEvalIntentRouter_AnchorRegistryIsCovered(t *testing.T) {
	t.Parallel()
	exported := EvalIntentAnchorSpecs()
	if len(exported) != len(evalIntentSpecs) {
		t.Fatalf("exported anchor map drifted from specs: %d vs %d", len(exported), len(evalIntentSpecs))
	}
	for intent, spec := range evalIntentSpecs {
		ea := exported[intent]
		if len(ea.Pos) != len(spec.pos) || len(ea.Neg) != len(spec.neg) {
			t.Fatalf("intent %s exported %d/%d anchors, spec has %d/%d",
				intent, len(ea.Pos), len(ea.Neg), len(spec.pos), len(spec.neg))
		}
		if ea.Threshold != spec.threshold {
			t.Fatalf("intent %s exported threshold %v, spec has %v", intent, ea.Threshold, spec.threshold)
		}
	}
}
