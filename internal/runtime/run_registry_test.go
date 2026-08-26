package runtime

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

type registryRunner struct {
	cancelled   bool
	closed      bool
	cancelOK    bool
	cancelCalls int64
	closeCalls  int64
}

func (r *registryRunner) Run(context.Context, string, string, trpcmodel.Message, ...trpcagent.RunOption) (<-chan *trpcevent.Event, error) {
	return nil, nil
}

func (r *registryRunner) Close() error {
	atomic.AddInt64(&r.closeCalls, 1)
	r.closed = true
	return nil
}

func (r *registryRunner) Cancel(string) bool {
	atomic.AddInt64(&r.cancelCalls, 1)
	r.cancelled = true
	return r.cancelOK
}

func (r *registryRunner) RunStatus(string) (trpcrunner.RunStatus, bool) {
	return trpcrunner.RunStatus{}, false
}

type steerableRegistryRunner struct {
	registryRunner
	enqueuedRequestID string
	enqueuedContent   string
	enqueueErr        error
}

func (r *steerableRegistryRunner) EnqueueUserMessage(requestID string, message trpcmodel.Message) error {
	r.enqueuedRequestID = requestID
	r.enqueuedContent = message.Content
	return r.enqueueErr
}

func TestRunRegistryCancelManagedRunner(t *testing.T) {
	reg := NewRunRegistry()
	runner := &registryRunner{cancelOK: true}
	reg.StoreRunner("session-1", "run-1", runner)

	if stopped, _ := reg.Cancel("session-1", ""); !stopped {
		t.Fatalf("Cancel() = false, want true")
	}
	if !runner.cancelled {
		t.Fatalf("runner was not cancelled")
	}
	if runner.closed {
		t.Fatalf("runner was closed after successful managed cancel")
	}
}

func TestRunRegistryCancelFallsBackToClose(t *testing.T) {
	reg := NewRunRegistry()
	runner := &registryRunner{}
	reg.StoreRunner("session-1", "run-1", runner)

	if stopped, _ := reg.Cancel("session-1", ""); !stopped {
		t.Fatalf("Cancel() = false, want true")
	}
	if !runner.closed {
		t.Fatalf("runner was not closed after failed managed cancel")
	}
	if reg.HasActive("session-1") {
		t.Fatalf("run remains active after close fallback")
	}
}

func TestRunRegistryCancelableRunUpdatesStatus(t *testing.T) {
	reg := NewRunRegistry()
	cancelled := false
	reg.StoreCancelable("session-1", "run-1", func() { cancelled = true })

	if stopped, _ := reg.Cancel("session-1", ""); !stopped {
		t.Fatalf("Cancel() = false, want true")
	}
	if !cancelled {
		t.Fatalf("cancel func was not called")
	}
	status, ok := reg.GetStatus("session-1")
	if !ok || status.Status != biz.SessionRunPhaseCancelled || status.RunID != "run-1" {
		t.Fatalf("GetStatus() = (%+v, %v), want cancelled run-1", status, ok)
	}
}

// TestRunRegistryCancel_RecordsReasonInStatus 锁定 79-runtime-governance
// P2.5：Cancel 的 reason 记入状态条目 ErrMsg，终止来源（no_progress /
// team_token_budget_exceeded / user_cancel …）可被终态落库与前端轮询分列。
func TestRunRegistryCancel_RecordsReasonInStatus(t *testing.T) {
	reg := NewRunRegistry()
	reg.StoreCancelable("session-1", "run-1", func() {})

	if stopped, _ := reg.Cancel("session-1", "no_progress"); !stopped {
		t.Fatalf("Cancel() = false, want true")
	}
	status, ok := reg.GetStatus("session-1")
	if !ok {
		t.Fatalf("GetStatus() missing entry")
	}
	if status.Status != biz.SessionRunPhaseCancelled {
		t.Fatalf("status = %q, want cancelled", status.Status)
	}
	if status.ErrMsg != "no_progress" {
		t.Fatalf("ErrMsg = %q, want no_progress（reason 落入状态条目）", status.ErrMsg)
	}
}

