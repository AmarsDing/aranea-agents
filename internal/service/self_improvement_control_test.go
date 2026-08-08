package service

import (
	"testing"

	v1 "aranea-agents/api/kratos/self_improvement/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── ControlRun（R2/S1 用户介入入口） ────────────────────────────────────────

func siSvcWithControl(run *biz.SelfImprovementRun) (*SelfImprovementService, *biz.SIControlPlane) {
	cp := biz.NewSIControlPlane()
	svc := &SelfImprovementService{
		uc:      &siAdminPortFake{getRun: run},
		control: cp,
		lg:      loggateway.NewNoop(),
	}
	return svc, cp
}

// 合法指令 + 在途状态 → Issue 进控制面（异步消费，不同步改状态）。
func TestSelfImprovementService_ControlRunIssuesCommand(t *testing.T) {
	svc, cp := siSvcWithControl(&biz.SelfImprovementRun{ID: "run-1", Status: biz.RunStatusPatching})
	if _, err := svc.ControlRun(siSvcAdminCtx(), &v1.ControlRunRequest{Id: "run-1", Command: "pause"}); err != nil {
		t.Fatalf("ControlRun: %v", err)
	}
	cmd, ok := cp.Poll("run-1")
	if !ok || cmd != biz.SIControlPause {
		t.Fatalf("Poll = %q,%v, want pause,true", cmd, ok)
	}
}

// 非法指令 → BadRequest，不进控制面。
func TestSelfImprovementService_ControlRunBadCommand(t *testing.T) {
	svc, cp := siSvcWithControl(&biz.SelfImprovementRun{ID: "run-1", Status: biz.RunStatusPatching})
	if _, err := svc.ControlRun(siSvcAdminCtx(), &v1.ControlRunRequest{Id: "run-1", Command: "bogus"}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("err = %v, want BadRequest", err)
	}
	if _, ok := cp.Poll("run-1"); ok {
		t.Fatal("非法指令不得进控制面")
	}
}

// 非在途状态（无 poll 点，指令会滞留）→ Conflict。
func TestSelfImprovementService_ControlRunStatusConflict(t *testing.T) {
	for _, st := range []biz.SelfImprovementRunStatus{
		biz.RunStatusAwaitingGovernance, biz.RunStatusApplying, biz.RunStatusApplied,
		biz.RunStatusObserving, biz.RunStatusClosed, biz.RunStatusRejected,
	} {
		svc, _ := siSvcWithControl(&biz.SelfImprovementRun{ID: "run-1", Status: st})
		if _, err := svc.ControlRun(siSvcAdminCtx(), &v1.ControlRunRequest{Id: "run-1", Command: "pause"}); !apierror.IsCode(err, apierror.CodeConflict) {
			t.Fatalf("status %s: err = %v, want Conflict", st, err)
		}
	}
}

// run 不存在 → Get 的 NotFound 透传（防 bogus ID 残留控制面）。
func TestSelfImprovementService_ControlRunNotFound(t *testing.T) {
	cp := biz.NewSIControlPlane()
	svc := &SelfImprovementService{
		uc:      &siAdminPortFake{getErr: apierror.NotFound("SELF_IMPROVEMENT", "run nope not found")},
		control: cp,
		lg:      loggateway.NewNoop(),
	}
	if _, err := svc.ControlRun(siSvcAdminCtx(), &v1.ControlRunRequest{Id: "nope", Command: "pause"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("err = %v, want NotFound", err)
	}
	if _, ok := cp.Poll("nope"); ok {
		t.Fatal("不存在 run 不得进控制面")
	}
}

// 控制面未接线 → Unavailable（uc 在但 control nil 的降级形态）。
func TestSelfImprovementService_ControlRunNilPlane(t *testing.T) {
	svc := siSvcNew(&siAdminPortFake{getRun: &biz.SelfImprovementRun{ID: "run-1", Status: biz.RunStatusPatching}})
	if _, err := svc.ControlRun(siSvcAdminCtx(), &v1.ControlRunRequest{Id: "run-1", Command: "pause"}); !apierror.IsCode(err, apierror.CodeUnavailable) {
		t.Fatalf("err = %v, want Unavailable", err)
	}
}
