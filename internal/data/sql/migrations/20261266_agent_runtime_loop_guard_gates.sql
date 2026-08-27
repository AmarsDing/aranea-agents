-- Version 20261266: Agent runtime settings — 行为模式闸阈值（79-runtime-governance 二轮 Q1）
-- S02「合法失控」根修：24 次异质 tool_load 全部"合法"（签名不同/结果成功/算有产出），
-- 同参/轮换/空结果/空转轮四重守卫均不命中——工具装载行为本身无闸。
-- 三列语义（0=跟随内置常量默认，>0 覆盖）：
--   loop_guard_tool_load_max  单节点 tool_load 成功装载配额（默认 8；S02 观测 24 次为失控）
--   loop_guard_wall_soft_sec  单节点 wall-time 软闸秒数（默认 240，超线注降级引导 cue）
--   loop_guard_wall_hard_sec  单节点 wall-time 硬闸秒数（默认 600，超线 StopError 强终止节点）
-- 经 PolicyResolver 每调用读取（classResolverManaged），变更零重建生效；
-- SQL 直改需重启或等 Reload（与 tools_execution_timeout_sec 同语义）。
-- Idempotent: "duplicate column" errors are treated as success by the migration runner (DB-N6).
ALTER TABLE agent_runtime_settings ADD COLUMN loop_guard_tool_load_max INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agent_runtime_settings ADD COLUMN loop_guard_wall_soft_sec INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agent_runtime_settings ADD COLUMN loop_guard_wall_hard_sec INTEGER NOT NULL DEFAULT 0;
