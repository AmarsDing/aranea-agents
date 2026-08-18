//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package runner

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/memory"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/session"
	sessioninmemory "trpc.group/trpc-go/trpc-agent-go/session/inmemory"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// mockMemoryServiceForAutoMemory implements memory.Service for testing auto memory.
type mockMemoryServiceForAutoMemory struct {
	enqueueCalled bool
	enqueueCount  atomic.Int32
	enqueueErr    error
	sess          *session.Session
}

type mockIngestor struct {
	enqueueCalled bool
	enqueueErr    error
	sess          *session.Session
	lastOptions   session.IngestOptions
}

type mockMemoryReaderIngestor struct {
	mockIngestor
	readCalled bool
}

func (m *mockIngestor) IngestSession(
	ctx context.Context,
	sess *session.Session,
	opts ...session.IngestOption,
) error {
	m.enqueueCalled = true
	m.sess = sess
	m.lastOptions = session.IngestOptions{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&m.lastOptions)
	}
	return m.enqueueErr
}

func (m *mockMemoryReaderIngestor) ReadMemories(
	ctx context.Context,
	userKey memory.UserKey,
	limit int,
) ([]*memory.Entry, error) {
	m.readCalled = true
	return nil, nil
}

func (m *mockMemoryReaderIngestor) SearchMemories(
	ctx context.Context,
	userKey memory.UserKey,
	query string,
	_ ...memory.SearchOption,
) ([]*memory.Entry, error) {
	return nil, nil
}

func (m *mockMemoryServiceForAutoMemory) AddMemory(ctx context.Context, userKey memory.UserKey, memoryStr string, topics []string, _ ...memory.AddOption) error {
	return nil
}

func (m *mockMemoryServiceForAutoMemory) UpdateMemory(ctx context.Context, memoryKey memory.Key, memoryStr string, topics []string, _ ...memory.UpdateOption) error {
	return nil
}

func (m *mockMemoryServiceForAutoMemory) DeleteMemory(ctx context.Context, memoryKey memory.Key) error {
	return nil
}

func (m *mockMemoryServiceForAutoMemory) ClearMemories(ctx context.Context, userKey memory.UserKey) error {
	return nil
}

func (m *mockMemoryServiceForAutoMemory) ReadMemories(ctx context.Context, userKey memory.UserKey, limit int) ([]*memory.Entry, error) {
	return nil, nil
}

func (m *mockMemoryServiceForAutoMemory) SearchMemories(ctx context.Context, userKey memory.UserKey, query string, _ ...memory.SearchOption) ([]*memory.Entry, error) {
	return nil, nil
}

func (m *mockMemoryServiceForAutoMemory) Tools() []tool.Tool {
	return nil
}

func (m *mockMemoryServiceForAutoMemory) EnqueueAutoMemoryJob(ctx context.Context, sess *session.Session) error {
	m.enqueueCalled = true
	m.enqueueCount.Add(1)
	m.sess = sess
	return m.enqueueErr
}

func (m *mockMemoryServiceForAutoMemory) Close() error {
	return nil
}

func (m *mockMemoryServiceForAutoMemory) ProactiveRecall(ctx context.Context, userKey memory.UserKey, convCtx memory.ConversationContext) ([]*memory.Entry, error) {
	return nil, nil
}

func TestEnqueueAutoMemoryJob(t *testing.T) {
	t.Run("nil memory service", func(t *testing.T) {
		r := &runner{memoryService: nil}
		sess := session.NewSession("app", "user", "sess")
		// Should not panic with nil memory service.
		r.enqueueAutoMemoryJob(context.Background(), sess)
	})

	t.Run("nil session", func(t *testing.T) {
		mockSvc := &mockMemoryServiceForAutoMemory{}
		r := &runner{memoryService: mockSvc}
		// Should not panic with nil session.
		r.enqueueAutoMemoryJob(context.Background(), nil)
		require.False(t, mockSvc.enqueueCalled)
	})

	t.Run("enqueues job with session", func(t *testing.T) {
		mockSvc := &mockMemoryServiceForAutoMemory{}
		r := &runner{memoryService: mockSvc}
		sess := session.NewSession("app", "user", "sess")
		r.enqueueAutoMemoryJob(context.Background(), sess)
		require.True(t, mockSvc.enqueueCalled)
		require.Same(t, sess, mockSvc.sess)
	})

	t.Run("handles enqueue error gracefully", func(t *testing.T) {
		mockSvc := &mockMemoryServiceForAutoMemory{enqueueErr: errors.New("queue full")}
		r := &runner{memoryService: mockSvc}
		sess := session.NewSession("app", "user", "sess")
		// Should not panic even if enqueue fails.
		r.enqueueAutoMemoryJob(context.Background(), sess)
		require.True(t, mockSvc.enqueueCalled)
	})
}

