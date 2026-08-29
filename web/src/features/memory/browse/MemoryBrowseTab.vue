// Container: approved — 记忆浏览 Tab 容器；层级 chips 控制分段显示，分段内容由父页经 slot 注入。
<template>
  <div class="column q-gutter-md">
    <div class="row items-center q-gutter-x-sm">
      <span class="text-caption text-grey-7">{{ t('memory.browse.filterLabel') }}</span>
      <q-chip
        v-for="opt in layerOptions"
        :key="opt.value"
        clickable
        dense
        :outline="layer !== opt.value"
        :style="chipStyle(opt)"
        text-color="white"
        @click="layer = opt.value"
      >
        {{ opt.label }}
      </q-chip>
    </div>
    <slot :show="showLayer" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { MEMORY_LAYER_META } from '../panorama/layerMeta';

/** 浏览 Tab 层级过滤：all 显示全部段；单选层级仅显示对应段。 */
export type BrowseLayer = 'all' | 'L0' | 'L1' | 'L2' | 'L3';

const layer = defineModel<BrowseLayer>('layer', { default: 'all' });

const { t } = useI18n();

type LayerChip = { value: BrowseLayer; label: string; color: string };

const layerOptions = computed<LayerChip[]>(() => [
  { value: 'all', label: t('memory.browse.layers.all'), color: 'var(--color-text-secondary)' },
  ...(['L0', 'L1', 'L2', 'L3'] as const).map((key) => ({
    value: key as BrowseLayer,
    label: `${key} · ${t(`memory.panorama.layers.${key}.name`)}`,
    color: MEMORY_LAYER_META[key].color,
  })),
]);

function chipStyle(opt: LayerChip): { background?: string; color?: string; borderColor?: string } {
  if (layer.value === opt.value) {
    return { background: opt.color };
  }
  return { color: opt.color, borderColor: opt.color };
}

function showLayer(target: Exclude<BrowseLayer, 'all'>): boolean {
  return layer.value === 'all' || layer.value === target;
}
</script>
