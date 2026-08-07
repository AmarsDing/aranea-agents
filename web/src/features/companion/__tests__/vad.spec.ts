import { describe, expect, it } from 'vitest';
import { createVad, type VadEvent } from '../voice/vad';

const FRAME_MS = 20;
const RATE = 16000;
const FRAME_LEN = (RATE / 1000) * FRAME_MS; // 320

function silenceFrame(): Float32Array {
  return new Float32Array(FRAME_LEN);
}

/** 200Hz 正弦 ≈ 浊音：低过零率 + 稳定能量。 */
function speechFrame(amplitude = 0.2): Float32Array {
  const out = new Float32Array(FRAME_LEN);
  for (let i = 0; i < FRAME_LEN; i++) {
    out[i] = amplitude * Math.sin((2 * Math.PI * 200 * i) / RATE);
  }
  return out;
}

/** 逐样本正负交替 = 满过零率噪声（非语音特征）。 */
function highZcrNoiseFrame(): Float32Array {
  const out = new Float32Array(FRAME_LEN);
  for (let i = 0; i < FRAME_LEN; i++) {
    out[i] = i % 2 === 0 ? 0.5 : -0.5;
  }
  return out;
}

function feed(vad: ReturnType<typeof createVad>, frame: Float32Array, count: number): VadEvent[] {
  const events: VadEvent[] = [];
  for (let i = 0; i < count; i++) {
    const ev = vad.process(frame);
    if (ev) events.push(ev);
  }
  return events;
}

describe('createVad', () => {
  it('stays silent on pure silence', () => {
    const vad = createVad({ sampleRate: RATE, frameMs: FRAME_MS });
    expect(feed(vad, silenceFrame(), 100)).toEqual([]);
    expect(vad.speaking).toBe(false);
  });

  it('emits speech_sustained once when voice persists >= onset threshold (200ms)', () => {
    const vad = createVad({ sampleRate: RATE, frameMs: FRAME_MS });
    // 9 frames = 180ms < 200ms → nothing
    expect(feed(vad, speechFrame(), 9)).toEqual([]);
    expect(vad.speaking).toBe(true);
    // 10th frame crosses 200ms
    expect(feed(vad, speechFrame(), 1)).toEqual(['speech_sustained']);
    // sustained must not re-fire while speech continues
    expect(feed(vad, speechFrame(), 20)).toEqual([]);
  });

  it('does not fire onset for a short burst below threshold', () => {
    const vad = createVad({ sampleRate: RATE, frameMs: FRAME_MS });
    expect(feed(vad, speechFrame(), 5)).toEqual([]); // 100ms
    expect(feed(vad, silenceFrame(), 3)).toEqual([]);
    // onset accumulation resets after silence gap
    expect(feed(vad, speechFrame(), 5)).toEqual([]);
  });

  it('emits silence_timeout after speech followed by >= hangover silence (700ms)', () => {
    const vad = createVad({ sampleRate: RATE, frameMs: FRAME_MS });
    feed(vad, speechFrame(), 12); // onset fired
    // 34 frames silence = 680ms < 700ms → nothing
    expect(feed(vad, silenceFrame(), 34)).toEqual([]);
    // 35th frame crosses 700ms
    expect(feed(vad, silenceFrame(), 1)).toEqual(['silence_timeout']);
    // no double fire
    expect(feed(vad, silenceFrame(), 10)).toEqual([]);
  });

  it('does not emit silence_timeout when onset never fired', () => {
    const vad = createVad({ sampleRate: RATE, frameMs: FRAME_MS });
    feed(vad, speechFrame(), 3); // 60ms burst, below onset
    expect(feed(vad, silenceFrame(), 50)).toEqual([]);
  });

  it('rejects high-ZCR noise even at high energy', () => {
    const vad = createVad({ sampleRate: RATE, frameMs: FRAME_MS });
    expect(feed(vad, highZcrNoiseFrame(), 30)).toEqual([]);
    expect(vad.speaking).toBe(false);
  });

  it('re-arms onset after a completed utterance (silence_timeout)', () => {
    const vad = createVad({ sampleRate: RATE, frameMs: FRAME_MS });
    feed(vad, speechFrame(), 12);
    feed(vad, silenceFrame(), 36); // silence_timeout fired
    // new utterance → onset must fire again
    const events = feed(vad, speechFrame(), 10);
    expect(events).toContain('speech_sustained');
  });

  it('reset() clears all state', () => {
    const vad = createVad({ sampleRate: RATE, frameMs: FRAME_MS });
    feed(vad, speechFrame(), 9);
    vad.reset();
    expect(vad.speaking).toBe(false);
    // after reset, onset needs full 200ms again
    expect(feed(vad, speechFrame(), 9)).toEqual([]);
  });

  it('honours custom thresholds', () => {
    const vad = createVad({
      sampleRate: RATE,
      frameMs: FRAME_MS,
      speechOnsetMs: 100,
      silenceHangoverMs: 200,
    });
    expect(feed(vad, speechFrame(), 5)).toEqual(['speech_sustained']); // 100ms
    expect(feed(vad, silenceFrame(), 10)).toEqual(['silence_timeout']); // 200ms
  });
});
