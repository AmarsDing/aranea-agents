package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/memory"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// memoryEditToolNames lists the tools that require L3 fact edit dependencies.
var memoryEditToolNames = map[string]bool{
	"memory_replace": true,
	"memory_rethink": true,
	"memory_insert":  true,
}

// newMemoryEditContextBeforeHook returns a BeforeToolHook that injects
// FactReader, FactWriter, ActionLogWriter, agentID, and userID into the
// tool execution context for memory_replace / memory_rethink /
// memory_insert tools.
//
// Index sync is handled internally by MemoryAdminUsecase.UpsertFactRow
// (which calls syncFactIndexBestEffort), so no separate IndexSyncer
// injection is needed.
func newMemoryEditContextBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	if deps.MemoryAdmin == nil {
		return nil
	}
	policy := biz.ResolveMemoryRuntimePolicy(ag.Settings)
	if !policy.MasterEnabled {
		return nil
	}
	return callbacks.NewBeforeToolHook(-10, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		if args == nil || args.Declaration == nil {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		if !memoryEditToolNames[args.Declaration.Name] {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		inv, ok := trpcagent.InvocationFromContext(ctx)
		if !ok || inv == nil || inv.Session == nil {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		agentID := strings.TrimSpace(ag.ID)
		if agentID == "" {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		// deps.MemoryAdmin (biz.SessionAdminStore) satisfies both FactReader
		// (via L3FactReader.GetFactRowsByIDs) and FactWriter (via
		// L3FactWriter.UpsertFactRow).
		ctx = memory.WithL3FactReader(ctx, deps.MemoryAdmin)
		ctx = memory.WithL3FactWriter(ctx, deps.MemoryAdmin)
		ctx = memory.WithEditAgentID(ctx, agentID)
		if uid := strings.TrimSpace(inv.Session.UserID); uid != "" {
			ctx = memory.WithEditUserID(ctx, uid)
		}
		// ActionLogWriter is optional — edit tools handle nil gracefully.
		if deps.MemoryActionLogWriter != nil {
			ctx = memory.WithActionLogWriter(ctx, deps.MemoryActionLogWriter)
		}
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	})
}
