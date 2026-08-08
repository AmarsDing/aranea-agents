import { beforeEach, describe, expect, it } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { useCompanionStore } from '../companion';

describe('useCompanionStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('starts in idle with voice mode off and chat closed', () => {
    const store = useCompanionStore();
    expect(store.voiceState).toBe('idle');
    expect(store.voiceModeOn).toBe(false);
    expect(store.chatOpen).toBe(false);
    expect(store.subtitlePartial).toBe('');
    expect(store.lastError).toBeNull();
  });

  it('mirrors server voice.state broadcasts (禁止前端本地推测状态机)', () => {
    const store = useCompanionStore();
    store.setVoiceState('listening');
    expect(store.voiceState).toBe('listening');
    store.setVoiceState('speaking');
    expect(store.voiceState).toBe('speaking');
  });

  it('tracks realtime subtitle as transient state (不落消息流)', () => {
    const store = useCompanionStore();
    store.setSubtitlePartial('你好');
    store.setSubtitlePartial('你好世');
    expect(store.subtitlePartial).toBe('你好世');
    store.clearSubtitle();
    expect(store.subtitlePartial).toBe('');
  });

  it('records voice errors for degraded-mode display', () => {
    const store = useCompanionStore();
    store.setVoiceError({ code: 'TTS_UNAVAILABLE', message: 'tts down', retryable: true });
    expect(store.lastError?.code).toBe('TTS_UNAVAILABLE');
    store.clearVoiceError();
    expect(store.lastError).toBeNull();
  });

  it('voice.replaced forces voice mode off and back to idle', () => {
    const store = useCompanionStore();
    store.setVoiceMode(true);
    store.setVoiceState('listening');
    store.onVoiceReplaced();
    expect(store.voiceModeOn).toBe(false);
    expect(store.voiceState).toBe('idle');
    expect(store.subtitlePartial).toBe('');
  });

  it('toggles chat panel', () => {
    const store = useCompanionStore();
    store.toggleChat();
    expect(store.chatOpen).toBe(true);
    store.toggleChat();
    expect(store.chatOpen).toBe(false);
  });

  it('exiting voice mode clears subtitle and returns to idle', () => {
    const store = useCompanionStore();
    store.setVoiceMode(true);
    store.setVoiceState('speaking');
    store.setSubtitlePartial('abc');
    store.setVoiceMode(false);
    expect(store.voiceState).toBe('idle');
    expect(store.subtitlePartial).toBe('');
  });

  // V2-T8 差距2：麦克风置灰门控。三态语义——null=未知（探测失败/未拉取，
  // 不置灰，点击后由 voice.error 降级条兜底）；false=明确未配置（置灰）。
  it('gates the mic only when voice is explicitly unavailable', () => {
    const store = useCompanionStore();
    expect(store.voiceAvailable).toBeNull();
    expect(store.voiceMicDisabled).toBe(false); // 未知不置灰

    store.setVoiceAvailable(true);
    expect(store.voiceMicDisabled).toBe(false);

    store.setVoiceAvailable(false);
    expect(store.voiceMicDisabled).toBe(true); // 明确未配置 → 置灰

    store.setVoiceAvailable(null);
    expect(store.voiceMicDisabled).toBe(false); // 回到未知 → 解除置灰
  });
});
