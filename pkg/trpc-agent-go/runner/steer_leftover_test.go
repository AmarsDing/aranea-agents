//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package runner

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// enqueueDuringCallModel invokes onCall synchronously inside the model call
// (i.e. while the run is registered and its steer queue is open) and then
// returns a fixed final response. It reproduces the clean-completion window
// where a steer message is accepted during the last model step, after the
// flow's final safe-boundary drain.
type enqueueDuringCallModel struct {
	name     string
	onCall   func()
	response *model.Response

	mu       sync.Mutex
	requests []*capturedModelRequest
}

func (m *enqueueDuringCallModel) GenerateContent(
	_ context.Context,
	req *model.Request,
) (<-chan *model.Response, error) {
	m.mu.Lock()
	if req != nil {
		m.requests = append(m.requests, cloneCapturedModelRequest(req))
	}
	onCall := m.onCall
	m.onCall = nil // fire at most once, on the first call
	m.mu.Unlock()

	if onCall != nil {
		onCall()
	}
	ch := make(chan *model.Response, 1)
	ch <- m.response
	close(ch)
	return ch, nil
}

func (m *enqueueDuringCallModel) Info() model.Info {
	return model.Info{Name: m.name}
}

type leftoverHandlerRecorder struct {
	mu    sync.Mutex
	calls int
	got   []model.Message
}

func (r *leftoverHandlerRecorder) handler() func([]model.Message) {
	return func(messages []model.Message) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.calls++
		r.got = append(r.got, messages...)
	}
}

func (r *leftoverHandlerRecorder) snapshot() (int, []model.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, append([]model.Message(nil), r.got...)
}

// Regression: a steer message accepted during the final model step of a
// cleanly completing run must be handed to the run's
// UnconsumedQueuedUserMessagesHandler exactly once instead of being dropped
// silently when the run handle is unregistered.
func TestRunner_UnconsumedQueuedUserMessages_DeliveredOnCleanCompletion(t *testing.T) {
	const (
		appName      = "runner-steer-leftover"
		userID       = "user-1"
		sessionID    = "session-1"
		requestID    = "req-steer-leftover"
		steerContent = "queued while the last step was in flight"
		finalAnswer  = "done"
	)

	var (
		runnerInstance Runner
		enqueueErr     error
	)
	modelStub := &enqueueDuringCallModel{
		name: "steer-leftover-model",
		onCall: func() {
			enqueueErr = EnqueueUserMessage(
				runnerInstance,
				requestID,
				model.NewUserMessage(steerContent),
			)
		},
		response: &model.Response{
			ID:   "resp-final",
			Done: true,
			Choices: []model.Choice{{
				Index:   0,
				Message: model.NewAssistantMessage(finalAnswer),
			}},
		},
	}
	ag := llmagent.New("steer-leftover-agent", llmagent.WithModel(modelStub))
	runnerInstance = NewRunner(appName, ag)

	recorder := &leftoverHandlerRecorder{}
	events, err := runnerInstance.Run(
		context.Background(),
		userID,
		sessionID,
		model.NewUserMessage("initial"),
		agent.WithRequestID(requestID),
		agent.WithUnconsumedQueuedUserMessagesHandler(recorder.handler()),
	)
	require.NoError(t, err)

	sawFinal := false
	for evt := range events {
		if evt != nil && evt.IsFinalResponse() {
			sawFinal = true
		}
	}
	require.True(t, sawFinal, "run should complete with a final response")
	require.NoError(t, enqueueErr, "steer enqueue must succeed while the run is active")

	calls, got := recorder.snapshot()
	require.Equal(t, 1, calls, "handler must fire exactly once at run termination")
	require.Len(t, got, 1)
	require.Equal(t, steerContent, got[0].Content)
}

