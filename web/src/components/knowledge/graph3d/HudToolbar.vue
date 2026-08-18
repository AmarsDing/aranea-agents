<template>
  <!-- D5：左中竖排图标胶囊（玻璃、青描边、tooltip）。纯展示组件：props in / emits out。 -->
  <div class="hud-toolbar">
    <q-btn flat dense round size="sm" icon="fit_screen" :aria-label="t('knowledgePage.graphFitView')" @click="$emit('fit')">
      <q-tooltip anchor="center right" self="center left">{{ t('knowledgePage.graphFitView') }}</q-tooltip>
    </q-btn>
    <q-btn
      flat
      dense
      round
      size="sm"
      :icon="layout === 'galaxy' ? 'blur_circular' : 'hub'"
      :aria-label="t('knowledgePage.graphLayoutSwitch')"
      @click="$emit('toggle-layout')"
    >
      <q-tooltip anchor="center right" self="center left">
        {{ t('knowledgePage.graphLayoutSwitch') }} ·
        {{ layout === 'galaxy' ? t('knowledgePage.graphLayoutGalaxy') : t('knowledgePage.graphLayoutForce') }}
      </q-tooltip>
    </q-btn>
    <q-btn
      flat
      dense
      round
      size="sm"
      icon="color_lens"
      :class="{ 'hud-toolbar__btn--on': colorMode === 'structure' }"
      :aria-label="t('knowledgePage.graphColorModeSwitch')"
      @click="$emit('toggle-color-mode')"
    >
      <q-tooltip anchor="center right" self="center left">
        {{ t('knowledgePage.graphColorModeSwitch') }} ·
        {{
          colorMode === 'structure'
            ? t('knowledgePage.graphColorModeStructure')
            : t('knowledgePage.graphColorModeGroup')
        }}
      </q-tooltip>
    </q-btn>
    <q-btn
      flat
      dense
      round
      size="sm"
      icon="rotate_right"
      :class="{ 'hud-toolbar__btn--on': autoRotate }"
      :aria-label="t('knowledgePage.graphAutoRotate')"
      @click="$emit('toggle-auto-rotate')"
    >
      <q-tooltip anchor="center right" self="center left">
        {{ t('knowledgePage.graphAutoRotate') }} [ {{ autoRotate ? 'ON' : 'OFF' }} ]
      </q-tooltip>
    </q-btn>
    <q-btn
      flat
      dense
      round
      size="sm"
      :icon="showLabels ? 'label' : 'label_off'"
      :class="{ 'hud-toolbar__btn--on': showLabels }"
      :aria-label="t('knowledgePage.graphShowLabels')"
      @click="$emit('toggle-labels')"
    >
      <q-tooltip anchor="center right" self="center left">
        {{ t('knowledgePage.graphShowLabels') }} [ {{ showLabels ? 'ON' : 'OFF' }} ]
      </q-tooltip>
    </q-btn>
    <q-btn flat dense round size="sm" icon="palette" :aria-label="t('knowledgePage.graphLegendTitle')">
      <q-tooltip anchor="center right" self="center left">{{ t('knowledgePage.graphLegendTitle') }}</q-tooltip>
      <q-menu anchor="top right" self="top left" class="hud-toolbar__legend">
        <div class="hud-toolbar__legend-section">
          <div class="hud-toolbar__legend-title">{{ t('knowledgePage.graphLegendNodes') }}</div>
          <div v-for="item in nodeLegend" :key="item.type" class="hud-toolbar__legend-row">
            <span class="hud-toolbar__chip-dot" :style="{ background: item.color, color: item.color }" />
            <span class="ellipsis">{{ item.type }}</span>
            <span class="hud-toolbar__legend-count">{{ item.count }}</span>
          </div>
        </div>
        <div class="hud-toolbar__legend-section">
          <div class="hud-toolbar__legend-title">{{ t('knowledgePage.graphLegendEdges') }}</div>
          <div v-for="lt in linkTypes" :key="lt" class="hud-toolbar__legend-row">
            <span
              class="hud-toolbar__chip-dot"
              :style="{ background: graphLinkColor(lt), color: graphLinkColor(lt) }"
            />
            <span>{{ t(`knowledgePage.linkType${lt.charAt(0).toUpperCase() + lt.slice(1)}`) }}</span>
          </div>
        </div>
      </q-menu>
    </q-btn>
    <q-btn
      flat
      dense
      round
      size="sm"
      :icon="fullscreen ? 'fullscreen_exit' : 'fullscreen'"
      :aria-label="fullscreen ? t('knowledgePage.graphExitFullscreen') : t('knowledgePage.graphFullscreen')"
      @click="$emit('toggle-fullscreen')"
    >
      <q-tooltip anchor="center right" self="center left">
        {{ fullscreen ? `${t('knowledgePage.graphExitFullscreen')} (Esc)` : t('knowledgePage.graphFullscreen') }}
      </q-tooltip>
    </q-btn>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { graphLinkColor } from '../../../features/knowledge/graphUi';
import type { GraphColorMode } from '../../../features/knowledge/graph3d/structurePalette';

defineProps<{
  /** 当前布局（图标随态）。 */
  layout: 'force' | 'galaxy';
  /** 节点着色模式（structure 高亮态）。 */
  colorMode: GraphColorMode;
  autoRotate: boolean;
  showLabels: boolean;
  fullscreen: boolean;
  /** 图例弹层：doc_type 计数（Top8）。 */
  nodeLegend: { type: string; count: number; color: string }[];
  /** 图例弹层：边类型清单。 */
  linkTypes: string[];
}>();

defineEmits<{
  fit: [];
  'toggle-layout': [];
  'toggle-color-mode': [];
  'toggle-auto-rotate': [];
  'toggle-labels': [];
  'toggle-fullscreen': [];
}>();

const { t } = useI18n();
</script>

<style lang="scss" scoped>
// 玻璃胶囊：CSS 变量由祖先 .kg-hud 提供（G5-E 深空皮肤）。
.hud-toolbar {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 4px 2px;
  border-radius: 999px;
  background: rgba(5, 8, 16, 0.72);
  border: 1px solid var(--kg-edge, #1a3a4a);
  backdrop-filter: blur(6px);
  -webkit-backdrop-filter: blur(6px);

  :deep(.q-btn) {
    color: var(--kg-text-dim, #7fa3b8);

    &:hover {
      color: var(--kg-cyan, #00d4ff);
    }
  }

  &__btn--on {
    color: var(--kg-cyan, #00d4ff) !important;
    text-shadow: 0 0 6px rgba(0, 212, 255, 0.35);
  }

  &__legend {
    min-width: 180px;
    max-width: 240px;
    padding: 10px 12px;
    background: var(--kg-panel, rgba(5, 8, 16, 0.88));
    border: 1px solid var(--kg-edge, #1a3a4a);
    box-shadow: 0 0 15px rgba(0, 212, 255, 0.13);
    color: var(--kg-text, #cfe8f5);
  }

  &__legend-section + &__legend-section {
    margin-top: 8px;
    padding-top: 8px;
    border-top: 1px solid var(--kg-edge, #1a3a4a);
  }

  &__legend-title {
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    color: var(--kg-text, #cfe8f5);
    margin-bottom: 6px;
  }

  &__legend-row {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    padding: 2px 0;
    min-width: 0;
  }

  &__legend-count {
    margin-left: auto;
    font-size: 11px;
    color: var(--kg-text-dim, #7fa3b8);
    font-variant-numeric: tabular-nums;
  }

  &__chip-dot {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-right: 4px;
    box-shadow: 0 0 6px currentColor;
  }
}
</style>
