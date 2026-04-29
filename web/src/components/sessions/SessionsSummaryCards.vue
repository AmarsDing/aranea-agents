<template>
  <div class="row q-col-gutter-md q-mb-md">
    <div v-for="card in cards" :key="card.label" class="col-12 col-sm-6 col-lg-3">
      <q-card flat class="sessions-summary-card">
        <q-card-section>
          <div class="sessions-muted text-caption">{{ card.label }}</div>
          <div class="text-h5 text-weight-bold q-mt-xs" style="color: var(--color-text-primary)">{{ card.value }}</div>
          <div class="sessions-muted text-caption q-mt-xs">{{ card.hint }}</div>
        </q-card-section>
      </q-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { SessionsSummaryCard } from "./sessionUi";

defineProps<{ cards: SessionsSummaryCard[] }>();
</script>
