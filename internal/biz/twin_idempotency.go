package biz

import "context"

// TwinIdempotencyStore 机器面（TwinOpenAPI）run 提交幂等键的持久化端口。
// P0 底座安全前置（2026-11-02，三仓联动）：原 service 层进程内 map 重启
// 即失效，消费方（HotCraftFlow AGT / NovaTrader harness / twinmonitor）的
// 「idempotency_key 天然 at-least-once 安全」契约只在进程生命周期内成立。
// 本端口把 (caller, key) → run_id 落库（表 twin_openapi_idempotency，
// 迁移 20261306），使底座重启后重试仍命中原 run。
//
// 稳定性分级：Internal（仅 TwinOpenAPI 门面消费；不对外承诺）。
type TwinIdempotencyStore interface {
	// LookupRunID 查询幂等键已绑定的 run_id；未命中返回 ("", nil)。
	// caller 为调用方身份（多 token 部署的 caller name；存量单 token 恒为
	// "twinmonitor"），键空间按 caller 隔离，不同业务方同键互不影响。
	LookupRunID(ctx context.Context, caller, key string) (string, error)
	// SaveRunID 以先到者赢语义保存幂等键（INSERT ... ON CONFLICT DO
	// NOTHING；严禁 UPDATE 翻案——并发重试下首个成功提交的 run 永恒有效）。
	// 实现方在写路径附带 lazy GC（清理早于保留窗口的行）。
	SaveRunID(ctx context.Context, caller, key, runID, graphID string) error
}
