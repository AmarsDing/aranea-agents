<template>
  <q-card flat bordered class="monitor-card monitor-alert-rules">
    <q-card-section class="row items-center">
      <div>
        <div class="text-h6 text-weight-bold">{{ t('monitorPage.alerts.title') }}</div>
        <div class="text-caption text-grey-6">{{ t('monitorPage.alerts.subtitle') }}</div>
      </div>
      <q-space />
      <div class="app-actions-bar">
        <q-btn flat rounded no-caps icon="add" :label="t('monitorPage.alerts.add')" @click="addRule" />
        <q-btn flat rounded no-caps icon="refresh" :label="t('monitorPage.alerts.refresh')" :loading="loading" @click="$emit('reload')" />
        <q-btn
          unelevated
          rounded
          no-caps
          class="app-accent-btn"
          icon="save"
          :label="t('monitorPage.alerts.save')"
          :loading="saving"
          :disable="!editableRules.length || editableRules.some((r) => !r.name?.trim() || !r.metric_key?.trim())"
          @click="onSave"
        />
      </div>
    </q-card-section>
    <q-separator />
    <q-card-section>
      <MonitorAlertMetricCatalog :metrics="metrics" :loading="metricsLoading" />
    </q-card-section>
    <q-separator />
    <q-card-section>
      <div class="text-subtitle1 text-weight-medium q-mb-sm">{{ t('monitorPage.alerts.rulesTitle') }}</div>
      <div v-if="!editableRules.length" class="text-caption text-grey-6 q-pa-md">
        {{ t('monitorPage.alerts.emptyRules') }}
      </div>
      <MonitorAlertRuleCard
        v-for="(rule, idx) in editableRules"
        :key="rule.id || idx"
        v-model:rule="editableRules[idx]"
        class="q-mb-md"
        :metrics="metrics"
        :channel-options="channelOptions"
        @remove="removeRule(idx)"
      />
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { ref, watch, toRaw } from 'vue';
import { useI18n } from 'vue-i18n';
import type { AlertMetricInfo, MonitorAlertRule } from '../../features/monitor/types';
import MonitorAlertMetricCatalog from './MonitorAlertMetricCatalog.vue';
import MonitorAlertRuleCard from './MonitorAlertRuleCard.vue';

const props = defineProps<{
  rules: MonitorAlertRule[];
  metrics: AlertMetricInfo[];
  channelOptions: { label: string; value: string }[];
  loading: boolean;
  saving: boolean;
  metricsLoading: boolean;
}>();

const emit = defineEmits<{
  reload: [];
  save: [rules: MonitorAlertRule[]];
}>();

const { t } = useI18n();

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