func TestRunner_WithMemoryService_AutoMemoryIntegration(t *testing.T) {
	mockMemSvc := &mockMemoryServiceForAutoMemory{}
	sessSvc := sessioninmemory.NewSessionService()
	mockAgent := &mockAgent{name: "test-agent"}

	r := NewRunner("test-app", mockAgent,
		WithSessionService(sessSvc),
		WithMemoryService(mockMemSvc),
	)

	ctx := context.Background()
	eventCh, err := r.Run(ctx, "user", "session", model.NewUserMessage("hello"))
	require.NoError(t, err)

	for range eventCh {
	}

	require.True(t, mockMemSvc.enqueueCalled)
	require.NotNil(t, mockMemSvc.sess)
	require.Equal(t, "test-app", mockMemSvc.sess.AppName)
	require.Equal(t, "user", mockMemSvc.sess.UserID)
}

func TestResolveMemoryReader(t *testing.T) {
	t.Run("memory service wins", func(t *testing.T) {
		mockMemSvc := &mockMemoryServiceForAutoMemory{}
		mockIngestor := &mockMemoryReaderIngestor{}

		reader := resolveMemoryReader(mockMemSvc, mockIngestor)

		require.Same(t, mockMemSvc, reader)
	})

	t.Run("reader ingestor is used without memory service", func(t *testing.T) {
		mockIngestor := &mockMemoryReaderIngestor{}

		reader := resolveMemoryReader(nil, mockIngestor)

		require.Same(t, mockIngestor, reader)
	})

	t.Run("plain ingestor returns nil", func(t *testing.T) {
		reader := resolveMemoryReader(nil, &mockIngestor{})

		require.Nil(t, reader)
	})
}

func TestRunner_NewRunInvocationSetsMemoryReader(t *testing.T) {
	t.Run("from memory service", func(t *testing.T) {
		mockMemSvc := &mockMemoryServiceForAutoMemory{}
		mockIngestor := &mockMemoryReaderIngestor{}
		r := &runner{
			sessionService: sessioninmemory.NewSessionService(),
			memoryService:  mockMemSvc,
			ingestor:       mockIngestor,
		}

		inv := r.newRunInvocation(
			session.NewSession("app", "user", "session"),
			model.NewUserMessage("hello"),
			&mockAgent{name: "test-agent"},
			agent.RunOptions{},
			"app",
			"",
			"",
		)

		require.Same(t, mockMemSvc, inv.MemoryReader)
		require.Same(t, mockMemSvc, inv.MemoryService)
	})

	t.Run("from reader ingestor", func(t *testing.T) {
		mockIngestor := &mockMemoryReaderIngestor{}
		r := &runner{
			sessionService: sessioninmemory.NewSessionService(),
			ingestor:       mockIngestor,
		}

		inv := r.newRunInvocation(
			session.NewSession("app", "user", "session"),
			model.NewUserMessage("hello"),
			&mockAgent{name: "test-agent"},
			agent.RunOptions{},
			"app",
			"",
			"",
		)

		require.Same(t, mockIngestor, inv.MemoryReader)
		require.Nil(t, inv.MemoryService)
	})
}

