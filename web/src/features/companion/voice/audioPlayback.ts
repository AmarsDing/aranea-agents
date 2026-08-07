/**
 * TTS 下行音频播放调度（设计 §7.2 audioPlayback）。
 *
 * 下行帧为 PCM f32le 16k（tts.start 声明）。chunk 即产即推，
 * 本模块按序 gapless 调度（nextStartTime 接续，NFR4 句间 <150ms）；
 * barge_in 时 50ms 淡出并清空队列（设计 §7.2 / §2.2 验收）。
 *
 * AudioContext 经接口注入，单测以假时钟上下文验证调度时序。
 */

export type PlaybackBufferLike = {
  duration: number;
  getChannelData(channel: number): Float32Array;
};

export type PlaybackSourceLike = {
  buffer: PlaybackBufferLike | null;
  connect(dst: unknown): void;
  start(when: number): void;
  stop(when?: number): void;
  onended: (() => void) | null;
};

export type PlaybackGainLike = {
  gain: {
    value: number;
    setValueAtTime(value: number, time: number): void;
    linearRampToValueAtTime(value: number, time: number): void;
  };
  connect(dst: unknown): void;
  disconnect(): void;
};

export type PlaybackAudioContextLike = {
  currentTime: number;
  destination: unknown;
  createBuffer(channels: number, length: number, sampleRate: number): PlaybackBufferLike;
  createBufferSource(): PlaybackSourceLike;
  createGain(): PlaybackGainLike;
  close(): Promise<void>;
  state?: string;
  resume?: () => Promise<void>;
};

export type PcmPlayerOptions = {
  /** 下行采样率（默认 16k，与 tts.start 声明一致）。 */
  sampleRate?: number;
  /** 测试注入用；缺省为真实 AudioContext。 */
  contextFactory?: () => PlaybackAudioContextLike;
  /** 新时间线的起始提前量（秒，默认 20ms，防 under-run）。 */
  scheduleLeadSec?: number;
  /** 队列自然播干（非 stop）时回调。 */
  onDrained?: () => void;
};

export type PcmPlayer = {
  /** 追加一块 PCM f32 音频，gapless 接续调度。 */
  enqueue(chunk: Float32Array): void;
  /** barge-in：淡出并截断全部已调度 source，时间线归零。 */
  stop(fadeMs?: number): void;
  /** 关闭上下文；之后的 enqueue 静默忽略。 */
  dispose(): void;
  readonly playing: boolean;
};

export function createPcmPlayer(opts: PcmPlayerOptions = {}): PcmPlayer {
  const sampleRate = opts.sampleRate ?? 16000;
  const leadSec = opts.scheduleLeadSec ?? 0.02;
  const contextFactory = opts.contextFactory ?? (() => new AudioContext() as unknown as PlaybackAudioContextLike);

  let ctx: PlaybackAudioContextLike | null = null;
  let master: PlaybackGainLike | null = null;
  let nextStartTime = 0;
  let generation = 0;
  let disposed = false;
  let gainMuted = false;
  const active = new Set<PlaybackSourceLike>();

  function ensureContext(): PlaybackAudioContextLike {
    if (!ctx) {
      ctx = contextFactory();
      master = ctx.createGain();
      master.connect(ctx.destination);
    }
    // 浏览器自动播放策略：上下文可能处于 suspended，尝试恢复（失败静默，
    // 下一帧 enqueue 会重试）。
    if (ctx.state === 'suspended') {
      void ctx.resume?.().catch(() => undefined);
    }
    return ctx;
  }

  return {
    enqueue(chunk: Float32Array): void {
      if (disposed || chunk.length === 0) return;
      const c = ensureContext();
      if (gainMuted && master) {
        // stop() 后复播：主增益恢复
        master.gain.setValueAtTime(1, c.currentTime);
        gainMuted = false;
      }
      const buffer = c.createBuffer(1, chunk.length, sampleRate);
      buffer.getChannelData(0).set(chunk);
      const source = c.createBufferSource();
      source.buffer = buffer;
      source.connect(master);
      const now = c.currentTime;
      // gapless：接续上一块结束时刻；队列已播干则以当前时刻 + 提前量重起时间线
      const startAt = Math.max(nextStartTime, now + leadSec);
      const gen = generation;
      source.onended = () => {
        active.delete(source);
        if (gen === generation && active.size === 0 && !disposed) {
          opts.onDrained?.();
        }
      };
      source.start(startAt);
      nextStartTime = startAt + buffer.duration;
      active.add(source);
    },

    stop(fadeMs = 50): void {
      if (!ctx) return;
      generation++;
      const now = ctx.currentTime;
      const stopAt = now + fadeMs / 1000;
      if (master) {
        master.gain.setValueAtTime(master.gain.value, now);
        master.gain.linearRampToValueAtTime(0, stopAt);
        gainMuted = true;
      }
      for (const source of active) {
        try {
          source.stop(stopAt);
        } catch {
          // source 可能从未 start（理论不可达，防御性忽略）
        }
      }
      active.clear();
      nextStartTime = 0;
    },

    dispose(): void {
      if (disposed) return;
      disposed = true;
      generation++;
      for (const source of active) {
        try {
          source.stop(0);
        } catch {
          // 同上
        }
      }
      active.clear();
      const c = ctx;
      ctx = null;
      master = null;
      if (c) void c.close().catch(() => undefined);
    },

    get playing() {
      return active.size > 0;
    },
  };
}
