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
	cancelled bool
	closed    bool
	cancelOK  bool
}

func (r *registryRunner) Run(context.Context, string, string, trpcmodel.Message, ...trpcagent.RunOption) (<-chan *trpcevent.Event, error) {
	return nil, nil
}

func (r *registryRunner) Close() error {
	r.closed = true
	return nil
}

func (r *registryRunner) Cancel(string) bool {
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
