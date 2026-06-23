<template>
  <q-card flat class="command-center-stat-panel">
    <q-card-section class="q-pb-sm">
      <div class="command-center-stat-panel__header">
        <q-icon
          name="dns"
          size="16px"
          class="command-center-stat-panel__icon command-center-stat-panel__icon--provider"
        />
        <span class="command-center-stat-panel__title">模型端点健康</span>
      </div>
    </q-card-section>
    <q-card-section class="q-pt-none">
      <div v-if="loading" class="row justify-center q-py-md">
        <q-skeleton type="text" width="60px" />
      </div>
      <template v-else>
        <div class="command-center-stat-panel__health-row">
          <span class="command-center-stat-panel__health-dot command-center-stat-panel__health-dot--ok" />
          <span class="command-center-stat-panel__health-label">活跃</span>
          <span class="command-center-stat-panel__health-value">{{ active }}</span>
        </div>
        <div class="command-center-stat-panel__health-row">
          <span class="command-center-stat-panel__health-dot command-center-stat-panel__health-dot--degraded" />
          <span class="command-center-stat-panel__health-label">降级</span>
          <span class="command-center-stat-panel__health-value command-center-stat-panel__health-value--danger">{{
            degraded
          }}</span>
        </div>
        <div class="command-center-stat-panel__health-row">
          <span class="command-center-stat-panel__health-label" style="margin-left: 14px">总计</span>
          <span class="command-center-stat-panel__health-value">{{ total }}</span>
        </div>
        <div v-if="degraded > 0" class="command-center-stat-panel__health-warn">
          <q-icon name="warning_amber" size="14px" color="warning" />
          <span>{{ degraded }} 个模型降级</span>
        </div>
      </template>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
defineProps<{
  active: number;
  degraded: number;
  total: number;
  loading: boolean;
}>();
</script>

<style lang="sass">
.command-center-stat-panel__icon--provider
  color: var(--color-success)
</style>
