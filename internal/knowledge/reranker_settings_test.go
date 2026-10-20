package knowledge

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"aranea-agents/internal/biz"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/reranker/cohere"
)

// U2-5: BuildRerankerWithSettings priority matrix — settings > env factory >
// bigram-Jaccard degrade; every degradation logs exactly once through logf.

// pinRerankEnv forces the env tier into a deterministic state for a test:
// kind "" = env factory off (falls to Jaccard), "topk" = env factory builds,
// any other value = env factory errors.
func pinRerankEnv(t *testing.T, kind string) {
	t.Helper()
	t.Setenv("KRATOS_KNOWLEDGE_RERANKER", kind)
	t.Setenv("KRATOS_KNOWLEDGE_RERANK_TOP_K", "")
	t.Setenv("COHERE_API_KEY", "")
	t.Setenv("COHERE_RERANK_MODEL", "")
	t.Setenv("COHERE_RERANK_ENDPOINT", "")
	t.Setenv("INFINITY_RERANK_ENDPOINT", "")
	t.Setenv("INFINITY_RERANK_MODEL", "")
	t.Setenv("INFINITY_RERANK_API_KEY", "")
}

type logCapture struct {
	entries []string
}

func (c *logCapture) logf(format string, args ...any) {
	c.entries = append(c.entries, fmt.Sprintf(format, args...))
}

func settingsGetter(s biz.KnowledgeRerankSettings, err error) func(context.Context) (KnowledgeRerankSettingsView, error) {
	return func(context.Context) (KnowledgeRerankSettingsView, error) {
		if err != nil {
			return KnowledgeRerankSettingsView{}, err
		}
		return KnowledgeRerankSettingsViewFromBiz(s), nil
	}
}

func isCrossEncoder(r biz.Reranker) bool {
	_, ok := r.(*biz.CrossEncoderReranker)
	return ok
}

func isKnowledgeAdapter(r biz.Reranker) bool {
	_, ok := r.(*KnowledgeRerankerAdapter)
	return ok
}

func TestBuildRerankerWithSettings_SettingsWinOverEnv(t *testing.T) {
	pinRerankEnv(t, "") // env off: a settings hit must come from tier 1.
	logs := &logCapture{}
	r := BuildRerankerWithSettings(settingsGetter(biz.KnowledgeRerankSettings{
		Provider: "topk", Enabled: true,
	}, nil), logs.logf)
	if !isKnowledgeAdapter(r) {
		t.Fatalf("enabled topk settings must build the framework adapter, got %T", r)
	}
	if len(logs.entries) != 0 {
		t.Fatalf("clean settings hit must not log, got %v", logs.entries)
	}
	// Smoke: adapter scores without a backend error (topk preserves input).
	if got := r.Score("alpha beta", "alpha beta"); got != 0 {
		t.Fatalf("topk passthrough score = %v, want 0 (zero input score)", got)
	}
}

func TestBuildRerankerWithSettings_CohereSettingsWinOverEnv(t *testing.T) {
	pinRerankEnv(t, "topk") // env WOULD build; settings must still win.
	logs := &logCapture{}
	r := BuildRerankerWithSettings(settingsGetter(biz.KnowledgeRerankSettings{
		Provider: "cohere", APIKey: "ck", Model: "rerank-v3.5", Enabled: true,
	}, nil), logs.logf)
	ka, ok := r.(*KnowledgeRerankerAdapter)
	if !ok {
		t.Fatalf("enabled cohere settings must build the framework adapter, got %T", r)
	}
	if _, isCohere := ka.reranker.(*cohere.Reranker); !isCohere {
		t.Fatalf("wrapped backend must be *cohere.Reranker (settings tier), got %T", ka.reranker)
	}
	if len(logs.entries) != 0 {
		t.Fatalf("clean settings hit must not log, got %v", logs.entries)
	}
}

func TestBuildRerankerWithSettings_InvalidProviderFallsToEnv(t *testing.T) {
	pinRerankEnv(t, "") // env off → Jaccard expected.
	logs := &logCapture{}
	r := BuildRerankerWithSettings(settingsGetter(biz.KnowledgeRerankSettings{
		Provider: "not-a-provider", Enabled: true,
	}, nil), logs.logf)
	if !isCrossEncoder(r) {
		t.Fatalf("invalid settings provider must degrade, got %T", r)
	}
	if len(logs.entries) != 1 {
		t.Fatalf("degradation must log exactly once, got %v", logs.entries)
	}
}

