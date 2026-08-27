-- 包C C6 岗位 agent 装配预算闸（session-eval-20260827 P11v / optimization-package.md）
--
-- 实证依据（S05 session cac18413 的 L0 快照逐轮对账）：
--   S05-t1 121K = 6 次模型调用 × ~20K（轮次型累积，非单次爆炸）；单次 ~20K 中
--   ~13K 为 19 件 twin_*/gns3_* 静态工具 schema，message 侧 est 仅 ~7K。
--   assembly_budget 闸计量口径只含 messages，对静态 schema 结构性失明——
--   故 C6 落地 = deferred 工具（主杠杆）+ assembly_budget 开闸（message 侧兜底）。
--
-- 机制：registry_map.go 已补 15 件 twin_* 查询/配置类恒等映射；auto-split 路径
--   （ToolsDeferredJSON 为空时）自动把「已映射且不在 profile 核心集」的工具并入
--   延迟目录（tool_load 按需加载）。常驻五件（gns3_health_check/exec/fault_inject/
--   fault_clear + twin_remediation_status）无映射 = 代码层保证永不 defer
--   （fixB 图编排定式：常驻可见、禁止 tool_search）。
--
-- 本脚本两件事：
--   1) 清掉 4 岗的 tools_deferred_json='[]' 手动覆盖（恢复 auto-split）；
--   2) 11 个 ops 岗统一开 assembly_budget soft/hard 40000/60000（对齐管理层灰度值）。
BEGIN;

-- 1) 恢复 auto-split（'[]' = 手动空清单 = 禁止任何 defer，是 121K 的配置根因）
UPDATE agent_runtime_settings s
SET tools_deferred_json = '', updated_at = NOW()
FROM agents a
WHERE a.id = s.agent_id
  AND a.agent_key IN ('ops_change_execution','ops_fault_diagnosis','ops_system_inspection','ops_doc_generation')
  AND s.tools_deferred_json = '[]';

-- 2) 全部 11 个 ops 岗开装配预算闸（soft=40K / hard=60K，0=关闭 → 对齐管理层）
UPDATE agent_runtime_settings s
SET assembly_budget_soft_tokens = 40000,
    assembly_budget_hard_tokens = 60000,
    updated_at = NOW()
FROM agents a
WHERE a.id = s.agent_id
  AND a.agent_key IN (
    'ops_alarm_handler','ops_auto_inspection','ops_change_execution','ops_command_expert',
    'ops_compliance_check','ops_database','ops_doc_generation','ops_fault_diagnosis',
    'ops_log_analysis','ops_network_inspection','ops_server_command'
  );

COMMIT;

-- 验证：2 岗 deferred 覆盖已清、11 岗预算已开
SELECT a.agent_key, s.tools_profile, s.tools_deferred_json,
       s.assembly_budget_soft_tokens, s.assembly_budget_hard_tokens
FROM agents a JOIN agent_runtime_settings s ON s.agent_id = a.id
WHERE a.agent_key IN (
  'ops_alarm_handler','ops_auto_inspection','ops_change_execution','ops_command_expert',
  'ops_compliance_check','ops_database','ops_doc_generation','ops_fault_diagnosis',
  'ops_log_analysis','ops_network_inspection','ops_server_command'
)
ORDER BY a.agent_key;
