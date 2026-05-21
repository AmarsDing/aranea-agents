<template>
  <q-card flat bordered class="monitor-card q-mt-md">
    <q-card-section>
      <q-banner rounded class="monitor-info-banner">
        <template #avatar>
          <q-icon name="dashboard" color="primary" />
        </template>
        完整用量与成本大盘在<strong>概览</strong>页维护；本 Tab 仅保留 Runner 运行时指标。时间范围请使用页面顶栏筛选（与 Traces 共用）。
      </q-banner>
    </q-card-section>
    <q-card-section class="app-actions-bar app-actions-bar--start">
      <q-btn
        unelevated
        no-caps
        color="primary"
        icon="open_in_new"
        label="打开概览"
        :to="overviewTo"
      />
      <q-btn flat no-caps icon="receipt_long" label="查看明细" :to="eventsTo" />
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = withDefaults(
  defineProps<{
    range?: string;
  }>(),
  { range: "30d" }
);

const overviewTo = computed(() => ({
  path: "/overview",
  query: { range: props.range || "30d" }
}));

const eventsTo = computed(() => ({
  path: "/usage/events",
  query: { range: props.range || "30d" }
}));
</script>
