<template>
  <q-page class="app-standard-page monitor-page">
    <div class="monitor-page-shell">
      <MonitorHeroSection
        kicker="Observability"
        title="运行监控"
        subtitle="审计、实时事件、模型用量总览、真实模型调用 Trace 与日志预留入口统一查看。"
      >
        <template #actions>
          <q-select
            v-model="filters.range"
            dense
            outlined
            emit-value
            map-options
            label="时间范围"
            :options="rangeOptions"
            class="monitor-range"
            @update:model-value="loadTraces"
          />
          <q-btn
            rounded
            no-caps
            unelevated
            class="monitor-primary-btn"
            icon="refresh"
            label="刷新"
            :loading="loading"
            @click="loadAll"
          />
        </template>
      </MonitorHeroSection>

      <MonitorErrorBanner v-if="error" :message="error" @retry="loadAll" />

      <div class="monitor-tabs-wrap">
        <MonitorGlassPanel>
          <q-tabs v-model="tab" class="monitor-tabs" align="left" no-caps outside-arrows mobile-arrows>
            <q-tab name="usage" icon="dashboard" label="Usage" />
            <q-tab name="alerts" icon="notifications_active" label="Alerts" />
            <q-tab name="audit" icon="fact_check" label="Audit" />
            <q-tab name="events" icon="sensors" label="Events" />
            <q-tab name="traces" icon="account_tree" label="Traces" />
            <q-tab name="logs" icon="terminal" label="Logs" />
          </q-tabs>
        </MonitorGlassPanel>
      </div>

      <q-tab-panels
        v-model="tab"
        animated
        class="monitor-panels"
        :class="{ 'monitor-panels--logs-fill': tab === 'logs' }"
      >
        <q-tab-panel name="usage">
          <MonitorRunnerMetrics />
          <MonitorUsageDashboardLink :range="filters.range" />
        </q-tab-panel>
        <q-tab-panel name="alerts">
          <MonitorAlertRules
            :rules="alertRules"
            :channel-options="alertChannelOptions"
            :loading="alertRulesLoading"
            :saving="alertRulesSaving"
            @reload="loadAlertRules"
            @save="saveAlertRules"
          />
        </q-tab-panel>
        <q-tab-panel name="audit">
          <AuditTable :rows="auditRows" :loading="loadingAudit" @reload="loadAudit" @notify="onChildNotify" />
        </q-tab-panel>
        <q-tab-panel name="events">
          <RealtimeEvents :persisted-events="events" :traces="traces" @clear="confirmClearEvents" />
        </q-tab-panel>
        <q-tab-panel name="traces">
          <TraceList
            :rows="traces"
            :loading="loadingTraces"
            :highlight-usage-event-id="highlightUsageEventId"
            @reload="loadTraces"
            @notify="onChildNotify"
          />
        </q-tab-panel>
        <q-tab-panel name="logs" class="monitor-logs-panel">
          <LogStreamPanel @clear-flow="confirmClearFlow" />
        </q-tab-panel>
      </q-tab-panels>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import MonitorErrorBanner from '../components/monitor/MonitorErrorBanner.vue';
import MonitorGlassPanel from '../components/monitor/MonitorGlassPanel.vue';
import MonitorHeroSection from '../components/monitor/MonitorHeroSection.vue';
import AuditTable from '../components/monitor/AuditTable.vue';
import LogStreamPanel from '../components/monitor/LogStreamPanel.vue';
import RealtimeEvents from '../components/monitor/RealtimeEvents.vue';
import TraceList from '../components/monitor/TraceList.vue';
import MonitorUsageDashboardLink from '../components/monitor/MonitorUsageDashboardLink.vue';
import MonitorRunnerMetrics from '../components/monitor/MonitorRunnerMetrics.vue';
import MonitorAlertRules from '../components/monitor/MonitorAlertRules.vue';
import { useMonitorAlertRules } from '../features/monitor/useMonitorAlertRules';
import { useMonitorPage } from '../features/monitor/useMonitorPage';
import { useQuasar } from 'quasar';
import { useMonitorStore } from '../stores/monitor';

const $q = useQuasar();
const monitorStore = useMonitorStore();

const {
  tab,
  highlightUsageEventId,
  auditRows,
  events,
  traces,
  loadingAudit,
  loadingTraces,
  error,
  filters,
  rangeOptions,
  loading,
  loadAll,
  loadAudit,
  loadTraces,
} = useMonitorPage();

const {
  rules: alertRules,
  channelOptions: alertChannelOptions,
  loading: alertRulesLoading,
  saving: alertRulesSaving,
  load: loadAlertRules,
  save: saveAlertRules,
} = useMonitorAlertRules();

function onChildNotify(payload: { message: string; type: 'positive' | 'negative' | 'warning' }) {
  $q.notify({ message: payload.message, type: payload.type, position: 'top' });
}

function confirmClearEvents() {
  $q.dialog({
    title: '清除事件',
    message: '确定清除所有实时事件？此操作不可撤销。',
    cancel: true,
    persistent: true,
  }).onOk(() => {
    monitorStore.clearRuntimeEvents();
  });
}

function confirmClearFlow() {
  $q.dialog({
    title: '清除日志',
    message: '确定清除所有流程日志？此操作不可撤销。',
    cancel: true,
    persistent: true,
  }).onOk(() => {
    monitorStore.clearFlowLogs();
  });
}
</script>
