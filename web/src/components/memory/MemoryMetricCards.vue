// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <div class="app-metrics-grid app-metrics-grid--5 memory-metric-grid">
    <q-card v-for="card in cards" :key="card.label" flat class="app-metrics-card memory-metric-card app-metrics-grid__item">
      <q-card-section class="memory-metric-card__body">
        <div class="memory-metric-card__icon" :class="`memory-metric-card__icon--${card.color}`">
          <q-icon :name="card.icon" size="20px" />
        </div>
        <div class="memory-metric-card__content">
          <div class="app-metrics-card__label">{{ card.label }}</div>
          <div class="app-metrics-card__value">{{ card.value }}</div>
          <div class="app-metrics-card__hint">{{ card.hint }}</div>
        </div>
      </q-card-section>
    </q-card>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  cards: Array<{
    label: string;
    value: string | number;
    hint: string;
    icon: string;
    color: string;
  }>;
}>();
</script>
