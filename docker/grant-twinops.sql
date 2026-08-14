-- 方案10 §3.1 岗位工具白名单落地（twinops 域 17 工具最小授权）
-- 高危工具 gns3_fault_inject/gns3_fault_clear 仅执行岗持有，DB 层 requires_confirmation=true 已触发 HITL 审批
BEGIN;

-- 值班长（部门主管-智能运维部）：态势感知只读 3 件
UPDATE agent_runtime_settings
SET tools_allow_json = '["twin_alarm_query","twin_device_search","twin_remediation_status"]',
    updated_at = NOW()
WHERE agent_id = 'b8aac85fdd616524975f11b1';

-- 诊断岗：告警/线路/设备/采集全取证链 10 件
UPDATE agent_runtime_settings
SET tools_allow_json = '["twin_alarm_query","twin_alarm_get","twin_alarm_ack","twin_alarm_rule_get","twin_line_status","twin_line_events","twin_device_get","twin_device_search","twin_device_metrics","twin_collector_status"]',
    updated_at = NOW()
WHERE agent_id = '71096314087d86e2caa20488';

-- 执行岗：GNS3 执行 + 处置单查询（保留已有 todo_write）
UPDATE agent_runtime_settings
SET tools_allow_json = '["todo_write","gns3_exec","gns3_fault_inject","gns3_fault_clear","twin_remediation_status"]',
    updated_at = NOW()
WHERE agent_id = '90fb01daa4c14a1580d8c828';

-- 验证岗：健康探测/主动复测/巡检（保留已有 memory_search）
UPDATE agent_runtime_settings
SET tools_allow_json = '["memory_search","gns3_health_check","gns3_exec","twin_line_status","twin_line_probe","twin_device_metrics","twin_inspection_query"]',
    updated_at = NOW()
WHERE agent_id = 'ba71aa004484fe6814a9b26a';

-- 复盘岗：时间线/处置单/巡检取证 4 件
UPDATE agent_runtime_settings
SET tools_allow_json = '["twin_alarm_query","twin_line_events","twin_remediation_status","twin_inspection_query"]',
    updated_at = NOW()
WHERE agent_id = 'a31090a3bab084b83f397a30';

COMMIT;

-- 验证
SELECT a.agent_key, s.tools_allow_json
FROM agents a JOIN agent_runtime_settings s ON s.agent_id = a.id
WHERE a.agent_key IN ('ops_fault_diagnosis','ops_change_execution','ops_system_inspection','ops_doc_generation','__dept_lead_aiops__')
ORDER BY a.agent_key;
