SELECT id, left(coalesce(title,''),30) AS title, agent_id, created_at FROM sessions WHERE id IN ('49dd810d-87f8-4ebf-9636-8a433164e479','d78029b9-c305-4bc1-9583-ac9f743cdc60') ORDER BY created_at;