func TestRunRegistryEnqueueUserMessage(t *testing.T) {
	reg := NewRunRegistry()
	runner := &steerableRegistryRunner{}
	reg.StoreRunner("session-1", "run-1", runner)

	enqueued, err := reg.EnqueueUserMessage("session-1", "hello")
	if err != nil {
		t.Fatalf("EnqueueUserMessage() error = %v", err)
	}
	if !enqueued {
		t.Fatalf("EnqueueUserMessage() enqueued = false, want true")
	}
	if runner.enqueuedRequestID != "session-1" || runner.enqueuedContent != "hello" {
		t.Fatalf("enqueued (%q, %q), want session-1 hello", runner.enqueuedRequestID, runner.enqueuedContent)
	}
}

func TestRunRegistryStoreRunnerPreservesCancel(t *testing.T) {
	reg := NewRunRegistry()
	cancelled := false
	reg.StoreCancelable("session-1", "run-outer", func() { cancelled = true })
	reg.StoreRunner("session-1", "run-inner", &registryRunner{})

	if stopped, _ := reg.Cancel("session-1", ""); !stopped {
		t.Fatalf("Cancel() = false, want true")
	}
	if !cancelled {
		t.Fatalf("cancel func was not preserved after StoreRunner")
	}
}

func TestRunRegistryActiveRunner(t *testing.T) {
	reg := NewRunRegistry()
	runner := &registryRunner{}
	reg.StoreRunner("session-1", "run-1", runner)

	got, runID, ok := reg.ActiveRunner("session-1")
	if !ok || got != runner || runID != "run-1" {
		t.Fatalf("ActiveRunner() = (%v, %q, %v), want runner run-1 true", got, runID, ok)
	}
	reg.StorePlaceholder("session-2")
	if _, _, ok := reg.ActiveRunner("session-2"); ok {
		t.Fatalf("ActiveRunner() on placeholder = true, want false")
	}
}

func TestRunRegistryEnqueueUserMessageFallsBackWhenUnsupported(t *testing.T) {
	reg := NewRunRegistry()
	reg.StoreRunner("session-1", "run-1", &registryRunner{})

	enqueued, err := reg.EnqueueUserMessage("session-1", "hello")
	if err != nil {
		t.Fatalf("EnqueueUserMessage() error = %v", err)
	}
	if enqueued {
		t.Fatalf("EnqueueUserMessage() enqueued = true, want false")
	}
}

// TestStoreRunner_ConcurrentNoTOCTOU verifies that concurrent StoreRunner
// calls on the same session cannot lose the cancel func registered by a
// prior StoreCancelable. Before T2.2, load+store was non-atomic, so a
// StoreRunner could overwrite the entry while a StoreCancelable was in
// flight, dropping the cancel reference.
//
// Run with -race to detect data races.
func TestStoreRunner_ConcurrentNoTOCTOU(t *testing.T) {
	const goroutines = 64
	reg := NewRunRegistry()

	// Seed with a cancel so StoreRunner must preserve it.
	cancelCalled := int64(0)
	reg.StoreCancelable("session-1", "run-seed", func() { atomic.AddInt64(&cancelCalled, 1) })

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			reg.StoreRunner("session-1", fmt.Sprintf("run-%d", idx), &registryRunner{})
		}(i)
	}
	wg.Wait()

	// After all concurrent stores, the cancel func must still be present.
	if stopped, _ := reg.Cancel("session-1", ""); !stopped {
		t.Fatalf("Cancel() = false, want true (cancel func lost during concurrent StoreRunner)")
	}
	if got := atomic.LoadInt64(&cancelCalled); got != 1 {
		t.Fatalf("cancel called %d times, want 1", got)
	}
}

