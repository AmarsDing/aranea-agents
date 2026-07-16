//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package toolcache

import (
	"context"
	"os"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/agent"
)

const stateKeyFileViews = "tool:file:views"

// FileView is a per-invocation cached view of a text file under the workspace.
type FileView struct {
	Content    string
	MtimeMs    int64
	Encoding   string
	LineEnding string
	Mode       os.FileMode
}

// StoreFileViewFromContext stores a FileView keyed by absolute path.
func StoreFileViewFromContext(ctx context.Context, absPath string, view FileView) {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return
	}
	StoreFileView(inv, absPath, view)
}

// LookupFileViewFromContext returns a cached FileView for absPath.
func LookupFileViewFromContext(ctx context.Context, absPath string) (FileView, bool) {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return FileView{}, false
	}
	return LookupFileView(inv, absPath)
}

// StoreFileView stores a FileView on the invocation.
func StoreFileView(inv *agent.Invocation, absPath string, view FileView) {
	if inv == nil {
		return
	}
	key := strings.TrimSpace(absPath)
	if key == "" {
		return
	}
	merged := make(map[string]FileView)
	if existing, ok := inv.GetState(stateKeyFileViews); ok {
		if m, ok := existing.(map[string]FileView); ok {
			for k, v := range m {
				merged[k] = v
			}
		}
	}
	merged[key] = view
	inv.SetState(stateKeyFileViews, merged)
}

// LookupFileView looks up a FileView by absolute path.
func LookupFileView(inv *agent.Invocation, absPath string) (FileView, bool) {
	if inv == nil {
		return FileView{}, false
	}
	key := strings.TrimSpace(absPath)
	if key == "" {
		return FileView{}, false
	}
	existing, ok := inv.GetState(stateKeyFileViews)
	if !ok {
		return FileView{}, false
	}
	m, ok := existing.(map[string]FileView)
	if !ok {
		return FileView{}, false
	}
	v, ok := m[key]
	return v, ok
}
