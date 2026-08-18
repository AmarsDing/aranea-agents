/**
 * structurePalette：V13 沉浸深空 · 结构层级着色与边统一着色（设计 D2/D3）。
 *
 * - 结构色语义（参考视频）：品红(ultranode) > 金(supernode) > 青(regular) > 暗青(末梢 degree≤1)
 * - 节点双模式：'structure'（默认）/ 'group'（doc_type 分组调色板，V12.8 语义）
 * - 边双模式：'unified'（默认，暗青灰隐入背景）/ 'typed'（explicit/entity/semantic 三色）
 * - 纯函数 + localStorage 键解析，渲染层零状态
 */
import { graphLinkColor } from '../graphUi';
import { hexToRgbFloat } from './palette';
import { TIER_SUPERNODE, TIER_ULTRANODE } from './tiering';

export type GraphColorMode = 'structure' | 'group';
export type EdgeColorMode = 'unified' | 'typed';

export const COLOR_MODE_STORAGE_KEY = 'kg3d-color-mode';
export const EDGE_COLOR_MODE_STORAGE_KEY = 'kg3d-edge-color-mode';

/** 结构层级色（V13 视频同款三带配色）。 */
export const STRUCTURE_ULTRA = '#f472b6';
export const STRUCTURE_SUPER = '#f5c542';
export const STRUCTURE_REGULAR = '#35c8d0';
export const STRUCTURE_LEAF = '#16434f';

/** 边统一色：暗青灰（rest 亮度系数下隐入深空底，hover/聚焦提亮通道不受影响）。 */
export const UNIFIED_EDGE_COLOR = '#4f6b7d';

/** localStorage 解析：仅认合法值，其余回退默认。 */
export function resolveColorMode(stored: string | null | undefined): GraphColorMode {
  return stored === 'group' ? 'group' : 'structure';
}

export function resolveEdgeColorMode(stored: string | null | undefined): EdgeColorMode {
  return stored === 'typed' ? 'typed' : 'unified';
}

/** 节点颜色缓冲：structure 按层级/度数，group 按 doc_type 分组调色板。 */
export function buildNodeColorBuffer(
  count: number,
  groupId: Uint16Array,
  groups: string[],
  tiers: Uint8Array,
  degree: Uint16Array,
  mode: GraphColorMode,
  groupPalette: readonly string[],
): Float32Array {
  const out = new Float32Array(count * 3);
  const paletteRgb = groupPalette.map((hex) => hexToRgbFloat(hex));
  for (let i = 0; i < count; i++) {
    let rgb: [number, number, number];
    if (mode === 'group') {
      rgb = paletteRgb[groupId[i]] ?? [0.5, 0.5, 0.5];
    } else if (tiers[i] === TIER_ULTRANODE) {
      rgb = hexToRgbFloat(STRUCTURE_ULTRA);
    } else if (tiers[i] === TIER_SUPERNODE) {
      rgb = hexToRgbFloat(STRUCTURE_SUPER);
    } else {
      // 末梢（degree≤1）暗青降档：满屏孤立点不再与主干争视觉
      rgb = hexToRgbFloat(degree[i] <= 1 ? STRUCTURE_LEAF : STRUCTURE_REGULAR);
    }
    out[i * 3] = rgb[0];
    out[i * 3 + 1] = rgb[1];
    out[i * 3 + 2] = rgb[2];
  }
  return out;
}

/** 边颜色缓冲：unified 全统一色；typed 按边类型（未知类型回退统一色）。 */
export function buildEdgeColorBuffer(edgeTypes: string[], mode: EdgeColorMode): Float32Array {
  const out = new Float32Array(edgeTypes.length * 3);
  const unified = hexToRgbFloat(UNIFIED_EDGE_COLOR);
  for (let e = 0; e < edgeTypes.length; e++) {
    const rgb = mode === 'typed' && edgeTypes[e] !== 'unknown' ? hexToRgbFloat(graphLinkColor(edgeTypes[e])) : unified;
    out[e * 3] = rgb[0];
    out[e * 3 + 1] = rgb[1];
    out[e * 3 + 2] = rgb[2];
  }
  return out;
}