// Guard: a steer message that the flow actually consumes at a safe boundary
// must NOT be delivered to the handler — the hand-off must not double-answer
// messages the run already processed.
func TestRunner_UnconsumedQueuedUserMessages_SkippedWhenConsumed(t *testing.T) {
	const (
		appName      = "runner-steer-consumed"
		userID       = "user-1"
		sessionID    = "session-1"
		requestID    = "req-steer-consumed"
		toolName     = "lookup"
		steerContent = "consumed at the next boundary"
		finalAnswer  = "done"
	)

	type lookupInput struct {
		Topic string `json:"topic"`
	}
	type lookupOutput struct {
		Result string `json:"result"`
	}

	modelStub := &sequentialModel{
		name: "steer-consumed-model",
		responses: []*model.Response{
			{
				ID:   "resp-tool-call",
				Done: true,
				Choices: []model.Choice{{
					Index: 0,
					Message: model.Message{
						Role: model.RoleAssistant,
						ToolCalls: []model.ToolCall{{
							ID:   "tool-call-1",
							Type: "function",
							Function: model.FunctionDefinitionParam{
								Name:      toolName,
								Arguments: []byte(`{"topic":"alpha"}`),
							},
						}},
					},
				}},
			},
			{
				ID:   "resp-final",
				Done: true,
				Choices: []model.Choice{{
					Index:   0,
					Message: model.NewAssistantMessage(finalAnswer),
				}},
			},
		},
	}

	var (
		runnerInstance Runner
		enqueueErr     error
	)
	toolImpl := function.NewFunctionTool(
		func(_ context.Context, input lookupInput) (lookupOutput, error) {
			enqueueErr = EnqueueUserMessage(
				runnerInstance,
				requestID,
				model.NewUserMessage(steerContent),
			)
			return lookupOutput{Result: "tool result for " + input.Topic}, nil
		},
		function.WithName(toolName),
		function.WithDescription("Looks up a topic"),
	)
	ag := llmagent.New(
		"steer-consumed-agent",
		llmagent.WithModel(modelStub),
		llmagent.WithTools([]tool.Tool{toolImpl}),
	)
	runnerInstance = NewRunner(appName, ag)

	recorder := &leftoverHandlerRecorder{}
	events, err := runnerInstance.Run(
		context.Background(),
		userID,
		sessionID,
		model.NewUserMessage("initial"),
		agent.WithRequestID(requestID),
		agent.WithUnconsumedQueuedUserMessagesHandler(recorder.handler()),
	)
	require.NoError(t, err)
	for range events {
	}

	require.NoError(t, enqueueErr)
	// The flow consumed the steer at the next boundary: it reaches the
	// second model request as a user message.
	requests := modelStub.Requests()
	require.Len(t, requests, 2)
	consumedIdx := findMessageIndex(
		requests[1].messages,
		func(message model.Message) bool {
			return message.Role == model.RoleUser &&
				message.Content == steerContent
		},
	)
	require.NotEqual(t, -1, consumedIdx, "steer must be consumed by the flow")

	calls, got := recorder.snapshot()
	require.Equal(t, 0, calls, "handler must not fire for consumed messages")
	require.Empty(t, got)
}

// Guard: a panicking handler is recovered and does not break run completion
// or the event stream.
func TestRunner_UnconsumedQueuedUserMessages_HandlerPanicContained(t *testing.T) {
	const (
		appName      = "runner-steer-panic"
		userID       = "user-1"
		sessionID    = "session-1"
		requestID    = "req-steer-panic"
		steerContent = "leftover"
		finalAnswer  = "done"
	)

	var (
		runnerInstance Runner
		enqueueErr     error
	)
	modelStub := &enqueueDuringCallModel{
		name: "steer-panic-model",
		onCall: func() {
			enqueueErr = EnqueueUserMessage(
				runnerInstance,
				requestID,
				model.NewUserMessage(steerContent),
			)
		},
		response: &model.Response{
			ID:   "resp-final",
			Done: true,
			Choices: []model.Choice{{
				Index:   0,
				Message: model.NewAssistantMessage(finalAnswer),
			}},
		},
	}
	ag := llmagent.New("steer-panic-agent", llmagent.WithModel(modelStub))
	runnerInstance = NewRunner(appName, ag)

	handlerCalls := 0
	events, err := runnerInstance.Run(
		context.Background(),
		userID,
		sessionID,
		model.NewUserMessage("initial"),
		agent.WithRequestID(requestID),
		agent.WithUnconsumedQueuedUserMessagesHandler(
			func([]model.Message) {
				handlerCalls++
				panic("boom")
			},
		),
	)
	require.NoError(t, err)

	sawFinal := false
	for evt := range events {
		if evt != nil && evt.IsFinalResponse() {
			sawFinal = true
		}
	}
	require.True(t, sawFinal, "run must complete despite handler panic")
	require.NoError(t, enqueueErr)
	require.Equal(t, 1, handlerCalls)
}
