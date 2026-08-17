package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfile "trpc.group/trpc-go/trpc-agent-go/tool/file"
)

var worktreeCallSeq atomic.Uint64

// wrapFileToolSetWithWorktree routes IsolationStrategyWorktree tools through
// a git worktree when workspaceDir is inside a repository. Non-git workspaces
// (typical agent dirs) pass through unchanged so writes stay on the live tree.
func wrapFileToolSetWithWorktree(ts trpctool.ToolSet, workspaceDir string, lg loggateway.Logger) trpctool.ToolSet {
	if ts == nil {
		return nil
	}
	root := LookupGitRoot(workspaceDir)
	if root == "" {
		return ts
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	iso, err := NewWorktreeIsolator(root, nil, lg)
	if err != nil {
		lg.Warn("file worktree isolator unavailable, writing live workspace",
			loggateway.StepID("tool.file.worktree"),
			loggateway.Err(err))
		return ts
	}
	return &worktreeFileToolSet{inner: ts, iso: iso, lg: lg}
}

type worktreeFileToolSet struct {
	inner trpctool.ToolSet
	iso   *WorktreeIsolator
	lg    loggateway.Logger
}

func (w *worktreeFileToolSet) Name() string {
	if w.inner == nil {
		return ""
	}
	return w.inner.Name()
}

func (w *worktreeFileToolSet) Close() error {
	if w.inner == nil {
		return nil
	}
	return w.inner.Close()
}

func (w *worktreeFileToolSet) Tools(ctx context.Context) []trpctool.Tool {
	if w.inner == nil {
		return nil
	}
	raw := w.inner.Tools(ctx)
	out := make([]trpctool.Tool, len(raw))
	for i, t := range raw {
		ct, ok := t.(trpctool.CallableTool)
		if !ok {
			out[i] = t
			continue
		}
		name := ""
		if decl := ct.Declaration(); decl != nil {
			name = decl.Name
		}
		if IsolationStrategyForTool(name) != IsolationStrategyWorktree {
			out[i] = t
			continue
		}
		out[i] = &worktreeFileCallable{inner: ct, iso: w.iso, lg: w.lg}
	}
	return out
}

type worktreeFileCallable struct {
	inner trpctool.CallableTool
	iso   *WorktreeIsolator
	lg    loggateway.Logger
}

func (w *worktreeFileCallable) Declaration() *trpctool.Declaration {
	if w == nil || w.inner == nil {
		return nil
	}
	return w.inner.Declaration()
}

func (w *worktreeFileCallable) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if w == nil || w.inner == nil {
		return nil, fmt.Errorf("worktree file tool not initialized")
	}
	name := ""
	if decl := w.Declaration(); decl != nil {
		name = decl.Name
	}
	jsonArgs = NormalizeInvocationWithLog(w.lg, name, jsonArgs)
	var innerOut any
	var innerErr error
	res := w.iso.ExecuteWithHandler(ctx, ToolCall{
		ID:        worktreeFileCallID(name, jsonArgs),
		Name:      name,
		Arguments: jsonArgs,
	}, func(ctx context.Context, worktreeDir string, call ToolCall) ToolResult {
		ts, err := trpcfile.NewToolSet(trpcfile.WithBaseDir(worktreeDir))
		if err != nil {
			return ToolResult{CallID: call.ID, Name: call.Name, Error: err.Error()}
		}
		defer ts.Close()
		var target trpctool.CallableTool
		for _, t := range ts.Tools(ctx) {
			ct, ok := t.(trpctool.CallableTool)
			if !ok || ct.Declaration() == nil || ct.Declaration().Name != call.Name {
				continue
			}
			target = ct
			break
		}
		if target == nil {
			return ToolResult{CallID: call.ID, Name: call.Name, Error: "tool missing in worktree toolset"}
		}
		innerOut, innerErr = target.Call(ctx, NormalizeInvocation(call.Name, call.Arguments))
		if innerErr != nil {
			return ToolResult{CallID: call.ID, Name: call.Name, Error: innerErr.Error()}
		}
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true}
	})
	if res.Success {
		return innerOut, nil
	}
	if strings.HasPrefix(res.Error, "create worktree:") {
		w.lg.Warn("file worktree create failed, writing live workspace",
			loggateway.StepID("tool.file.worktree"),
			loggateway.Str("tool", name),
			loggateway.Str("error", res.Error))
		return w.inner.Call(ctx, jsonArgs)
	}
	if innerErr != nil {
		return nil, innerErr
	}
	return nil, fmt.Errorf("%s", res.Error)
}

func worktreeFileCallID(name string, jsonArgs []byte) string {
	sum := sha256.Sum256(jsonArgs)
	seq := worktreeCallSeq.Add(1)
	return name + "-" + strconv.FormatUint(seq, 10) + "-" + hex.EncodeToString(sum[:4]) + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}
