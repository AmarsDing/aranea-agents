package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/working_memory"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// newWorkingMemoryContextBeforeHook returns a BeforeToolHook that injects
// L1TaskWriter, L1FieldWriter, L1AdminReader, sessionID, and agentID into the tool execution
// context for working_memory.* tools.
func newWorkingMemoryContextBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	if !deps.HasWorkingMemory() {
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
		name := args.Declaration.Name
		if !strings.HasPrefix(name, "working_memory_") {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		// Extract session and agent info from invocation context
		inv, ok := trpcagent.InvocationFromContext(ctx)
		if !ok || inv == nil || inv.Session == nil {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		sessID := strings.TrimSpace(inv.Session.ID)
		agentID := strings.TrimSpace(ag.ID)
		if sessID == "" || agentID == "" {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		ctx = working_memory.WithL1TaskWriter(ctx, deps.L1Tasks)
		ctx = working_memory.WithL1FieldWriter(ctx, deps.L1Fields)
		ctx = working_memory.WithL1Reader(ctx, deps.L1Reader)
		ctx = working_memory.WithSessionID(ctx, sessID)
		ctx = working_memory.WithAgentID(ctx, agentID)
		ctx = working_memory.WithL1HistoryEnabled(ctx, ag.Settings != nil && ag.Settings.L1HistoryEnabled)
		if deps.L1Schema != nil {
			ctx = working_memory.WithL1SchemaReader(ctx, deps.L1Schema)
		}
		if ag.Settings != nil && ag.Settings.L1DefaultSchemaID != "" {
			ctx = working_memory.WithL1DefaultSchemaID(ctx, ag.Settings.L1DefaultSchemaID)
		}
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	})
}