// TestStoreCancelable_ConcurrentNoTOCTOU verifies that concurrent
// StoreCancelable calls on the same session cannot lose the runner
// registered by a prior StoreRunner. Before T2.2, load+store was
// non-atomic, so a StoreCancelable could overwrite the entry while a
// StoreRunner was in flight, dropping the runner reference.
//
// Run with -race to detect data races.
func TestStoreCancelable_ConcurrentNoTOCTOU(t *testing.T) {
	const goroutines = 64
	reg := NewRunRegistry()

	// Seed with a runner so StoreCancelable must preserve it.
	runner := &steerableRegistryRunner{}
	reg.StoreRunner("session-1", "run-seed", runner)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			reg.StoreCancelable("session-1", fmt.Sprintf("run-%d", idx), func() {})
		}(i)
	}
	wg.Wait()

	// After all concurrent stores, the runner must still be present and
	// usable for enqueue.
	enqueued, err := reg.EnqueueUserMessage("session-1", "hello")
	if err != nil {
		t.Fatalf("EnqueueUserMessage() error = %v (runner lost during concurrent StoreCancelable)", err)
	}
	if !enqueued {
		t.Fatalf("EnqueueUserMessage() enqueued = false, want true (runner lost)")
	}
	if runner.enqueuedContent != "hello" {
		t.Fatalf("enqueued content = %q, want hello", runner.enqueuedContent)
	}
}

// TestStoreRunnerAndStoreCancelable_MixedConcurrent verifies the most
// dangerous TOCTOU scenario: StoreRunner and StoreCallable racing on
// the same session from different goroutines. Both must end up visible
// in the final entry — neither may overwrite the other.
//
// Run with -race to detect data races.
func TestStoreRunnerAndStoreCancelable_MixedConcurrent(t *testing.T) {
	const iterations = 200
	reg := NewRunRegistry()

	cancelCalled := int64(0)
	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine A: repeatedly store runner.
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			reg.StoreRunner("session-1", fmt.Sprintf("run-a-%d", i), &registryRunner{})
		}
	}()

	// Goroutine B: repeatedly store cancel.
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			reg.StoreCancelable("session-1", fmt.Sprintf("run-b-%d", i), func() {
				atomic.AddInt64(&cancelCalled, 1)
			})
		}
	}()

	wg.Wait()

	// Final state: cancel must still be callable (not lost to StoreRunner).
	if stopped, _ := reg.Cancel("session-1", ""); !stopped {
		t.Fatalf("Cancel() = false, want true (cancel func lost during mixed concurrent stores)")
	}
	if got := atomic.LoadInt64(&cancelCalled); got != 1 {
		t.Fatalf("cancel called %d times, want 1", got)
	}
}

// TestRunRegistryCancel_DoubleCancelOnlyOnce verifies that concurrent Cancel
// calls on the same session are idempotent: the runner is cancelled exactly
// once and the active run is removed without double-close.
func TestRunRegistryCancel_DoubleCancelOnlyOnce(t *testing.T) {
	reg := NewRunRegistry()
	runner := &registryRunner{cancelOK: true}
	reg.StoreRunner("session-1", "run-1", runner)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reg.Cancel("session-1", "")
		}()
	}
	wg.Wait()

	if !runner.cancelled {
		t.Fatalf("runner was not cancelled")
	}
	if runner.closed {
		t.Fatalf("runner was closed after successful managed cancel")
	}
	if got := atomic.LoadInt64(&runner.cancelCalls); got != 1 {
		t.Fatalf("Cancel() called %d times, want 1", got)
	}
	if reg.HasActive("session-1") {
		t.Fatalf("run remains active after cancel")
	}
	status, ok := reg.GetStatus("session-1")
	if !ok || status.Status != biz.SessionRunPhaseCancelled || status.RunID != "run-1" {
		t.Fatalf("GetStatus() = (%+v, %v), want cancelled run-1", status, ok)
	}
}

