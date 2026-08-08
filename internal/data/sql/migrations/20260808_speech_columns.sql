-- Version 20260808: Speech service configuration columns (M74 V2-T7)
-- System Settings「语音服务」分组：ASR/TTS Provider 连接与凭据、音色、语速、语音留档开关。
-- Same raw-SQL pattern as planner_model_columns / refine_llm (ent generator blocked by
-- tablewriter version conflict) — columns are managed via DDL and accessed via raw SQL.
--
-- 读取语义（DB-first / env-fallback 字段级合并，见 internal/data/speech/system_config.go）：
--   - string 字段空串 = 未设置 → 回退 env（SPEECH_ASR_* / SPEECH_TTS_*）
--   - speech_tts_speed_ratio = 0 表示未设置 → 回退 env / 默认 1.0（proto3 零值对齐）
--   - speech_archive_user_audio 为 nullable 三态：NULL = 未设置 → 回退 env
--     SPEECH_ARCHIVE_USER_AUDIO（升级兼容：V1 env 开关不被静默覆盖）

-- ASR settings
ALTER TABLE system_settings ADD COLUMN speech_asr_driver TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN speech_asr_endpoint TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN speech_asr_app_key TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN speech_asr_access_key TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN speech_asr_resource_id TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN speech_asr_language TEXT NOT NULL DEFAULT '';
-- TTS settings
ALTER TABLE system_settings ADD COLUMN speech_tts_driver TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN speech_tts_endpoint TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN speech_tts_app_key TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN speech_tts_access_key TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN speech_tts_resource_id TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN speech_tts_voice TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN speech_tts_speed_ratio DOUBLE PRECISION NOT NULL DEFAULT 0;
-- Voice archive toggle (nullable tri-state: NULL = unset → env fallback)
ALTER TABLE system_settings ADD COLUMN speech_archive_user_audio BOOLEAN DEFAULT NULL;
