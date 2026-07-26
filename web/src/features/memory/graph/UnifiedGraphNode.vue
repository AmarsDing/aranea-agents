// Presentation: 跨层图谱自定义节点 — 圆形=L4 实体 / 圆角方块=L3 事实 / 三角=L2 情景（设计 §10.4）。 // P4: activation >
0 时按归一化强度发光（box-shadow + 微放大）。
<template>
  <div
    class="ug-node"
    :class="{
      'ug-node--focus': data.isFocus,
      'ug-node--selected': selected,
      'ug-node--activated': data.activation > 0,
    }"
  >
    <Handle type="target" :position="Position.Top" class="ug-node__handle" />
    <div class="ug-node__shape" :class="`ug-node__shape--${shapeKind}`" :style="shapeStyle">
      <q-icon :name="icon" size="18px" color="white" />
    </div>
    <div class="ug-node__label" :title="data.label">{{ data.label }}</div>
    <div class="ug-node__weight text-caption">{{ data.weight }}</div>
    <Handle type="source" :position="Position.Bottom" class="ug-node__handle" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { Handle, Position } from '@vue-flow/core';
import { memoryLayerColor } from '../panorama/layerMeta';

export type UnifiedNodeData = {
  label: string;
  layer: string;
  kind: string;
  weight: number;
  isFocus: boolean;
  /** P4 扩散激活强度（0~1 归一化），> 0 时节点发光。 */
  activation: number;
};

const props = defineProps<{ data: UnifiedNodeData; selected?: boolean }>();

const shapeKind = computed(() => {
  if (props.data.layer === 'L4') return 'circle';
  if (props.data.layer === 'L2') return 'triangle';
  return 'square';
});

const icon = computed(() => {
  if (props.data.kind === 'entity') return 'hub';
  if (props.data.kind === 'episode') return 'timeline';
  return 'psychology';
});

const shapeStyle = computed(() => {
  const color = memoryLayerColor(props.data.layer);
  const base =
    props.data.layer === 'L2'
      ? ({ '--ug-color': color, color } as Record<string, string>)
      : ({ '--ug-color': color, background: color } as Record<string, string>);

  // P4: activation 发光 — 强度归一化到 0~1，映射 box-shadow 扩散半径 + 微放大
  const act = props.data.activation;
  if (act > 0) {
    const intensity = Math.min(act, 1);
    const glowRadius = Math.round(4 + intensity * 12); // 4~16px
    const scale = 1 + intensity * 0.15; // 1.0~1.15x
    base.boxShadow = `0 0 ${glowRadius}px ${Math.round(intensity * 8)}px color-mix(in srgb, ${color} ${Math.round(intensity * 60)}%, transparent)`;
    base.transform = `scale(${scale.toFixed(3)})`;
    base.zIndex = '10';
  }

  return base;
});
</script>

<style scoped>
.ug-node {
  align-items: center;
  display: flex;
  flex-direction: column;
  height: 96px;
  position: relative;
  width: 132px;
}

.ug-node__shape {
  align-items: center;
  display: flex;
  height: 44px;
  justify-content: center;
  position: relative;
  transition:
    box-shadow 0.3s ease,
    transform 0.3s ease;
  width: 44px;
}

.ug-node__shape--circle {
  border-radius: 50%;
}

.ug-node__shape--square {
  border-radius: 10px;
}

.ug-node__shape--triangle {
  background: transparent !important;
}

.ug-node__shape--triangle::before {
  border-bottom: 40px solid currentColor;
  border-left: 23px solid transparent;
  border-right: 23px solid transparent;
  content: '';
  height: 0;
  left: -1px;
  position: absolute;
  top: 2px;
  width: 0;
}

.ug-node__shape--triangle .q-icon {
  margin-top: 18px;
  z-index: 1;
}

.ug-node--focus .ug-node__shape {
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--ug-color, #9e9e9e) 35%, transparent);
}

.ug-node--selected .ug-node__shape {
  box-shadow: 0 0 0 3px var(--q-primary);
}

.ug-node--activated .ug-node__shape {
  /* activation 样式由 shapeStyle 内联驱动，此处仅确保 transition 生效 */
}

.ug-node__label {
  color: var(--color-text-heading);
  font-size: 12px;
  line-height: 1.25;
  margin-top: 6px;
  max-width: 128px;
  overflow: hidden;
  text-align: center;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ug-node__weight {
  color: var(--color-text-secondary);
  font-size: 10px;
}

.ug-node__handle {
  opacity: 0;
  pointer-events: none;
}
</style>
