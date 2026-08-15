-- 回滚：dept_lead 装配不进 spirit 编排工具，恢复值班长纯态势感知 3 件（最小授权）
UPDATE agent_runtime_settings
SET tools_allow_json = '["twin_alarm_query","twin_device_search","twin_remediation_status"]',
    updated_at = NOW()
WHERE agent_id = 'b8aac85fdd616524975f11b1';
