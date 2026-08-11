<template>
  <div class="monitor-alert-rules-page">
    <!-- 页头 -->
    <div class="row items-center q-mb-md">
      <div>
        <div class="text-h6 text-weight-bold">{{ t('monitorPage.alerts.title') }}</div>
        <div class="text-caption text-grey-6">{{ t('monitorPage.alerts.subtitle') }}</div>
      </div>
      <q-space />
      <div class="app-actions-bar">
        <q-btn flat rounded no-caps icon="add" :label="t('monitorPage.alerts.add')" @click="addRule" />
        <q-btn flat rounded no-caps icon="refresh" :label="t('monitorPage.alerts.refresh')" :loading="loading" @click="$emit('reload')" />
        <!-- span 包裹：disabled 按钮不触发 tooltip，需要宿主元素 -->
        <span class="inline-block">
          <q-btn
            unelevated
            rounded
            no-caps
            class="app-accent-btn"
            icon="save"
            :label="t('monitorPage.alerts.save')"
            :loading="saving"
            :disable="saveDisabled"
            @click="onSave"
          />
          <q-tooltip v-if="saveDisabledReason">{{ saveDisabledReason }}</q-tooltip>
        </span>
      </div>
    </div>

    <!-- 指标目录（可折叠） -->
    <q-card flat bordered class="monitor-card q-mb-md">
      <q-card-section
        class="row items-center cursor-pointer"
        @click="catalogExpanded = !catalogExpanded"
      >
        <q-icon name="speed" size="sm" color="primary" class="q-mr-sm" />
        <div>
          <div class="text-subtitle1 text-weight-medium">{{ t('monitorPage.alerts.catalog.title') }}</div>
          <div class="text-caption text-grey-6">{{ t('monitorPage.alerts.catalog.subtitle') }}</div>
        </div>
        <q-space />
        <q-spinner v-if="metricsLoading" size="sm" />
        <q-btn flat round dense :icon="catalogExpanded ? 'expand_less' : 'expand_more'" />
      </q-card-section>
      <q-slide-transition>
        <div v-show="catalogExpanded">
          <q-separator />
          <q-card-section>
            <MonitorAlertMetricCatalog :metrics="metrics" :loading="metricsLoading" />
          </q-card-section>
        </div>
      </q-slide-transition>
    </q-card>

    <!-- 规则列表 -->
    <q-card flat bordered class="monitor-card">
      <q-card-section class="row items-center">
        <q-icon name="notifications_active" size="sm" color="primary" class="q-mr-sm" />
        <div class="text-subtitle1 text-weight-medium">{{ t('monitorPage.alerts.rulesTitle') }}</div>
        <q-chip v-if="editableRules.length" dense size="sm" color="primary" text-color="white" class="q-ml-sm">
          {{ editableRules.length }}
        </q-chip>
      </q-card-section>
      <q-separator />
      <q-card-section v-if="!editableRules.length" class="text-center q-pa-xl">
        <q-icon name="notifications_off" size="48px" color="grey-4" />
        <div class="text-grey-6 q-mt-sm">{{ t('monitorPage.alerts.emptyRules') }}</div>
        <q-btn
          flat
          rounded
          no-caps
          color="primary"
          icon="add"
          :label="t('monitorPage.alerts.add')"
          class="q-mt-md"
          @click="addRule"
        />
      </q-card-section>
      <template v-else>
        <MonitorAlertRuleCard
          v-for="(rule, idx) in editableRules"
          :key="rule.id || idx"
          v-model:rule="editableRules[idx]"
          class="monitor-alert-rule-item"
          :class="{ 'monitor-alert-rule-item--first': idx === 0 }"
          :metrics="metrics"
          :channel-options="channelOptions"
          @remove="removeRule(idx)"
        />
      </template>
    </q-card>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, toRaw } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
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
const $q = useQuasar();

const editableRules = ref<MonitorAlertRule[]>([]);

// 指标目录默认折叠（已有规则时），减少视觉干扰。
const catalogExpanded = ref(false);

/** 首条无效规则的校验原因（空串 = 全部有效）；保存按钮禁用时经 tooltip 展示。 */
const saveDisabledReason = computed(() => {
  for (const rule of editableRules.value) {
    if (!rule.name.trim()) return t('monitorPage.alerts.validation.nameRequired');
    if (!rule.metric_key.trim()) return t('monitorPage.alerts.validation.metricRequired');
    if (!(rule.threshold > 0)) return t('monitorPage.alerts.validation.thresholdPositive');
    if (!(rule.window_minutes > 0)) return t('monitorPage.alerts.validation.windowPositive');
  }
  return '';
});
const saveDisabled = computed(() => saveDisabledReason.value !== '');

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
  const rule = editableRules.value[idx];
  // 已持久化的规则（有 id）删除后需点保存才生效，先确认避免误删。
  // 未保存的新规则（无 id）仅存在于本地，直接移除。
  if (!rule?.id) {
    editableRules.value.splice(idx, 1);
    return;
  }
  $q.dialog({
    title: t('monitorPage.alerts.removeConfirmTitle'),
    message: t('monitorPage.alerts.removeConfirmMessage', { name: rule.name || rule.id }),
    cancel: { label: t('common.cancel'), flat: true, noCaps: true },
    ok: { label: t('monitorPage.alerts.rule.delete'), noCaps: true, color: 'negative' },
    persistent: true,
  }).onOk(() => {
    editableRules.value.splice(idx, 1);
  });
}

function onSave() {
  emit(
    'save',
    editableRules.value.map((r) => ({ ...toRaw(r) })),
  );
}
</script>

<style scoped>
.monitor-alert-rule-item {
  border-bottom: 1px solid var(--q-color-grey-3, #e0e0e0);
}

.monitor-alert-rule-item--first {
  border-top: none;
}

.monitor-alert-rule-item:last-child {
  border-bottom: none;
}
</style>
