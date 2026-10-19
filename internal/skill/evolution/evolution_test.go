package evolution

import (
	"context"
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

func TestResolve_DoesNotLatchTriedOnEmptyCatalog(t *testing.T) {
	catalog := biz.NewLlmProviderModelUsecase(stubModelReader{}, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	svc := NewService(Config{
		Catalog: catalog,
		RT:      &provider.RoundTrip{},
		Repo:    stubRepo{},
		Lg:      loggateway.NewNoop(),
	})
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if got := svc.resolve(context.Background()); got != nil {
		t.Fatalf("resolve with empty catalog = %v, want nil", got)
	}
	if svc.tried {
		t.Fatal("SKL-02: tried must remain false after failed resolve so Enqueue can retry")
	}
	_ = svc.resolve(context.Background())
	if svc.tried {
		t.Fatal("tried still false after second failed resolve")
	}
}

func TestResetBuild_ClearsTried(t *testing.T) {
	svc := NewService(Config{
		Catalog: biz.NewLlmProviderModelUsecase(stubModelReader{}, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop()),
		RT:      &provider.RoundTrip{},
		Repo:    stubRepo{},
		Lg:      loggateway.NewNoop(),
	})
	svc.tried = true
	if err := svc.ResetBuild(); err != nil {
		t.Fatal(err)
	}
	if svc.tried {
		t.Fatal("ResetBuild should clear tried")
	}
}

type stubModelReader struct{}

func (stubModelReader) ListProviderModels(context.Context) ([]biz.ProviderModel, error) {
	return nil, nil
}
func (stubModelReader) SearchProviderModels(context.Context, biz.ProviderModelListQuery) (biz.ProviderModelListResult, error) {
	return biz.ProviderModelListResult{}, nil
}
func (stubModelReader) GetProviderModel(context.Context, string) (biz.ProviderModel, error) {
	return biz.ProviderModel{}, nil
}
func (stubModelReader) GetProviderModelByProviderAndModel(context.Context, string, string) (biz.ProviderModel, error) {
	return biz.ProviderModel{}, nil
}

