package runtime

import (
	"context"
	"testing"

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

	if !reg.Cancel("session-1") {
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

	if !reg.Cancel("session-1") {
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

	if !reg.Cancel("session-1") {
		t.Fatalf("Cancel() = false, want true")
	}
	if !cancelled {
		t.Fatalf("cancel func was not called")
	}
	status, ok := reg.GetStatus("session-1")
	if !ok || status.Status != "cancelled" || status.RunID != "run-1" {
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

	if !reg.Cancel("session-1") {
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
