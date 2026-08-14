package service

import (
	"context"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/evolution/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// ── P3 M5-API：平台级进化多样性观测端点 ─────────────────────────────────────

type fakeDiversityReader struct {
	stats    []biz.EvolutionDiversitySourceStat
	gotSince time.Time
	gotTop   int
}

func (f *fakeDiversityReader) GetDiversityOverview(_ context.Context, since time.Time, topTools int) ([]biz.EvolutionDiversitySourceStat, error) {
	f.gotSince = since
	f.gotTop = topTools
	return f.stats, nil
}

// adminCtx returns a context carrying an admin principal.
func adminCtx() context.Context {
	return auth.NewContext(context.Background(), &auth.Auth{UserID: 1, Access: "admin"})
}

// since 缺省时 service 给默认窗口（最近 24h）；topTools<=0 透传由 data 层默认。
func TestEvolutionService_GetDiversityOverview_DefaultWindow(t *testing.T) {
	fake := &fakeDiversityReader{}
	svc := NewEvolutionService(fake, loggateway.NewNoop())

	before := time.Now()
	resp, err := svc.GetEvolutionDiversityOverview(adminCtx(), &v1.GetEvolutionDiversityOverviewRequest{})
	if err != nil {
		t.Fatalf("rpc: %v", err)
	}
	if resp == nil {
		t.Fatal("resp must not be nil")
	}
	// 默认窗口：since ≈ now-24h（允许 1 分钟执行偏差）。
	want := before.Add(-24 * time.Hour)
	if fake.gotSince.Before(want.Add(-time.Minute)) || fake.gotSince.After(time.Now().Add(-23*time.Hour)) {
		t.Fatalf("default since=%v, want ≈%v", fake.gotSince, want)
	}
}

// 显式参数透传 + biz stat → proto bucket 转换。
func TestEvolutionService_GetDiversityOverview_Mapping(t *testing.T) {
	latest := time.Now().UTC().Truncate(time.Second)
	fake := &fakeDiversityReader{stats: []biz.EvolutionDiversitySourceStat{
		{TriggerSource: "pattern", Count: 3, LatestAt: latest, TopTools: []string{"shell_exec", "query_db"}},
		{TriggerSource: "case_distill", Count: 1, LatestAt: latest},
	}}
	svc := NewEvolutionService(fake, loggateway.NewNoop())

	since := latest.Add(-48 * time.Hour)
	resp, err := svc.GetEvolutionDiversityOverview(adminCtx(), &v1.GetEvolutionDiversityOverviewRequest{
		Since:    timestamppb.New(since),
		TopTools: 7,
	})
	if err != nil {
		t.Fatalf("rpc: %v", err)
	}
	if !fake.gotSince.Equal(since) || fake.gotTop != 7 {
		t.Fatalf("passthrough: since=%v top=%d", fake.gotSince, fake.gotTop)
	}
	if len(resp.GetBuckets()) != 2 {
		t.Fatalf("buckets=%d", len(resp.GetBuckets()))
	}
	b0 := resp.GetBuckets()[0]
	if b0.GetTriggerSource() != "pattern" || b0.GetCount() != 3 || len(b0.GetTopTools()) != 2 || b0.GetTopTools()[0] != "shell_exec" {
		t.Fatalf("bucket0=%+v", b0)
	}
	if b0.GetLatestAt().AsTime().Unix() != latest.Unix() {
		t.Fatalf("latest_at=%v want %v", b0.GetLatestAt().AsTime(), latest)
	}
}

// reader 为 nil（未装配）时返回 Unavailable 而非 panic。
func TestEvolutionService_GetDiversityOverview_NilReader(t *testing.T) {
	svc := NewEvolutionService(nil, loggateway.NewNoop())
	_, err := svc.GetEvolutionDiversityOverview(adminCtx(), &v1.GetEvolutionDiversityOverviewRequest{})
	if err == nil {
		t.Fatal("nil reader must return error")
	}
}

// 非 admin / 非 system 调用方必须被 Forbidden 拒绝（P1-4）。
func TestEvolutionService_GetDiversityOverview_NonAdminDenied(t *testing.T) {
	fake := &fakeDiversityReader{}
	svc := NewEvolutionService(fake, loggateway.NewNoop())

	// 无 auth 信息 → Forbidden。
	_, err := svc.GetEvolutionDiversityOverview(context.Background(), &v1.GetEvolutionDiversityOverviewRequest{})
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("no-auth must return Forbidden, got %v", err)
	}

	// 普通用户（非 admin）→ Forbidden。
	userCtx := auth.NewContext(context.Background(), &auth.Auth{UserID: 2, Access: "user"})
	_, err = svc.GetEvolutionDiversityOverview(userCtx, &v1.GetEvolutionDiversityOverviewRequest{})
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("non-admin must return Forbidden, got %v", err)
	}
}

// system 调用方（cron/内部）可正常访问。
func TestEvolutionService_GetDiversityOverview_SystemAllowed(t *testing.T) {
	fake := &fakeDiversityReader{}
	svc := NewEvolutionService(fake, loggateway.NewNoop())

	sysCtx := workspace.WithSystemWorkspace(context.Background())
	resp, err := svc.GetEvolutionDiversityOverview(sysCtx, &v1.GetEvolutionDiversityOverviewRequest{})
	if err != nil {
		t.Fatalf("system caller must succeed, got %v", err)
	}
	if resp == nil {
		t.Fatal("resp must not be nil")
	}
}
