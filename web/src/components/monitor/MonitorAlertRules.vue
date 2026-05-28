<template>
  <q-card flat bordered class="monitor-card monitor-alert-rules">
    <q-card-section class="row items-center">
      <div class="text-h6 text-weight-bold">告警规则</div>
      <q-space />
      <div class="app-actions-bar">
        <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="$emit('reload')" />
        <q-btn color="primary" unelevated rounded no-caps icon="save" label="保存" :loading="saving" :disable="!rules.length || rules.some(r => !r.name?.trim() || !r.metric_key?.trim())" @click="$emit('save')" />
      </div>
    </q-card-section>
    <q-separator />
    <q-card-section>
      <q-banner rounded class="monitor-info-banner q-mb-md">
        默认冷却 60 分钟。指标示例：runner.error_rate。超阈后写入 alert.fired 与 Events，并按规则出站 Webhook / Channel。
      </q-banner>
      <div v-for="(rule, idx) in rules" :key="rule.id || idx" class="monitor-alert-rule-row q-mb-md q-pa-md rounded-borders">
        <div class="app-form-field-grid">
          <q-input v-model="rule.name" dense outlined label="名称" />
          <q-input v-model="rule.metric_key" dense outlined label="指标键" />
          <q-input v-model.number="rule.threshold" dense outlined type="number" label="阈值" />
          <q-input v-model.number="rule.window_minutes" dense outlined type="number" label="窗口(分)" />
          <q-toggle v-model="rule.enabled" class="app-grid-span-full" label="启用" />
          <q-input v-model="rule.notify_webhook_url" class="app-field-long" dense outlined label="Webhook URL" />
          <q-select
            v-model="rule.notify_channel_id"
            dense
            outlined
            emit-value
            map-options
            clearable
            label="通知 Channel"
            :options="channelOptions"
          />
          <q-input v-model.number="rule.cooldown_minutes" dense outlined type="number" label="冷却(分)" />
        </div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import type { MonitorAlertRule } from "../../features/monitor/types";

defineProps<{
  rules: MonitorAlertRule[];
  channelOptions: { label: string; value: string }[];
  loading: boolean;
  saving: boolean;
}>();

defineEmits<{
  reload: [];
  save: [];
}>();
</script>
