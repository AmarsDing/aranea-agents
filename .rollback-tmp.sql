SELECT left(payload_json::text, 700) AS payload, created_at FROM flow_log_events
WHERE created_at >= '2026-08-18 16:20' AND created_at <= '2026-08-18 17:05'
  AND payload_json::jsonb->>'step_id' IN ('system.ws.read_error','system.ws.send_failed','monitor.selfcheck.run')
ORDER BY created_at;
