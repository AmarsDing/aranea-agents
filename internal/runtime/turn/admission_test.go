package turn

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/runtime"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

func TestDecideAdmission(t *testing.T) {
	cases := []struct {
		name   string
		active bool
		runner bool
		want   biz.TurnAdmissionDecision
	}{
		{"idle", false, false, biz.AdmitRun},
		{"running steer", true, true, biz.AdmitEnqueue},
		{"starting busy", true, false, biz.AdmitReject},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DecideAdmission(tc.active, tc.runner); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestEvaluateAdmission_respectsAllowQueue(t *testing.T) {
	if got := EvaluateAdmission(true, true, false); got != biz.AdmitReject {
		t.Fatalf("channel busy should reject, got %v", got)
	}
	if got := EvaluateAdmission(true, true, true); got != biz.AdmitEnqueue {
		t.Fatalf("web steer should enqueue, got %v", got)
	}
}

func TestRunRegistry_StartingPhaseRejectsSecondTurn(t *testing.T) {
	reg := runtime.NewRunRegistry()
	reg.StoreCancelable("sess-1", "run-1", func() {})

	if !reg.HasActive("sess-1") {
		t.Fatal("expected active after StoreCancelable")
	}
	_, _, hasRunner := reg.ActiveRunner("sess-1")
	if hasRunner {
		t.Fatal("expected no ActiveRunner during starting phase")
	}
	if got := DecideAdmission(reg.HasActive("sess-1"), hasRunner); got != biz.AdmitReject {
		t.Fatalf("decision=%v want AdmitReject", got)
	}
}

func TestRunRegistry_ActiveRunnerAllowsEnqueue(t *testing.T) {
	reg := runtime.NewRunRegistry()
	reg.StoreRunner("sess-1", "run-1", admissionTestRunner{})
	_, _, hasRunner := reg.ActiveRunner("sess-1")
	if !hasRunner {
		t.Fatal("expected ActiveRunner")
	}
	if got := DecideAdmission(reg.HasActive("sess-1"), hasRunner); got != biz.AdmitEnqueue {
		t.Fatalf("decision=%v want AdmitEnqueue", got)
	}
}

type admissionTestRunner struct{}

func (admissionTestRunner) Run(context.Context, string, string, trpcmodel.Message, ...trpcagent.RunOption) (<-chan *trpcevent.Event, error) {
	return nil, nil
}
func (admissionTestRunner) Close() error { return nil }
func (admissionTestRunner) Cancel(string) bool {
	return true
}
func (admissionTestRunner) RunStatus(string) (trpcrunner.RunStatus, bool) {
	return trpcrunner.RunStatus{}, false
}
func (admissionTestRunner) EnqueueUserMessage(string, trpcmodel.Message) error {
	return nil
}
