-- Version 20261265: tool_param_rules builtin 规则加固 reseed
-- （2026-08-27 二轮审查 H4，与 20261263 修订同步）。
-- 1) deny 分隔符类 [;&|/\s"'] 补 ( $ 反引号——$(rm -rf /)/`rm -rf /`/(rm -rf /)
--    命令替换与子 shell 形态此前全部绕过 deny；rm flags 归一
--    -(rf|fr|r\s+-f|f\s+-r)，-fr、-r -f、-f -r 变形此前绕过。
-- 2) gns3 allow 由 glob 改单行锚定 regex：globToRegexp 补 s 旗标后 glob '*' 跨
--    换行，多行注入（"show version\nwrite erase"）会被 allow 放行；改
--    ^cmd [^\n]*$ 后多行/变形一律落兜底 ask（安全方向）。
-- 3) 分隔符类保留 /：/bin/rm、/sbin/reboot 绝对路径执行形态依赖它命中；代价是
--    路径片段=命令名被 fail-safe 误拒（方向安全，优于放开 /sbin/reboot 旁路）。
-- WHERE 带旧 pattern 条件保证幂等、且管理员已自行改过的 pattern 不被回冲。
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''($`])rm\s+-(rf|fr|r\s+-f|f\s+-r)\s+(/|~|\$HOME|\*)'
  WHERE id = 'builtin-exec-deny-rmrf-abs' AND pattern = 're:(?i)(^|[;&|/\s"''])rm\s+-rf\s+(/|~|\$HOME|\*)';
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''($`])sudo\s+(-\S+\s+)*rm\s+-(rf|fr|r\s+-f|f\s+-r)\s+(/|~|\$HOME|\*)'
  WHERE id = 'builtin-exec-deny-sudo-rmrf' AND pattern = 're:(?i)(^|[;&|/\s"''])sudo\s+(-\S+\s+)*rm\s+-rf\s+(/|~|\$HOME|\*)';
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''($`])mkfs[\s.]'
  WHERE id = 'builtin-exec-deny-mkfs' AND pattern = 're:(?i)(^|[;&|/\s"''])mkfs[\s.]';
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''($`])dd\s+[^;&|]*of=/dev/'
  WHERE id = 'builtin-exec-deny-dd-dev' AND pattern = 're:(?i)(^|[;&|/\s"''])dd\s+[^;&|]*of=/dev/';
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''($`])shutdown(\s|$)'
  WHERE id = 'builtin-exec-deny-shutdown' AND pattern = 're:(?i)(^|[;&|/\s"''])shutdown(\s|$)';
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''($`])reboot(\s|$)'
  WHERE id = 'builtin-exec-deny-reboot' AND pattern = 're:(?i)(^|[;&|/\s"''])reboot(\s|$)';
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''($`])poweroff(\s|$)'
  WHERE id = 'builtin-exec-deny-poweroff' AND pattern = 're:(?i)(^|[;&|/\s"''])poweroff(\s|$)';
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''($`])halt(\s|$)'
  WHERE id = 'builtin-exec-deny-halt' AND pattern = 're:(?i)(^|[;&|/\s"''])halt(\s|$)';
UPDATE tool_param_rules SET pattern = 're:(?i)^show [^\n]*$'
  WHERE id = 'builtin-gns3-allow-show' AND pattern = 'show *';
UPDATE tool_param_rules SET pattern = 're:(?i)^ping [^\n]*$'
  WHERE id = 'builtin-gns3-allow-ping' AND pattern = 'ping *';
UPDATE tool_param_rules SET pattern = 're:(?i)^traceroute [^\n]*$'
  WHERE id = 'builtin-gns3-allow-traceroute' AND pattern = 'traceroute *';
