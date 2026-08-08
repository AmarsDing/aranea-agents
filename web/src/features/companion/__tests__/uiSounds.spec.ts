import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  SOUND_SPECS,
  UI_SOUND_KINDS,
  UiSoundEngine,
  soundEnabledFromStorage,
  type AudioContextLike,
  type GainNodeLike,
  type OscillatorLike,
} from '../audio/uiSounds';

/** Mock 振荡器：记录频率/启停调用。 */
type MockOsc = OscillatorLike & {
  started: number[];
  stopped: number[];
  freqCalls: Array<{ method: string; value: number; at: number }>;
};

type MockGain = GainNodeLike & {
  gainCalls: Array<{ method: string; value: number; at: number }>;
};

function createMockContext(): AudioContextLike & { oscillators: MockOsc[]; gains: MockGain[] } {
  const oscillators: MockOsc[] = [];
  const gains: MockGain[] = [];
  return {
    currentTime: 10,
    destination: {},
    oscillators,
    gains,
    createOscillator() {
      const osc: MockOsc = {
        type: 'sine',
        started: [],
        stopped: [],
        freqCalls: [],
        frequency: {
          setValueAtTime: (v: number, at: number) => osc.freqCalls.push({ method: 'set', value: v, at }),
          exponentialRampToValueAtTime: (v: number, at: number) =>
            osc.freqCalls.push({ method: 'expRamp', value: v, at }),
        },
        connect: () => undefined,
        start: (at: number) => osc.started.push(at),
        stop: (at: number) => osc.stopped.push(at),
      };
      oscillators.push(osc);
      return osc;
    },
    createGain() {
      const gain: MockGain = {
        gainCalls: [],
        gain: {
          value: 0,
          setValueAtTime: (v: number, at: number) => gain.gainCalls.push({ method: 'set', value: v, at }),
          linearRampToValueAtTime: (v: number, at: number) =>
            gain.gainCalls.push({ method: 'linRamp', value: v, at }),
        },
        connect: () => undefined,
      };
      gains.push(gain);
      return gain;
    },
  };
}

describe('SOUND_SPECS — 合成参数纯数据（设计 §7.4 v2 音效引擎）', () => {
  it('覆盖全部 6 种音效：boot/chirp/ping/ding/buzz/cut', () => {
    expect(UI_SOUND_KINDS).toEqual(['boot', 'chirp', 'ping', 'ding', 'buzz', 'cut']);
    for (const kind of UI_SOUND_KINDS) {
      expect(SOUND_SPECS[kind].length).toBeGreaterThan(0);
    }
  });

  it('每段参数合法：时长>0、频率>0、增益∈(0,1]、延迟≥0', () => {
    for (const kind of UI_SOUND_KINDS) {
      for (const seg of SOUND_SPECS[kind]) {
        expect(seg.duration).toBeGreaterThan(0);
        expect(seg.fromFreq).toBeGreaterThan(0);
        expect(seg.toFreq).toBeGreaterThan(0);
        expect(seg.gain).toBeGreaterThan(0);
        expect(seg.gain).toBeLessThanOrEqual(1);
        expect(seg.delay ?? 0).toBeGreaterThanOrEqual(0);
      }
    }
  });

  it('boot 为上电扫频（主频升高），buzz 为拒绝降频', () => {
    const bootMain = SOUND_SPECS.boot[0];
    expect(bootMain.toFreq).toBeGreaterThan(bootMain.fromFreq);
    const buzzMain = SOUND_SPECS.buzz[0];
    expect(buzzMain.toFreq).toBeLessThan(buzzMain.fromFreq);
  });
});

describe('soundEnabledFromStorage', () => {
  it('默认开（null → true），"0" 关，"1" 开', () => {
    expect(soundEnabledFromStorage(null)).toBe(true);
    expect(soundEnabledFromStorage('0')).toBe(false);
    expect(soundEnabledFromStorage('1')).toBe(true);
  });
});

describe('UiSoundEngine — 播放调度（mock AudioContext）', () => {
  let ctx: ReturnType<typeof createMockContext>;

  beforeEach(() => {
    ctx = createMockContext();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('play 为每段创建 osc+gain 并按包络调度 start/stop', () => {
    const engine = new UiSoundEngine({ context: ctx, enabled: true, volume: 0.5 });
    engine.play('boot');
    expect(ctx.oscillators.length).toBe(SOUND_SPECS.boot.length);
    // 每段：频率 set → expRamp；增益 set(0) → linRamp(peak) → linRamp(0)
    const osc = ctx.oscillators[0];
    const seg = SOUND_SPECS.boot[0];
    expect(osc.freqCalls[0]).toEqual({ method: 'set', value: seg.fromFreq, at: 10 });
    expect(osc.freqCalls[1].method).toBe('expRamp');
    expect(osc.freqCalls[1].value).toBe(seg.toFreq);
    expect(osc.started).toEqual([10 + (seg.delay ?? 0)]);
    expect(osc.stopped[0]).toBeCloseTo(10 + (seg.delay ?? 0) + seg.duration + 0.05, 5);
  });

  it('禁用时 play 不创建任何节点', () => {
    const engine = new UiSoundEngine({ context: ctx, enabled: false });
    engine.play('ding');
    expect(ctx.oscillators.length).toBe(0);
  });

  it('主音量作用于每段峰值增益（volume × seg.gain）', () => {
    const engine = new UiSoundEngine({ context: ctx, enabled: true, volume: 0.25 });
    engine.play('chirp');
    const seg = SOUND_SPECS.chirp[0];
    const gain = ctx.gains[0];
    const peak = gain.gainCalls.find((c) => c.method === 'linRamp' && c.value > 0.001);
    expect(peak?.value).toBeCloseTo(0.25 * seg.gain, 5);
  });

  it('思考 ping 循环：startThinkingLoop 每 2s 一次，stop 后不再触发', () => {
    const engine = new UiSoundEngine({ context: ctx, enabled: true });
    engine.startThinkingLoop();
    const perPing = SOUND_SPECS.ping.length;
    vi.advanceTimersByTime(5000);
    const afterLoop = ctx.oscillators.length;
    expect(afterLoop).toBe(perPing * 3); // 0s/2s/4s 共 3 次
    engine.stopThinkingLoop();
    vi.advanceTimersByTime(4000);
    expect(ctx.oscillators.length).toBe(afterLoop);
  });

  it('setEnabled(false) 持久化 "0" 并立即停止 ping 循环', () => {
    const storage = new Map<string, string>();
    const engine = new UiSoundEngine({
      context: ctx,
      enabled: true,
      persist: (v) => storage.set('k', v),
    });
    engine.startThinkingLoop();
    engine.setEnabled(false);
    expect(storage.get('k')).toBe('0');
    const before = ctx.oscillators.length;
    vi.advanceTimersByTime(4000);
    expect(ctx.oscillators.length).toBe(before);
  });
});
