import type { SpeechSettings } from '../../services/kratos/system_setting/v1/index';

// SpeechFormState mirrors the System Settings「语音服务」分组 (M74 V2-T7).
// Credential fields (app_key/access_key) are write-only: never populated from
// the server; empty on save = keep stored (same convention as web-research).
export type SpeechFormState = {
  asr_driver: string;
  asr_endpoint: string;
  asr_app_key: string;
  asr_access_key: string;
  asr_resource_id: string;
  asr_language: string;
  tts_driver: string;
  tts_endpoint: string;
  tts_app_key: string;
  tts_access_key: string;
  tts_resource_id: string;
  tts_voice: string;
  tts_speed_ratio: number;
  archive_user_audio: boolean;
};

export const DEFAULT_SPEECH_FORM: SpeechFormState = {
  asr_driver: 'volcengine',
  asr_endpoint: '',
  asr_app_key: '',
  asr_access_key: '',
  asr_resource_id: '',
  asr_language: 'zh-CN',
  tts_driver: 'volcengine',
  tts_endpoint: '',
  tts_app_key: '',
  tts_access_key: '',
  tts_resource_id: '',
  tts_voice: '',
  tts_speed_ratio: 1.0,
  archive_user_audio: false,
};

export const SPEECH_DRIVER_OPTIONS = [{ label: 'Volcengine', value: 'volcengine' }] as const;

export const SPEECH_ASR_LANGUAGE_OPTIONS = [
  { label: 'zh-CN', value: 'zh-CN' },
  { label: 'en-US', value: 'en-US' },
] as const;

export function speechFromSettings(row?: SpeechSettings | null): SpeechFormState {
  if (!row) return { ...DEFAULT_SPEECH_FORM };
  return {
    asr_driver: row.asr?.driver || DEFAULT_SPEECH_FORM.asr_driver,
    asr_endpoint: row.asr?.endpoint || '',
    asr_app_key: '',
    asr_access_key: '',
    asr_resource_id: row.asr?.resourceId || '',
    asr_language: row.asr?.language || DEFAULT_SPEECH_FORM.asr_language,
    tts_driver: row.tts?.driver || DEFAULT_SPEECH_FORM.tts_driver,
    tts_endpoint: row.tts?.endpoint || '',
    tts_app_key: '',
    tts_access_key: '',
    tts_resource_id: row.tts?.resourceId || '',
    tts_voice: row.tts?.voice || '',
    tts_speed_ratio:
      (row.tts?.speedRatio ?? 0) > 0 ? row.tts!.speedRatio! : DEFAULT_SPEECH_FORM.tts_speed_ratio,
    archive_user_audio: Boolean(row.archiveUserAudio),
  };
}

export type SpeechPatch = {
  asrDriver?: string;
  asrEndpoint?: string;
  asrResourceId?: string;
  asrLanguage?: string;
  asrAppKey?: string;
  asrAccessKey?: string;
  ttsDriver?: string;
  ttsEndpoint?: string;
  ttsResourceId?: string;
  ttsVoice?: string;
  ttsSpeedRatio?: number;
  ttsAppKey?: string;
  ttsAccessKey?: string;
  // Tri-state: undefined = keep stored (proto3 optional not set); boolean =
  // explicit set (ends env SPEECH_ARCHIVE_USER_AUDIO fallback).
  archiveUserAudio?: boolean;
};

// speechToPatch builds the update payload as a DIFF against the loaded
// snapshot: only changed fields are included, plus any non-empty credentials
// (write-only). Returns undefined when nothing changed — the request then
// omits the whole speech group (hasSpeechUpdate=false server-side).
//
// Why diff-based (unlike web-research's always-send): every speech field has
// SPEECH_* env fallback with "empty/0 = unset" semantics. Always-sending the
// collapsed display value (e.g. speed 1.0 for stored 0, archive false for
// stored NULL) would silently end the env override on any unrelated save.
export function speechToPatch(form: SpeechFormState, loaded: SpeechFormState): SpeechPatch | undefined {
  const patch: SpeechPatch = {};
  let changed = false;
  type StrKey =
    | 'asrDriver'
    | 'asrEndpoint'
    | 'asrResourceId'
    | 'asrLanguage'
    | 'ttsDriver'
    | 'ttsEndpoint'
    | 'ttsResourceId'
    | 'ttsVoice';
  const setStr = (key: StrKey, next: string, prev: string) => {
    const v = next.trim();
    if (v !== prev) {
      patch[key] = v;
      changed = true;
    }
  };
  setStr('asrDriver', form.asr_driver, loaded.asr_driver);
  setStr('asrEndpoint', form.asr_endpoint, loaded.asr_endpoint);
  setStr('asrResourceId', form.asr_resource_id, loaded.asr_resource_id);
  setStr('asrLanguage', form.asr_language, loaded.asr_language);
  setStr('ttsDriver', form.tts_driver, loaded.tts_driver);
  setStr('ttsEndpoint', form.tts_endpoint, loaded.tts_endpoint);
  setStr('ttsResourceId', form.tts_resource_id, loaded.tts_resource_id);
  setStr('ttsVoice', form.tts_voice, loaded.tts_voice);
  if (form.tts_speed_ratio !== loaded.tts_speed_ratio) {
    patch.ttsSpeedRatio = form.tts_speed_ratio;
    changed = true;
  }
  if (form.archive_user_audio !== loaded.archive_user_audio) {
    patch.archiveUserAudio = form.archive_user_audio;
    changed = true;
  }
  const asrAppKey = form.asr_app_key.trim();
  if (asrAppKey) {
    patch.asrAppKey = asrAppKey;
    changed = true;
  }
  const asrAccessKey = form.asr_access_key.trim();
  if (asrAccessKey) {
    patch.asrAccessKey = asrAccessKey;
    changed = true;
  }
  const ttsAppKey = form.tts_app_key.trim();
  if (ttsAppKey) {
    patch.ttsAppKey = ttsAppKey;
    changed = true;
  }
  const ttsAccessKey = form.tts_access_key.trim();
  if (ttsAccessKey) {
    patch.ttsAccessKey = ttsAccessKey;
    changed = true;
  }
  return changed ? patch : undefined;
}
