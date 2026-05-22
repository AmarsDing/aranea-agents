//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package toolcache

import (
	"context"
	"os"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/agent"
)

const stateKeyFileViews = "tool:file:views"

// FileView holds cached text file content for the current invocation.
type FileView struct {
	Content    string
	MtimeMs    int64
	Encoding   string
	LineEnding string
	Mode       os.FileMode
}

// StoreFileViewFromContext caches a file view on the invocation in ctx.
func StoreFileViewFromContext(ctx context.Context, absPath string, view FileView) {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return
	}
	StoreFileView(inv, absPath, view)
}

// StoreFileView caches a file view on inv.
func StoreFileView(inv *agent.Invocation, absPath string, view FileView) {
	if inv == nil {
		return
	}
	absPath = strings.TrimSpace(absPath)
	if absPath == "" {
		return
	}
	merged := fileViewMap(inv)
	merged[absPath] = view
	inv.SetState(stateKeyFileViews, merged)
}

// LookupFileViewFromContext returns a cached view when present.
func LookupFileViewFromContext(ctx context.Context, absPath string) (FileView, bool) {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return FileView{}, false
	}
	return LookupFileView(inv, absPath)
}

// LookupFileView returns a cached view when present.
func LookupFileView(inv *agent.Invocation, absPath string) (FileView, bool) {
	if inv == nil {
		return FileView{}, false
	}
	merged := fileViewMap(inv)
	view, ok := merged[strings.TrimSpace(absPath)]
	return view, ok
}

// InvalidateFileViewFromContext removes a cached view for absPath.
func InvalidateFileViewFromContext(ctx context.Context, absPath string) {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return
	}
	InvalidateFileView(inv, absPath)
}

// InvalidateFileView removes a cached view for absPath.
func InvalidateFileView(inv *agent.Invocation, absPath string) {
	if inv == nil {
		return
	}
	merged := fileViewMap(inv)
	delete(merged, strings.TrimSpace(absPath))
	if len(merged) == 0 {
		inv.DeleteState(stateKeyFileViews)
		return
	}
	inv.SetState(stateKeyFileViews, merged)
}

func fileViewMap(inv *agent.Invocation) map[string]FileView {
	if existing, ok := inv.GetState(stateKeyFileViews); ok {
		if m, ok := existing.(map[string]FileView); ok {
			return m
		}
	}
	return make(map[string]FileView)
}
