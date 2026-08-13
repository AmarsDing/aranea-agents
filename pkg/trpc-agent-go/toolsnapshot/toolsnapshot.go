//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

// Package toolsnapshot exposes invocation-scoped LLM tool snapshot
// invalidation for hosts that dynamically change tool visibility mid-run
// (e.g. a meta-tool that activates deferred tools). The snapshot itself is
// owned and maintained by the internal flow package; this package only
// proxies the invalidation operation so module-external code can trigger a
// rebuild without importing internal packages.
package toolsnapshot

import (
	"context"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	internalsnapshot "trpc.group/trpc-go/trpc-agent-go/internal/flow/toolsnapshot"
)

// Invalidate clears the cached LLM tool snapshot for the invocation, forcing
// the next model request in the same invocation to rebuild the visible tool
// list (re-applying tool filters and activation state).
func Invalidate(inv *agent.Invocation) {
	internalsnapshot.Invalidate(inv)
}

// InvalidateFromContext clears the cached tool snapshot using the invocation
// carried by ctx. It returns false when ctx carries no invocation.
func InvalidateFromContext(ctx context.Context) bool {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return false
	}
	internalsnapshot.Invalidate(inv)
	return true
}
