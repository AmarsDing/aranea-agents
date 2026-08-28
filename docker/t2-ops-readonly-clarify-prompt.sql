-- T2（session-eval-20260827 战役裁定②）：ops 岗 system.md 尾部追加「默认只读」释义与高危工具调用纪律。
-- 根因：S05 ops_change_execution 两度拒调 gns3_fault_inject 并自撰「read-only 模式」——把模板头部
-- 「风险五级（readonly/low/medium/high/destructive），默认只读」（工具风险分级默认值）误读为
-- Agent 级运行模式限制。配置层并无此限制（C6 复测证实 gns3_exec 正常挂 HITL）。
-- 覆盖范围：共享同一模板头部（「默认只读」锚点）的全部 12 个 ops_* 岗——同款误读风险同源同修
-- （沿用 C6「全岗统一」裁定先例）；条件化措辞对无高危工具的岗无行为副作用。
-- 缓存纪律：沿用 fixb 模式，仅尾部追加，前文一字不动，LLM 前缀缓存仍命中。
-- 冲突消解：追加段第 4 条显式声明与 fixb 图编排定式及一般 HITL 约束的关系。
-- 三层校验：同事务内前置 COUNT → UPDATE → 后置核验。

BEGIN;

-- ① 前置：确认命中范围（预期 12 行）
SELECT count(*) AS pre_hit
FROM agent_prompt_files f
JOIN agents a ON a.id = f.agent_id
WHERE f.file_name = 'system.md'
  AND a.agent_key IN ('ops_alarm_handler','ops_auto_inspection','ops_change_execution','ops_command_expert',
                      'ops_compliance_check','ops_database','ops_doc_generation','ops_fault_diagnosis',
                      'ops_log_analysis','ops_network_inspection','ops_server_command','ops_system_inspection')
  AND f.body LIKE '%默认只读%'
  AND f.body NOT LIKE '%「默认只读」释义%';

-- ② 尾部追加（幂等：已含追加段的行被 WHERE 排除）
UPDATE agent_prompt_files f
SET body = f.body || $t2$

## 「默认只读」释义与高危工具调用纪律（2026-08-28 追加）

1. **「默认只读」不是运行模式**：上文「平台与边界」中「风险五级……默认只读」描述的是 MCP 工具风险分级的默认值（未显式标级的工具按只读对待），**不是你的运行模式**。系统不存在「read-only 模式 / 只读模式」这类 Agent 级限制，禁止虚构此类模式并以其为由拒绝调用工具。
2. **高危工具照常发起**：任务需要 high/destructive 级工具（如 `gns3_fault_inject`、`gns3_fault_clear`、设备写命令）时**正常发起调用**——系统门禁会自动挂起该调用并等待人工审批（HITL），审批通过后自动续跑。你的职责是发起调用并输出待审批说明，不是拒绝发起。
3. **拒调 ≠ 待审批**：「未获审批时输出待审批说明」指发起调用后等待审批，而非不发起调用。以「只读模式」「权限不足」「安全限制」等自撰理由直接拒调，属于违规行为。
4. 本条与上文各约束及「图编排自动处置场景操作定式」不冲突：图编排预授权场景仍按定式直接执行；一般场景高危调用仍必须经 HITL 审批通过后才真正执行。
$t2$,
    updated_at = NOW()
FROM agents a
WHERE a.id = f.agent_id
  AND f.file_name = 'system.md'
  AND a.agent_key IN ('ops_alarm_handler','ops_auto_inspection','ops_change_execution','ops_command_expert',
                      'ops_compliance_check','ops_database','ops_doc_generation','ops_fault_diagnosis',
                      'ops_log_analysis','ops_network_inspection','ops_server_command','ops_system_inspection')
  AND f.body LIKE '%默认只读%'
  AND f.body NOT LIKE '%「默认只读」释义%';

-- ③ 后置核验：已追加段落行数（预期 = 前置命中数）+ 无 CR 污染
SELECT a.agent_key, length(f.body) AS new_len, position(chr(13) in f.body) AS cr_pos
FROM agent_prompt_files f
JOIN agents a ON a.id = f.agent_id
WHERE f.file_name = 'system.md'
  AND a.agent_key IN ('ops_alarm_handler','ops_auto_inspection','ops_change_execution','ops_command_expert',
                      'ops_compliance_check','ops_database','ops_doc_generation','ops_fault_diagnosis',
                      'ops_log_analysis','ops_network_inspection','ops_server_command','ops_system_inspection')
  AND f.body LIKE '%「默认只读」释义%'
ORDER BY a.agent_key;

COMMIT;
