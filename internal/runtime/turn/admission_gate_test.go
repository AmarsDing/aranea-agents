package turn

import (
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/runtime"
)

type gateTestLock struct{}

func (gateTestLock) LockSession(string) func() { return func() {} }

// gateSharedLock 模拟生产装配：AdmissionGate 的 SessionLocker 与
// ChatUsecase.EnqueueUserMessage 内部的 locker 是同一把非可重入互斥锁
// （chatUCSessionLocker → uc.LockSession → uc.locker.Lock）。
type gateSharedLock struct {
	mu *sync.Mutex
}

func newGateSharedLock() *gateSharedLock { return &gateSharedLock{mu: &sync.Mutex{}} }

func (l *gateSharedLock) LockSession(string) func() {
	l.mu.Lock()
	return l.mu.Unlock
}

// sharedLockEnqueue 模拟 ChatUsecase.EnqueueUserMessage：内部再次获取同一把会话锁。
type sharedLockEnqueue struct {
	lock      *gateSharedLock
	accepted  bool
	pendingID string
}

func (e sharedLockEnqueue) EnqueueUserMessage(string, string) (bool, string, string, error) {
	unlock := e.lock.LockSession("s1")
	defer unlock()
	return e.accepted, e.pendingID, "", nil
}

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

// TestAdmissionGate_enqueueDoesNotDeadlockWithSharedLock 复现生产死锁：
// 澄清挂起的 turn 保持 run+runner 注册，用户提交澄清后 resumeTurnWithClarification
// → Execute → Check 持会话锁 → tryEnqueue → EnqueueUserMessage 重入同一把锁，
// goroutine 永久阻塞（无任何日志）。修复后 Check 必须在释锁后再调用 Enqueue。
func TestAdmissionGate_enqueueDoesNotDeadlockWithSharedLock(t *testing.T) {
	shared := newGateSharedLock()
	reg := runtime.NewRunRegistry()
	reg.StoreRunner("s1", "run-1", admissionTestRunner{})
	g := NewAdmissionGate(AdmissionGateDeps{
		Runs:    RunRegistryAdapter{Registry: reg},
		Lock:    shared,
		Enqueue: sharedLockEnqueue{lock: shared, accepted: true, pendingID: "p-1"},
	})
	done := make(chan AdmissionVerdict, 1)
	go func() {
		done <- g.Check(biz.TurnInput{
			SessionID:   "s1",
			Content:     "follow up",
			EntryConfig: biz.TurnEntryPointConfig{AllowQueue: true},
		})
	}()
	select {
	case v := <-done:
		if v.Action != AdmissionQueued || v.PendingID != "p-1" {
			t.Fatalf("got=%+v", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Check deadlocked: enqueue re-acquired session lock held by gate")
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
