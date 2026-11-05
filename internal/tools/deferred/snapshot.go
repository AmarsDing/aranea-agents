package deferred

import (
	"context"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// LLM 工具快照失效/追加的应用层实现。
//
// 背景（框架库化迁移 2026-11-03）：此前通过 fork 的公开包
// trpc-agent-go/toolsnapshot 代理框架 internal/flow/toolsnapshot 的
// Invalidate/Append；官方库中该实现位于 internal 包不可引用，但快照本身
// 以公开 Invocation 状态键存储，故此处经 agent.Invocation 公共 API
// （GetState/SetState/DeleteState）等价实现。
//
// 与 fork 的已知差异：fork 的 pendingMutations 解决了并行 tool call worker
// 只持有 invocation 视图副本（state map 拷贝）导致的失效丢失；官方库下
// 并行批内激活的工具可能到下一轮才可见（tool_load 返回文案已提示不要
// 在同批并行调用中立即使用目标工具）。
//
// 以下键名与官方库 internal/flow/toolsnapshot 的常量保持一致。
const (
	toolsSnapshotKey                  = "llmflow:tools_snapshot"
	hasFilteredUserToolsKey           = "llmflow:has_filtered_user_tools"
	filteredTraceableUserToolNamesKey = "llmflow:filtered_traceable_user_tool_names"
)

// SnapshotInvalidateFromContext clears the cached LLM tool snapshot for the
// invocation carried by ctx, forcing the next model request in the same
// invocation to rebuild the visible tool list (re-applying tool filters and
// activation state). It returns false when ctx carries no invocation.
func SnapshotInvalidateFromContext(ctx context.Context) bool {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return false
	}
	inv.DeleteState(toolsSnapshotKey)
	inv.DeleteState(hasFilteredUserToolsKey)
	inv.DeleteState(filteredTraceableUserToolNamesKey)
	return true
}

// SnapshotAppendFromContext adds t to the cached LLM tool snapshot for the
// invocation carried by ctx so a newly activated deferred tool is visible on
// the next model request in the same turn. Returns false when ctx carries no
// invocation, t is nil, or no snapshot is cached (callers should then
// invalidate instead).
func SnapshotAppendFromContext(ctx context.Context, t tool.Tool) bool {
	if t == nil {
		return false
	}
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return false
	}
	cached, ok := inv.GetState(toolsSnapshotKey)
	if !ok {
		return false
	}
	tools, ok := cached.([]tool.Tool)
	if !ok {
		return false
	}
	for _, existing := range tools {
		if existing != nil && existing.Declaration() != nil &&
			t.Declaration() != nil &&
			existing.Declaration().Name == t.Declaration().Name {
			return true // already present
		}
	}
	inv.SetState(toolsSnapshotKey, append(append([]tool.Tool(nil), tools...), t))
	return true
}
