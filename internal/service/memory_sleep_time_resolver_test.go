package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

// fakeSleepTimeAgentGetter implements the narrow agent-getter interface used
// by SleepTimeLLMResolver.
type fakeSleepTimeAgentGetter struct {
	ag     biz.Agent
	err    error
	gotIDs []string
}

func (f *fakeSleepTimeAgentGetter) Get(_ context.Context, id string) (biz.Agent, error) {
	f.gotIDs = append(f.gotIDs, id)
	if f.err != nil {
		return biz.Agent{}, f.err
	}
	return f.ag, nil
}

// fakeSleepTimeCatalog implements biz.TeamModelCatalog for testing.
type fakeSleepTimeCatalog struct {
	err      error
	gotProv  string
	gotModel string
	calls    int
}

func (f *fakeSleepTimeCatalog) GetByProviderAndModel(_ context.Context, provider, model string) (biz.ProviderModel, error) {
	f.calls++
	f.gotProv = provider
	f.gotModel = model
	if f.err != nil {
		return biz.ProviderModel{}, f.err
	}
	return biz.ProviderModel{}, nil
}

func (f *fakeSleepTimeCatalog) List(_ context.Context) ([]biz.ProviderModel, error) {
	return nil, nil
}

func TestSleepTimeLLMResolver_ResolveProviderModel_PrefersMemoryWorker(t *testing.T) {
	getter := &fakeSleepTimeAgentGetter{ag: biz.Agent{
		Provider: "openai",
		Model:    "gpt-4",
		Settings: &biz.AgentRuntimeSettings{
			MemoryWorkerProvider: "anthropic",
			MemoryWorkerModel:    "claude-3-haiku",
		},
	}}
	r := NewSleepTimeLLMResolver(getter, &fakeSleepTimeCatalog{}, nil, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	prov, mod, err := r.ResolveProviderModel(context.Background(), uk)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if prov != "anthropic" || mod != "claude-3-haiku" {
		t.Fatalf("got %s/%s", prov, mod)
	}
	if len(getter.gotIDs) != 1 || getter.gotIDs[0] != "agent-1" {
		t.Fatalf("expected agent lookup with AppName agent-1, got %v", getter.gotIDs)
	}
}

func TestSleepTimeLLMResolver_ResolveProviderModel_FallsBackToAgentDefault(t *testing.T) {
	getter := &fakeSleepTimeAgentGetter{ag: biz.Agent{Provider: "openai", Model: "gpt-4"}}
	r := NewSleepTimeLLMResolver(getter, &fakeSleepTimeCatalog{}, nil, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	prov, mod, err := r.ResolveProviderModel(context.Background(), uk)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if prov != "openai" || mod != "gpt-4" {
		t.Fatalf("got %s/%s", prov, mod)
	}
}

func TestSleepTimeLLMResolver_ResolveProviderModel_AgentGetError(t *testing.T) {
	getter := &fakeSleepTimeAgentGetter{err: errors.New("agent not found")}
	r := NewSleepTimeLLMResolver(getter, &fakeSleepTimeCatalog{}, nil, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "missing", UserID: "user-1"}
	if _, _, err := r.ResolveProviderModel(context.Background(), uk); err == nil {
		t.Fatal("expected error when agent lookup fails")
	}
}

func TestSleepTimeLLMResolver_ResolveLLM_NilWhenAgentUnresolvable(t *testing.T) {
	getter := &fakeSleepTimeAgentGetter{err: errors.New("agent not found")}
	catalog := &fakeSleepTimeCatalog{}
	r := NewSleepTimeLLMResolver(getter, catalog, nil, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "missing", UserID: "user-1"}
	if m := r.ResolveLLM(context.Background(), uk); m != nil {
		t.Fatalf("expected nil model, got %v", m)
	}
	if catalog.calls != 0 {
		t.Fatalf("expected catalog not to be called, got %d calls", catalog.calls)
	}
}

func TestSleepTimeLLMResolver_ResolveLLM_NilWhenNoModelConfigured(t *testing.T) {
	getter := &fakeSleepTimeAgentGetter{ag: biz.Agent{}}
	catalog := &fakeSleepTimeCatalog{}
	r := NewSleepTimeLLMResolver(getter, catalog, nil, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if m := r.ResolveLLM(context.Background(), uk); m != nil {
		t.Fatalf("expected nil model, got %v", m)
	}
	if catalog.calls != 0 {
		t.Fatalf("expected catalog not to be called when provider/model empty, got %d calls", catalog.calls)
	}
}

func TestSleepTimeLLMResolver_ResolveLLM_NilWhenCatalogFails(t *testing.T) {
	getter := &fakeSleepTimeAgentGetter{ag: biz.Agent{Provider: "openai", Model: "gpt-4"}}
	catalog := &fakeSleepTimeCatalog{err: errors.New("catalog unavailable")}
	r := NewSleepTimeLLMResolver(getter, catalog, &provider.RoundTrip{}, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if m := r.ResolveLLM(context.Background(), uk); m != nil {
		t.Fatalf("expected nil model on catalog failure, got %v", m)
	}
	if catalog.calls != 1 || catalog.gotProv != "openai" || catalog.gotModel != "gpt-4" {
		t.Fatalf("expected catalog called with openai/gpt-4, got %d calls (%s/%s)", catalog.calls, catalog.gotProv, catalog.gotModel)
	}
}
