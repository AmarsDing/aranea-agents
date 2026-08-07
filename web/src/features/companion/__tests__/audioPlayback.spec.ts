import { describe, expect, it } from 'vitest';
import {
  createPcmPlayer,
  type PlaybackAudioContextLike,
  type PlaybackBufferLike,
  type PlaybackGainLike,
  type PlaybackSourceLike,
} from '../voice/audioPlayback';

/** 手动推进时钟的假 AudioContext。 */
class FakeAudioContext implements PlaybackAudioContextLike {
  currentTime = 0;
  destination = {};
  closed = false;
  sources: FakeSource[] = [];
  gains: FakeGain[] = [];

  createBuffer(_channels: number, length: number, sampleRate: number): PlaybackBufferLike {
    const data = new Float32Array(length);
    return {
      duration: length / sampleRate,
      getChannelData: () => data,
    };
  }
  createBufferSource(): PlaybackSourceLike {
    const src = new FakeSource();
    this.sources.push(src);
    return src;
  }
  createGain(): PlaybackGainLike {
    const g = new FakeGain();
    this.gains.push(g);
    return g;
  }
  async close(): Promise<void> {
    this.closed = true;
  }
}

class FakeSource implements PlaybackSourceLike {
  buffer: PlaybackBufferLike | null = null;
  startedAt: number | null = null;
  stoppedAt: number | null = null;
  connectedTo: unknown = null;
  onended: (() => void) | null = null;
  connect(dst: unknown): void {
    this.connectedTo = dst;
  }
  start(when: number): void {
    this.startedAt = when;
  }
  stop(when?: number): void {
    this.stoppedAt = when ?? 0;
  }
  /** 测试辅助：模拟自然播完。 */
  finish(): void {
    this.onended?.();
  }
}

class FakeGain implements PlaybackGainLike {
  gain = {
    value: 1,
    setValueAtTime: (v: number, _t: number) => {
      this.gain.value = v;
    },
    linearRampToValueAtTime: (v: number, _t: number) => {
      this.gain.value = v;
    },
  };
  connectedTo: unknown = null;
  connect(dst: unknown): void {
    this.connectedTo = dst;
  }
  disconnect(): void {
    this.connectedTo = null;
  }
}

function makePlayer(ctx: FakeAudioContext, onDrained?: () => void) {
  return createPcmPlayer({ contextFactory: () => ctx, onDrained });
}

describe('createPcmPlayer', () => {
  it('schedules the first chunk with a small lead time', () => {
    const ctx = new FakeAudioContext();
    const player = makePlayer(ctx);
    player.enqueue(new Float32Array(1600)); // 0.1s @16k
    expect(ctx.sources.length).toBe(1);
    expect(ctx.sources[0].startedAt).toBeGreaterThan(0);
    expect(ctx.sources[0].startedAt).toBeLessThan(0.1);
    expect(player.playing).toBe(true);
  });

  it('chains chunks gaplessly (second starts exactly when first ends)', () => {
    const ctx = new FakeAudioContext();
    const player = makePlayer(ctx);
    player.enqueue(new Float32Array(1600)); // 0.1s
    player.enqueue(new Float32Array(3200)); // 0.2s
    const [a, b] = ctx.sources;
    const gap = b.startedAt! - (a.startedAt! + 0.1);
    expect(Math.abs(gap)).toBeLessThan(1e-9); // 句间无间隙（NFR4 <150ms 由调度保证）
  });

  it('re-bases the timeline when enqueueing after the queue ran dry', () => {
    const ctx = new FakeAudioContext();
    const player = makePlayer(ctx);
    player.enqueue(new Float32Array(1600));
    ctx.currentTime = 5; // 远超首块结束时间
    player.enqueue(new Float32Array(1600));
    const second = ctx.sources[1];
    expect(second.startedAt).toBeGreaterThanOrEqual(5);
    expect(second.startedAt).toBeLessThan(5.1);
  });

  it('stop(fadeMs) ramps master gain to 0 and schedules source stops', () => {
    const ctx = new FakeAudioContext();
    const player = makePlayer(ctx);
    player.enqueue(new Float32Array(1600));
    player.enqueue(new Float32Array(1600));
    ctx.currentTime = 0.05;
    player.stop(50);
    const master = ctx.gains[0];
    expect(master.gain.value).toBe(0); // ramp 终点
    for (const src of ctx.sources) {
      expect(src.stoppedAt).not.toBeNull();
      expect(src.stoppedAt!).toBeGreaterThanOrEqual(0.1); // now + 50ms
    }
    expect(player.playing).toBe(false);
  });

  it('starts a fresh timeline after stop (barge-in 后可立即再播)', () => {
    const ctx = new FakeAudioContext();
    const player = makePlayer(ctx);
    player.enqueue(new Float32Array(16000)); // 1s 长块
    ctx.currentTime = 0.2;
    player.stop(50);
    player.enqueue(new Float32Array(1600));
    const fresh = ctx.sources[1];
    expect(fresh.startedAt!).toBeGreaterThanOrEqual(0.2);
    expect(fresh.startedAt!).toBeLessThan(0.3);
  });

  it('fires onDrained when the last source ends and nothing is queued', () => {
    const ctx = new FakeAudioContext();
    let drained = 0;
    const player = makePlayer(ctx, () => drained++);
    player.enqueue(new Float32Array(1600));
    player.enqueue(new Float32Array(1600));
    ctx.sources[0].finish();
    expect(drained).toBe(0);
    ctx.sources[1].finish();
    expect(drained).toBe(1);
    expect(player.playing).toBe(false);
  });

  it('does not fire onDrained after stop() (interrupted 不是自然播完)', () => {
    const ctx = new FakeAudioContext();
    let drained = 0;
    const player = makePlayer(ctx, () => drained++);
    player.enqueue(new Float32Array(1600));
    player.stop(50);
    // 被打断的 source 后续触发 onended 也不应算 drained
    ctx.sources[0].finish();
    expect(drained).toBe(0);
  });

  it('dispose() closes the context and rejects further enqueue', () => {
    const ctx = new FakeAudioContext();
    const player = makePlayer(ctx);
    player.enqueue(new Float32Array(1600));
    player.dispose();
    expect(ctx.closed).toBe(true);
    player.enqueue(new Float32Array(1600)); // 静默忽略
    expect(ctx.sources.length).toBe(1);
  });
});
