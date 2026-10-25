package data

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// evalIntentTestEmbedder is a deterministic EmbeddingService+BatchService
// double for the AMC P3 VIR: every live anchor from biz.EvalIntentAnchorSpecs
// maps onto its own basis axis (pos = +1, neg = -1) — mirroring the biz
// intentTestEmbedder scheme — and caller-specified statement vectors are
// layered on top. Unknown texts fall back to the neutral off-axis (axis 7).
type evalIntentTestEmbedder struct {
	vectors     map[string][]float32
	embedCalls  int
	batchCalls  int
	failEmbedding bool
}

var evalIntentAxesTable = map[biz.EvalIntent]int{
	biz.EvalIntentChronological: 0,
	biz.EvalIntentAggregate:     1,
	biz.EvalIntentEnumerative:   2,
	biz.EvalIntentRelational:    3,
	biz.EvalIntentCorrection:    4,
	biz.EvalIntentForget:        5,
	biz.EvalIntentTrend:         6,
}

func evalIntentTestAxis(axis int, sign float32) []float32 {
	v := make([]float32, 8)
	v[axis] = sign
	return v
}

func newEvalIntentTestEmbedder(overrides map[string][]float32) *evalIntentTestEmbedder {
	vectors := map[string][]float32{}
	for intent, anchors := range biz.EvalIntentAnchorSpecs() {
		axis, ok := evalIntentAxesTable[intent]
		if !ok {
			axis = 6
		}
		for _, t := range anchors.Pos {
			vectors[t] = evalIntentTestAxis(axis, 1)
		}
		for _, t := range anchors.Neg {
			vectors[t] = evalIntentTestAxis(axis, -1)
		}
	}
	for k, v := range overrides {
		vectors[k] = v
	}
	return &evalIntentTestEmbedder{vectors: vectors}
}

func (e *evalIntentTestEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	e.embedCalls++
	if e.failEmbedding {
		return nil, errors.New("embed outage")
	}
	if v, ok := e.vectors[text]; ok {
		return v, nil
	}
	return evalIntentTestAxis(7, 1), nil
}

func (e *evalIntentTestEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	e.batchCalls++
	if e.failEmbedding {
		return nil, errors.New("embed outage")
	}
	// Mirror a real server-side batch: counts as ONE call on the embedder,
	// vectors resolved from the same table without touching embedCalls.
	out := make([][]float32, len(texts))
	for i, t := range texts {
		if v, ok := e.vectors[t]; ok {
			out[i] = v
		} else {
			out[i] = evalIntentTestAxis(7, 1)
		}
	}
	return out, nil
}

func newIntentTestStore(emb *evalIntentTestEmbedder) *evalMemoryStore {
	return &evalMemoryStore{emb: emb, lg: loggateway.NewNoop()}
}

// TestEvalIntentRouter_LazyInitAndBackoff covers AMC P3 VIR data wiring:
// the router builds once on first use and caches; an embedding outage at
// first build must NOT retry within the backoff window (DashScope 抖动先例),
// so embed-calls stay at the initial attempt count.
func TestEvalIntentRouter_LazyInitAndBackoff(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	emb := newEvalIntentTestEmbedder(nil)
	s := newIntentTestStore(emb)
	if r := s.intentRouter(ctx); r == nil {
		t.Fatal("router should build on first call")
	}
	builtCalls := emb.embedCalls + emb.batchCalls
	if r := s.intentRouter(ctx); r == nil {
		t.Fatal("router should be cached")
	}
	if got := emb.embedCalls + emb.batchCalls; got != builtCalls {
		t.Fatalf("cached router must not re-embed anchors: %d != %d", got, builtCalls)
	}

	// Outage at first build → nil within the retry window, no re-embed storm.
	fail := newEvalIntentTestEmbedder(nil)
	fail.failEmbedding = true
	s2 := newIntentTestStore(fail)
	if r := s2.intentRouter(ctx); r != nil {
		t.Fatal("router must stay nil after build failure")
	}
	failCalls := fail.embedCalls + fail.batchCalls
	if r := s2.intentRouter(ctx); r != nil {
		t.Fatal("router must stay nil inside retry window")
	}
	if got := fail.embedCalls + fail.batchCalls; got != failCalls {
		t.Fatalf("retry window must not re-embed anchors: %d != %d", got, failCalls)
	}
	// Retry window expiry is honored once the backoff elapses.
	s2.routerMu.Lock()
	s2.routerErrAt = time.Now().Add(-2 * evalIntentRouterRetry)
	s2.routerMu.Unlock()
	fail.failEmbedding = false
	if r := s2.intentRouter(ctx); r == nil {
		t.Fatal("router must rebuild after the retry window elapses")
	}
}

