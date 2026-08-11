/**
 * `/v1/voice` 语音通道客户端（M74 设计 §2）+ 语音会话 Composable。
 *
 * `createVoiceSessionClient`：协议核心——连接生命周期、JSON 控制帧上行、
 * 二进制 PCM 帧上行/下行、下行事件分发。socketFactory 注入以便单测。
 *
 * 状态机不做前端本地推测（交叉参考 §2.7 红线）：voiceState 完全以服务端
 * `voice.state` 广播为准，经 handlers.onState 上报。
 */

import { buildVoiceWsUrl } from '../../../config/runtime';
import { VOICE_TARGET_SAMPLE_RATE } from './pcm';
import type { VoiceState } from '../types';

export type { VoiceState } from '../types';

const KNOWN_STATES: ReadonlySet<string> = new Set([
  'idle',
  'listening',
  'thinking',
  'speaking',
  'interrupted',
  'error',
]);

export type VoiceDownstreamHandlers = {
  onState(state: VoiceState): void;
  onPartial(text: string): void;
  onFinal(text: string, durationMs: number | undefined): void;
  onTurnAccepted(turnId: string): void;
  onTtsStart(info: { encoding: string; sampleRate: number }): void;
  onTtsAudio(pcm: ArrayBuffer): void;
  /** interrupted=true 表示被打断（barge_in/cancel）。 */
  onTtsEnd(interrupted: boolean): void;
  onVoiceError(err: { code: string; message: string; retryable: boolean }): void;
  /** 同会话第二连接到达，本连接被替换（设计 §2.1）。 */
  onReplaced(): void;
  onOpen?(): void;
  onClose?(): void;
};

export type VoiceStartParams = {
  sampleRate?: number;
  language?: string;
  dialogMode?: string;
  agentKey?: string;
  teamId?: string;
  /** 会话模式：空=对话（默认）；'dictation'=听写（终稿仅下行文本，不建 Turn 不播报）。 */
  mode?: string;
};

export type VoiceSessionClientOptions = {
  /** 聊天会话 ID；显式传 url 时忽略。 */
  sessionId?: string;
  /** 显式 WS URL（测试注入用）。 */
  url?: string;
  socketFactory?: (url: string) => WebSocket;
  handlers: VoiceDownstreamHandlers;
};

export type VoiceSessionClient = {
  connect(): void;
  disconnect(): void;
  startVoice(params?: VoiceStartParams): void;
  stopVoice(): void;
  commit(): void;
  bargeIn(detectMs: number): void;
  cancel(): void;
  /** 上行音频帧（s16le 16k）；连接未就绪时丢弃（实时音频不排队）。 */
  sendAudio(frame: ArrayBuffer): void;
  readonly connected: boolean;
};

