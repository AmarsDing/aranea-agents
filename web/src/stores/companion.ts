import { ref } from 'vue';
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
