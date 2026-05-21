-- Runner: Ralph Loop per-agent settings (see docs/需求/40 runner.design.md)
ALTER TABLE agent_runtime_settings ADD COLUMN ralph_loop_max_iterations INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agent_runtime_settings ADD COLUMN ralph_loop_completion_promise TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_runtime_settings ADD COLUMN ralph_loop_verify_command TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_runtime_settings ADD COLUMN ralph_loop_verify_timeout_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agent_runtime_settings ADD COLUMN ralph_loop_promise_tag_open TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_runtime_settings ADD COLUMN ralph_loop_promise_tag_close TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_runtime_settings ADD COLUMN ralph_loop_verify_work_dir TEXT NOT NULL DEFAULT '';
