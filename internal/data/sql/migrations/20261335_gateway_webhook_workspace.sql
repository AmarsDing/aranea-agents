-- 20261335 gateway_webhook_workspace（Harness › 连接 P0：出站推送按工作区隔离）
-- 原表没有 workspace：任一工作区的运行 / 工作流 / 商机事件会推给所有工作区的订阅，
-- 管理面也看得到别人的推送。增 workspace_id，存量行为空串 = default 工作区
-- （与 workspace.AssertWorkspace 的历史兼容口径一致）。幂等 IF NOT EXISTS。
ALTER TABLE gateway_webhooks ADD COLUMN IF NOT EXISTS workspace_id TEXT NOT NULL DEFAULT '';
-- 分发按 (workspace_id, enabled) 取订阅。
CREATE INDEX IF NOT EXISTS idx_gateway_webhooks_ws_enabled ON gateway_webhooks ("workspace_id", "enabled");
