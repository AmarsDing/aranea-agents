import { computed, ref } from 'vue';
import { defineStore } from 'pinia';

import type { VoiceError, VoiceState } from '../features/companion/types';

export type { VoiceError, VoiceState } from '../features/companion/types';

/**
 * 语音伴侣 Store（M74 设计 §7.2）：voiceState / 实时字幕 / 聊天面板状态。
 *
 * 单一数据源铁律：voiceState 完全镜像服务端 voice.state 广播（经
 * useVoiceSession handlers.onState 写入），前端不做本地状态机推测。
 */
export const useCompanionStore = defineStore('companion', () => {
  const voiceState = ref<VoiceState>('idle');
  const voiceModeOn = ref(false);
  const chatOpen = ref(false);
  const subtitlePartial = ref('');
  const lastError = ref<VoiceError | null>(null);
  /**
   * 语音服务可用性（V2-T8 差距2，GET /v1/voice/status 探测结果）。
   * 三态：null=未知（探测失败/未拉取，不置灰，点击后 voice.error 兜底）；
   * false=明确未配置（麦克风置灰门控）；true=可用。
   */
  const voiceAvailable = ref<boolean | null>(null);

  /** 麦克风置灰门控：仅明确不可用时置灰（未知不阻断，避免误伤）。 */
  const voiceMicDisabled = computed(() => voiceAvailable.value === false);

  function setVoiceAvailable(available: boolean | null) {
    voiceAvailable.value = available;
  }

  function setVoiceState(state: VoiceState) {
    voiceState.value = state;
  }

  function setVoiceMode(on: boolean) {
    voiceModeOn.value = on;
    if (!on) {
      // 退出语音模式：回到 idle 并清空瞬时字幕（设计 §7.3 字幕为 transient）
      voiceState.value = 'idle';
      subtitlePartial.value = '';
    }
  }

  function setSubtitlePartial(text: string) {
    subtitlePartial.value = text;
  }

  function clearSubtitle() {
    subtitlePartial.value = '';
  }

  function setVoiceError(err: VoiceError) {
    lastError.value = err;
  }

  function clearVoiceError() {
    lastError.value = null;
  }

  /** 同会话第二连接替换本连接（voice.replaced）：强制退出语音模式。 */
  function onVoiceReplaced() {
    voiceModeOn.value = false;
    voiceState.value = 'idle';
    subtitlePartial.value = '';
  }

  function toggleChat() {
    chatOpen.value = !chatOpen.value;
  }

  return {
    voiceState,
    voiceModeOn,
    chatOpen,
    subtitlePartial,
    lastError,
    voiceAvailable,
    voiceMicDisabled,
    setVoiceAvailable,
    setVoiceState,
    setVoiceMode,
    setSubtitlePartial,
    clearSubtitle,
    setVoiceError,
    clearVoiceError,
    onVoiceReplaced,
    toggleChat,
  };
});
