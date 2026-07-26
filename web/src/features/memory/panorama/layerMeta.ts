/** 五层记忆色码与图标（设计 §10.4：卡片、图谱节点、表格徽章全站统一）。 */
export type MemoryLayerKey = 'L0' | 'L1' | 'L2' | 'L3' | 'L4';

export const MEMORY_LAYER_ORDER: MemoryLayerKey[] = ['L0', 'L1', 'L2', 'L3', 'L4'];

export const MEMORY_LAYER_META: Record<MemoryLayerKey, { color: string; icon: string }> = {
  L0: { color: '#90a4ae', icon: 'preview' },
  L1: { color: '#7986cb', icon: 'assignment' },
  L2: { color: '#4db6ac', icon: 'timeline' },
  L3: { color: '#ba68c8', icon: 'psychology' },
  L4: { color: '#ff8a65', icon: 'hub' },
};

export function memoryLayerColor(layer: string): string {
  return MEMORY_LAYER_META[layer as MemoryLayerKey]?.color ?? '#9e9e9e';
}
