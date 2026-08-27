-- Version 20261264: tool_param_rules builtin deny 规则正则化 reseed
-- （79-runtime-governance R9 P5.4 审计 P5，与 20261263 修订同步）。
-- 存量库已应用 20261263 的 INSERT OR IGNORE 不会覆盖既有 glob 行，本迁移逐行
-- UPDATE 为正则子串语义；WHERE 带旧 pattern 条件保证幂等、且用户已自行改过的
-- pattern 不被回冲。created_at=0（旧种子占位）顺手修为种子发布时点。
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''])rm\s+-rf\s+(/|~|\$HOME|\*)',
  created_at = CASE WHEN created_at = 0 THEN 1787760000 ELSE created_at END
  WHERE id = 'builtin-exec-deny-rmrf-abs' AND pattern = 'rm -rf /*';
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''])sudo\s+(-\S+\s+)*rm\s+-rf\s+(/|~|\$HOME|\*)',
  created_at = CASE WHEN created_at = 0 THEN 1787760000 ELSE created_at END
  WHERE id = 'builtin-exec-deny-sudo-rmrf' AND pattern = 'sudo rm -rf /*';
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''])mkfs[\s.]',
  created_at = CASE WHEN created_at = 0 THEN 1787760000 ELSE created_at END
  WHERE id = 'builtin-exec-deny-mkfs' AND pattern = 'mkfs*';
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''])dd\s+[^;&|]*of=/dev/',
  created_at = CASE WHEN created_at = 0 THEN 1787760000 ELSE created_at END
  WHERE id = 'builtin-exec-deny-dd-dev' AND pattern = 'dd *of=/dev/*';
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''])shutdown(\s|$)',
  created_at = CASE WHEN created_at = 0 THEN 1787760000 ELSE created_at END
  WHERE id = 'builtin-exec-deny-shutdown' AND pattern = 'shutdown*';
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''])reboot(\s|$)',
  created_at = CASE WHEN created_at = 0 THEN 1787760000 ELSE created_at END
  WHERE id = 'builtin-exec-deny-reboot' AND pattern = 'reboot*';
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''])poweroff(\s|$)',
  created_at = CASE WHEN created_at = 0 THEN 1787760000 ELSE created_at END
  WHERE id = 'builtin-exec-deny-poweroff' AND pattern = 'poweroff*';
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''])halt(\s|$)',
  created_at = CASE WHEN created_at = 0 THEN 1787760000 ELSE created_at END
  WHERE id = 'builtin-exec-deny-halt' AND pattern = 'halt*';
