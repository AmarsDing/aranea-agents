<template>
  <q-card flat bordered>
    <q-card-section class="text-subtitle1 text-weight-bold">数据集</q-card-section>
    <q-separator />
    <q-list separator>
      <q-item
        v-for="ds in datasets"
        :key="ds.id"
        clickable
        :active="selectedId === ds.id"
        active-class="bg-primary text-white"
        @click="$emit('select', ds.id)"
      >
        <q-item-section>
          <q-item-label>{{ ds.name }}</q-item-label>
          <q-item-label caption :class="selectedId === ds.id ? 'text-white' : ''">{{ ds.case_count }} 用例</q-item-label>
        </q-item-section>
      </q-item>
    </q-list>
    <q-card-section v-if="!loading && !datasets.length" class="text-grey-7 text-center">暂无数据集</q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import type { EvalDataset } from "../../features/evaluation/types";

defineProps<{
  datasets: EvalDataset[];
  selectedId: string;
  loading?: boolean;
}>();

defineEmits<{
  select: [id: string];
}>();
</script>