export function createVoiceSessionClient(opts: VoiceSessionClientOptions): VoiceSessionClient {
  const url = opts.url ?? buildVoiceWsUrl({ sessionId: opts.sessionId ?? '' });
  let ws: WebSocket | null = null;
  let pendingJson: string[] = [];

  function isOpen(): boolean {
    return ws !== null && ws.readyState === WebSocket.OPEN;
  }

  function sendJson(msg: Record<string, unknown>): void {
    const raw = JSON.stringify(msg);
    if (isOpen()) {
      ws!.send(raw);
    } else {
      pendingJson.push(raw);
    }
  }

  function handleMessage(data: unknown): void {
    if (typeof data !== 'string') {
      // 二进制帧 = TTS 音频 chunk
      if (data instanceof ArrayBuffer) {
        opts.handlers.onTtsAudio(data);
      }
      return;
    }
    let msg: Record<string, unknown>;
    try {
      msg = JSON.parse(data) as Record<string, unknown>;
    } catch {
      return; // 畸形帧忽略
    }
    switch (msg.type) {
      case 'voice.state':
        if (KNOWN_STATES.has(msg.state as string)) {
          opts.handlers.onState(msg.state as VoiceState);
        }
        return;
      case 'asr.partial':
        opts.handlers.onPartial((msg.text as string) ?? '');
        return;
      case 'asr.final':
        opts.handlers.onFinal((msg.text as string) ?? '', msg.duration_ms as number | undefined);
        return;
      case 'turn.accepted':
        opts.handlers.onTurnAccepted((msg.turn_id as string) ?? '');
        return;
      case 'tts.start':
        opts.handlers.onTtsStart({
          encoding: (msg.encoding as string) ?? 'pcm_f32le_16k',
          sampleRate: (msg.sample_rate as number) ?? VOICE_TARGET_SAMPLE_RATE,
        });
        return;
      case 'tts.end':
        opts.handlers.onTtsEnd(msg.interrupted === true);
        return;
      case 'voice.error':
        opts.handlers.onVoiceError({
          code: (msg.code as string) ?? 'UNKNOWN',
          message: (msg.message as string) ?? '',
          retryable: msg.retryable === true,
        });
        return;
      case 'voice.replaced':
        opts.handlers.onReplaced();
        return;
      case 'pong':
        return;
      default:
        return;
    }
  }

  return {
    connect(): void {
      if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
        return;
      }
      const socket = opts.socketFactory ? opts.socketFactory(url) : new WebSocket(url);
      ws = socket;
      // 二进制帧 = TTS 音频：必须显式声明 arraybuffer，浏览器默认 'blob'
      // 会让 onmessage 收到 Blob 而被静默丢弃（全部 TTS 音频无声）。
      socket.binaryType = 'arraybuffer';
      socket.onopen = () => {
        if (ws !== socket) return;
        for (const raw of pendingJson) socket.send(raw);
        pendingJson = [];
        opts.handlers.onOpen?.();
      };
      socket.onmessage = (ev: MessageEvent) => {
        if (ws !== socket) return;
        handleMessage(ev.data);
      };
      socket.onclose = () => {
        if (ws !== socket) return;
        ws = null;
        opts.handlers.onClose?.();
      };
      socket.onerror = () => {
        // 错误经 onclose 收口；语音错误由服务端 voice.error 帧承载
      };
    },

    disconnect(): void {
      const socket = ws;
      ws = null;
      pendingJson = [];
      if (socket) {
        socket.onclose = null;
        socket.close();
      }
    },

    startVoice(params: VoiceStartParams = {}): void {
      sendJson({
        type: 'voice.start',
        sample_rate: params.sampleRate ?? VOICE_TARGET_SAMPLE_RATE,
        language: params.language ?? '',
        dialog_mode: params.dialogMode ?? '',
        agent_key: params.agentKey ?? '',
        team_id: params.teamId ?? '',
        mode: params.mode ?? '',
      });
    },

    stopVoice(): void {
      sendJson({ type: 'voice.stop' });
    },

    commit(): void {
      sendJson({ type: 'voice.commit' });
    },

    bargeIn(detectMs: number): void {
      sendJson({ type: 'voice.barge_in', detect_ms: detectMs });
    },

    cancel(): void {
      sendJson({ type: 'voice.cancel' });
    },

    sendAudio(frame: ArrayBuffer): void {
      if (isOpen()) {
        ws!.send(frame);
      }
    },

    get connected() {
      return isOpen();
    },
  };
}

// ---------------------------------------------------------------------------
// Composable：语音会话编排（采集 → /v1/voice → 播放），状态写入 companion store。
// ---------------------------------------------------------------------------

import { onUnmounted, shallowRef, type ShallowRef } from 'vue';
import { useI18n } from 'vue-i18n';

import { useCompanionStore } from '../../../stores/companion';
import { createAudioCapture, type AudioCapture } from './audioCapture';
import { createPcmPlayer, type PcmPlayer } from './audioPlayback';
import { createVad, decideVadAction, type Vad } from './vad';

/** barge-in 人声持续阈值（ms），与 vad 默认 speechOnsetMs 一致；上行 detect_ms 语义。 */
const BARGE_IN_DETECT_MS = 200;

export type UseVoiceSessionReturn = {
  /** listening 态采集侧 FFT 频谱（HUD 频谱环数据源）。 */
  spectrum: ShallowRef<Uint8Array | null>;
  /** speaking 态播放侧振幅 [0,1]（HUD 能量核脉动数据源）。 */
  amplitude: ShallowRef<number>;
  /** 进入/退出语音模式（麦克风按钮）。 */
  toggleVoiceMode(): Promise<void>;
  /** 显式取消当前 Turn（voice.cancel）。 */
  cancelTurn(): void;
};

/** TTS chunk 时域能量 → 可视振幅（增益 ×3 后钳制）。 */
function chunkAmplitude(f32: Float32Array): number {
  if (f32.length === 0) return 0;
  let sum = 0;
  for (let i = 0; i < f32.length; i++) sum += f32[i] * f32[i];
  return Math.min(1, Math.sqrt(sum / f32.length) * 3);
}

