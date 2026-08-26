-- Version 20261262: tool_param_rules（79-runtime-governance R9 Phase 5.4）
-- 工具参数模式权限：BeforeTool paramRuleGate（循环守卫之前）按 deny > ask >
-- allow > fallback 语义求值。pattern 为 glob（'re:' 前缀正则）。与 D-P2 审批
-- 路由共享求值核心 internal/biz/policyrule（C3）。
-- 注：设计 §11 原分配版本 20261247 已被 M82 builtin_platform_tools_sandbox_fs_reseed
-- 占用，顺延取下一可用号。双方言通用（SQLite 风格，PG 经翻译）。幂等，重跑安全。
CREATE TABLE IF NOT EXISTS tool_param_rules (
  id         TEXT PRIMARY KEY,
  tool_key   TEXT NOT NULL,
  pattern    TEXT NOT NULL,
  effect     TEXT NOT NULL,
  priority   INTEGER NOT NULL DEFAULT 100,
  enabled    INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_tpr_tool ON tool_param_rules(tool_key, enabled, priority);
