<template>
  <!-- M4 聚焦节点信息卡：真折射玻璃（M1），画布右侧浮层，可收起。
       操作：在编辑器打开（open-in-explorer 既有链路）+ 重新向量化（B1 入口②，emit 由上层接线 RPC）。 -->
  <div class="kg3d-focus-card" :style="{ left: `${pos.x}px`, top: `${pos.y}px` }">
    <GlassPanel strong refract :title="collapsed ? node.name : t('knowledgePage.graphFocusCardTitle')" icon="my_location">
      <template #header-actions>
        <q-btn flat dense round size="xs" :icon="collapsed ? 'expand_more' : 'expand_less'" data-test="focus-collapse" @click="collapsed = !collapsed" />
        <q-btn flat dense round size="xs" icon="close" data-test="focus-close" @click="$emit('close')" />
      </template>
      <div v-if="!collapsed" data-test="focus-body" class="kg3d-focus-card__body">
        <div class="kg3d-focus-card__name">{{ node.name }}</div>
        <div class="kg3d-focus-card__meta">
          <span class="kg3d-focus-card__type">{{ node.docType }}</span>
          <span class="kg3d-focus-card__degree">{{ t('knowledgePage.graphFocusDegree', { n: node.degree }) }}</span>
        </div>
        <div class="kg3d-focus-card__path">{{ node.relPath }}</div>
        <div class="kg3d-focus-card__actions">
          <q-btn dense outline size="sm" icon="open_in_new" :label="t('knowledgePage.graphOpenInEditor')" data-test="focus-open"
                 @click="$emit('open-in-explorer', { docId: node.docId, relPath: node.relPath })" />
          <q-btn dense outline size="sm" icon="psychology" :label="t('knowledgePage.graphReembed')" data-test="focus-reembed"
                 :disable="!canReembed" @click="$emit('reembed', node.docId)" />
        </div>
      </div>
    </GlassPanel>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import GlassPanel from '../effects/GlassPanel.vue';

export interface FocusCardNode {
  docId: string;
  name: string;
  relPath: string;
  docType: string;
  degree: number;
}

defineProps<{
  node: FocusCardNode;
  /** B1：所属集合有语义层时可重新向量化。 */
  canReembed: boolean;
}>();

defineEmits<{
  'open-in-explorer': [payload: { docId: string; relPath: string }];
  reembed: [docId: string];
  close: [];
}>();

const { t } = useI18n();
const collapsed = ref(false);
/** 画布右侧浮层初始位（top 生效；left 由 CSS right 定位覆盖，预留 pointer 拖动接线）。 */
const pos = reactive({ x: 16, y: 76 });
</script>

<style lang="sass" scoped>
.kg3d-focus-card
  position: absolute
  right: 16px
  left: auto !important
  z-index: 5
  width: 280px

  &__name
    font-size: 14px
    font-weight: 600
    margin-bottom: 6px

  &__meta
    display: flex
    gap: 8px
    font-size: 11px
    color: var(--kb-text-dim)
    margin-bottom: 4px

  &__path
    font-size: 11px
    color: var(--kb-text-dim)
    opacity: 0.7
    margin-bottom: 10px
    word-break: break-all

  &__actions
    display: flex
    gap: 8px
</style>
