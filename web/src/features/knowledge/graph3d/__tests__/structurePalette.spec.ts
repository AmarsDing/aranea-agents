/**
 * structurePalette.spec：V13 沉浸深空 · 结构层级着色与边统一着色（设计 D2/D3）。
 *
 * 契约：
 * - 结构色：ultranode=品红 > supernode=金 > regular=青 > 末梢（degree≤1）=暗青
 * - 双模式：'structure'（默认）/ 'group'（doc_type 分组调色板）；非法/空存储回退 'structure'
 * - 边：'unified'（默认，全部暗青灰隐入背景）/ 'typed'（explicit/entity/semantic 三色）
 */
import { describe, expect, it } from 'vitest';
import {
  buildEdgeColorBuffer,
  buildNodeColorBuffer,
  EDGE_COLOR_MODE_STORAGE_KEY,
  COLOR_MODE_STORAGE_KEY,
  resolveColorMode,
  resolveEdgeColorMode,
  STRUCTURE_LEAF,
  STRUCTURE_REGULAR,
  STRUCTURE_SUPER,
  STRUCTURE_ULTRA,
  UNIFIED_EDGE_COLOR,
} from '../structurePalette';
import { hexToRgbFloat } from '../palette';
import { TIER_REGULAR, TIER_SUPERNODE, TIER_ULTRANODE } from '../tiering';

/** Float32Array 存储有精度损失，统一按 5 位小数比较。 */
function rgbAt(buf: Float32Array, i: number): [number, number, number] {
  return [+buf[i * 3].toFixed(5), +buf[i * 3 + 1].toFixed(5), +buf[i * 3 + 2].toFixed(5)];
}

function rgbOf(hex: string): [number, number, number] {
  const [r, g, b] = hexToRgbFloat(hex);
  return [+r.toFixed(5), +g.toFixed(5), +b.toFixed(5)];
}

describe('structurePalette · 存储键解析', () => {
  it('COLOR_MODE_STORAGE_KEY / EDGE_COLOR_MODE_STORAGE_KEY 稳定', () => {
    expect(COLOR_MODE_STORAGE_KEY).toBe('kg3d-color-mode');
    expect(EDGE_COLOR_MODE_STORAGE_KEY).toBe('kg3d-edge-color-mode');
  });

  it('resolveColorMode：空/非法回退 structure，仅认 group', () => {
    expect(resolveColorMode(null)).toBe('structure');
    expect(resolveColorMode(undefined)).toBe('structure');
    expect(resolveColorMode('')).toBe('structure');
    expect(resolveColorMode('typed')).toBe('structure');
    expect(resolveColorMode('group')).toBe('group');
    expect(resolveColorMode('structure')).toBe('structure');
  });

  it('resolveEdgeColorMode：空/非法回退 unified，仅认 typed', () => {
    expect(resolveEdgeColorMode(null)).toBe('unified');
    expect(resolveEdgeColorMode('')).toBe('unified');
    expect(resolveEdgeColorMode('group')).toBe('unified');
    expect(resolveEdgeColorMode('typed')).toBe('typed');
    expect(resolveEdgeColorMode('unified')).toBe('unified');
  });
});

describe('structurePalette · buildNodeColorBuffer', () => {
  const groups = ['entries', 'journal'];
  // 4 节点：0=ultra(组0) 1=super(组1) 2=regular·度3(组0) 3=regular·度1 末梢(组1)
  const groupId = Uint16Array.from([0, 1, 0, 1]);
  const tiers = Uint8Array.from([TIER_ULTRANODE, TIER_SUPERNODE, TIER_REGULAR, TIER_REGULAR]);
  const degree = Uint16Array.from([40, 16, 3, 1]);
  const groupPalette = ['#ff0000', '#00ff00'];

  it('structure 模式：ultra/super/regular/末梢 四档结构色', () => {
    const buf = buildNodeColorBuffer(4, groupId, groups, tiers, degree, 'structure', groupPalette);
    expect(rgbAt(buf, 0)).toEqual(rgbOf(STRUCTURE_ULTRA));
    expect(rgbAt(buf, 1)).toEqual(rgbOf(STRUCTURE_SUPER));
    expect(rgbAt(buf, 2)).toEqual(rgbOf(STRUCTURE_REGULAR));
    expect(rgbAt(buf, 3)).toEqual(rgbOf(STRUCTURE_LEAF));
  });

  it('structure 模式：末梢判定仅作用 regular（super/ultra 低度不降档）', () => {
    const t2 = Uint8Array.from([TIER_SUPERNODE, TIER_ULTRANODE]);
    const d2 = Uint16Array.from([1, 0]);
    const buf = buildNodeColorBuffer(2, Uint16Array.from([0, 0]), groups, t2, d2, 'structure', groupPalette);
    expect(rgbAt(buf, 0)).toEqual(rgbOf(STRUCTURE_SUPER));
    expect(rgbAt(buf, 1)).toEqual(rgbOf(STRUCTURE_ULTRA));
  });

  it('group 模式：按 doc_type 分组调色板（V12.8 现状语义）', () => {
    const buf = buildNodeColorBuffer(4, groupId, groups, tiers, degree, 'group', groupPalette);
    expect(rgbAt(buf, 0)).toEqual(rgbOf('#ff0000'));
    expect(rgbAt(buf, 1)).toEqual(rgbOf('#00ff00'));
    expect(rgbAt(buf, 2)).toEqual(rgbOf('#ff0000'));
    expect(rgbAt(buf, 3)).toEqual(rgbOf('#00ff00'));
  });

  it('group 模式：组索引越界回退中性灰', () => {
    const buf = buildNodeColorBuffer(1, Uint16Array.from([9]), groups, Uint8Array.from([0]), Uint16Array.from([1]), 'group', groupPalette);
    expect(rgbAt(buf, 0)).toEqual([0.5, 0.5, 0.5]);
  });
});

describe('structurePalette · buildEdgeColorBuffer', () => {
  const types = ['explicit', 'entity', 'semantic'];

  it('unified 模式：全部边统一暗青灰（隐入背景）', () => {
    const buf = buildEdgeColorBuffer(types, 'unified');
    const unified = rgbOf(UNIFIED_EDGE_COLOR);
    expect(buf).toHaveLength(9);
    expect(rgbAt(buf, 0)).toEqual(unified);
    expect(rgbAt(buf, 1)).toEqual(unified);
    expect(rgbAt(buf, 2)).toEqual(unified);
  });

  it('typed 模式：按边类型三色（显式青/实体紫/语义蓝），未知类型回退统一色', () => {
    const buf = buildEdgeColorBuffer([...types, 'unknown'], 'typed');
    expect(rgbAt(buf, 0)).not.toEqual(rgbAt(buf, 1)); // explicit ≠ entity
    expect(rgbAt(buf, 3)).toEqual(rgbOf(UNIFIED_EDGE_COLOR));
  });
});
