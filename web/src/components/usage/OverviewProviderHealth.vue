<template>
  <q-card flat class="overview-panel overview-provider-health">
    <q-card-section>
      <div class="row items-center q-gutter-sm">
        <q-icon name="dns" class="overview-provider-health__icon" :class="healthIconClass" />
        <div>
          <div class="text-h6 overview-section-title">Provider 健康</div>
          <div class="text-caption overview-section-caption">模型端点连通性状态</div>
        </div>
      </div>
    </q-card-section>
    <q-separator class="overview-separator" />
    <q-card-section class="q-py-sm">
      <div v-if="loading" class="row q-gutter-sm">
        <q-skeleton type="QChip" width="80px" />
        <q-skeleton type="QChip" width="80px" />
      </div>
      <div v-else-if="!models.length" class="overview-empty-inline">暂无 Provider 模型</div>
      <div v-else class="overview-provider-health__stats">
        <div class="overview-provider-health__stat">
          <span class="health-dot health-dot--ok" />
          <span class="overview-provider-health__label">活跃</span>
          <span class="overview-provider-health__value">{{ activeCount }}</span>
        </div>
        <div class="overview-provider-health__stat">
          <span class="health-dot health-dot--degraded" />
          <span class="overview-provider-health__label">降级</span>
          <span class="overview-provider-health__value overview-provider-health__value--danger">{{ degradedCount }}</span>
        </div>
        <div v-if="otherCount > 0" class="overview-provider-health__stat">
          <span class="health-dot health-dot--other" />
          <span class="overview-provider-health__label">其他</span>
          <span class="overview-provider-health__value">{{ otherCount }}</span>
        </div>
        <div class="overview-provider-health__stat">
          <span class="overview-provider-health__label">总计</span>
          <span class="overview-provider-health__value">{{ models.length }}</span>
        </div>
      </div>
      <div v-if="degradedModels.length > 0" class="overview-provider-health__degraded-list">
        <div v-for="m in degradedModels" :key="m.id" class="overview-provider-health__degraded-item">
          <q-icon name="warning_amber" size="14px" color="warning" />
          <span>{{ m.provider }} / {{ m.model || m.name }}</span>
        </div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { PlatformResource } from "../../features/platform/types";

const props = defineProps<{
  models: PlatformResource[];
  loading: boolean;
}>();

const activeCount = computed(() => props.models.filter((m) => m.status === "active" || !m.status).length);
const degradedCount = computed(() => props.models.filter((m) => m.status === "degraded").length);
const otherCount = computed(() => props.models.length - activeCount.value - degradedCount.value);

const degradedModels = computed(() =>
  props.models
    .filter((m) => m.status === "degraded")
    .slice(0, 5)
);

const healthIconClass = computed(() => {
  if (degradedCount.value > 0) return "overview-provider-health__icon--warn";
  return "overview-provider-health__icon--ok";
});
</script>
