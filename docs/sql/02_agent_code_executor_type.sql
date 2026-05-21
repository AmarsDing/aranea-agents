-- CodeExecutor: per-agent execution backend (local | docker | e2b | container)
ALTER TABLE agent_runtime_settings ADD COLUMN code_executor_type TEXT NOT NULL DEFAULT 'local';
