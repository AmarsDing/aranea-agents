/**
 * 科幻音效状态机联动（M74 设计 §7.4 v2，V5-T5）。
 *
 * 监听 companion store 的 voiceModeOn / voiceState 变化播放程序合成音效：
 * - 语音模式开启 → 上电扫频 boot
 * - idle → listening（新唤醒）→ chirp
 * - thinking 期间 → 声纳 ping 循环
 * - →interrupted → 打断切音 cut
 * 确认卡 approve/deny 由页面调用 play('ding'/'buzz')。
 */

import { ref, watch } from 'vue';

import { useCompanionStore } from '../../../stores/companion';
import {
  soundEnabledFromStorage,
  UI_SOUNDS_STORAGE_KEY,
  UiSoundEngine,
  type AudioContextLike,
  type UISoundKind,
} from './uiSounds';

let sharedEngine: UiSoundEngine | null = null;

/** 真实 AudioContext 适配（浏览器自动播放策略：首次播放时 resume）。 */
type RealAudioContext = AudioContextLike & { state?: string; resume?: () => Promise<void> };

function getSharedEngine(): UiSoundEngine {
  if (!sharedEngine) {
    const ctx = new AudioContext() as RealAudioContext;
    sharedEngine = new UiSoundEngine({
      context: ctx,
      enabled: soundEnabledFromStorage(localStorage.getItem(UI_SOUNDS_STORAGE_KEY)),
      resume: () => void ctx.resume?.().catch(() => undefined),
    });
  }
  return sharedEngine;
}

export function useUiSounds() {
  const companion = useCompanionStore();
  const engine = getSharedEngine();
  const soundsEnabled = ref(engine.isEnabled());

  /** 播放一次性音效。 */
  function play(kind: UISoundKind): void {
    engine.play(kind);
  }

  function setSoundsEnabled(on: boolean): void {
    engine.setEnabled(on);
    soundsEnabled.value = engine.isEnabled();
  }

  // 语音模式开启 → 上电扫频
  watch(
    () => companion.voiceModeOn,
    (on, was) => {
      if (on && !was) play('boot');
    },
  );

  // 语音状态联动（仅语音模式内）
  watch(
    () => companion.voiceState,
    (state, prev) => {
      if (!companion.voiceModeOn) return;
      // chirp = 进入聆听反馈：idle→listening（旧直进）+ dormant→listening（V10 唤醒）
      if (state === 'listening' && (prev === 'idle' || prev === 'dormant')) play('chirp');
      if (state === 'thinking') {
        engine.startThinkingLoop();
      } else {
        engine.stopThinkingLoop();
      }
      if (state === 'interrupted') play('cut');
    },
  );

  return { soundsEnabled, setSoundsEnabled, play };
}
