-- T2 后续修正（session-eval-20260827 阶段3 交叉影响扫描）：从 10 个 read_only ops 岗撤回
-- 「默认只读」释义追加段，仅保留 2 个 minimal 岗（ops_change_execution / ops_fault_diagnosis）。
--
-- 背景：T2 追加段第 1 条「系统不存在 read-only 模式，禁止虚构」与 <permission_state> 块的
-- 「Mode: read-only…对写请求说你在只读模式」字面矛盾。根修 = 代码层域裁剪
-- （internal/agent/permission_state.go：business minimal 岗 / spirit / system_* 不再注入该块）：
--   - 2 个 minimal 岗：误分类块移除，T2 段成为唯一权限指引（载荷所在，保留）
--   - 10 个 read_only 岗：块如实（确无写工具），T2 段四条全部惰性（无高危工具可发起），
--     仅存矛盾副作用 → 撤回
-- 形态：该段为尾部追加（t2-ops-readonly-clarify-prompt.sql 的 body || $t2$），撤回即锚点截断 + rtrim。
-- 三层校验：同事务内前置 COUNT → UPDATE → 后置核验。

BEGIN;

-- ① 前置：确认命中范围（预期 10 行）
SELECT count(*) AS pre_hit
FROM agent_prompt_files f
JOIN agents a ON a.id = f.agent_id
WHERE f.file_name = 'system.md'
  AND a.agent_key IN ('ops_alarm_handler','ops_auto_inspection','ops_command_expert',
                      'ops_compliance_check','ops_database','ops_doc_generation',
                      'ops_log_analysis','ops_network_inspection','ops_server_command','ops_system_inspection')
  AND f.body LIKE '%「默认只读」释义与高危工具调用纪律%';

-- ② 锚点截断（幂等：不含锚点的行被 WHERE 排除）
UPDATE agent_prompt_files f
SET body = rtrim(left(f.body, position('## 「默认只读」释义与高危工具调用纪律' in f.body) - 1)),
    updated_at = NOW()
FROM agents a
WHERE a.id = f.agent_id
  AND f.file_name = 'system.md'
  AND a.agent_key IN ('ops_alarm_handler','ops_auto_inspection','ops_command_expert',
                      'ops_compliance_check','ops_database','ops_doc_generation',
                      'ops_log_analysis','ops_network_inspection','ops_server_command','ops_system_inspection')
  AND f.body LIKE '%「默认只读」释义与高危工具调用纪律%';

-- ③ 后置核验 a：10 个 read_only 岗应已全部不含锚点（预期 0 行）
SELECT count(*) AS residual
FROM agent_prompt_files f
JOIN agents a ON a.id = f.agent_id
WHERE f.file_name = 'system.md'
  AND a.agent_key IN ('ops_alarm_handler','ops_auto_inspection','ops_command_expert',
                      'ops_compliance_check','ops_database','ops_doc_generation',
                      'ops_log_analysis','ops_network_inspection','ops_server_command','ops_system_inspection')
  AND f.body LIKE '%「默认只读」释义与高危工具调用纪律%';

-- ③ 后置核验 b：2 个 minimal 岗必须仍含该段（预期 2 行），且模板头部锚点完好
SELECT a.agent_key,
       f.body LIKE '%「默认只读」释义与高危工具调用纪律%' AS has_t2,
       f.body LIKE '%默认只读%' AS has_template_anchor,
       length(f.body) AS len
FROM agent_prompt_files f
JOIN agents a ON a.id = f.agent_id
WHERE f.file_name = 'system.md'
  AND a.agent_key IN ('ops_change_execution','ops_fault_diagnosis')
ORDER BY a.agent_key;

COMMIT;
