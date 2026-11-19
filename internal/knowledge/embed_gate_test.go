package knowledge

// D6 L-3 embedding 密级闸 knowledge 侧测试：ctx 载体 / 本地性判定 /
// 漏斗阻断零流量 / 摄取词法降级与密级传播。

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// captureGate 记录每次判定入参并按 blockAt 拒绝。
type captureGate struct {
	blockAt int
	got     []biz.EmbedGateInfo
}

func (g *captureGate) CheckEmbed(_ context.Context, info biz.EmbedGateInfo) error {
	g.got = append(g.got, info)
	if g.blockAt > 0 && info.Classification >= g.blockAt {
		return fmt.Errorf("test gate: L%d blocked", info.Classification)
	}
	return nil
}

func TestEmbedClassificationCtxRoundtrip(t *testing.T) {
	ctx := context.Background()
	if got := embedClassificationFromContext(ctx); got.level != 0 || got.subject != "" {
		t.Fatalf("empty ctx must yield zero carrier, got %+v", got)
	}
	// 零值不注入（零开销直通）。
	if c2 := WithEmbedClassification(ctx, 0, ""); c2 != ctx {
		t.Fatal("zero carrier must not wrap ctx")
	}
	c2 := WithEmbedClassification(ctx, 3, "doc-9")
	got := embedClassificationFromContext(c2)
	if got.level != 3 || got.subject != "doc-9" {
		t.Fatalf("carrier = %+v", got)
	}
}

func TestIsLocalEmbedTarget(t *testing.T) {
	cases := []struct {
		provider, baseURL string
		want              bool
	}{
		{ProviderOllama, "", true},     // 空 baseURL 回退本机默认
		{ProviderHuggingFace, "", true}, // 同上（localhost:8080）
		{ProviderOpenAI, "", false},    // openai 无端点默认=未配置，保守按 cloud
		{ProviderOpenAI, "https://api.openai.com", false},
		{ProviderGemini, "", false},
		{ProviderOpenAI, "http://127.0.0.1:8080", true}, // 本地 vllm 兼容网关
		{ProviderOpenAI, "http://localhost:8080", true},
		{ProviderOpenAI, "http://[::1]:8080", true},
		{ProviderOpenAI, "http://192.168.1.10:8080", true}, // RFC1918 私网
		{ProviderOpenAI, "http://10.0.0.2", true},
		{ProviderOpenAI, "http://172.16.0.3", true},
		{ProviderOpenAI, "http://172.31.255.254", true},
		{ProviderOpenAI, "http://172.15.0.1", false}, // 172.16/12 界外
		{ProviderOpenAI, "https://8.8.8.8", false},
		{ProviderOllama, "https://ollama.example.com", false}, // 显式远端 ollama 不是本地
		{ProviderOpenAI, "://bad-url", false},                 // 解析失败保守按 cloud
	}
	for _, tc := range cases {
		if got := IsLocalEmbedTarget(tc.provider, tc.baseURL); got != tc.want {
			t.Errorf("IsLocalEmbedTarget(%q, %q) = %v, want %v", tc.provider, tc.baseURL, got, tc.want)
		}
	}
}

