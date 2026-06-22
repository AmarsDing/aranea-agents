package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/runtime"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

func TestDecideTurnAdmission(t *testing.T) {
	cases := []struct {
		name   string
		active bool
		runner bool
		want   TurnAdmissionDecision
	}{
		{"idle", false, false, TurnAdmitNewTurn},
		{"running steer", true, true, TurnAdmitEnqueue},
		{"starting busy", true, false, TurnRejectBusy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DecideTurnAdmission(tc.active, tc.runner); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
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
	if got := DecideTurnAdmission(reg.HasActive("sess-1"), hasRunner); got != TurnRejectBusy {
		t.Fatalf("decision=%v want RejectBusy", got)
	}
}

func TestRunRegistry_ActiveRunnerAllowsEnqueue(t *testing.T) {
	reg := runtime.NewRunRegistry()
	reg.StoreRunner("sess-1", "run-1", &admissionTestRunner{})
	_, _, hasRunner := reg.ActiveRunner("sess-1")
	if !hasRunner {
		t.Fatal("expected ActiveRunner")
	}
	if got := DecideTurnAdmission(reg.HasActive("sess-1"), hasRunner); got != TurnAdmitEnqueue {
		t.Fatalf("decision=%v want Enqueue", got)
	}
}

func TestClassifyEnqueueOutcome(t *testing.T) {
	if !errors.Is(classifyEnqueueOutcome(true, "", nil), ErrTurnMessageQueued) {
		t.Fatal("expected queued sentinel")
	}
	if classifyEnqueueOutcome(false, biz.ChatEnqueueRejectQueueFull, nil) == nil {
		t.Fatal("expected queue full error")
	}
}

var _ trpcrunner.Runner = (*admissionTestRunner)(nil)
var _ trpcrunner.SteerableRunner = (*admissionTestRunner)(nil)

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
