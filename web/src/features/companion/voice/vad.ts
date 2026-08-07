/**
 * 前端 VAD（设计 §7.2）：能量 + 过零率双重判定。
 *
 * 双重职责：
 * ① 播报期人声检测 —— 持续人声 ≥ speechOnsetMs（默认 200ms）发 speech_sustained（触发 barge_in）；
 * ② 判停兜底 —— onset 之后静音 ≥ silenceHangoverMs（默认 700ms）发 silence_timeout（触发 voice.commit）。
 *
 * 语句端点判定以火山 ASR 服务端 VAD 为主，本模块仅为兜底（设计 §7.2 vad 职责②）。
 */

import type { VoiceState } from '../types';

export type VadEvent = 'speech_sustained' | 'silence_timeout';

/** VAD 事件驱动的高层动作（V2-T1 链路：前端 VAD → 本地停播 → 上行控制帧）。 */
export type VadAction = 'barge_in' | 'commit';

export type VadOptions = {
  /** 采样率（上行 16k）。 */
  sampleRate: number;
  /** 帧长 ms（默认 20）。 */
  frameMs?: number;
  /** RMS 能量阈值（默认 0.01）。 */
  energyThreshold?: number;
  /** 过零率上限：超过视为噪声而非浊音（默认 0.3）。 */
  zcrThreshold?: number;
  /** barge-in 人声持续阈值 ms（默认 200，设计 §2.2 上行帧 detect_ms 语义）。 */
  speechOnsetMs?: number;
  /** 判停兜底静音时长 ms（默认 700）。 */
  silenceHangoverMs?: number;
};

export type Vad = {
  /** 喂一帧 PCM（Float32，长度 = sampleRate * frameMs / 1000），有事件时返回。 */
  process(frame: Float32Array): VadEvent | null;
  /** 清空全部计时状态。 */
  reset(): void;
  /** 当前是否处于人声段（能量判定层面，未达 onset 也为 true）。 */
  readonly speaking: boolean;
};

export function createVad(opts: VadOptions): Vad {
  const energyThreshold = opts.energyThreshold ?? 0.01;
  const zcrThreshold = opts.zcrThreshold ?? 0.3;
  const speechOnsetMs = opts.speechOnsetMs ?? 200;
  const silenceHangoverMs = opts.silenceHangoverMs ?? 700;
  const frameMs = opts.frameMs ?? 20;

  let speechMs = 0;
  let silenceMs = 0;
  let onsetFired = false;
  let timeoutFired = false;

  function isSpeechFrame(frame: Float32Array): boolean {
    if (frame.length === 0) return false;
    let sumSquares = 0;
    let zeroCrossings = 0;
    for (let i = 0; i < frame.length; i++) {
      sumSquares += frame[i] * frame[i];
      if (i > 0 && frame[i - 1] * frame[i] < 0) zeroCrossings++;
    }
    const rms = Math.sqrt(sumSquares / frame.length);
    const zcr = zeroCrossings / frame.length;
    return rms > energyThreshold && zcr <= zcrThreshold;
  }

  return {
    process(frame: Float32Array): VadEvent | null {
      if (isSpeechFrame(frame)) {
        speechMs += frameMs;
        silenceMs = 0;
        if (!onsetFired && speechMs >= speechOnsetMs) {
          onsetFired = true;
          timeoutFired = false;
          return 'speech_sustained';
        }
        return null;
      }
      // 静音帧
      if (speechMs > 0 && !onsetFired) {
        // 未达 onset 的短促噪声段：直接清零，不进入判停计时
        speechMs = 0;
        return null;
      }
      if (onsetFired && !timeoutFired) {
        silenceMs += frameMs;
        if (silenceMs >= silenceHangoverMs) {
          timeoutFired = true;
          // 一句话结束：重新武装下一轮 onset
          speechMs = 0;
          silenceMs = 0;
          onsetFired = false;
          return 'silence_timeout';
        }
      }
      return null;
    },
    reset(): void {
      speechMs = 0;
      silenceMs = 0;
      onsetFired = false;
      timeoutFired = false;
    },
    get speaking() {
      return speechMs > 0;
    },
  };
}

/**
 * VAD 事件 → 高层动作决策（纯函数，与状态机镜像解耦以便单测）。
 *
 * - speech_sustained：仅 speaking/thinking 态触发 barge_in（设计 §2.2/§5：
 *   播报或等待回复期间用户开口即打断）；listening 态的 onset 是正常语句起点，不动作。
 * - silence_timeout：仅 listening 态发 voice.commit 判停兜底（设计 §7.2 职责②）；
 *   其余态说明服务端已端点（或无需提交），不发。
 */
export function decideVadAction(evt: VadEvent | null, state: VoiceState): VadAction | null {
  if (evt === 'speech_sustained') {
    return state === 'speaking' || state === 'thinking' ? 'barge_in' : null;
  }
  if (evt === 'silence_timeout') {
    return state === 'listening' ? 'commit' : null;
  }
  return null;
}
