-- 20261237_remove_workspace_exec_tool.sql
-- workspace_exec 整套下线（2026-08-21 工具契约评审 S5）：运行时装配路径从未实现
-- （registry 占位 + prune 强制关闭），真实 shell 为 hostexec 族。种子行已从
-- builtin_tools_seed.go 移除；本迁移删除存量库中的目录行，避免目录「撒谎」
-- （展示一个永远无法装配的工具）。幂等：DELETE 不命中即为 0 行。
DELETE FROM tools WHERE tool_key = 'workspace_exec';
