-- Version 20261255: memory_fact_pending 增加 payload_json（79-runtime-governance R3 Phase 3.3）
-- 审批中心接入（C7 桥 twinmonitor ai_approvals，source=memory_fact_write）要求 approve
-- 回写后忠实重放原 bi-temporal 写；既有列（proposed_body/prior_body/reason）不足以重建
-- 完整 FactWriteDecision（缺 scope/kind/confidence/importance/tags/source 等元数据）。
-- payload_json 存扣留时刻的 decision 快照（candidate 全字段 + target_fact_id），
-- 决议执行时反序列化重放，避免信息丢失导致的写入质量降级。
-- TEXT 双方言通用。幂等，重跑安全（DB-N6 duplicate column 视为成功）。
ALTER TABLE memory_fact_pending ADD COLUMN payload_json TEXT NOT NULL DEFAULT '';
