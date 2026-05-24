package turn

import (
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/runtime"
)

type gateTestLock struct{}

func (gateTestLock) LockSession(string) func() { return func() {} }

type gateTestEnqueue struct {
	accepted     bool
	pendingID    string
	rejectReason string
	err          error
}

func (e gateTestEnqueue) EnqueueUserMessage(string, string) (bool, string, string, error) {
	return e.accepted, e.pendingID, e.rejectReason, e.err
}

func TestAdmissionGate_proceedWhenIdle(t *testing.T) {
	g := NewAdmissionGate(AdmissionGateDeps{
		Runs: RunRegistryAdapter{Registry: runtime.NewRunRegistry()},
		Lock: gateTestLock{},
	})
	v := g.Check(biz.TurnInput{SessionID: "s1", Content: "hi"})
	if v.Action != AdmissionProceed {
		t.Fatalf("action=%v", v.Action)
	}
}

func TestAdmissionGate_rejectBusyDuringStarting(t *testing.T) {
	reg := runtime.NewRunRegistry()
	reg.StoreCancelable("s1", "run-1", func() {})
	g := NewAdmissionGate(AdmissionGateDeps{
		Runs: RunRegistryAdapter{Registry: reg},
		Lock: gateTestLock{},
	})
	v := g.Check(biz.TurnInput{
		SessionID: "s1",
		Content:   "hi",
		EntryConfig: biz.TurnEntryPointConfig{
			EntryPoint: biz.EntryPointWeb,
			AllowQueue: true,
		},
	})
	if v.Action != AdmissionRejectBusy {
		t.Fatalf("action=%v want reject busy", v.Action)
	}
}

func TestAdmissionGate_enqueueWhenSteerAllowed(t *testing.T) {
	reg := runtime.NewRunRegistry()
	reg.StoreRunner("s1", "run-1", admissionTestRunner{})
	g := NewAdmissionGate(AdmissionGateDeps{
		Runs: RunRegistryAdapter{Registry: reg},
		Lock: gateTestLock{},
		Enqueue: gateTestEnqueue{
			accepted:  true,
			pendingID: "p-1",
		},
	})
	v := g.Check(biz.TurnInput{
		SessionID: "s1",
		Content:   "follow up",
		EntryConfig: biz.TurnEntryPointConfig{
			EntryPoint: biz.EntryPointWS,
			AllowQueue: true,
		},
	})
	if v.Action != AdmissionQueued || v.PendingID != "p-1" {
		t.Fatalf("got=%+v", v)
	}
}

func TestAdmissionGate_channelRejectsWhenQueueDisabled(t *testing.T) {
	reg := runtime.NewRunRegistry()
	reg.StoreRunner("s1", "run-1", admissionTestRunner{})
	g := NewAdmissionGate(AdmissionGateDeps{
		Runs: RunRegistryAdapter{Registry: reg},
		Lock: gateTestLock{},
		Enqueue: gateTestEnqueue{
			accepted: true,
		},
	})
	v := g.Check(biz.TurnInput{
		SessionID: "s1",
		Content:   "hi",
		EntryConfig: biz.TurnEntryPointConfig{
			EntryPoint: biz.EntryPointChannel,
			AllowQueue: false,
		},
	})
	if v.Action != AdmissionRejectBusy {
		t.Fatalf("action=%v want reject for channel", v.Action)
	}
}

func TestAdmissionGate_enqueueRejectReason(t *testing.T) {
	reg := runtime.NewRunRegistry()
	reg.StoreRunner("s1", "run-1", admissionTestRunner{})
	g := NewAdmissionGate(AdmissionGateDeps{
		Runs: RunRegistryAdapter{Registry: reg},
		Lock: gateTestLock{},
		Enqueue: gateTestEnqueue{
			accepted:     false,
			rejectReason: biz.ChatEnqueueRejectQueueFull,
		},
	})
	v := g.Check(biz.TurnInput{
		SessionID:   "s1",
		Content:     "hi",
		EntryConfig: biz.TurnEntryPointConfig{AllowQueue: true},
	})
	if v.Action != AdmissionRejectEnqueue || v.RejectReason != biz.ChatEnqueueRejectQueueFull {
		t.Fatalf("got=%+v", v)
	}
}

func TestAdmissionGate_enqueueError(t *testing.T) {
	reg := runtime.NewRunRegistry()
	reg.StoreRunner("s1", "run-1", admissionTestRunner{})
	wantErr := errors.New("db down")
	g := NewAdmissionGate(AdmissionGateDeps{
		Runs: RunRegistryAdapter{Registry: reg},
		Lock: gateTestLock{},
		Enqueue: gateTestEnqueue{
			err: wantErr,
		},
	})
	v := g.Check(biz.TurnInput{
		SessionID:   "s1",
		Content:     "hi",
		EntryConfig: biz.TurnEntryPointConfig{AllowQueue: true},
	})
	if v.Action != AdmissionRejectEnqueue || !errors.Is(v.Err, wantErr) {
		t.Fatalf("got=%+v", v)
	}
}
