-- Version 20261263: tool_param_rules 首批内置规则（79-runtime-governance R9 Phase 5.5）
-- gns3_exec：只读白名单 allow + 兜底 ask（非白名单命令逐次确认，可由 approve_always
--   grant 满足）；exec_command（shell/shell_exec 家簇定点）：危险模式 deny。
-- builtin- 前缀行 effect 只读、不可删除（biz 层强制）；pattern/priority/enabled 可改。
-- tool_key 存家簇定点（CanonicalParamRuleToolKey）：运行时别名家簇同效。
-- INSERT OR IGNORE 幂等（PG 翻译为 ON CONFLICT DO NOTHING），重跑/补种安全。
INSERT OR IGNORE INTO tool_param_rules (id, tool_key, pattern, effect, priority, enabled, created_at) VALUES
  ('builtin-gns3-allow-show',       'gns3_exec', 'show *',       'allow', 10,  1, 0),
  ('builtin-gns3-allow-ping',       'gns3_exec', 'ping *',       'allow', 10,  1, 0),
  ('builtin-gns3-allow-traceroute', 'gns3_exec', 'traceroute *', 'allow', 10,  1, 0),
  ('builtin-gns3-fallback-ask',     'gns3_exec', '*',            'ask',   900, 1, 0),
  ('builtin-exec-deny-rmrf-abs',    'exec_command', 'rm -rf /*',      'deny', 10, 1, 0),
  ('builtin-exec-deny-sudo-rmrf',   'exec_command', 'sudo rm -rf /*', 'deny', 10, 1, 0),
  ('builtin-exec-deny-mkfs',        'exec_command', 'mkfs*',          'deny', 10, 1, 0),
  ('builtin-exec-deny-dd-dev',      'exec_command', 'dd *of=/dev/*',  'deny', 10, 1, 0),
  ('builtin-exec-deny-shutdown',    'exec_command', 'shutdown*',      'deny', 10, 1, 0),
  ('builtin-exec-deny-reboot',      'exec_command', 'reboot*',        'deny', 10, 1, 0),
  ('builtin-exec-deny-poweroff',    'exec_command', 'poweroff*',      'deny', 10, 1, 0),
  ('builtin-exec-deny-halt',        'exec_command', 'halt*',          'deny', 10, 1, 0);
