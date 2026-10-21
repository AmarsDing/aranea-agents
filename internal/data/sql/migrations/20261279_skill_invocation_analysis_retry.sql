-- Version 20261278: skill_invocation 报告分析队列重试/死信列（审计 W5②）
-- 背景：ScanAndGenerateReports 对 GenerateReport 失败的激活仅 Warn + continue，
-- 行保持 analyzed_at=''；ListUnanalyzed 按 created_at ASC LIMIT 100 取批——
-- 永久失败的毒行每轮（15min）重扫（日志轰炸），积压 100 条后整个分析队列
-- 队头阻塞，新激活再也生成不了经验报告。
-- 方案：analysis_attempts 记录失败次数，RecordAnalysisFailure 自增并落最近
-- 错误；ListUnanalyzed 谓词排除 attempts 达上限的行（隔离死信，可经
-- analysis_error 排查后手工清零复活）。
-- 双方言通用（INTEGER/TEXT）；Idempotent: "duplicate column" 由迁移 runner 视为成功（DB-N6）。
ALTER TABLE skill_invocation ADD COLUMN analysis_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE skill_invocation ADD COLUMN analysis_error TEXT NOT NULL DEFAULT '';
