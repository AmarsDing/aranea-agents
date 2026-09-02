package deferred

import (
	"context"
	"encoding/json"
	"sort"
	"sync"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// activatedStateKey 是 session state 中存储已激活延迟工具列表的 key。
// 值为 JSON []string，存储已激活的工具名称。
//
// 使用 temp: 前缀，不持久化到数据库（session 重建后需重新 tool_load），
// 但在同一 session 的多个 invocation 之间共享（同一聊天会话的多个 turn）。
const activatedStateKey = "temp:deferred:activated"

// activatedRegistry 是进程级激活注册表：sessionID → 已激活工具名集合。
//
// 存在原因（2026-09-01 根修）：框架并行执行同一批 tool call 时，每个 worker
// 持有 invocation 视图 + 克隆 session（state-delta 基线设计，见
// functioncall.go newParallelInvocationView）。tool_load 在 worker 中写
// inv.Session 的激活态会随 worker 退出丢失（tool_load 未实现
// stateDeltaProvider，不会经事件 StateDelta 合并回真实 session）。
// worker 的克隆 session 与父 invocation 共享同一 session ID，因此以
// session ID 为键的进程级注册表可以穿透 worker 隔离，对后续所有
// LLM 请求可见。进程内存语义与 temp: 前缀一致：重启后需重新 tool_load。
var activatedRegistry sync.Map // map[string]*activatedToolSet

type activatedToolSet struct {
	mu    sync.RWMutex
	names map[string]bool
}

func registryActivated(sessionID, toolName string) bool {
	if sessionID == "" {
		return false
	}
	v, ok := activatedRegistry.Load(sessionID)
	if !ok {
		return false
	}
	set := v.(*activatedToolSet)
	set.mu.RLock()
	hit := set.names[toolName]
	set.mu.RUnlock()
	return hit
}

func registryActivate(sessionID, toolName string) {
	if sessionID == "" {
		return
	}
	v, _ := activatedRegistry.LoadOrStore(sessionID, &activatedToolSet{
		names: make(map[string]bool),
	})
	set := v.(*activatedToolSet)
	set.mu.Lock()
	set.names[toolName] = true
	set.mu.Unlock()
}

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

	// 进程级注册表：穿透并行 worker 的克隆 session 隔离，
	// 保证同 session 的后续 LLM 请求（父 invocation）能看到激活态。
	registryActivate(inv.Session.ID, toolName)

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
	// 优先查进程级注册表（并行 worker 写入的唯一可靠通道）。
	if inv, ok := trpcagent.InvocationFromContext(ctx); ok &&
		inv != nil && inv.Session != nil &&
		registryActivated(inv.Session.ID, toolName) {
		return true
	}
	set := readActivatedSet(ctx)
	return set[toolName]
}
