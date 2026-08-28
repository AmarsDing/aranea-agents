-- Version 20261270: intent_skip_enabled=false for Spirit + governance leads
-- (P0 P-INTENT-SKIP). Column default remains 1 for ordinary agents; management
-- agents must not skip intent pass when QuickAssess mis-labels a task simple.
-- INTEGER 0/1. Idempotent UPDATE.
UPDATE agent_runtime_settings
SET intent_skip_enabled = 0
WHERE intent_skip_enabled != 0
  AND agent_id IN (
    SELECT id FROM agents
    WHERE deleted_at = ''
      AND (
        agent_key = '__spirit__'
        OR agent_variant IN ('company_lead', 'dept_lead')
        OR agent_key LIKE '__company_lead_%'
        OR agent_key LIKE '__dept_lead_%'
      )
  );
