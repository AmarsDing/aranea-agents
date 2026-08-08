/**
 * 科幻音效引擎（M74 设计 §7.4 v2，V5-T5）。
 *
 * Web Audio 程序合成，零音频资产：上电扫频 / 唤醒 chirp / 思考声纳 ping /
 * 确认 ding / 拒绝 buzz / 打断切音。合成参数为纯数据（SOUND_SPECS）可单测；
 * 播放层注入 AudioContextLike 接口便于 mock。开关持久化 localStorage。
 */

export const UI_SOUND_KINDS = ['boot', 'chirp', 'ping', 'ding', 'buzz', 'cut'] as const;
export type UISoundKind = (typeof UI_SOUND_KINDS)[number];

/** 单段合成音参数（纯数据）。 */
export type ToneSegment = {
  /** 波形。 */
  type: OscillatorType;
  /** 起始频率 Hz。 */
  fromFreq: number;
  /** 结束频率 Hz（指数扫频）。 */
  toFreq: number;
  /** 时长秒。 */
  duration: number;
  /** 峰值增益 [0,1]（再乘主音量）。 */
  gain: number;
  /** 起音秒（默认 0.01）。 */
  attack?: number;
  /** 相对音效起点延迟秒。 */
  delay?: number;
};

/** 六种贾维斯式音效合成配方。 */
export const SOUND_SPECS: Record<UISoundKind, ToneSegment[]> = {
  /** 上电扫频：主频 180→880Hz + 高八度谐波延迟进入。 */
  boot: [
    { type: 'sine', fromFreq: 180, toFreq: 880, duration: 0.5, gain: 0.5, attack: 0.05 },
    { type: 'triangle', fromFreq: 360, toFreq: 1760, duration: 0.35, gain: 0.2, attack: 0.03, delay: 0.12 },
  ],
  /** 唤醒 chirp：短促上扫。 */
  chirp: [{ type: 'sine', fromFreq: 660, toFreq: 990, duration: 0.12, gain: 0.4, attack: 0.01 }],
  /** 思考声纳 ping：低沉短音（循环播放）。 */
  ping: [{ type: 'sine', fromFreq: 520, toFreq: 500, duration: 0.25, gain: 0.22, attack: 0.005 }],
  /** 确认 ding：双音叠加大三度感。 */
  ding: [
    { type: 'sine', fromFreq: 880, toFreq: 880, duration: 0.18, gain: 0.35, attack: 0.005 },
    { type: 'sine', fromFreq: 1320, toFreq: 1318, duration: 0.22, gain: 0.2, attack: 0.005, delay: 0.05 },
  ],
  /** 拒绝 buzz：方波降频。 */
  buzz: [{ type: 'square', fromFreq: 140, toFreq: 110, duration: 0.25, gain: 0.18, attack: 0.005 }],
  /** 打断切音：锯齿快速下坠。 */
  cut: [{ type: 'sawtooth', fromFreq: 300, toFreq: 60, duration: 0.15, gain: 0.25, attack: 0.003 }],
};

/** localStorage 开关键。 */
export const UI_SOUNDS_STORAGE_KEY = 'aranea.companion.uiSounds';

/** 解析持久化开关：默认开。 */
export function soundEnabledFromStorage(value: string | null): boolean {
  return value !== '0';
}

/** 播放所需的最小 AudioContext 子集（便于测试 mock）。 */
export type AudioContextLike = {
  currentTime: number;
  destination: unknown;
  createOscillator(): OscillatorLike;
  createGain(): GainNodeLike;
};

export type OscillatorLike = {
  type: OscillatorType;
  frequency: {
    setValueAtTime(value: number, at: number): unknown;
    exponentialRampToValueAtTime(value: number, at: number): unknown;
  };
  connect(node: unknown): unknown;
  start(at: number): void;
  stop(at: number): void;
};

