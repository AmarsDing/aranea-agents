import { canvasPalette } from '../../../utils/canvasPalette';

/** 五层记忆色码与图标（设计 §10.4：卡片、图谱节点、表格徽章全站统一；色值见 --nt-memory-layer-*）。 */
export type MemoryLayerKey = 'L0' | 'L1' | 'L2' | 'L3' | 'L4';

export const MEMORY_LAYER_ORDER: MemoryLayerKey[] = ['L0', 'L1', 'L2', 'L3', 'L4'];

export const MEMORY_LAYER_META: Record<MemoryLayerKey, { icon: string }> = {
  L0: { icon: 'preview' },
  L1: { icon: 'assignment' },
  L2: { icon: 'timeline' },
  L3: { icon: 'psychology' },
  L4: { icon: 'hub' },
};

export function memoryLayerColor(layer: string): string {
  const palette = canvasPalette().memoryLayer;
  const key = layer as MemoryLayerKey;
  return key in MEMORY_LAYER_META ? palette[key] : palette.fallback;
}

const LAYER_TONE: Record<string, string> = { L0: 'l0', L1: 'l1', L2: 'l2', L3: 'l3', L4: 'l4' };

/** 层色类名后缀：取值域与 memoryLayerColor 完全一致（L0–L4 + fallback）。 */
export function memoryLayerTone(layer: string): string {
  return LAYER_TONE[layer] ?? 'fallback';
}
