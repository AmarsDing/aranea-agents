<template>
  <q-page class="app-standard-page monitor-page">
    <div class="monitor-page-shell">
      <MonitorHeroSection
        kicker="Observability"
        title="运行监控"
        subtitle="审计、实时事件、模型用量总览、真实模型调用 Trace 与日志预留入口统一查看。"
      >
        <template #actions>
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
      <MonitorErrorBanner
        v-if="backpressureMessage"
        :message="backpressureMessage"
        action-label="知道了"
        @retry="clearBackpressure"
      />

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
          <SelfCheckStatusPanel
            :loading="selfCheckLoading"
            :triggering="selfCheckTriggering"
            :latest-report="selfCheckLatestReport"
            @refresh="loadSelfCheckReports"
            @trigger="triggerSelfCheckAction"
          />
          <MonitorRunnerMetrics
            :metrics="runnerMetrics"
            :loading="runnerLoading"
            :window-minutes="runnerWindowMinutes"
            @update:window-minutes="runnerWindowMinutes = $event"
            @refresh="reloadRunnerMetrics"
            @drill="openRunsTab({ tab: 'traces' })"
          />
          <MonitorUsageDashboardLink :range="''" />
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
          <AuditTable :rows="auditRows" :loading="loadingAudit" @reload="loadAudit" @notify="notify" />
        </q-tab-panel>
        <q-tab-panel name="events">
          <RealtimeEvents
            :visible-events="visibleEvents"
            :stream-text="streamText"
            :stream-color="streamColor"
            :paused="paused"
            :category="category"
            :category-options="categoryOptions"
            :empty-hint="emptyHint"
            :selected="selected"
            :detail-open="detailOpen"
            :selected-j-s-o-n="selectedJSON"
            :traces="traces"
            @clear="confirmClearEvents"
            @toggle-stream="toggleStream"
            @open-detail="openDetail"
            @open-linked-run="openLinkedRun"
            @open-chat-session="openChatSession"
            @copy-j-s-o-n="copyJSON"
            @update:category="category = $event"
            @update:detail-open="detailOpen = $event"
          />
        </q-tab-panel>
        <q-tab-panel name="traces">
          <TraceList
            v-model:detail-open="traceDetailOpen"
            :rows="traces"
            :loading="loadingTraces"
            :highlight-usage-event-id="highlightUsageEventId"
            :flow-lines="flowLines"
            :active-correlation="activeCorrelation"
            :detail="traceDetail"
            @reload="loadTraces"
            @notify="notify"
            @open-trace="openTraceDetail"
            @open-chat-session="openChatSession"
          />
        </q-tab-panel>
        <q-tab-panel name="logs" class="monitor-logs-panel">
          <LogStreamPanel :sub-tab="subTab" @update:sub-tab="subTab = $event" @clear-flow="confirmClearFlow" />
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
import SelfCheckStatusPanel from '../components/monitor/SelfCheckStatusPanel.vue';
import { useMonitorAlertRules } from '../features/monitor/useMonitorAlertRules';
import { useMonitorPage } from '../features/monitor/useMonitorPage';

const {
  tab,
  highlightUsageEventId,
  auditRows,
  traces,
  loadingAudit,
  loadingTraces,
  error,
  loading,
  loadAll,
  loadAudit,
  loadTraces,
  // Runner metrics
  runnerMetrics,
  runnerLoading,
  runnerWindowMinutes,
  reloadRunnerMetrics,
  openRunsTab,
  openChatSession,
  // Realtime events
  visibleEvents,
  streamText,
  streamColor,
  paused,
  category,
  categoryOptions,
  emptyHint,
  selected,
  detailOpen,
  selectedJSON,
  toggleStream,
  openDetail,
  openLinkedRun,
  copyJSON,
  // Trace flow
  flowLines,
  activeCorrelation,
  openTraceDetail,
  traceDetail,
  traceDetailOpen,
  // Log stream
  subTab,
  backpressureMessage,
  clearBackpressure,
  // SelfCheck
  selfCheckLoading,
  selfCheckTriggering,
  selfCheckLatestReport,
  loadSelfCheckReports,
  triggerSelfCheckAction,
  // Notify & confirm
  notify,
  confirmClearEvents,
  confirmClearFlow,
} = useMonitorPage();

const {
  rules: alertRules,
  channelOptions: alertChannelOptions,
  loading: alertRulesLoading,
  saving: alertRulesSaving,
  load: loadAlertRules,
  save: saveAlertRules,
} = useMonitorAlertRules();
</script>
