-- Version 20261244: Agent runtime settings — 包A 装配预算硬闸（session-eval-20260825 A1/A4）
-- assembly_budget_soft/hard_tokens：单轮完全注入请求的绝对 token 预算（0=关闭）。
-- soft 超线注容量告警 cue（R2 MemGPT 范式，once-per-turn）；hard 超线按保护序
-- 丢弃尾部 cue → 驱逐最旧历史，静态头骨架永保，截断落 flowlog prompt.assembly.trimmed。
-- 与窗口比例压缩闸（hard_trigger_ratio）互补：比例闸管窗口溢出，本闸管绝对烧钱。
-- Idempotent: "duplicate column" errors are treated as success by the migration runner (DB-N6).
ALTER TABLE agent_runtime_settings ADD COLUMN assembly_budget_soft_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agent_runtime_settings ADD COLUMN assembly_budget_hard_tokens INTEGER NOT NULL DEFAULT 0;
