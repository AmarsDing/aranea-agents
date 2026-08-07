/**
 * palette：G5 深空图谱分组调色板（纯 TS）。
 *
 * 复用 G4 graphUi 调色板语义（doc_type 稳定哈希取色，同类同色），
 * 保证图谱节点配色与 G4 操作台图例一致。
 */
import { graphDocTypeColor } from '../graphUi';

/** groupId 对应的 doc_type → 稳定 hex 色（复用 G4 调色板）。 */
export function groupColorHex(docType: string): string {
  return graphDocTypeColor(docType);
}

/** groups（doc_type 列表）→ 每组一色。 */
export function buildGroupPalette(groups: string[]): string[] {
  return groups.map((g) => graphDocTypeColor(g));
}

/** '#rrggbb' → [r,g,b] 浮点三通道（0..1，渲染层 instanceColor 用）。 */
export function hexToRgbFloat(hex: string): [number, number, number] {
  const v = parseInt(hex.slice(1), 16);
  return [((v >> 16) & 0xff) / 255, ((v >> 8) & 0xff) / 255, (v & 0xff) / 255];
}
