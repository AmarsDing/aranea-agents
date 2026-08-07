/**
 * palette.spec：G5-A 分组调色板契约（沿用 G4 graphUi 调色板语义，设计 §V12.8-1 palette.ts）。
 */
import { describe, expect, it } from 'vitest';
import { buildGroupPalette, groupColorHex, hexToRgbFloat } from '../palette';

describe('groupColorHex', () => {
  it('同 doc_type 同色（稳定哈希），空类型 fallback 灰', () => {
    expect(groupColorHex('note')).toBe(groupColorHex('note'));
    expect(groupColorHex('NOTE')).toBe(groupColorHex('note')); // 大小写不敏感
    expect(groupColorHex('')).toMatch(/^#[0-9a-f]{6}$/i);
  });

  it('返回合法 hex 色', () => {
    for (const t of ['note', 'report', 'manual', 'faq', 'unknown-xyz']) {
      expect(groupColorHex(t)).toMatch(/^#[0-9a-f]{6}$/i);
    }
  });
});

describe('buildGroupPalette', () => {
  it('groups → 每组一色，与 groupColorHex 一致', () => {
    const p = buildGroupPalette(['note', 'report', '']);
    expect(p).toHaveLength(3);
    expect(p[0]).toBe(groupColorHex('note'));
    expect(p[1]).toBe(groupColorHex('report'));
    expect(p[2]).toBe(groupColorHex(''));
  });
});

describe('hexToRgbFloat', () => {
  it('#rrggbb → [0..1] 浮点三通道', () => {
    expect(hexToRgbFloat('#000000')).toEqual([0, 0, 0]);
    expect(hexToRgbFloat('#ffffff')).toEqual([1, 1, 1]);
    const [r, g, b] = hexToRgbFloat('#4c8dff');
    expect(r).toBeCloseTo(0x4c / 255, 5);
    expect(g).toBeCloseTo(0x8d / 255, 5);
    expect(b).toBeCloseTo(1, 5);
  });
});
