<template>
  <div class="kb-panel-list">
    <template v-if="frontmatter && frontmatter.entries.length">
      <div v-for="e in frontmatter.entries" :key="e.key" class="kb-prop">
        <span class="kb-prop__key">{{ e.key }}</span>
        <span v-if="e.values.length" class="kb-prop__tags">
          <span v-for="v in e.values" :key="v" class="kb-prop__tag">{{ v }}</span>
        </span>
        <span v-else class="kb-prop__value ellipsis" :title="e.value">{{ e.value || '—' }}</span>
      </div>
    </template>
    <div v-else class="kb-panel-empty">{{ t('knowledgePage.workbench.panels.noProperties') }}</div>
  </div>
</template>

<script setup lang="ts">
// 属性面板（SP2 §SP2-8）：frontmatter 只读键值；列表值芯片化。
import { useI18n } from 'vue-i18n';
import type { Frontmatter } from '../../../features/knowledge/frontmatter';

defineProps<{
  frontmatter: Frontmatter | null;
}>();

const { t } = useI18n();
</script>

<style lang="sass" scoped>
@use './panel-shared'

.kb-prop
  display: flex
  align-items: baseline
  gap: 10px
  padding: 4px 8px
  font-size: 12.5px

  &__key
    flex: none
    min-width: 72px
    color: var(--kb-text-dim)
    font-size: 11px
    text-transform: uppercase
    letter-spacing: 0.05em

  &__value
    min-width: 0
    color: var(--kb-text-primary)

  &__tags
    display: flex
    flex-wrap: wrap
    gap: 4px

  &__tag
    padding: 0 8px
    border-radius: 999px
    font-size: 11px
    color: var(--kb-accent-cyan)
    background: rgba(79, 216, 255, 0.1)
    border: 1px solid rgba(79, 216, 255, 0.22)
</style>
