package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// D6-F4（87-orchestration-chain-hardening 批次 A3）：ProvideGraphUsecase 构造
// 后恰好触发一次 standalone 孤儿执行启动扫描（business 层窄端口，零 wire 改动）。
//
// 标注（规格契约）：本用例随 internal/service WIP（knowledge_asof_test.go 未入库
// 半成品）编译恢复后运行；当前以 `go build ./internal/service` 验证编译。
// ---------------------------------------------------------------------------

// stubSweepRunRepo 记录 GraphOrphanRunScanner 调用；不实现 GraphRunRepo 之外
// 的其他端口。
type stubSweepRunRepo struct {
	scans  int
	status string
	limit  int
}

var _ biz.GraphRunRepo = (*stubSweepRunRepo)(nil)
var _ biz.GraphOrphanRunScanner = (*stubSweepRunRepo)(nil)

func (s *stubSweepRunRepo) SaveRun(context.Context, *biz.GraphExecution) error { return nil }

func (s *stubSweepRunRepo) GetRun(context.Context, string) (*biz.GraphExecution, error) {
	return nil, biz.ErrNotFound
}

func (s *stubSweepRunRepo) ListRunsByGraph(context.Context, string, int, string, ...biz.GraphRunListOption) ([]*biz.GraphExecution, string, error) {
	return nil, "", nil
}

func (s *stubSweepRunRepo) UpdateRun(context.Context, *biz.GraphExecution) error { return nil }

func (s *stubSweepRunRepo) ListOrphanedStandaloneRuns(_ context.Context, status string, limit int) ([]*biz.GraphExecution, error) {
	s.scans++
	s.status = status
	s.limit = limit
	return nil, nil
}

// ProvideGraphUsecase 返回前必须恰好执行一次全量孤儿扫描（status=running）。
func TestProvideGraphUsecase_InvokesOrphanSweepOnce(t *testing.T) {
	runRepo := &stubSweepRunRepo{}
	uc := ProvideGraphUsecase(
		nil, runRepo, nil, nil, nil,
		NewGraphExecutionTelemetry(),
		NewGraphOrchestrationProjector(nil),
		loggateway.NewNoop(),
	)
	if uc == nil {
		t.Fatal("ProvideGraphUsecase returned nil")
	}
	defer uc.ExecUC().Close()

	if runRepo.scans != 1 {
		t.Fatalf("启动扫描必须恰好一次: scans=%d, want 1", runRepo.scans)
	}
	if runRepo.status != string(biz.GraphExecRunning) {
		t.Fatalf("扫描 status=%q, want %q", runRepo.status, biz.GraphExecRunning)
	}
	if runRepo.limit <= 0 {
		t.Fatalf("扫描 limit 必须为正: %d", runRepo.limit)
	}
}
