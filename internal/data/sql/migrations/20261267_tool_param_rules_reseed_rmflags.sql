-- Version 20261267: tool_param_rules builtin rm deny 规则 flags 全覆盖 reseed
-- （2026-08-27 三轮审查根修，与 20261263 修订同步）。
-- 1) 20261265 的 flags 归一 -(rf|fr|r\s+-f|f\s+-r) 只覆盖短选项排列且 target
--    必须紧跟 flags——rm -rf --no-preserve-root /（GNU coreutils 下真实可删根
--    的唯一形态）、rm --recursive --force /、rm -r --force /、rm -rfv / 等
--    变形全部不命中。改为「任意多段 flags（含长选项与 -- 分隔）中至少一段
--    短选项簇含 r/R 或 --recursive，随后落危险 target」。短选项簇限定单
--    dash 前缀（-[a-zA-Z]*r[a-zA-Z]*），长选项仅 --recursive 计入递归语义，
--    --verbose/--force 等含 r 字母的长选项不误伤合法命令。
-- 2) 顺手修 gns3 allow 三行 created_at=0（最早期种子占位，20261264 只修了
--    exec deny 行）——仅展示/排序元数据，无安全影响。
-- WHERE 带旧 pattern 条件保证幂等、且管理员已自行改过的 pattern 不被回冲。
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''($`])rm(?:\s+(?:-{1,2}[\w=-]+|--))*\s+(?:-[a-zA-Z]*r[a-zA-Z]*|--recursive)(?:\s+(?:-{1,2}[\w=-]+|--))*\s+(/|~|\$HOME|\*)'
  WHERE id = 'builtin-exec-deny-rmrf-abs' AND pattern = 're:(?i)(^|[;&|/\s"''($`])rm\s+-(rf|fr|r\s+-f|f\s+-r)\s+(/|~|\$HOME|\*)';
UPDATE tool_param_rules SET pattern = 're:(?i)(^|[;&|/\s"''($`])sudo\s+(-\S+\s+)*rm(?:\s+(?:-{1,2}[\w=-]+|--))*\s+(?:-[a-zA-Z]*r[a-zA-Z]*|--recursive)(?:\s+(?:-{1,2}[\w=-]+|--))*\s+(/|~|\$HOME|\*)'
  WHERE id = 'builtin-exec-deny-sudo-rmrf' AND pattern = 're:(?i)(^|[;&|/\s"''($`])sudo\s+(-\S+\s+)*rm\s+-(rf|fr|r\s+-f|f\s+-r)\s+(/|~|\$HOME|\*)';
UPDATE tool_param_rules SET created_at = 1787760000
  WHERE id IN ('builtin-gns3-allow-show', 'builtin-gns3-allow-ping', 'builtin-gns3-allow-traceroute') AND created_at = 0;
