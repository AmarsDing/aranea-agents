/**
 * 语音上行 PCM 处理（设计 §10：PCM s16le 16kHz mono，20ms/帧 640B）。
 *
 * 纯函数模块：从 AudioWorklet 拿到的 Float32 原生采样率帧，
 * 重采样到 16kHz 后编码为小端 s16，供 /v1/voice 二进制帧直发。
 */

/** 语音链路目标采样率（NFR3）。 */
export const VOICE_TARGET_SAMPLE_RATE = 16000;

/**
 * 线性插值重采样到 16kHz。
 * 输入为单声道 Float32；输入率即 16k 时返回拷贝（保持调用方不可变语义）。
 */
export function downsampleTo16k(input: Float32Array, inputSampleRate: number): Float32Array {
  if (input.length === 0) return new Float32Array(0);
  if (inputSampleRate === VOICE_TARGET_SAMPLE_RATE) return new Float32Array(input);
  const ratio = inputSampleRate / VOICE_TARGET_SAMPLE_RATE;
  const outLength = Math.round(input.length / ratio);
  const out = new Float32Array(outLength);
  for (let i = 0; i < outLength; i++) {
    const srcPos = i * ratio;
    const left = Math.floor(srcPos);
    const right = Math.min(left + 1, input.length - 1);
    const frac = srcPos - left;
    out[i] = input[left] + (input[right] - input[left]) * frac;
  }
  return out;
}

/**
 * Float32 [-1,1] → s16le Int16Array（越界裁剪）。
 * 负半轴满幅映射 -32768，正半轴满幅映射 32767（常规非对称映射防爆音）。
 * 经 DataView 强制小端，跨平台字节序一致。
 */
export function floatToS16le(input: Float32Array): Int16Array {
  const out = new Int16Array(input.length);
  const view = new DataView(out.buffer);
  for (let i = 0; i < input.length; i++) {
    const v = Math.max(-1, Math.min(1, input[i]));
    const s = v < 0 ? Math.round(v * 0x8000) : Math.round(v * 0x7fff);
    view.setInt16(i * 2, s, true);
  }
  return out;
}

/** 一帧上行编码：原生采样率 Float32 → 16k s16le ArrayBuffer（直发 WS 二进制帧）。 */
export function encodeVoiceFrame(input: Float32Array, inputSampleRate: number): ArrayBuffer {
  const pcm16 = floatToS16le(downsampleTo16k(input, inputSampleRate));
  return pcm16.buffer.slice(pcm16.byteOffset, pcm16.byteOffset + pcm16.byteLength) as ArrayBuffer;
}
