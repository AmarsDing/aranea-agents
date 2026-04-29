<template>
  <div class="row q-col-gutter-md q-mb-md">
    <div v-for="card in cards" :key="card.label" class="col-12 col-sm-6 col-lg-3">
      <q-card flat bordered class="memory-metric-card">
        <q-card-section>
          <div class="row items-start justify-between no-wrap">
            <div>
              <div class="text-caption text-grey-7">{{ card.label }}</div>
              <div class="text-h5 text-weight-bold q-mt-xs">{{ card.value }}</div>
            </div>
            <q-avatar :color="card.color" text-color="white" :icon="card.icon" size="38px" />
          </div>
          <div class="text-caption text-grey-7 q-mt-sm">{{ card.hint }}</div>
        </q-card-section>
      </q-card>
    </div>
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
