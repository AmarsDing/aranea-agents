-- 20261241_remove_read_image_tool.sql
-- read_image 死工具清理（2026-08-24 agent 配置全面审计 P5）：无任何 factory /
-- 装配路径实现（builtin_tools_seed.go 目录行 + toolGroupsMedia 引用之外无代码），
-- enabled=false 且非 opt-in → 被 applyRegistryAdminDenials 全员硬 deny，目录「撒谎」。
-- 种子行与 toolGroupsMedia 引用已移除；本迁移删除存量库中的目录行。
-- 幂等：DELETE 不命中即为 0 行。
DELETE FROM tools WHERE tool_key = 'read_image';
