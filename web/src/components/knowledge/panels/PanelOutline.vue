<template>
  <div class="kb-panel-list">
    <template v-if="items.length">
      <button
        v-for="(it, i) in items"
        :key="i"
        type="button"
        class="kb-panel-item kb-panel-outline__item"
        :style="{ paddingLeft: `${8 + (it.level - 1) * 12}px` }"
        @click="$emit('jump', it.offset)"
      >
        <span class="ellipsis">{{ it.text }}</span>
      </button>
    </template>
    <div v-else class="kb-panel-empty">{{ t('knowledgePage.workbench.panels.noOutline') }}</div>
  </div>
</template>

<script setup lang="ts">
// 大纲面板（SP2 §SP2-8）：ATX 标题树，点击 → 编辑器滚动定位。
import { useI18n } from 'vue-i18n';
import type { OutlineItem } from '../../../features/knowledge/outline';

defineProps<{
  items: OutlineItem[];
}>();

defineEmits<{
  jump: [offset: number];
}>();

const { t } = useI18n();
</script>

<style lang="sass" scoped>
@use './panel-shared'

.kb-panel-outline__item
  font-size: 12.5px
</style>
