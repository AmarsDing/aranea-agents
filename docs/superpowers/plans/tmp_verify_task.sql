SELECT id, task_no, status, aranea_run_id, current_node_id, completed_nodes, result_summary IS NOT NULL AS has_output, duration_ms
FROM ai_tasks WHERE id = 2;
