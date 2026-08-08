/**
 * 聊天页听写 Composable（M74 dictation 模式）：
 * 麦克风 → /v1/voice（mode=dictation）→ ASR 终稿文本经 onFinalText 填入输入框。
 *
 * 与 useVoiceSession（语音精灵对话模式）的差异：
 * - 不订阅 TTS、不建 Chat Turn（服务端语义，见 internal/voice/session.go）；
 * - 语音状态以前端本地 dictating 为准（服务端恒 listening），不写 companion store；
 * - 连续听写：终稿下行后会话保持 listening，可继续说下一句。
 */

import { getCurrentInstance, onUnmounted, ref, type Ref } from 'vue';

import type { VoiceError } from '../../companion/types';
import {
  createAudioCapture,
  type AudioCapture,
  type AudioCaptureOptions,
} from '../../companion/voice/audioCapture';
import { VOICE_TARGET_SAMPLE_RATE } from '../../companion/voice/pcm';
import {
  createVoiceSessionClient,
  type VoiceSessionClient,
  type VoiceSessionClientOptions,
} from '../../companion/voice/useVoiceSession';

/** voice.start 听写模式标识（与后端 voice.ModeDictation 对齐）。 */
export const VOICE_MODE_DICTATION = 'dictation';

export type VoiceDictationDeps = {
  /** 当前聊天会话 ID（voice WS 按会话归属）；为 null 时拒绝启动。 */
  sessionId: () => string | null;
  /** ASR 终稿回调：由调用方拼入输入框。 */
  onFinalText: (text: string) => void;
  /** 错误上报（NO_SESSION / MIC_UNAVAILABLE / VOICE_CHANNEL_CLOSED / 服务端 voice.error）。 */
  onError: (err: VoiceError) => void;
  /** 测试注入：协议客户端工厂。 */
  clientFactory?: (opts: VoiceSessionClientOptions) => VoiceSessionClient;
  /** 测试注入：麦克风采集工厂。 */
  captureFactory?: (opts: AudioCaptureOptions) => AudioCapture;
};

export type UseVoiceDictationReturn = {
  /** 听写中（启动成功 → 再次点击/出错/通道关闭后复位）。 */
  dictating: Ref<boolean>;
  /** 识别中的部分文本（输入框上方实时字幕）。 */
  partial: Ref<string>;
  /** 麦克风按钮：启动/停止切换。 */
  toggle(): Promise<void>;
  /** 显式停止（会话切换 / 组件卸载时调用）。 */
  stop(): void;
};

/**
 * 终稿文本拼入输入框：中文/标点语境直接拼接；
 * 英文或数字结尾且新增以英文/数字开头时补一个空格。
 */
export function joinDictationText(existing: string, addition: string): string {
  if (!existing) return addition;
  if (!addition) return existing;
  const needSpace = /[A-Za-z0-9]$/.test(existing) && /^[A-Za-z0-9]/.test(addition);
  return existing + (needSpace ? ' ' : '') + addition;
}

export function useVoiceDictation(deps: VoiceDictationDeps): UseVoiceDictationReturn {
  const dictating = ref(false);
  const partial = ref('');

  let client: VoiceSessionClient | null = null;
  let capture: AudioCapture | null = null;

  const makeClient = deps.clientFactory ?? createVoiceSessionClient;
  const makeCapture = deps.captureFactory ?? createAudioCapture;

  function teardown(): void {
    capture?.stop();
    capture = null;
    client?.disconnect();
    client = null;
    dictating.value = false;
    partial.value = '';
  }

  async function start(): Promise<void> {
    const sessionId = deps.sessionId();
    if (!sessionId) {
      deps.onError({ code: 'NO_SESSION', message: '', retryable: true });
      return;
    }

    client = makeClient({
      sessionId,
      handlers: {
        // 听写态以前端 dictating 为准；服务端恒广播 listening，无需镜像。
        onState: () => undefined,
        onPartial: (text) => {
          partial.value = text;
        },
        onFinal: (text) => {
          partial.value = '';
          if (text) deps.onFinalText(text);
        },
        onTurnAccepted: () => undefined,
        onTtsStart: () => undefined,
        onTtsAudio: () => undefined,
        onTtsEnd: () => undefined,
        onVoiceError: (err) => {
          teardown();
          deps.onError(err);
        },
        onReplaced: () => {
          // 同会话第二连接替换：本地静默停止（对方继续）
          teardown();
        },
        onClose: () => {
          if (!dictating.value) return; // 主动停止后的迟到 close 不报错
          teardown();
          deps.onError({ code: 'VOICE_CHANNEL_CLOSED', message: '', retryable: true });
        },
      },
    });
    client.connect();
    client.startVoice({ sampleRate: VOICE_TARGET_SAMPLE_RATE, mode: VOICE_MODE_DICTATION });

    capture = makeCapture({
      onVoiceFrame: (frame) => client?.sendAudio(frame),
    });
    try {
      await capture.start();
    } catch {
      teardown();
      deps.onError({ code: 'MIC_UNAVAILABLE', message: '', retryable: true });
      return;
    }
    dictating.value = true;
  }

  function stop(): void {
    if (!client && !capture) return;
    client?.stopVoice();
    teardown();
  }

  // 组件内使用时随卸载自动停止；测试（无组件实例）由调用方显式 stop()。
  if (getCurrentInstance()) {
    onUnmounted(stop);
  }

  return {
    dictating,
    partial,
    async toggle(): Promise<void> {
      if (dictating.value) {
        stop();
      } else {
        await start();
      }
    },
    stop,
  };
}
