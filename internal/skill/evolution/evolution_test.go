package evolution

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

func TestNewService_NilGuards(t *testing.T) {
	if got := NewService(Config{}); got != nil {
		t.Fatalf("NewService(empty) = %v, want nil", got)
	}
	if got := NewService(Config{Catalog: &biz.LlmProviderModelUsecase{}}); got != nil {
		t.Fatalf("NewService(missing RT/Repo) = %v, want nil", got)
	}
	if got := NewService(Config{
		Catalog: &biz.LlmProviderModelUsecase{},
		RT:      &provider.RoundTrip{},
		Repo:    stubRepo{},
	}); got == nil {
		t.Fatal("NewService(full) = nil, want non-nil")
	}
}

func TestResolveCandidatesDir(t *testing.T) {
	t.Setenv("EVOLUTION_CANDIDATES_DIR", "")
	if got := resolveCandidatesDir(`D:\custom\dir`); got != `D:\custom\dir` {
		t.Fatalf("configured dir = %q", got)
	}
	t.Setenv("EVOLUTION_CANDIDATES_DIR", `D:\env\dir`)
	if got := resolveCandidatesDir(""); got != `D:\env\dir` {
		t.Fatalf("env dir = %q", got)
	}
	t.Setenv("EVOLUTION_CANDIDATES_DIR", "")
	if got := resolveCandidatesDir(""); got == "" {
		t.Fatal("default dir empty")
	}
}

// 未触发 resolve 前 Close 必须为 no-op（底层 worker 未构建）。
func TestClose_BeforeResolve(t *testing.T) {
	svc := NewService(Config{
		Catalog: &biz.LlmProviderModelUsecase{},
		RT:      &provider.RoundTrip{},
		Repo:    stubRepo{},
		Lg:      loggateway.NewNoop(),
	})
	if err := svc.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

type stubRepo struct{}

func (stubRepo) Summaries() []trpcskill.Summary       { return nil }
func (stubRepo) Get(string) (*trpcskill.Skill, error) { return nil, nil }
func (stubRepo) Path(string) (string, error)          { return "", nil }
