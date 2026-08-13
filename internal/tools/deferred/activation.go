package deferred

import (
	"context"
	"encoding/json"
	"sort"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// activatedStateKey 是 session state 中存储已激活延迟工具列表的 key。
// 值为 JSON []string，存储已激活的工具名称。
//
// 使用 temp: 前缀，不持久化到数据库（session 重建后需重新 tool_load），
// 但在同一 session 的多个 invocation 之间共享（同一聊天会话的多个 turn）。
const activatedStateKey = "temp:deferred:activated"

// readActivatedSet 从 session state 读取已激活的工具名称集合。
// 通过 ctx 中的 invocation 获取 session，无 invocation 时返回空集合。
func readActivatedSet(ctx context.Context) map[string]bool {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return nil
	}
	data, ok := inv.Session.GetState(activatedStateKey)
	if !ok || len(data) == 0 {
		return nil
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return nil
	}
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}

// writeActivatedSet 将激活的工具名称写入 session state。
// 返回 true 表示写入成功（有 invocation + session）。
func writeActivatedSet(ctx context.Context, toolName string) bool {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return false
	}

	// 读取现有集合，追加新工具（幂等）
	existing := readActivatedSet(ctx)
	if existing == nil {
		existing = make(map[string]bool)
	}
	if existing[toolName] {
		return true // 已激活，无需重复写入
	}
	existing[toolName] = true

	// 序列化并写入
	names := make([]string, 0, len(existing))
	for n := range existing {
		names = append(names, n)
	}
	sort.Strings(names) // 确定性排序，保证序列化结果稳定
	data, err := json.Marshal(names)
	if err != nil {
		return false
	}
	inv.Session.SetState(activatedStateKey, data)
	return true
}

// isActivatedForSession 检查指定工具是否已在当前 session 中激活。
func isActivatedForSession(ctx context.Context, toolName string) bool {
	set := readActivatedSet(ctx)
	return set[toolName]
}