// TestRunRegistryCancel_RaceWithFinish verifies that Cancel racing with Finish
// does not panic or double-close the runner.
func TestRunRegistryCancel_RaceWithFinish(t *testing.T) {
	reg := NewRunRegistry()
	runner := &registryRunner{}
	reg.StoreRunner("session-1", "run-1", runner)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		reg.Finish("session-1", "run-1")
	}()
	go func() {
		defer wg.Done()
		reg.Cancel("session-1", "")
	}()
	wg.Wait()

	if got := atomic.LoadInt64(&runner.closeCalls); got > 1 {
		t.Fatalf("runner.Close() called %d times, want at most 1", got)
	}
}

// TestRunRegistryCancel_DoubleCheckActive verifies that a cancel that loses the
// race with natural run completion does not delete a newer run for the same session.
func TestRunRegistryCancel_DoubleCheckActive(t *testing.T) {
	reg := NewRunRegistry()
	runner1 := &registryRunner{}
	reg.StoreRunner("session-1", "run-1", runner1)

	stopped, runID := reg.Cancel("session-1", "")
	if !stopped || runID != "run-1" {
		t.Fatalf("Cancel() = (%v, %q), want (true, run-1)", stopped, runID)
	}

	// A new run starts for the same session before the second Cancel arrives.
	runner2 := &registryRunner{cancelOK: true}
	reg.StoreRunner("session-1", "run-2", runner2)

	stopped, runID = reg.Cancel("session-1", "")
	if !stopped || runID != "run-2" {
		t.Fatalf("second Cancel() = (%v, %q), want (true, run-2)", stopped, runID)
	}
	if got := atomic.LoadInt64(&runner1.closeCalls); got != 1 {
		t.Fatalf("first runner.Close() = %d, want 1", got)
	}
	if got := atomic.LoadInt64(&runner2.cancelCalls); got != 1 {
		t.Fatalf("second runner.Cancel() = %d, want 1", got)
	}
}

// TestRunRegistryCancel_KeepsEntryUntilFinish verifies P1-01 fix: Cancel marks
// the entry as "cancelling" but does NOT delete it. HasActive continues to
// return true, blocking new turns until Finish is called by the Runner's defer.
//
// Regression: previously Cancel deleted the entry immediately, allowing a new
// turn to start before the old Runner had drained/exited.
func TestRunRegistryCancel_KeepsEntryUntilFinish(t *testing.T) {
	reg := NewRunRegistry()
	runner := &registryRunner{cancelOK: true}
	reg.StoreRunner("session-1", "run-1", runner)

	// Before Cancel: active.
	if !reg.HasActive("session-1") {
		t.Fatalf("HasActive = false before Cancel, want true")
	}

	stopped, runID := reg.Cancel("session-1", "")
	if !stopped || runID != "run-1" {
		t.Fatalf("Cancel() = (%v, %q), want (true, run-1)", stopped, runID)
	}
	if !runner.cancelled {
		t.Fatalf("runner.Cancel not called")
	}

	// After Cancel: entry must still be active (cancelling state) to block
	// new turns until the Runner's defer calls Finish.
	if !reg.HasActive("session-1") {
		t.Fatalf("HasActive = false after Cancel, want true (entry should be in cancelling state)")
	}

	// Status should be "cancelled".
	status, ok := reg.GetStatus("session-1")
	if !ok || status.Status != biz.SessionRunPhaseCancelled || status.RunID != "run-1" {
		t.Fatalf("GetStatus() = (%+v, %v), want cancelled run-1", status, ok)
	}

	// Second Cancel force-deletes the stuck cancelling entry (safety valve).
	stopped2, _ := reg.Cancel("session-1", "")
	if !stopped2 {
		t.Fatalf("second Cancel (safety valve) = false, want true")
	}
	if reg.HasActive("session-1") {
		t.Fatalf("HasActive = true after safety-valve Cancel, want false")
	}
}

