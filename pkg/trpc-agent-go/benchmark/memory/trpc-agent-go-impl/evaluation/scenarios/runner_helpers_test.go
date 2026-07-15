//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package scenarios

import (
	"strings"
	"testing"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

func TestQAMemorySearchInstruction_SingleSearch(t *testing.T) {
	got := qaMemorySearchInstruction(1)
	if got != qaSingleSearchInstruction {
		t.Fatalf("unexpected instruction for single search")
	}
}

func TestQAMemorySearchInstruction_MultiSearch(t *testing.T) {
	got := qaMemorySearchInstruction(2)
	if !strings.Contains(got, "exactly 2 times") {
		t.Fatalf("missing multi-search rule: %q", got)
	}
	if !strings.Contains(got, fallbackAnswer) {
		t.Fatalf("missing fallback answer: %q", got)
	}
	if !strings.Contains(got, "Search #1") {
		t.Fatalf("missing workflow search marker: %q", got)
	}
}

func TestCollectFinalTextAndUsage_WaitsForRunnerCompletion(t *testing.T) {
	ch := make(chan *event.Event)
	resultCh := make(chan collectResult, 1)
	errCh := make(chan error, 1)

	go func() {
		res, err := collectFinalTextAndUsage(ch)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- res
	}()

	final := event.NewResponseEvent(
		"invocation",
		"author",
		&model.Response{
			Done: true,
			Choices: []model.Choice{
				{Message: model.NewAssistantMessage("answer")},
			},
		},
	)
	ch <- final

	select {
	case err := <-errCh:
		t.Fatalf("unexpected error before runner completion: %v", err)
	case res := <-resultCh:
		t.Fatalf("returned before runner completion: %+v", res)
	case <-time.After(50 * time.Millisecond):
	}

	completion := event.NewResponseEvent(
		"invocation",
		"author",
		&model.Response{
			Object: model.ObjectTypeRunnerCompletion,
			Done:   true,
		},
	)
	ch <- completion
	close(ch)

	select {
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case res := <-resultCh:
		if res.text != "answer" {
			t.Fatalf("unexpected collected text: %q", res.text)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runner completion")
	}
}
