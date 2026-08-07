import { describe, expect, it } from 'vitest';
import { VOICE_TARGET_SAMPLE_RATE, downsampleTo16k, floatToS16le, encodeVoiceFrame } from '../voice/pcm';

describe('downsampleTo16k', () => {
  it('returns a copy unchanged when input is already 16kHz', () => {
    const input = new Float32Array([0, 0.5, -0.5, 1, -1]);
    const out = downsampleTo16k(input, 16000);
    expect(out.length).toBe(input.length);
    expect(Array.from(out)).toEqual(Array.from(input));
    // must be a copy, not the same reference
    expect(out).not.toBe(input);
  });

  it('downsamples 48kHz to 16kHz at exactly 1/3 length', () => {
    // 480 samples @48k = 10ms → 160 samples @16k
    const input = new Float32Array(480);
    for (let i = 0; i < input.length; i++) input[i] = Math.sin((i / 480) * Math.PI * 2);
    const out = downsampleTo16k(input, 48000);
    expect(out.length).toBe(160);
    // first sample identical
    expect(out[0]).toBeCloseTo(input[0], 6);
    // midpoint sample should interpolate between source samples
    const srcPos = 80 * 3; // out[80] maps to input position 240
    expect(out[80]).toBeCloseTo(input[srcPos], 6);
  });

  it('handles non-integer ratios (44.1kHz → 16kHz)', () => {
    const input = new Float32Array(441);
    for (let i = 0; i < input.length; i++) input[i] = i / 441; // ramp 0..1
    const out = downsampleTo16k(input, 44100);
    // 441 samples @44.1k = 10ms → 160 samples @16k
    expect(out.length).toBe(160);
    // ramp is monotonic → output must be monotonic non-decreasing
    for (let i = 1; i < out.length; i++) {
      expect(out[i]).toBeGreaterThanOrEqual(out[i - 1]);
    }
    expect(out[0]).toBeCloseTo(0, 5);
    expect(out[out.length - 1]).toBeLessThanOrEqual(1);
  });

  it('returns empty output for empty input', () => {
    expect(downsampleTo16k(new Float32Array(0), 48000).length).toBe(0);
  });

  it('exports 16kHz as the voice target rate', () => {
    expect(VOICE_TARGET_SAMPLE_RATE).toBe(16000);
  });
});

describe('floatToS16le', () => {
  it('converts zero and full-scale values', () => {
    const out = floatToS16le(new Float32Array([0, 1, -1]));
    expect(out[0]).toBe(0);
    expect(out[1]).toBe(32767);
    expect(out[2]).toBe(-32768);
  });

  it('clips out-of-range values', () => {
    const out = floatToS16le(new Float32Array([1.5, -1.5]));
    expect(out[0]).toBe(32767);
    expect(out[1]).toBe(-32768);
  });

  it('scales mid-range values linearly', () => {
    const out = floatToS16le(new Float32Array([0.5, -0.5]));
    expect(out[0]).toBe(Math.round(0.5 * 32767));
    expect(out[1]).toBe(Math.round(-0.5 * 32768));
  });

  it('produces little-endian bytes regardless of platform', () => {
    const out = floatToS16le(new Float32Array([1]));
    const bytes = new Uint8Array(out.buffer, out.byteOffset, out.byteLength);
    // 32767 = 0x7FFF → LE bytes FF 7F
    expect(bytes[0]).toBe(0xff);
    expect(bytes[1]).toBe(0x7f);
  });
});

describe('encodeVoiceFrame', () => {
  it('resamples then encodes to s16le ArrayBuffer', () => {
    // 20ms @48k = 960 samples → 320 samples @16k = 640 bytes
    const input = new Float32Array(960);
    const buf = encodeVoiceFrame(input, 48000);
    expect(buf.byteLength).toBe(640);
  });

  it('passes through 16kHz input without resampling', () => {
    const input = new Float32Array(320).fill(0.25);
    const buf = encodeVoiceFrame(input, 16000);
    expect(buf.byteLength).toBe(640);
    const view = new DataView(buf);
    expect(view.getInt16(0, true)).toBe(Math.round(0.25 * 32767));
  });
});