func TestBuildRerankerWithSettings_ConstructionErrorFallsToEnv(t *testing.T) {
	pinRerankEnv(t, "") // env off → Jaccard expected.
	logs := &logCapture{}
	// infinity.New errors on an empty endpoint: forces the construction-error
	// tier without touching the network.
	r := BuildRerankerWithSettings(settingsGetter(biz.KnowledgeRerankSettings{
		Provider: "infinity", BaseURL: "", Enabled: true,
	}, nil), logs.logf)
	if !isCrossEncoder(r) {
		t.Fatalf("construction failure must degrade, got %T", r)
	}
	if len(logs.entries) != 1 {
		t.Fatalf("construction failure must log exactly once, got %v", logs.entries)
	}
}

func TestBuildRerankerWithSettings_GetterErrorThenEnvBuilds(t *testing.T) {
	pinRerankEnv(t, "topk") // env ON: getter error degrades exactly one tier.
	logs := &logCapture{}
	r := BuildRerankerWithSettings(settingsGetter(biz.KnowledgeRerankSettings{}, errors.New("store down")), logs.logf)
	if !isKnowledgeAdapter(r) {
		t.Fatalf("env tier must still build after a getter error, got %T", r)
	}
	if len(logs.entries) != 1 {
		t.Fatalf("getter error must log exactly once, got %v", logs.entries)
	}
}

func TestBuildRerankerWithSettings_GetterErrorThenJaccard(t *testing.T) {
	pinRerankEnv(t, "") // env off.
	logs := &logCapture{}
	r := BuildRerankerWithSettings(settingsGetter(biz.KnowledgeRerankSettings{}, errors.New("store down")), logs.logf)
	if !isCrossEncoder(r) {
		t.Fatalf("getter error with env off must degrade to Jaccard, got %T", r)
	}
	if len(logs.entries) != 1 {
		t.Fatalf("getter error must log exactly once, got %v", logs.entries)
	}
}

func TestBuildRerankerWithSettings_DisabledSkipsToEnv(t *testing.T) {
	pinRerankEnv(t, "topk") // env ON.
	logs := &logCapture{}
	r := BuildRerankerWithSettings(settingsGetter(biz.KnowledgeRerankSettings{
		Provider: "cohere", APIKey: "ck", Enabled: false,
	}, nil), logs.logf)
	if !isKnowledgeAdapter(r) {
		t.Fatalf("disabled settings must fall through to the env tier, got %T", r)
	}
	if len(logs.entries) != 0 {
		t.Fatalf("disabled settings are not an error, must not log: %v", logs.entries)
	}
}

func TestBuildRerankerWithSettings_NilGetterUsesEnv(t *testing.T) {
	pinRerankEnv(t, "topk")
	logs := &logCapture{}
	r := BuildRerankerWithSettings(nil, logs.logf)
	if !isKnowledgeAdapter(r) {
		t.Fatalf("nil getter must use the env tier, got %T", r)
	}
	if len(logs.entries) != 0 {
		t.Fatalf("nil getter path must not log, got %v", logs.entries)
	}
}

func TestBuildRerankerWithSettings_EnvErrorDegradesToJaccard(t *testing.T) {
	pinRerankEnv(t, "garbage-kind") // env factory errors on unknown kinds.
	logs := &logCapture{}
	r := BuildRerankerWithSettings(nil, logs.logf)
	if !isCrossEncoder(r) {
		t.Fatalf("env factory error must degrade to Jaccard, got %T", r)
	}
	if len(logs.entries) != 1 {
		t.Fatalf("env factory error must log exactly once, got %v", logs.entries)
	}
}

func TestBuildRerankerWithSettings_NilLogfSafe(t *testing.T) {
	pinRerankEnv(t, "garbage-kind")
	r := BuildRerankerWithSettings(nil, nil)
	if !isCrossEncoder(r) {
		t.Fatalf("nil logf must still degrade, got %T", r)
	}
}

func TestKnowledgeRerankSettingsViewFromBiz(t *testing.T) {
	t.Parallel()
	in := biz.KnowledgeRerankSettings{
		Provider: "cohere", Model: "rerank-v3.5", BaseURL: "https://api.cohere.com",
		APIKey: "ck", Enabled: true, UpdatedAt: "2026-10-20T01:02:03Z", HasAPIKey: true,
	}
	got := KnowledgeRerankSettingsViewFromBiz(in)
	want := KnowledgeRerankSettingsView{
		Provider: "cohere", Model: "rerank-v3.5", BaseURL: "https://api.cohere.com",
		APIKey: "ck", Enabled: true, UpdatedAt: "2026-10-20T01:02:03Z", HasAPIKey: true,
	}
	if got != want {
		t.Fatalf("view must copy every field: got %#v want %#v", got, want)
	}
}
