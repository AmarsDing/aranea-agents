<template>
  <div class="row q-col-gutter-md q-mb-md tools-metric-strip">
    <div v-for="card in cards" :key="card.label" class="col-12 col-sm-6 col-lg-3">
      <q-card flat class="tools-metric-card">
        <q-card-section>
          <div class="tools-metric-card__label">{{ card.label }}</div>
          <div class="tools-metric-card__value">{{ card.value }}</div>
          <div class="tools-metric-card__hint">{{ card.hint }}</div>
        </q-card-section>
      </q-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ToolMetricCard } from "./toolUi";

defineProps<{
  cards: ToolMetricCard[];
}>();
</script>

<style scoped lang="sass">
.tools-metric-card
  border-radius: 18px
  border: 1px solid var(--glass-border)
  background: var(--glass-surface)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))
  box-shadow: none

body.body--dark .tools-metric-card
  transition: border-color 0.2s ease, box-shadow 0.2s ease

body.body--dark .tools-metric-card:hover
  border-color: var(--glass-border-hover)

.tools-metric-card__label
  font-size: 12px
  color: var(--color-text-secondary)

.tools-metric-card__value
  font-size: 1.5rem
  font-weight: 700
  margin-top: 4px
  color: var(--color-text-primary)
  letter-spacing: -0.02em

.tools-metric-card__hint
  font-size: 12px
  margin-top: 6px
  color: var(--color-text-secondary)
</style>
