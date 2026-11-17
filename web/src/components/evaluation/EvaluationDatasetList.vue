<template>
  <q-card flat class="app-entity-glass-panel evaluation-dataset-list">
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
          <q-item-label caption :class="selectedId === ds.id ? 'text-white' : ''"
            >{{ ds.case_count }} 用例</q-item-label
          >
        </q-item-section>
      </q-item>
    </q-list>
    <q-card-section v-if="error" class="evaluation-dataset-list__error text-caption" role="alert">
      {{ error }}
    </q-card-section>
    <q-card-section v-else-if="!loading && !datasets.length" class="text-grey-7 text-center">暂无数据集</q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import type { EvalDataset } from '../../features/evaluation/types';

defineProps<{
  datasets: EvalDataset[];
  selectedId: string;
  loading?: boolean;
  /** 数据集列表加载失败：置位后不得再渲染空态（五态契约：失败与空数据互斥）。 */
  error?: string;
}>();

defineEmits<{
  select: [id: string];
}>();
</script>
