<template>
  <q-card flat bordered class="monitor-card monitor-alert-rules">
    <q-card-section class="row items-center">
      <div class="text-h6 text-weight-bold">告警规则</div>
      <q-space />
      <div class="app-actions-bar">
        <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="load" />
        <q-btn color="primary" unelevated rounded no-caps icon="save" label="保存" :loading="saving" @click="save" />
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
import { onMounted, ref } from "vue";
import { useQuasar } from "quasar";
import { listChannels } from "../../features/channels/api";
import { listMonitorAlertRules, putMonitorAlertRules, type MonitorAlertRule } from "../../features/monitor/api";

const $q = useQuasar();
const rules = ref<MonitorAlertRule[]>([]);
const channelOptions = ref<{ label: string; value: string }[]>([]);
const loading = ref(false);
const saving = ref(false);

async function loadChannels() {
  try {
    const rows = await listChannels();
    channelOptions.value = rows.map((c) => ({
      label: `${c.name || c.key} (${c.id})`,
      value: c.id
    }));
  } catch {
    channelOptions.value = [];
  }
}

async function load() {
  loading.value = true;
  try {
    rules.value = await listMonitorAlertRules();
  } catch (e) {
    $q.notify({ type: "negative", message: e instanceof Error ? e.message : "加载失败" });
  } finally {
    loading.value = false;
  }
}

async function save() {
  saving.value = true;
  try {
    rules.value = await putMonitorAlertRules(rules.value);
    $q.notify({ type: "positive", message: "告警规则已保存" });
  } catch (e) {
    $q.notify({ type: "negative", message: e instanceof Error ? e.message : "保存失败" });
  } finally {
    saving.value = false;
  }
}

onMounted(() => {
  void loadChannels();
  void load();
});
</script>
