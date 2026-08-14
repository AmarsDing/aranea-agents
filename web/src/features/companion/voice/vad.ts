/**
 * 前端 VAD（设计 §7.2）：能量 + 过零率双重判定。
 *
 * 双重职责：
 * ① 播报期人声检测 —— 持续人声 ≥ speechOnsetMs（默认 200ms）发 speech_sustained（武装判停计时）；
 * ② 判停兜底 —— onset 之后静音 ≥ silenceHangoverMs（默认 700ms）发 silence_timeout（触发 voice.commit）。
 *
 * V11-T2（设计 §17.3）：双阈值分级——speech_sustained 仅武装判停，不再触发打断；
 * 持续人声 ≥ bargeInOnsetMs（默认 450ms）才发 speech_barge_in（触发 barge_in），
 * 短促背景人声（咳嗽/旁人插话/电视对白）不再杀播报、不再取消在途 Turn。
 * NFR2 修订：持续人声确认 450ms + 本地停播 ≤300ms。
 *
 * 语句端点判定以火山 ASR 服务端 VAD 为主，本模块仅为兜底（设计 §7.2 vad 职责②）。
 */

import type { VoiceState } from '../types';

export type VadEvent = 'speech_sustained' | 'speech_barge_in' | 'silence_timeout';

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
  /** 语句 onset 人声持续阈值 ms（默认 200；仅武装判停计时，V11 起不触发打断）。 */
  speechOnsetMs?: number;
  /** barge-in 人声持续阈值 ms（默认 450，V11；设计 §17.3，上行 detect_ms 语义）。 */
  bargeInOnsetMs?: number;
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
  const bargeInOnsetMs = opts.bargeInOnsetMs ?? 450;
  const silenceHangoverMs = opts.silenceHangoverMs ?? 700;
  const frameMs = opts.frameMs ?? 20;

  let speechMs = 0;
  let silenceMs = 0;
  let onsetFired = false;
  let bargeInFired = false;
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
        if (onsetFired && !bargeInFired && speechMs >= bargeInOnsetMs) {
          bargeInFired = true;
          return 'speech_barge_in';
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
          // 一句话结束：重新武装下一轮 onset 与打断确认
          speechMs = 0;
          silenceMs = 0;
          onsetFired = false;
          bargeInFired = false;
          return 'silence_timeout';
        }
      }
      return null;
    },
    reset(): void {
      speechMs = 0;
      silenceMs = 0;
      onsetFired = false;
      bargeInFired = false;
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
 * V11-T2 契约修订（设计 §17.3）：
 * - speech_barge_in（≥450ms 持续人声）：仅 speaking/thinking 态触发 barge_in
 *   （设计 §2.2/§5：播报或等待回复期间用户持续开口才打断）；listening 态忽略。
 * - speech_sustained（200ms onset）：任何态都不动作——仅武装判停计时；
 *   listening 态的 onset 是正常语句起点，speaking/thinking 态的短促人声多为背景干扰。
 * - silence_timeout：仅 listening 态发 voice.commit 判停兜底（设计 §7.2 职责②）；
 *   其余态说明服务端已端点（或无需提交），不发。
 */
export function decideVadAction(evt: VadEvent | null, state: VoiceState): VadAction | null {
  if (evt === 'speech_barge_in') {
    return state === 'speaking' || state === 'thinking' ? 'barge_in' : null;
  }
  if (evt === 'silence_timeout') {
    return state === 'listening' ? 'commit' : null;
  }
  return null;
}
