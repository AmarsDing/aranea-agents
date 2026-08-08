-- Version 20260809: Speech X-Api-Key credential columns (M74 真机校准)
-- 火山控制台新 API Key 鉴权模式：单 key（X-Api-Key header）替代 legacy
-- AppKey+AccessKey 对。两模式并存——api_key 非空优先，否则回退 app_key/access_key
-- （biz.SpeechCredOK）。空串 = 未设置 → 回退 env（SPEECH_ASR_API_KEY / SPEECH_TTS_API_KEY）。

ALTER TABLE system_settings ADD COLUMN speech_asr_api_key TEXT NOT NULL DEFAULT '';
ALTER TABLE system_settings ADD COLUMN speech_tts_api_key TEXT NOT NULL DEFAULT '';
