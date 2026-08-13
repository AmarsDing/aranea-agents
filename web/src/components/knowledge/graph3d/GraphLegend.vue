<template>
  <!-- M5 过滤图例：点击切换组隐藏；悬停透镜（临时 dim 其他组）。真折射玻璃（M1）。 -->
  <div class="kg3d-legend">
    <GlassPanel strong refract :title="t('knowledgePage.graphLegendTitle')" icon="filter_list">
      <div
        v-for="g in groups"
        :key="g.docType"
        class="kg3d-legend__row"
        :class="{ 'kg3d-legend__row--hidden': hiddenGroups.includes(g.docType) }"
        :data-test="`legend-row-${g.docType}`"
        role="button"
        tabindex="0"
        @click="$emit('toggle-group', g.docType)"
        @keydown.enter="$emit('toggle-group', g.docType)"
        @pointerenter="$emit('lens-enter', g.docType)"
        @pointerleave="$emit('lens-leave')"
      >
        <span class="kg3d-legend__dot" :style="{ background: g.color }" />
        <span class="kg3d-legend__name">{{ g.docType || t('knowledgePage.graphLegendUntyped') }}</span>
        <span class="kg3d-legend__count">{{ g.count }}</span>
      </div>
      <div v-if="!groups.length" class="kg3d-legend__empty">{{ t('knowledgePage.graphLegendEmpty') }}</div>
    </GlassPanel>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import GlassPanel from '../effects/GlassPanel.vue';

export interface LegendGroup {
  docType: string;
  color: string;
  count: number;
}

defineProps<{
  groups: LegendGroup[];
  /** 已隐藏的 doc_type 列表。 */
  hiddenGroups: string[];
}>();

defineEmits<{
  'toggle-group': [docType: string];
  'lens-enter': [docType: string];
  'lens-leave': [];
}>();

const { t } = useI18n();
</script>

<style lang="scss" scoped>
.kg3d-legend {
  position: absolute;
  left: 16px;
  bottom: 16px;
  z-index: 5;
  min-width: 160px;

  &__row {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 6px;
    border-radius: 6px;
    cursor: pointer;
    font-size: 12px;

    &:hover {
      background: rgba(255, 255, 255, 0.06);
    }

    &--hidden {
      opacity: 0.4;
      font-style: italic;
    }
  }

  &__dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex: none;
  }

  &__name {
    flex: 1;
  }

  &__count {
    color: var(--kb-text-dim, var(--color-text-tertiary));
    font-size: 11px;
  }

  &__empty {
    font-size: 12px;
    color: var(--kb-text-dim, var(--color-text-secondary));
  }
}
</style>
