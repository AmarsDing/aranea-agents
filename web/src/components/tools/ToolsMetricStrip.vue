<template>
  <div class="app-metrics-grid tools-metric-strip">
    <q-card v-for="card in cards" :key="card.label" flat class="app-metrics-card">
      <q-card-section>
        <div class="app-metrics-card__label">{{ card.label }}</div>
        <div class="app-metrics-card__value">{{ card.value }}</div>
        <div class="app-metrics-card__hint">{{ card.hint }}</div>
      </q-card-section>
    </q-card>
  </div>
</template>

<script setup lang="ts">
import type { ToolMetricCard } from './toolUi';

defineProps<{
  cards: ToolMetricCard[];
}>();
</script>
