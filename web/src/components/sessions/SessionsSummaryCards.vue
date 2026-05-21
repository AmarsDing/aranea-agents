<template>
  <div class="app-metrics-grid app-metrics-grid--4">
    <q-card v-for="card in cards" :key="card.label" flat class="app-metrics-card app-metrics-grid__item">
      <q-card-section>
        <div class="app-metrics-card__label">{{ card.label }}</div>
        <div class="app-metrics-card__value">{{ card.value }}</div>
        <div class="app-metrics-card__hint">{{ card.hint }}</div>
      </q-card-section>
    </q-card>
  </div>
</template>

<script setup lang="ts">
import type { SessionsSummaryCard } from "./sessionUi";

defineProps<{ cards: SessionsSummaryCard[] }>();
</script>
