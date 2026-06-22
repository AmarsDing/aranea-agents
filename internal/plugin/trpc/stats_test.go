package plugintrpc

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type statsRepoStub struct {
	key   string
	count int
}

func (s *statsRepoStub) SearchPlugins(context.Context, biz.PluginListQuery) (biz.PluginListResult, error) {
	return biz.PluginListResult{}, nil
}
func (s *statsRepoStub) GetPlugin(context.Context, string) (biz.Plugin, error) {
	return biz.Plugin{}, nil
}
func (s *statsRepoStub) GetByKey(context.Context, string) (biz.Plugin, error) {
	return biz.Plugin{}, nil
}
func (s *statsRepoStub) CreatePlugin(context.Context, biz.Plugin) (biz.Plugin, error) {
	return biz.Plugin{}, nil
}
func (s *statsRepoStub) UpdatePluginEnabled(context.Context, string, bool) (biz.Plugin, error) {
	return biz.Plugin{}, nil
}
func (s *statsRepoStub) UpdatePluginConfig(context.Context, string, string) (biz.Plugin, error) {
	return biz.Plugin{}, nil
}
func (s *statsRepoStub) UpdateSortOrder(context.Context, string, int) (biz.Plugin, error) {
	return biz.Plugin{}, nil
}
func (s *statsRepoStub) UpdatePluginScope(context.Context, string, string) (biz.Plugin, error) {
	return biz.Plugin{}, nil
}
func (s *statsRepoStub) IncrementStats(_ context.Context, pluginKey string, delta biz.PluginStatUpdate) error {
	if pluginKey == s.key {
		s.count += delta.InvokeCount
	}
	return nil
}

func (s *statsRepoStub) Record(context.Context, string, string, string) {}

func TestRepoStatsRecorder_IncrementStats(t *testing.T) {
	repo := &statsRepoStub{key: "audit_log"}
	if err := repo.IncrementStats(context.Background(), "audit_log", biz.PluginStatUpdate{
		InvokeCount: 1,
		LastStatus:  "ok",
	}); err != nil {
		t.Fatal(err)
	}
	if repo.count != 1 {
		t.Fatalf("count=%d", repo.count)
	}
	rec := NewRepoStatsRecorder(repo, nil, false, loggateway.NewNoop())
	defer rec.Close()
	rec.Record(context.Background(), "audit_log", "after_tool", "ok")
}

func TestBuiltinCallbackPoints_auditLog(t *testing.T) {
	pts := BuiltinCallbackPoints("audit_log")
	if len(pts) < 5 {
		t.Fatalf("pts=%v", pts)
	}
}