export function useVoiceSession(deps: {
  sessionId: () => string | null;
  /**
   * M74 V9-T4（设计 74 §15.4-E）：进入语音模式前异步解析目标会话——
   * 选中/创建语音助手（__voice_butler__）会话。缺省回退 sessionId() 同步取值。
   */
  resolveSession?: () => Promise<string | null>;
  /** M74 V9-T4：退出语音模式后回调（恢复先前选中的会话）。 */
  onExit?: () => void;
}): UseVoiceSessionReturn {
  const { t } = useI18n();
  const store = useCompanionStore();

  const spectrum = shallowRef<Uint8Array | null>(null);
  const amplitude = shallowRef(0);

  let client: VoiceSessionClient | null = null;
  let capture: AudioCapture | null = null;
  let player: PcmPlayer | null = null;
  let vad: Vad | null = null;
  let rafId = 0;
  let fftBuf: Uint8Array<ArrayBuffer> | null = null;

  function ensurePlayer(): PcmPlayer {
    if (!player) player = createPcmPlayer({});
    return player;
  }

  function visualTick(): void {
    rafId = requestAnimationFrame(visualTick);
    const analyser = capture?.analyser ?? null;
    if (store.voiceState === 'listening' && analyser) {
      if (!fftBuf || fftBuf.length !== analyser.frequencyBinCount) {
        fftBuf = new Uint8Array(analyser.frequencyBinCount);
      }
      analyser.getByteFrequencyData(fftBuf);
      spectrum.value = fftBuf;
    } else {
      spectrum.value = null;
    }
  }

  function teardownLocal(): void {
    cancelAnimationFrame(rafId);
    rafId = 0;
    fftBuf = null;
    spectrum.value = null;
    amplitude.value = 0;
    capture?.stop();
    capture = null;
    vad = null;
    player?.stop(50);
  }

  /**
   * VAD 接线（V2-T1）：16k 浮点帧 → VAD → 按状态机镜像决策动作。
   * barge_in：先本地停播（≤300ms 停播实测预算）再上行控制帧，服务端终判。
   */
  function handleVadFrame(frame: Float32Array): void {
    if (!vad) return;
    const action = decideVadAction(vad.process(frame), store.voiceState);
    if (action === 'barge_in') {
      amplitude.value = 0;
      player?.stop(50);
      client?.bargeIn(BARGE_IN_DETECT_MS);
      vad.reset(); // 防残余人声立即重触发；新 onset 需重新累积 200ms
    } else if (action === 'commit') {
      client?.commit();
    }
  }

  async function enterVoiceMode(): Promise<void> {
    const sessionId = deps.resolveSession ? await deps.resolveSession() : deps.sessionId();
    if (!sessionId) {
      store.setVoiceError({ code: 'NO_SESSION', message: t('companion.voiceNoSession'), retryable: true });
      return;
    }
    store.clearVoiceError();

    client = createVoiceSessionClient({
      sessionId,
      handlers: {
        onState: (s) => {
          // 退出语音模式后忽略迟到的非 idle 广播，防 HUD 卡在非 idle 态
          if (!store.voiceModeOn && s !== 'idle') return;
          store.setVoiceState(s);
          if (s !== 'speaking') amplitude.value = 0;
        },
        onPartial: (text) => store.setSubtitlePartial(text),
        onFinal: () => store.clearSubtitle(),
        onTurnAccepted: () => undefined,
        onTtsStart: () => ensurePlayer(),
        onTtsAudio: (pcm) => {
          const f32 = new Float32Array(pcm);
          amplitude.value = chunkAmplitude(f32);
          ensurePlayer().enqueue(f32);
        },
        onTtsEnd: (interrupted) => {
          amplitude.value = 0;
          if (interrupted) player?.stop(50);
        },
        onVoiceError: (err) => store.setVoiceError(err),
        onReplaced: () => {
          // 同会话第二连接替换本连接：本地资源回收 + 状态复位（store 语义）
          teardownLocal();
          client?.disconnect();
          client = null;
          store.onVoiceReplaced();
        },
        onClose: () => {
          if (store.voiceModeOn) {
            teardownLocal();
            store.setVoiceMode(false);
            store.setVoiceError({
              code: 'VOICE_CHANNEL_CLOSED',
              message: t('companion.voiceChannelClosed'),
              retryable: true,
            });
          }
        },
      },
    });
    client.connect();
    client.startVoice({ sampleRate: VOICE_TARGET_SAMPLE_RATE });

    try {
      vad = createVad({ sampleRate: VOICE_TARGET_SAMPLE_RATE, speechOnsetMs: BARGE_IN_DETECT_MS });
      capture = createAudioCapture({
        onVoiceFrame: (frame) => client?.sendAudio(frame),
        onPcm16k: handleVadFrame,
      });
      await capture.start();
    } catch {
      vad = null;
      client.disconnect();
      client = null;
      capture = null;
      store.setVoiceError({ code: 'MIC_UNAVAILABLE', message: t('companion.micUnavailable'), retryable: true });
      return;
    }

    store.setVoiceMode(true);
    visualTick();
  }

  function exitVoiceMode(): void {
    client?.stopVoice();
    client?.disconnect();
    client = null;
    teardownLocal();
    store.setVoiceMode(false);
    deps.onExit?.();
  }

  onUnmounted(() => {
    exitVoiceMode();
    player?.dispose();
    player = null;
  });

  return {
    spectrum,
    amplitude,
    async toggleVoiceMode(): Promise<void> {
      if (store.voiceModeOn) {
        exitVoiceMode();
      } else {
        await enterVoiceMode();
      }
    },
    cancelTurn(): void {
      client?.cancel();
    },
  };
}
