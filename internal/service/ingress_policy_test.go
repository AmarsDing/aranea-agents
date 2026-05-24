package service

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestEvaluateIngressPolicy_admitWhenIdle(t *testing.T) {
	got := EvaluateIngressPolicy(IngressPolicyInput{})
	if got.Decision != IngressAdmit {
		t.Fatalf("decision=%q", got.Decision)
	}
}

func TestEvaluateIngressPolicy_routeAsync(t *testing.T) {
	got := EvaluateIngressPolicy(IngressPolicyInput{RouteAsync: true})
	if got.Decision != IngressRouteAsync {
		t.Fatalf("decision=%q", got.Decision)
	}
}

func TestEvaluateIngressPolicy_cancel(t *testing.T) {
	got := EvaluateIngressPolicy(IngressPolicyInput{IsCancelCommand: true, RouteAsync: true})
	if got.Decision != IngressCancel {
		t.Fatalf("cancel should win over async, got %q", got.Decision)
	}
}

func TestEvaluateIngressPolicy_statusWhenBusy(t *testing.T) {
	got := EvaluateIngressPolicy(IngressPolicyInput{
		IsStatusQuery: true,
		HasActiveRun:  true,
	})
	if got.Decision != IngressStatus {
		t.Fatalf("decision=%q", got.Decision)
	}
}

func TestEvaluateIngressPolicy_webQueue(t *testing.T) {
	got := EvaluateIngressPolicy(IngressPolicyInput{
		EntryPoint:      biz.EntryPointWS,
		AllowQueue:      true,
		HasActiveRun:    true,
		HasActiveRunner: true,
	})
	if got.Decision != IngressQueue {
		t.Fatalf("decision=%q", got.Decision)
	}
}

func TestEvaluateIngressPolicy_channelRejectWhenBusy(t *testing.T) {
	got := EvaluateIngressPolicy(IngressPolicyInput{
		EntryPoint:      biz.EntryPointChannel,
		AllowQueue:      false,
		HasActiveRun:    true,
		HasActiveRunner: true,
	})
	if got.Decision != IngressRejectBusy {
		t.Fatalf("channel interrupt mode should reject when busy, got %q", got.Decision)
	}
}

func TestEvaluateIngressPolicy_channelSteerWhenQueueMode(t *testing.T) {
	got := EvaluateIngressPolicy(IngressPolicyInput{
		EntryPoint:      biz.EntryPointChannel,
		AllowQueue:      true,
		HasActiveRun:    true,
		HasActiveRunner: true,
	})
	if got.Decision != IngressSteer {
		t.Fatalf("channel queue mode should steer, got %q", got.Decision)
	}
}

func TestEvaluateIngressPolicy_routeBackground(t *testing.T) {
	got := EvaluateIngressPolicy(IngressPolicyInput{IsBackgroundCommand: true, RouteAsync: true})
	if got.Decision != IngressRouteBackground {
		t.Fatalf("background should win over async, got %q", got.Decision)
	}
}

func TestEvaluateIngressPolicy_channelRejectBusyStarting(t *testing.T) {
	got := EvaluateIngressPolicy(IngressPolicyInput{
		EntryPoint:      biz.EntryPointChannel,
		HasActiveRun:    true,
		HasActiveRunner: false,
	})
	if got.Decision != IngressRejectBusy {
		t.Fatalf("decision=%q", got.Decision)
	}
}

func TestEvaluateIngressPolicy_contextPressureChannelReject(t *testing.T) {
	got := EvaluateIngressPolicy(IngressPolicyInput{
		EntryPoint:      biz.EntryPointChannel,
		AllowQueue:      true,
		HasActiveRun:    true,
		HasActiveRunner: true,
		ContextPressure: true,
	})
	if got.Decision != IngressRejectBusy || got.Intent != "context_pressure" {
		t.Fatalf("got=%+v", got)
	}
}

func TestEvaluateIngressPolicy_contextPressureWebForceQueue(t *testing.T) {
	got := EvaluateIngressPolicy(IngressPolicyInput{
		EntryPoint:      biz.EntryPointWS,
		AllowQueue:      true,
		HasActiveRun:    true,
		HasActiveRunner: true,
		ContextPressure: true,
	})
	if got.Decision != IngressQueue || got.Intent != "context_force_queue" {
		t.Fatalf("got=%+v", got)
	}
}

func TestIngressPolicyFromTurnInput_legacyAllowsQueue(t *testing.T) {
	got := ingressPolicyFromTurnInput(biz.TurnInput{Content: "hi"}, true, true, false)
	if got.Decision != IngressQueue {
		t.Fatalf("legacy web should queue, got %q", got.Decision)
	}
}

func TestEvaluateIngressPolicy_skipDuplicate(t *testing.T) {
	got := EvaluateIngressPolicy(IngressPolicyInput{IsRecentDuplicate: true})
	if got.Decision != IngressSkipDuplicate {
		t.Fatalf("decision=%q", got.Decision)
	}
}

func TestResolveChannelAcceptRoute_async(t *testing.T) {
	cfg := biz.ParseChannelLongTaskConfig(`{"config":{"execution_mode":"auto","async_graph_id":"g1"}}`)
	got := ResolveChannelAcceptRoute("/async summarize", cfg, false)
	if got.Decision != IngressRouteAsync {
		t.Fatalf("decision=%q", got.Decision)
	}
}

func TestResolveChannelAcceptRoute_suggestDurable(t *testing.T) {
	cfg := biz.ParseChannelLongTaskConfig(`{"config":{"execution_mode":"sync","async_graph_id":"g1"}}`)
	got := ResolveChannelAcceptRoute("请做全量研报分析", cfg, false)
	if got.Decision != IngressAdmit || !got.SuggestDurable {
		t.Fatalf("got=%+v", got)
	}
}

func TestChannelAcceptOutcomeFromRoute(t *testing.T) {
	if !channelAcceptOutcomeFromRoute(IngressPolicyResult{Decision: IngressRouteAsync}).DispatchAsync {
		t.Fatal("expected async")
	}
	if !channelAcceptOutcomeFromRoute(IngressPolicyResult{Decision: IngressAdmit}).ExecuteSync {
		t.Fatal("expected sync")
	}
}
