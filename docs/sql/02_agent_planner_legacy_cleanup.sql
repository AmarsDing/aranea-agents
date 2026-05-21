-- One-off cleanup: planner_kind empty but non-empty planner_config_json (pre-2026-05-21 validation).
-- Run after backup. New saves already reject non-{} config when planner_kind IS NULL OR ''.

-- Preview rows to fix:
-- SELECT id, planner_kind, planner_config_json FROM agent WHERE trim(coalesce(planner_kind,'')) = '' AND trim(coalesce(planner_config_json,'')) NOT IN ('', '{}');

UPDATE agent
SET planner_config_json = '{}'
WHERE trim(coalesce(planner_kind, '')) = ''
  AND trim(coalesce(planner_config_json, '')) NOT IN ('', '{}');
