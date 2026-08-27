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
	"testing"

	"github.com/stretchr/testify/require"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/session"
	sessioninmemory "trpc.group/trpc-go/trpc-agent-go/session/inmemory"
)

// TestWithRunInvocationID verifies the RunOption sets RunOptions.InvocationID.
func TestWithRunInvocationID(t *testing.T) {
	var ro agent.RunOptions
	agent.WithRunInvocationID("turn-123")(&ro)
	require.Equal(t, "turn-123", ro.InvocationID)
}

// TestNewRunInvocationInvocationIDOverride verifies the runner applies the
// RunOptions.InvocationID override to the root invocation, and that an empty
// value preserves the auto-generated uuid default.
func TestNewRunInvocationInvocationIDOverride(t *testing.T) {
	newRunner := func() *runner {
		return &runner{
			sessionService: sessioninmemory.NewSessionService(),
		}
	}
	newInv := func(r *runner, ro agent.RunOptions) *agent.Invocation {
		return r.newRunInvocation(
			session.NewSession("app", "user", "session"),
			model.NewUserMessage("hello"),
			&mockAgent{name: "test-agent"},
			ro,
			"app",
			"",
			"",
		)
	}

	t.Run("override applied to root invocation", func(t *testing.T) {
		inv := newInv(newRunner(), agent.RunOptions{InvocationID: "turn-abc"})
		require.Equal(t, "turn-abc", inv.InvocationID)
	})

	t.Run("empty keeps uuid default", func(t *testing.T) {
		inv := newInv(newRunner(), agent.RunOptions{})
		require.NotEmpty(t, inv.InvocationID)
		require.NotEqual(t, "turn-abc", inv.InvocationID)
	})

	t.Run("clone still gets fresh uuid", func(t *testing.T) {
		inv := newInv(newRunner(), agent.RunOptions{InvocationID: "turn-abc"})
		child := inv.Clone(agent.WithInvocationAgent(&mockAgent{name: "child"}))
		require.NotEmpty(t, child.InvocationID)
		require.NotEqual(t, inv.InvocationID, child.InvocationID)
		// The per-root-run override must not leak into the child's
		// RunOptions (Clone clears it) — otherwise a re-run with inherited
		// RunOptions would reuse the parent's id.
		require.Empty(t, child.RunOptions.InvocationID)
	})
}
