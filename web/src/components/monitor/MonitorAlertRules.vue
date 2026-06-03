<template>
  <q-card flat bordered class="monitor-card monitor-alert-rules">
    <q-card-section class="row items-center">
      <div class="text-h6 text-weight-bold">告警规则</div>
      <q-space />
      <div class="app-actions-bar">
        <q-btn flat rounded no-caps icon="add" label="新增" @click="addRule" />
        <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="$emit('reload')" />
        <q-btn
          unelevated
          rounded
          no-caps
          class="app-accent-btn"
          icon="save"
          label="保存"
          :loading="saving"
          :disable="!editableRules.length || editableRules.some((r) => !r.name?.trim() || !r.metric_key?.trim())"
          @click="onSave"
        />
      </div>
    </q-card-section>
    <q-separator />
    <q-card-section>
      <q-banner rounded class="monitor-info-banner q-mb-md">
        默认冷却 60 分钟。指标示例：runner.error_rate。超阈后写入 alert.fired 与 Events，并按规则出站 Webhook /
        Channel。
      </q-banner>
      <div
        v-for="(rule, idx) in editableRules"
        :key="rule.id || idx"
        class="monitor-alert-rule-row q-mb-md q-pa-md rounded-borders"
      >
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
          <div class="app-grid-span-full row justify-end">
            <q-btn flat dense no-caps icon="delete_outline" label="删除" color="negative" @click="removeRule(idx)" />
          </div>
        </div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { ref, watch, toRaw } from 'vue';
import type { MonitorAlertRule } from '../../features/monitor/types';

const props = defineProps<{
  rules: MonitorAlertRule[];
  channelOptions: { label: string; value: string }[];
  loading: boolean;
  saving: boolean;
}>();

const emit = defineEmits<{
  reload: [];
  save: [rules: MonitorAlertRule[]];
}>();

const editableRules = ref<MonitorAlertRule[]>([]);

watch(
  () => props.rules,
  (newRules) => {
    editableRules.value = newRules.map((r) => ({ ...toRaw(r) }));
  },
  { immediate: true },
);

function addRule() {
  editableRules.value.push({
    id: '',
    name: '',
    metric_key: '',
    threshold: 0,
    window_minutes: 60,
    enabled: true,
    severity: 'warning',
    notify_webhook_url: '',
    notify_channel_id: '',
    cooldown_minutes: 60,
  });
}

function removeRule(idx: number) {
  editableRules.value.splice(idx, 1);
}

function onSave() {
  emit(
    'save',
    editableRules.value.map((r) => ({ ...toRaw(r) })),
  );
}
</script>
