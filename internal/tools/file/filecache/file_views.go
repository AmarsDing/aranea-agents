//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package filecache

import (
	"context"
	"os"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/agent"
)

const stateKeyFileViews = "tool:file:views"

// FileView is a per-invocation snapshot of a file on disk. Editing
// tools use it to skip redundant reads when the file has not changed
// (matched by MtimeMs) and to preserve encoding/line endings on write.
type FileView struct {
	Content    string
	MtimeMs    int64
	Encoding   string
	LineEnding string
	Mode       os.FileMode
}

// StoreFileViewFromContext stores a file view into the invocation
// state carried by ctx. It is a no-op when ctx has no invocation.
func StoreFileViewFromContext(
	ctx context.Context,
	path string,
	view FileView,
) {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return
	}
	StoreFileView(inv, path, view)
}

// StoreFileView stores a file view keyed by path into inv.
func StoreFileView(inv *agent.Invocation, path string, view FileView) {
	if inv == nil {
		return
	}
	key := strings.TrimSpace(path)
	if key == "" {
		return
	}
	views := fileViews(inv)
	views[key] = view
	inv.SetState(stateKeyFileViews, views)
}

// LookupFileViewFromContext looks up a file view from the invocation
// in ctx.
func LookupFileViewFromContext(
	ctx context.Context,
	path string,
) (FileView, bool) {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return FileView{}, false
	}
	return LookupFileView(inv, path)
}

// LookupFileView looks up a file view by path from inv.
func LookupFileView(inv *agent.Invocation, path string) (FileView, bool) {
	if inv == nil {
		return FileView{}, false
	}
	key := strings.TrimSpace(path)
	if key == "" {
		return FileView{}, false
	}
	view, ok := fileViews(inv)[key]
	return view, ok
}

func fileViews(inv *agent.Invocation) map[string]FileView {
	views := make(map[string]FileView)
	if existing, ok := inv.GetState(stateKeyFileViews); ok {
		if m, ok := existing.(map[string]FileView); ok {
			for k, v := range m {
				views[k] = v
			}
		}
	}
	return views
}
