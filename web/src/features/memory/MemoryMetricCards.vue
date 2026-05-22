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

<style scoped>
.memory-metric-grid {
  margin-bottom: var(--space-4);
}

.memory-metric-card__body {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: start;
  gap: 12px;
  padding: 14px 16px;
  min-height: 96px;
}

.memory-metric-card__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 11px;
  flex-shrink: 0;
  background: color-mix(in srgb, var(--color-accent) 14%, var(--glass-surface));
  color: var(--color-accent);
}

.memory-metric-card__icon--primary {
  background: color-mix(in srgb, var(--color-accent) 14%, var(--glass-surface));
  color: var(--color-accent);
}

.memory-metric-card__icon--positive {
  background: color-mix(in srgb, var(--color-success) 16%, var(--glass-surface));
  color: var(--color-success);
}

.memory-metric-card__icon--warning {
  background: color-mix(in srgb, var(--color-warning) 16%, var(--glass-surface));
  color: var(--color-warning);
}

.memory-metric-card__icon--negative {
  background: color-mix(in srgb, var(--color-danger) 16%, var(--glass-surface));
  color: var(--color-danger);
}

.memory-metric-card__icon--deep-purple {
  background: color-mix(in srgb, var(--color-neon-violet) 18%, var(--glass-surface));
  color: var(--color-neon-violet);
}

.memory-metric-card__icon--teal {
  background: color-mix(in srgb, var(--color-accent) 14%, var(--glass-surface));
  color: var(--color-accent);
}

.memory-metric-card__icon--blue-grey {
  background: color-mix(in srgb, var(--color-text-secondary) 16%, var(--glass-surface));
  color: var(--color-text-secondary);
}

.memory-metric-card__content {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.memory-metric-card :deep(.app-metrics-card__value) {
  margin-top: 2px;
  font-size: clamp(22px, 2.2vw, 28px);
  line-height: 1.15;
}

.memory-metric-card :deep(.app-metrics-card__hint) {
  margin-top: 4px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
