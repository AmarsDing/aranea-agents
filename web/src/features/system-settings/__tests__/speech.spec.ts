import { describe, expect, it } from 'vitest';
import {
  DEFAULT_SPEECH_FORM,
  speechFromSettings,
  speechToPatch,
  type SpeechFormState,
} from '../speech';
import type { SpeechSettings } from '../../../services/kratos/system_setting/v1/index';

function loaded(overrides: Partial<SpeechFormState> = {}): SpeechFormState {
  return { ...DEFAULT_SPEECH_FORM, ...overrides };
}

describe('speechFromSettings', () => {
  it('returns defaults when row is nullish', () => {
    expect(speechFromSettings(null)).toEqual(DEFAULT_SPEECH_FORM);
    expect(speechFromSettings(undefined)).toEqual(DEFAULT_SPEECH_FORM);
  });

  it('maps stored values and never exposes credentials', () => {
    const row: SpeechSettings = {
      asr: {
        driver: 'volcengine',
        endpoint: 'wss://asr',
        resourceId: 'rid',
        language: 'en-US',
        configured: true,
        hasApiKey: true,
      },
      tts: {
        driver: 'volcengine',
        endpoint: 'wss://tts',
        resourceId: 'rid2',
        voice: 'zh_female_x',
        speedRatio: 1.5,
        configured: true,
        hasApiKey: true,
      },
      archiveUserAudio: true,
    };
    const form = speechFromSettings(row);
    expect(form.asr_endpoint).toBe('wss://asr');
    expect(form.asr_language).toBe('en-US');
    expect(form.tts_voice).toBe('zh_female_x');
    expect(form.tts_speed_ratio).toBe(1.5);
    expect(form.archive_user_audio).toBe(true);
    // Credentials are write-only: always empty in the form.
    expect(form.asr_app_key).toBe('');
    expect(form.asr_access_key).toBe('');
    expect(form.tts_app_key).toBe('');
    expect(form.tts_access_key).toBe('');
  });

  it('falls back to defaults for zero/empty stored values', () => {
    const form = speechFromSettings({
      asr: { driver: '', endpoint: '', resourceId: '', language: '', configured: false, hasApiKey: false },
      tts: { driver: '', endpoint: '', resourceId: '', voice: '', speedRatio: 0, configured: false, hasApiKey: false },
      archiveUserAudio: false,
    });
    expect(form.asr_driver).toBe(DEFAULT_SPEECH_FORM.asr_driver);
    expect(form.asr_language).toBe(DEFAULT_SPEECH_FORM.asr_language);
    expect(form.tts_speed_ratio).toBe(DEFAULT_SPEECH_FORM.tts_speed_ratio);
    expect(form.archive_user_audio).toBe(false);
  });
});

describe('speechToPatch', () => {
  it('returns undefined when nothing changed (env fallback untouched)', () => {
    const snap = loaded({ asr_endpoint: 'wss://asr', tts_speed_ratio: 1.2 });
    const form = { ...snap };
    expect(speechToPatch(form, snap)).toBeUndefined();
  });

  it('includes only changed fields', () => {
    const snap = loaded();
    const form = { ...snap, tts_voice: 'zh_female_y', tts_speed_ratio: 1.3 };
    const patch = speechToPatch(form, snap);
    expect(patch).toEqual({ ttsVoice: 'zh_female_y', ttsSpeedRatio: 1.3 });
  });

  it('sends archive toggle only when flipped', () => {
    const snap = loaded({ archive_user_audio: false });
    expect(speechToPatch({ ...snap }, snap)).toBeUndefined();
    expect(speechToPatch({ ...snap, archive_user_audio: true }, snap)).toEqual({ archiveUserAudio: true });
  });

  it('includes non-empty credentials regardless of diff', () => {
    const snap = loaded();
    const patch = speechToPatch({ ...snap, asr_app_key: '  ak-x ', tts_access_key: 'sk-y' }, snap);
    expect(patch).toEqual({ asrAppKey: 'ak-x', ttsAccessKey: 'sk-y' });
  });

  it('whitespace-only credential does not count as change', () => {
    const snap = loaded();
    expect(speechToPatch({ ...snap, asr_app_key: '   ' }, snap)).toBeUndefined();
  });

  it('trims string fields before comparing', () => {
    const snap = loaded({ tts_voice: 'v1' });
    // Same after trim → no change.
    expect(speechToPatch({ ...snap, tts_voice: ' v1 ' }, snap)).toBeUndefined();
  });
});