func TestRunner_WithSessionIngestor_Integration(t *testing.T) {
	mockIngestor := &mockIngestor{}
	sessSvc := sessioninmemory.NewSessionService()
	mockAgent := &mockAgent{name: "test-agent"}

	r := NewRunner("test-app", mockAgent,
		WithSessionService(sessSvc),
		WithSessionIngestor(mockIngestor),
	)

	ctx := context.Background()
	eventCh, err := r.Run(ctx, "user", "session", model.NewUserMessage("hello"))
	require.NoError(t, err)

	for range eventCh {
	}

	require.True(t, mockIngestor.enqueueCalled)
	require.NotNil(t, mockIngestor.sess)
	require.Equal(t, "test-app", mockIngestor.sess.AppName)
	require.Equal(t, "user", mockIngestor.sess.UserID)
	require.Equal(t, "session", mockIngestor.lastOptions.RunID)
	require.Equal(t, "test-agent", mockIngestor.lastOptions.AgentID)
}

// resolveIngestOpts is a small test helper that applies IngestOption values to
// a zero-value IngestOptions, mirroring what an Ingestor implementation does.
func resolveIngestOpts(opts ...session.IngestOption) session.IngestOptions {
	var got session.IngestOptions
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&got)
	}
	return got
}

func TestRunner_DefaultIngestOptions_PrefersInvocationAgent(t *testing.T) {
	r := &runner{defaultAgentName: "fallback-agent"}
	sess := session.NewSession("app", "user", "sess-id")

	withInv := resolveIngestOpts(r.defaultIngestOptions(sess, &agent.Invocation{AgentName: "live-agent"})...)
	require.Equal(t, "sess-id", withInv.RunID)
	require.Equal(t, "live-agent", withInv.AgentID)

	withoutInv := resolveIngestOpts(r.defaultIngestOptions(sess, nil)...)
	require.Equal(t, "sess-id", withoutInv.RunID)
	require.Equal(t, "fallback-agent", withoutInv.AgentID)
}

// multiEventMockAgent emits a configurable number of distinct response events.
// It is used to simulate multi-step runs for mid-run memory extraction tests.
type multiEventMockAgent struct {
	name      string
	numEvents int
}

func (m *multiEventMockAgent) Info() agent.Info {
	return agent.Info{Name: m.name, Description: "Multi-event mock agent for mid-run memory tests"}
}

func (m *multiEventMockAgent) SubAgents() []agent.Agent { return nil }

func (m *multiEventMockAgent) FindSubAgent(string) agent.Agent { return nil }

func (m *multiEventMockAgent) Tools() []tool.Tool { return nil }

func (m *multiEventMockAgent) Run(_ context.Context, invocation *agent.Invocation) (<-chan *event.Event, error) {
	ch := make(chan *event.Event, m.numEvents)
	for i := 0; i < m.numEvents; i++ {
		ch <- &event.Event{
			Response: &model.Response{
				ID:   fmt.Sprintf("resp-%d", i),
				Done: true,
				Choices: []model.Choice{
					{
						Index: 0,
						Message: model.Message{
							Role:    model.RoleAssistant,
							Content: fmt.Sprintf("response %d", i),
						},
					},
				},
			},
			InvocationID: invocation.InvocationID,
			Author:       m.name,
			ID:           fmt.Sprintf("event-%d", i),
			Timestamp:    time.Now(),
		}
	}
	close(ch)
	return ch, nil
}

// drainEventChannel reads all events from ch until it is closed.
func drainEventChannel(ch <-chan *event.Event) {
	for range ch {
	}
}

// TestMidRunMemory_TriggersAfterEveryNSteps verifies that mid-run memory
// extraction fires each time the step count reaches the configured interval,
// and that the counter resets so subsequent intervals also trigger.
func TestMidRunMemory_TriggersAfterEveryNSteps(t *testing.T) {
	mockMemSvc := &mockMemoryServiceForAutoMemory{}
	sessSvc := sessioninmemory.NewSessionService()
	ag := &multiEventMockAgent{name: "mid-run-agent", numEvents: 5}

	r := NewRunner("test-app", ag,
		WithSessionService(sessSvc),
		WithMemoryService(mockMemSvc),
		WithMidRunMemoryInterval(2),
	)

	ctx := context.Background()
	eventCh, err := r.Run(ctx, "user", "session", model.NewUserMessage("hello"))
	require.NoError(t, err)
	drainEventChannel(eventCh)

	// With interval=2 and 5 events:
	//   event 2 -> trigger (count=1), event 4 -> trigger (count=2),
	//   event 5 -> no trigger (stepCount=1 after reset).
	// Plus the turn-end extraction (count=3).
	require.Equal(t, int32(3), mockMemSvc.enqueueCount.Load(),
		"expected 2 mid-run extractions + 1 turn-end extraction")
	require.True(t, mockMemSvc.enqueueCalled)
	require.NotNil(t, mockMemSvc.sess)
}

