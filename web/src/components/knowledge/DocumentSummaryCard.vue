<template>
  <div v-if="hasCard" class="kb-summary-card" data-test="doc-summary-card">
    <div v-if="docType" class="kb-summary-card__type">{{ docType }}</div>
    <p v-if="summary" class="kb-summary-card__text">{{ summary }}</p>
    <p v-else class="kb-summary-card__empty">{{ t('knowledgePage.summaryCardEmpty') }}</p>
    <div v-if="tags.length" class="kb-summary-card__tags">
      <span v-for="tag in tags" :key="tag" class="kb-summary-card__tag">{{ tag }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

const props = defineProps<{
  summary?: string;
  tags?: string[];
  docType?: string;
}>();

const { t } = useI18n();
const tags = computed(() => props.tags ?? []);
const hasCard = computed(() => Boolean(props.summary || props.docType || tags.value.length));
</script>

<style lang="sass" scoped>
.kb-summary-card
  display: flex
  flex-direction: column
  gap: 6px
  padding: 8px 10px
  border-radius: 10px
  border: 1px solid var(--glass-border)
  background: color-mix(in srgb, var(--color-accent) 6%, transparent)

  &__type
    font-size: 10.5px
    letter-spacing: 0.06em
    text-transform: uppercase
    color: var(--color-accent)

  &__text
    margin: 0
    font-size: 12.5px
    line-height: 1.45
    color: var(--color-text-primary)

  &__empty
    margin: 0
    font-size: 12px
    color: var(--color-text-secondary)

  &__tags
    display: flex
    flex-wrap: wrap
    gap: 4px

  &__tag
    padding: 0 7px
    border-radius: 999px
    font-size: 11px
    color: var(--color-accent)
    background: rgba(79, 216, 255, 0.1)
</style>