// TestApplyEvalQueryModes covers AMC P3 VIR 读侧接线：模式仅由路由器命中
// 赋值；中性/空向量保持全 false（安全降级）。
func TestApplyEvalQueryModes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	emb := newEvalIntentTestEmbedder(nil)
	s := newIntentTestStore(emb)

	var qa biz.EvalQueryAnalysis
	s.applyEvalQueryModes(ctx, &qa, evalIntentTestAxis(2, 1))
	if !qa.Enumerative || qa.Chronological || qa.Aggregate || qa.Relational || qa.Trend {
		t.Errorf("enumerative-axis query routed wrongly: %+v", qa)
	}
	// P3-c：趋势轴 → 仅 Trend。
	qa = biz.EvalQueryAnalysis{}
	s.applyEvalQueryModes(ctx, &qa, evalIntentTestAxis(6, 1))
	if !qa.Trend || qa.Chronological || qa.Aggregate || qa.Enumerative || qa.Relational {
		t.Errorf("trend-axis query routed wrongly: %+v", qa)
	}
	qa = biz.EvalQueryAnalysis{}
	s.applyEvalQueryModes(ctx, &qa, evalIntentTestAxis(7, 1))
	if qa.Chronological || qa.Aggregate || qa.Enumerative || qa.Relational || qa.Trend {
		t.Errorf("neutral query must not route anywhere: %+v", qa)
	}
	qa = biz.EvalQueryAnalysis{}
	s.applyEvalQueryModes(ctx, &qa, nil)
	if qa.Chronological || qa.Aggregate || qa.Enumerative || qa.Relational || qa.Trend {
		t.Errorf("empty embedding must not touch modes: %+v", qa)
	}
	// Router unavailable → all modes stay false even on a clear intent axis.
	sNoRouter := &evalMemoryStore{lg: loggateway.NewNoop()}
	qa = biz.EvalQueryAnalysis{}
	sNoRouter.applyEvalQueryModes(ctx, &qa, evalIntentTestAxis(0, 1))
	if qa.Chronological || qa.Aggregate || qa.Enumerative || qa.Relational || qa.Trend {
		t.Errorf("degrade path must keep modes false: %+v", qa)
	}
}

// TestEmbedEvalStatements_BatchPreferred covers AMC P3 VIR 写侧 + 评审 #1：
// 一条消息的全部语句以「带前缀 + 去前缀」双形态走一次 EmbedBatch（成本与
// 外界抖动收敛为一次往返）；prefixed 供 0.82 向量通道、routed 供意图路由。
// 路由器缺席时零调用。
func TestEmbedEvalStatements_BatchPreferred(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	emb := newEvalIntentTestEmbedder(nil)
	s := newIntentTestStore(emb)
	texts := []string{"user: 我住在杭州", "user: 我喜欢红色", "user: 其实我喜欢蓝色"}
	// Build & cache the router first so its one-time anchor batch is out of
	// the accounting below.
	if s.intentRouter(ctx) == nil {
		t.Fatal("router should build")
	}
	beforeE, beforeB := emb.embedCalls, emb.batchCalls
	prefixed, routed := s.embedEvalStatements(ctx, texts)
	if len(prefixed) != len(texts) || len(routed) != len(texts) {
		t.Fatalf("maps size = %d/%d, want %d each", len(prefixed), len(routed), len(texts))
	}
	if got := emb.embedCalls - beforeE; got > 0 {
		t.Fatalf("batch path must not call per-text Embed: +%d calls", got)
	}
	if got := emb.batchCalls - beforeB; got != 1 {
		t.Fatalf("batch path must issue exactly one EmbedBatch, got %d", got)
	}
	// 双形态共享同一批向量：两 map 同索引条目同向量（本 double 对两种
	// 文本形态返回不同向量时这里应当一致——同一批返回按序拆分）。
	for i := range texts {
		if len(prefixed[i]) == 0 || len(routed[i]) == 0 {
			t.Fatalf("atom %d missing embedding: prefixed=%v routed=%v", i, prefixed[i], routed[i])
		}
	}
	// No embedder → nil maps, no panic.
	sNone := &evalMemoryStore{lg: loggateway.NewNoop()}
	p, r2 := sNone.embedEvalStatements(ctx, texts)
	if p != nil || r2 != nil {
		t.Fatalf("no-embedder store must return nil maps, got %v/%v", p, r2)
	}
}