// TestMidRunMemory_DisabledWhenIntervalZero verifies that when the interval
// is 0 (the default), no mid-run extraction occurs and only the existing
// turn-end extraction fires — preserving backward compatibility.
func TestMidRunMemory_DisabledWhenIntervalZero(t *testing.T) {
	mockMemSvc := &mockMemoryServiceForAutoMemory{}
	sessSvc := sessioninmemory.NewSessionService()
	ag := &multiEventMockAgent{name: "disabled-agent", numEvents: 5}

	r := NewRunner("test-app", ag,
		WithSessionService(sessSvc),
		WithMemoryService(mockMemSvc),
		// No WithMidRunMemoryInterval — default is 0 (disabled).
	)

	ctx := context.Background()
	eventCh, err := r.Run(ctx, "user", "session", model.NewUserMessage("hello"))
	require.NoError(t, err)
	drainEventChannel(eventCh)

	// Only the turn-end extraction should fire.
	require.Equal(t, int32(1), mockMemSvc.enqueueCount.Load(),
		"expected only turn-end extraction when interval is 0")
}

// TestMidRunMemory_NoTriggerBeforeInterval verifies that mid-run extraction
// does not fire when the total step count never reaches the configured
// interval.
func TestMidRunMemory_NoTriggerBeforeInterval(t *testing.T) {
	mockMemSvc := &mockMemoryServiceForAutoMemory{}
	sessSvc := sessioninmemory.NewSessionService()
	ag := &multiEventMockAgent{name: "short-agent", numEvents: 3}

	r := NewRunner("test-app", ag,
		WithSessionService(sessSvc),
		WithMemoryService(mockMemSvc),
		WithMidRunMemoryInterval(5),
	)

	ctx := context.Background()
	eventCh, err := r.Run(ctx, "user", "session", model.NewUserMessage("hello"))
	require.NoError(t, err)
	drainEventChannel(eventCh)

	// 3 events < interval 5, so no mid-run extraction.
	// Only the turn-end extraction fires.
	require.Equal(t, int32(1), mockMemSvc.enqueueCount.Load(),
		"expected no mid-run extraction when step count < interval")
}

// TestMidRunMemory_HandlesFailureGracefully verifies that when
// EnqueueAutoMemoryJob returns an error (simulating LLM extraction failure),
// the runner logs the error and continues processing all events without
// crashing.
func TestMidRunMemory_HandlesFailureGracefully(t *testing.T) {
	mockMemSvc := &mockMemoryServiceForAutoMemory{
		enqueueErr: errors.New("simulated LLM extraction failure"),
	}
	sessSvc := sessioninmemory.NewSessionService()
	ag := &multiEventMockAgent{name: "fail-agent", numEvents: 3}

	r := NewRunner("test-app", ag,
		WithSessionService(sessSvc),
		WithMemoryService(mockMemSvc),
		WithMidRunMemoryInterval(2),
	)

	ctx := context.Background()
	eventCh, err := r.Run(ctx, "user", "session", model.NewUserMessage("hello"))
	require.NoError(t, err)

	// The runner must drain all events despite extraction failures.
	receivedCount := 0
	for evt := range eventCh {
		receivedCount++
		require.NotNil(t, evt)
	}

	// 3 agent events + 1 runner completion event = 4 total.
	require.Equal(t, 4, receivedCount,
		"runner must emit all events despite mid-run extraction failure")

	// Mid-run extraction was attempted at event 2 (failed), plus turn-end
	// extraction (also failed). Both failures are logged, not propagated.
	require.Equal(t, int32(2), mockMemSvc.enqueueCount.Load(),
		"expected 1 mid-run attempt + 1 turn-end attempt despite errors")
}