export type GainNodeLike = {
  gain: {
    value: number;
    setValueAtTime(value: number, at: number): unknown;
    linearRampToValueAtTime(value: number, at: number): unknown;
  };
  connect(node: unknown): unknown;
};

export type UiSoundEngineOptions = {
  context: AudioContextLike;
  /** 初始开关（默认开）。 */
  enabled?: boolean;
  /** 主音量 [0,1]（默认 0.5，约 -18dB 混音级）。 */
  volume?: number;
  /** 开关持久化（默认 localStorage）。 */
  persist?: (value: '0' | '1') => void;
  /** 思考 ping 循环间隔 ms（默认 2000）。 */
  thinkingIntervalMs?: number;
  /** 播放前钩子（浏览器自动播放策略：挂起的 AudioContext 先 resume）。 */
  resume?: () => void;
};

const DEFAULT_VOLUME = 0.5;
const THINKING_INTERVAL_MS = 2000;
/** stop 相对段尾额外延后，避免指数包络未收尾被硬切。 */
const STOP_TAIL_SECONDS = 0.05;

export class UiSoundEngine {
  private readonly ctx: AudioContextLike;
  private readonly volume: number;
  private readonly persist: (value: '0' | '1') => void;
  private readonly thinkingIntervalMs: number;
  private readonly resume?: () => void;
  private enabled: boolean;
  private thinkingTimer: ReturnType<typeof setInterval> | null = null;

  constructor(options: UiSoundEngineOptions) {
    this.ctx = options.context;
    this.volume = options.volume ?? DEFAULT_VOLUME;
    this.persist = options.persist ?? ((v) => localStorage.setItem(UI_SOUNDS_STORAGE_KEY, v));
    this.thinkingIntervalMs = options.thinkingIntervalMs ?? THINKING_INTERVAL_MS;
    this.resume = options.resume;
    this.enabled = options.enabled ?? true;
  }

  isEnabled(): boolean {
    return this.enabled;
  }

  setEnabled(on: boolean): void {
    this.enabled = on;
    this.persist(on ? '1' : '0');
    if (!on) {
      this.stopThinkingLoop();
    }
  }

  /** 播放一次性音效；禁用时静默跳过。 */
  play(kind: UISoundKind): void {
    if (!this.enabled) return;
    this.resume?.();
    const now = this.ctx.currentTime;
    for (const seg of SOUND_SPECS[kind]) {
      this.scheduleSegment(seg, now);
    }
  }

  /** 思考声纳 ping 循环（幂等：重复调用不叠加）。 */
  startThinkingLoop(): void {
    if (this.thinkingTimer !== null) return;
    this.play('ping');
    this.thinkingTimer = setInterval(() => this.play('ping'), this.thinkingIntervalMs);
  }

  stopThinkingLoop(): void {
    if (this.thinkingTimer !== null) {
      clearInterval(this.thinkingTimer);
      this.thinkingTimer = null;
    }
  }

  dispose(): void {
    this.stopThinkingLoop();
  }

  private scheduleSegment(seg: ToneSegment, now: number): void {
    const t0 = now + (seg.delay ?? 0);
    const attack = seg.attack ?? 0.01;
    const osc = this.ctx.createOscillator();
    const gain = this.ctx.createGain();

    osc.type = seg.type;
    osc.frequency.setValueAtTime(seg.fromFreq, t0);
    osc.frequency.exponentialRampToValueAtTime(seg.toFreq, t0 + seg.duration);

    gain.gain.value = 0;
    gain.gain.setValueAtTime(0, t0);
    gain.gain.linearRampToValueAtTime(this.volume * seg.gain, t0 + attack);
    gain.gain.linearRampToValueAtTime(0.0001, t0 + seg.duration);

    osc.connect(gain);
    gain.connect(this.ctx.destination);
    osc.start(t0);
    osc.stop(t0 + seg.duration + STOP_TAIL_SECONDS);
  }
}
