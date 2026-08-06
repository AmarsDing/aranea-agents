//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tmcp "trpc.group/trpc-go/trpc-mcp-go"
)

// countRecordedMethod counts recorded MCP requests for a JSON-RPC method.
func countRecordedMethod(h *recordingMCPHTTPHandler, method string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, req := range h.requests {
		if req.method == method {
			n++
		}
	}
	return n
}

func newCacheTestToolSet(handler *recordingMCPHTTPHandler, opts ...ToolSetOption) *ToolSet {
	base := []ToolSetOption{
		WithMCPOptions(
			tmcp.WithClientGetSSEEnabled(false),
			tmcp.WithHTTPReqHandler(handler),
		),
	}
	return NewMCPToolSet(
		ConnectionConfig{Transport: "streamable", ServerURL: "http://mcp.test"},
		append(base, opts...)...,
	)
}

// TestToolSet_ToolsCacheTTL verifies that Tools() serves the cached tool list
// within the configured TTL instead of issuing a tools/list roundtrip per
// call (llmagent calls Tools() on every run when RefreshToolSetsOnRun is on).
func TestToolSet_ToolsCacheTTL(t *testing.T) {
	ctx := context.Background()

	t.Run("skips refresh within TTL", func(t *testing.T) {
		handler := &recordingMCPHTTPHandler{}
		ts := newCacheTestToolSet(handler, WithToolsCacheTTL(time.Hour))
		defer func() { _ = ts.Close() }()

		ts.Tools(ctx)
		ts.Tools(ctx)
		ts.Tools(ctx)

		require.Equal(t, 1, countRecordedMethod(handler, "tools/list"),
			"within TTL only the first Tools() call may hit tools/list")
	})

	t.Run("refreshes after TTL expiry", func(t *testing.T) {
		handler := &recordingMCPHTTPHandler{}
		ts := newCacheTestToolSet(handler, WithToolsCacheTTL(20*time.Millisecond))
		defer func() { _ = ts.Close() }()

		ts.Tools(ctx)
		require.Equal(t, 1, countRecordedMethod(handler, "tools/list"))

		time.Sleep(40 * time.Millisecond)
		ts.Tools(ctx)
		require.Equal(t, 2, countRecordedMethod(handler, "tools/list"),
			"after TTL expiry Tools() must refresh from the server")
	})

	t.Run("disabled by default keeps per-call refresh", func(t *testing.T) {
		handler := &recordingMCPHTTPHandler{}
		ts := newCacheTestToolSet(handler)
		defer func() { _ = ts.Close() }()

		ts.Tools(ctx)
		ts.Tools(ctx)

		require.Equal(t, 2, countRecordedMethod(handler, "tools/list"),
			"without WithToolsCacheTTL every Tools() call refreshes")
	})

	t.Run("non-positive TTL disables cache", func(t *testing.T) {
		handler := &recordingMCPHTTPHandler{}
		ts := newCacheTestToolSet(handler, WithToolsCacheTTL(0))
		defer func() { _ = ts.Close() }()

		ts.Tools(ctx)
		ts.Tools(ctx)

		require.Equal(t, 2, countRecordedMethod(handler, "tools/list"))
	})
}

// TestWithToolsCacheTTL verifies the option stores the configured TTL.
func TestWithToolsCacheTTL(t *testing.T) {
	cfg := &toolSetConfig{}
	WithToolsCacheTTL(5 * time.Minute)(cfg)
	require.Equal(t, 5*time.Minute, cfg.toolsCacheTTL)
}
