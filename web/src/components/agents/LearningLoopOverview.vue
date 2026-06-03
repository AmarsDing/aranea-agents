<template>
  <div class="app-metrics-grid">
    <q-card flat bordered class="overview-metric-card app-metrics-grid__item">
      <q-card-section class="overview-metric-card__body">
        <div class="row items-center q-gutter-sm">
          <q-icon name="visibility" color="primary" size="26px" />
          <div class="overview-metric-card__label">观察数</div>
        </div>
        <div class="overview-metric-card__value">{{ observationCount }}</div>
        <div class="overview-metric-card__caption">累计行为观察</div>
      </q-card-section>
    </q-card>
    <q-card flat bordered class="overview-metric-card app-metrics-grid__item">
      <q-card-section class="overview-metric-card__body">
        <div class="row items-center q-gutter-sm">
          <q-icon name="pattern" color="primary" size="26px" />
          <div class="overview-metric-card__label">模式数</div>
        </div>
        <div class="overview-metric-card__value">{{ patternCount }}</div>
        <div class="overview-metric-card__caption">已识别行为模式</div>
      </q-card-section>
    </q-card>
    <q-card flat bordered class="overview-metric-card app-metrics-grid__item">
      <q-card-section class="overview-metric-card__body">
        <div class="row items-center q-gutter-sm">
          <q-icon name="pending_actions" color="warning" size="26px" />
          <div class="overview-metric-card__label">待审批</div>
        </div>
        <div class="overview-metric-card__value">{{ pendingCount }}</div>
        <div class="overview-metric-card__caption">待审批知识提议</div>
      </q-card-section>
    </q-card>
    <q-card flat bordered class="overview-metric-card app-metrics-grid__item">
      <q-card-section class="overview-metric-card__body">
        <div class="row items-center q-gutter-sm">
          <q-icon name="school" color="positive" size="26px" />
          <div class="overview-metric-card__label">已注册</div>
        </div>
        <div class="overview-metric-card__value">{{ registeredCount }}</div>
        <div class="overview-metric-card__caption">已注册知识</div>
      </q-card-section>
    </q-card>
    <div class="app-metrics-grid__item--full">
      <q-btn
        color="primary"
        rounded
        unelevated
        no-caps
        icon="autorenew"
        label="运行学习闭环"
        :loading="runningLoop"
        @click="emit('run-loop')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  observationCount: number;
  patternCount: number;
  pendingCount: number;
  registeredCount: number;
  runningLoop: boolean;
}>();

const emit = defineEmits<{
  'run-loop': [];
}>();
</script>