// TestRunRegistryCancel_FinishReleasesCancellingEntry verifies that Finish
// (called by the Runner's defer) releases a cancelling entry, allowing new
// turns to start.
func TestRunRegistryCancel_FinishReleasesCancellingEntry(t *testing.T) {
	reg := NewRunRegistry()
	runner := &registryRunner{cancelOK: true}
	reg.StoreRunner("session-1", "run-1", runner)

	reg.Cancel("session-1", "")

	// Entry is in cancelling state.
	if !reg.HasActive("session-1") {
		t.Fatalf("HasActive = false after Cancel, want true")
	}

	// Runner's defer calls Finish.
	reg.Finish("session-1", "run-1")

	// Entry is now released.
	if reg.HasActive("session-1") {
		t.Fatalf("HasActive = true after Finish, want false")
	}
}

// TestRunRegistryCancel_CancelableFuncKeepsEntry verifies the cancelling
// behavior for Path 1 (context.CancelFunc), which is the most common path
// for chat/team turns.
func TestRunRegistryCancel_CancelableFuncKeepsEntry(t *testing.T) {
	reg := NewRunRegistry()
	cancelCalled := false
	reg.StoreCancelable("session-1", "run-1", func() { cancelCalled = true })

	stopped, _ := reg.Cancel("session-1", "")
	if !stopped {
		t.Fatalf("Cancel = false, want true")
	}
	if !cancelCalled {
		t.Fatalf("cancel func not called")
	}

	// Entry must remain (cancelling state) — not deleted.
	if !reg.HasActive("session-1") {
		t.Fatalf("HasActive = false after Cancel, want true (cancelling state)")
	}

	// Finish releases it.
	reg.Finish("session-1", "run-1")
	if reg.HasActive("session-1") {
		t.Fatalf("HasActive = true after Finish, want false")
	}
}

// TestRunRegistryFinish_StaleRunIDDoesNotDeleteNewEntry verifies C-09:
// Cancel → force Cancel → StoreRunner(new) → Finish(oldRunID) must not
// wipe the newer entry.
func TestRunRegistryFinish_StaleRunIDDoesNotDeleteNewEntry(t *testing.T) {
	reg := NewRunRegistry()
	oldRunner := &registryRunner{cancelOK: true}
	reg.StoreRunner("session-1", "run-old", oldRunner)

	// First Cancel marks cancelling; second force-deletes the stuck entry.
	if stopped, id := reg.Cancel("session-1", ""); !stopped || id != "run-old" {
		t.Fatalf("first Cancel() = (%v, %q), want (true, run-old)", stopped, id)
	}
	if stopped, _ := reg.Cancel("session-1", ""); !stopped {
		t.Fatal("force Cancel = false, want true")
	}
	if reg.HasActive("session-1") {
		t.Fatal("HasActive = true after force Cancel, want false")
	}

	newRunner := &registryRunner{cancelOK: true}
	reg.StoreRunner("session-1", "run-new", newRunner)
	if !reg.HasActive("session-1") {
		t.Fatal("HasActive = false after StoreRunner(new), want true")
	}

	// Old Runner's deferred Finish must not delete the new entry.
	reg.Finish("session-1", "run-old")
	if !reg.HasActive("session-1") {
		t.Fatal("HasActive = false after stale Finish(run-old), want true (run-new preserved)")
	}
	runner, runID, ok := reg.ActiveRunner("session-1")
	if !ok || runID != "run-new" || runner != newRunner {
		t.Fatalf("ActiveRunner() = (%v, %q, %v), want (newRunner, run-new, true)", runner, runID, ok)
	}

	// Matching Finish still clears the new entry.
	reg.Finish("session-1", "run-new")
	if reg.HasActive("session-1") {
		t.Fatal("HasActive = true after Finish(run-new), want false")
	}
}
