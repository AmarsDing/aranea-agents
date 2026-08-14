/**
 * 麦克风采集（设计 §7.2 audioCapture）：
 * getUserMedia（echoCancellation+noiseSuppression，NFR3 扬声器场景）
 * → AudioWorklet 转发 Float32 原生帧 → 累积 20ms → 重采样 16k → s16le 上行帧。
 *
 * Web Audio 图封装在本模块内：HUD 频谱经暴露的 analyser 取 FFT 数据（设计 §7.4），
 * VAD 经 onPcm16k 取 16k 浮点帧。重采样/编码核心在 pcm.ts（已单测）。
 */

import { downsampleTo16k, floatToS16le, VOICE_TARGET_SAMPLE_RATE } from './pcm';

export type AudioCaptureOptions = {
  /** 上行帧回调：s16le 16kHz mono 20ms（640B），直发 /v1/voice 二进制帧。 */
  onVoiceFrame: (buf: ArrayBuffer) => void;
  /** 16k 浮点帧回调（VAD / 能量可视化用），与上行帧同节奏。 */
  onPcm16k?: (frame: Float32Array) => void;
  /** 回声消除（默认 true；扬声器播报场景必须，NFR3）。 */
  echoCancellation?: boolean;
};

export type AudioCapture = {
  /** 申请麦克风并启动采集；权限拒绝/无设备时 reject。 */
  start(): Promise<void>;
  /** 停止采集并释放全部音频资源。幂等。 */
  stop(): void;
  readonly running: boolean;
  /** 采集侧 AnalyserNode（HUD 频谱环数据源）；未启动时为 null。 */
  readonly analyser: AnalyserNode | null;
};

/** AudioWorklet 处理器源码：把输入声道 0 的 Float32 块原样转发到主线程。 */
const WORKLET_SOURCE = `
class PcmForwarder extends AudioWorkletProcessor {
  process(inputs) {
    const ch = inputs[0] && inputs[0][0];
    if (ch && ch.length > 0) {
      const copy = ch.slice(0);
      this.port.postMessage(copy, [copy.buffer]);
    }
    return true;
  }
}
registerProcessor('pcm-forwarder', PcmForwarder);
`;

/**
 * V11-T1（设计 §17.2）：getUserMedia 采集约束。
 * voiceIsolation（Chrome 118+ ML 人声隔离，压制背景人声）与 autoGainControl
 * 为增量抗干扰约束；不支持的浏览器按 WebIDL 静默忽略未知基础约束，零风险降级。
 * TS DOM lib 未含 voiceIsolation 字段，故以扩展类型声明（不 cast any）。
 */
interface CompanionAudioConstraints extends MediaTrackConstraints {
  voiceIsolation?: boolean;
}

export function captureAudioConstraints(): MediaTrackConstraints {
  const c: CompanionAudioConstraints = {
    channelCount: 1,
    echoCancellation: true,
    noiseSuppression: true,
    autoGainControl: true,
    voiceIsolation: true,
  };
  return c;
}

export function createAudioCapture(options: AudioCaptureOptions): AudioCapture {
  let stream: MediaStream | null = null;
  let ctx: AudioContext | null = null;
  let worklet: AudioWorkletNode | null = null;
  let source: MediaStreamAudioSourceNode | null = null;
  let analyserNode: AnalyserNode | null = null;
  let workletUrl: string | null = null;
  // 原生采样率下的 20ms 帧累积缓冲
  let pending: Float32Array = new Float32Array(0);

  function handleBlock(block: Float32Array, nativeRate: number): void {
    const frameLen = Math.round((nativeRate * 20) / 1000);
    const merged = new Float32Array(pending.length + block.length);
    merged.set(pending, 0);
    merged.set(block, pending.length);
    let offset = 0;
    while (offset + frameLen <= merged.length) {
      const frame = merged.slice(offset, offset + frameLen);
      offset += frameLen;
      const pcm16k = downsampleTo16k(frame, nativeRate);
      options.onPcm16k?.(pcm16k);
      const s16 = floatToS16le(pcm16k);
      options.onVoiceFrame(s16.buffer.slice(s16.byteOffset, s16.byteOffset + s16.byteLength) as ArrayBuffer);
    }
    pending = merged.slice(offset);
  }

  return {
    async start(): Promise<void> {
      if (ctx) return;
      stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          ...captureAudioConstraints(),
          echoCancellation: options.echoCancellation ?? true,
        },
      });
      ctx = new AudioContext();
      workletUrl = URL.createObjectURL(new Blob([WORKLET_SOURCE], { type: 'application/javascript' }));
      await ctx.audioWorklet.addModule(workletUrl);
      source = ctx.createMediaStreamSource(stream);
      analyserNode = ctx.createAnalyser();
      analyserNode.fftSize = 256;
      worklet = new AudioWorkletNode(ctx, 'pcm-forwarder');
      const nativeRate = ctx.sampleRate;
      worklet.port.onmessage = (ev: MessageEvent<Float32Array>) => {
        handleBlock(ev.data, nativeRate);
      };
      source.connect(analyserNode);
      analyserNode.connect(worklet);
      // worklet 不挂 destination：仅采集转发，不回放（回放走 PcmPlayer）
    },

    stop(): void {
      if (worklet) {
        worklet.port.onmessage = null;
        worklet.disconnect();
        worklet = null;
      }
      if (source) {
        source.disconnect();
        source = null;
      }
      if (analyserNode) {
        analyserNode.disconnect();
        analyserNode = null;
      }
      if (stream) {
        for (const track of stream.getTracks()) track.stop();
        stream = null;
      }
      if (ctx) {
        void ctx.close().catch(() => undefined);
        ctx = null;
      }
      if (workletUrl) {
        URL.revokeObjectURL(workletUrl);
        workletUrl = null;
      }
      pending = new Float32Array(0);
    },

    get running() {
      return ctx !== null;
    },
    get analyser() {
      return analyserNode;
    },
  };
}

export { VOICE_TARGET_SAMPLE_RATE };
