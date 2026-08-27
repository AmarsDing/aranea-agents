-- Version 20261263: tool_param_rules 首批内置规则（79-runtime-governance R9 Phase 5.5）
-- gns3_exec：只读白名单 allow（单行锚定 regex）+ 兜底 ask（非白名单命令逐次确认，
--   可由 approve_always grant 满足）；exec_command（shell/shell_exec 家簇定点）：危险模式 deny。
-- builtin- 前缀行 effect/tool_key 只读、不可删除（biz 层强制）；pattern/priority/enabled 可改。
-- tool_key 存家簇定点（CanonicalParamRuleToolKey）：运行时别名家簇同效。
-- INSERT OR IGNORE 幂等（PG 翻译为 ON CONFLICT DO NOTHING），重跑/补种安全。
--
-- 2026-08-27 审计修订（P5.4 P5）：exec_command deny 规则从整串锚定 glob 升级为
-- 正则子串语义（re: 前缀）——旧 glob 字面量空格敏感，rm  -rf（双空格）、sudo 包装、
-- sh -c "..."、/bin/rm 绝对路径等变形均可绕过。存量库由 20261264 reseed 迁移升级
-- （INSERT OR IGNORE 不会覆盖既有行）。
--
-- 2026-08-27 二轮审查加固（存量库由 20261265 reseed 迁移同步）：
-- 1) deny 分隔符类 [;&|/\s"'] 补 ( $ 反引号——$(cmd)/`cmd`/(cmd) 命令替换与子
--    shell 形态此前全部绕过；rm flags 归一 -(rf|fr|r\s+-f|f\s+-r)，-fr、-r -f、
--    -f -r 变形此前绕过。
-- 2) gns3 allow 由 glob 改单行锚定 regex：globToRegexp 补 s 旗标后 glob '*' 跨换
--    行，多行注入（"show version\nwrite erase"）会被 allow 放行；改 ^cmd [^\n]*$
--    后多行/变形一律落兜底 ask（安全方向）。
-- 3) 分隔符类保留 /：/bin/rm、/sbin/reboot 绝对路径执行形态依赖它命中；代价是
--    ls /tmp/halt 这类「路径片段=命令名」被 fail-safe 误拒——可解释、方向安全，
--    优于放开 /sbin/reboot 旁路。
INSERT OR IGNORE INTO tool_param_rules (id, tool_key, pattern, effect, priority, enabled, created_at) VALUES
  ('builtin-gns3-allow-show',       'gns3_exec', 're:(?i)^show [^\n]*$',       'allow', 10,  1, 1787760000),
  ('builtin-gns3-allow-ping',       'gns3_exec', 're:(?i)^ping [^\n]*$',       'allow', 10,  1, 1787760000),
  ('builtin-gns3-allow-traceroute', 'gns3_exec', 're:(?i)^traceroute [^\n]*$', 'allow', 10,  1, 1787760000),
  ('builtin-gns3-fallback-ask',     'gns3_exec', '*',                          'ask',   900, 1, 1787760000),
  ('builtin-exec-deny-rmrf-abs',    'exec_command', 're:(?i)(^|[;&|/\s"''($`])rm\s+-(rf|fr|r\s+-f|f\s+-r)\s+(/|~|\$HOME|\*)',              'deny', 10, 1, 1787760000),
  ('builtin-exec-deny-sudo-rmrf',   'exec_command', 're:(?i)(^|[;&|/\s"''($`])sudo\s+(-\S+\s+)*rm\s+-(rf|fr|r\s+-f|f\s+-r)\s+(/|~|\$HOME|\*)', 'deny', 10, 1, 1787760000),
  ('builtin-exec-deny-mkfs',        'exec_command', 're:(?i)(^|[;&|/\s"''($`])mkfs[\s.]',                             'deny', 10, 1, 1787760000),
  ('builtin-exec-deny-dd-dev',      'exec_command', 're:(?i)(^|[;&|/\s"''($`])dd\s+[^;&|]*of=/dev/',                  'deny', 10, 1, 1787760000),
  ('builtin-exec-deny-shutdown',    'exec_command', 're:(?i)(^|[;&|/\s"''($`])shutdown(\s|$)',                        'deny', 10, 1, 1787760000),
  ('builtin-exec-deny-reboot',      'exec_command', 're:(?i)(^|[;&|/\s"''($`])reboot(\s|$)',                          'deny', 10, 1, 1787760000),
  ('builtin-exec-deny-poweroff',    'exec_command', 're:(?i)(^|[;&|/\s"''($`])poweroff(\s|$)',                        'deny', 10, 1, 1787760000),
  ('builtin-exec-deny-halt',        'exec_command', 're:(?i)(^|[;&|/\s"''($`])halt(\s|$)',                            'deny', 10, 1, 1787760000);