// newGateTestEmbedder 指向 httptest 本机端点（openai 协议形状），返回命中计数。
func newGateTestEmbedder(t *testing.T) (*MultiProviderEmbedder, *atomic.Int32) {
	t.Helper()
	hits := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[0.1,0.2,0.3]}],"usage":{"prompt_tokens":1,"total_tokens":1}}`))
	}))
	t.Cleanup(srv.Close)
	return NewMultiProviderEmbedder(ProviderOpenAI, srv.URL, "k", "m", 3, loggateway.NewNoop()), hits
}

func TestEmbedGateBlocksBeforeNetwork(t *testing.T) {
	e, hits := newGateTestEmbedder(t)
	gate := &captureGate{blockAt: 3}
	e.SetClassificationGate(gate)

	ctx := WithEmbedClassification(context.Background(), 3, "doc-1")
	_, err := e.Embed(ctx, []string{"秘密内容"})
	var berr *EmbedClassificationBlockedError
	if !errors.As(err, &berr) {
		t.Fatalf("want EmbedClassificationBlockedError, got %T %v", err, err)
	}
	if hits.Load() != 0 {
		t.Fatalf("blocked embed must not hit network, hits=%d", hits.Load())
	}
	if len(gate.got) != 1 {
		t.Fatalf("gate invocations = %d", len(gate.got))
	}
	info := gate.got[0]
	if info.Classification != 3 || info.Subject != "doc-1" || info.BatchSize != 1 {
		t.Fatalf("gate info = %+v", info)
	}
	if !info.Local {
		t.Fatal("127.0.0.1 httptest endpoint must be classified local")
	}
}

func TestEmbedGateNilPassesThrough(t *testing.T) {
	e, hits := newGateTestEmbedder(t)
	vecs, err := e.Embed(context.Background(), []string{"hello"})
	if err != nil || len(vecs) != 1 {
		t.Fatalf("no gate must pass: vecs=%d err=%v", len(vecs), err)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits=%d", hits.Load())
	}
}

func TestEmbedGateDefaultFallbackAndClear(t *testing.T) {
	gate := &captureGate{blockAt: 3}
	SetDefaultEmbedClassificationGate(gate)
	t.Cleanup(func() { SetDefaultEmbedClassificationGate(nil) })

	e, hits := newGateTestEmbedder(t) // 实例级未设 → 回落默认门
	ctx := WithEmbedClassification(context.Background(), 5, "")
	_, err := e.Embed(ctx, []string{"x"})
	var berr *EmbedClassificationBlockedError
	if !errors.As(err, &berr) {
		t.Fatalf("default gate must block L5, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("hits=%d", hits.Load())
	}

	// 清空默认门后放行。
	SetDefaultEmbedClassificationGate(nil)
	if _, err := e.Embed(WithEmbedClassification(context.Background(), 5, ""), []string{"x"}); err != nil {
		t.Fatalf("cleared gate must pass, got %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits=%d", hits.Load())
	}
}

func TestBuildIndexedChunks_BlockedDegradesToLexical(t *testing.T) {
	e, hits := newGateTestEmbedder(t)
	gate := &captureGate{blockAt: 3}
	e.SetClassificationGate(gate)

	chunks, err := BuildIndexedChunks(context.Background(), e, IngestParams{
		DocID:          "d1",
		CollectionID:   "c1",
		Text:           "包含秘密段落的文档正文",
		Classification: 4,
	}, nil)
	if err != nil {
		t.Fatalf("blocked embed must degrade, not fail ingest: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("chunks must still be produced")
	}
	for i, c := range chunks {
		if len(c.Embedding) != 0 {
			t.Fatalf("chunk %d must carry no embedding after degrade", i)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("hits=%d", hits.Load())
	}
	if len(gate.got) != 1 || gate.got[0].Classification != 4 {
		t.Fatalf("gate propagation = %+v", gate.got)
	}
	if gate.got[0].Subject != "d1" {
		t.Fatalf("subject must propagate doc id, got %q", gate.got[0].Subject)
	}
}

func TestBuildIndexedChunks_UnclassifiedEmbeds(t *testing.T) {
	e, hits := newGateTestEmbedder(t)
	gate := &captureGate{blockAt: 3}
	e.SetClassificationGate(gate)

	chunks, err := BuildIndexedChunks(context.Background(), e, IngestParams{
		DocID: "d2", CollectionID: "c1", Text: "普通文档", Classification: 0,
	}, nil)
	if err != nil {
		t.Fatalf("unclassified must embed: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits=%d", hits.Load())
	}
	for i, c := range chunks {
		if len(c.Embedding) != 3 {
			t.Fatalf("chunk %d embedding dim = %d", i, len(c.Embedding))
		}
	}
}
