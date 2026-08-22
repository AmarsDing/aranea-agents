<template>
  <q-page :class="['app-standard-page', 'monitor-page', { 'monitor-page--logs-fill': tab === 'logs' }]">
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
            <q-tab name="desktop" icon="desktop_windows" label="Desktop" />
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
        </q-tab-panel>
        <q-tab-panel name="alerts">
          <MonitorAlertRules
            :rules="alertRules"
            :metrics="alertMetrics"
            :channel-options="alertChannelOptions"
            :loading="alertRulesLoading"
            :saving="alertRulesSaving"
            :metrics-loading="alertMetricsLoading"
            :confirm-remove="confirmRemoveAlertRule"
            @reload="loadAlertRules"
            @save="saveAlertRules"
          />
        </q-tab-panel>
        <q-tab-panel name="audit">
          <AuditTable
            :rows="auditRows"
            :total="auditTotal"
            :loading="loadingAudit"
            @load="loadAudit"
            @notify="notify"
            @clear="handleClearAudit"
          />
        </q-tab-panel>
        <q-tab-panel name="events">
          <RealtimeEvents
            :pulse-events="pulseEvents"
            :stream-state="eventsStreamState"
            :paused="paused"
            :history-events="historyEvents"
            :history-total="historyTotal"
            :history-loading="historyLoading"
            :page="eventsPage"
            :page-size="eventsPageSize"
            :type-filter="typeFilter"
            :severity-filter="severityFilter"
            :show-system-events="showSystemEvents"
            @toggle-stream="toggleStream"
            @clear-pulse="confirmClearEvents"
            @update:page="eventsPage = $event"
            @update:page-size="eventsPageSize = $event"
            @update:type-filter="typeFilter = $event"
            @update:severity-filter="severityFilter = $event"
            @update:show-system-events="showSystemEvents = $event"
            @refresh-history="refreshHistory"
            @open-session="(evt) => openChatSession(evt.completionSessionId || evt.sessionId || '')"
            @open-in-runs="openLinkedRun"
            @notify="notify"
          />
        </q-tab-panel>
        <q-tab-panel name="traces">
          <TraceList
            v-model:keyword="traceKeyword"
            v-model:status="traceStatus"
            v-model:domain="traceDomain"
            v-model:page="tracePage"
            v-model:page-size="tracePageSize"
            :rows="traces"
            :total="tracesTotal"
            :loading="loadingTraces"
            :status-counts="traceStatusCounts"
            :domain-counts="traceDomainCounts"
            :live-state="runsLiveState"
            :highlight-usage-event-id="highlightUsageEventId"
            @refresh="loadTraces"
            @reset="resetTraceFilters"
            @open-trace="openTraceDetail"
          />
          <TraceDetailDialog
            v-model:open="traceDetailOpen"
            :detail="traceDetail"
            :detail-spans="detailSpans"
            :flow-lines="flowLines"
            :active-correlation="activeCorrelation"
            @open-chat-session="openChatSession"
            @notify="notify"
          />
        </q-tab-panel>
        <q-tab-panel name="logs" class="monitor-logs-panel">
          <LogStreamPanel :sub-tab="subTab" @update:sub-tab="subTab = $event" @clear-flow="confirmClearFlow" />
        </q-tab-panel>
        <q-tab-panel name="desktop">
          <MonitorDesktopPanel />
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
import TraceDetailDialog from '../components/monitor/TraceDetailDialog.vue';
import MonitorRunnerMetrics from '../components/monitor/MonitorRunnerMetrics.vue';
import MonitorAlertRules from '../components/monitor/MonitorAlertRules.vue';
import SelfCheckStatusPanel from '../components/monitor/SelfCheckStatusPanel.vue';
import MonitorDesktopPanel from '../features/computeruse/MonitorDesktopPanel.vue';
import { useMonitorAlertRules } from '../features/monitor/useMonitorAlertRules';
import { useMonitorPage } from '../features/monitor/useMonitorPage';

const {
  tab,
  highlightUsageEventId,
  auditRows,
  auditTotal,
  loadingAudit,
  error,
  loading,
  loadAll,
  loadAudit,
  handleClearAudit,
  // Runs（Traces）
  traces,
  tracesTotal,
  traceStatusCounts,
  traceDomainCounts,
  loadingTraces,
  traceKeyword,
  traceStatus,
  traceDomain,
  tracePage,
  tracePageSize,
  runsLiveState,
  loadTraces,
  resetTraceFilters,
  // Runner metrics
  runnerMetrics,
  runnerLoading,
  runnerWindowMinutes,
  reloadRunnerMetrics,
  openRunsTab,
  openChatSession,
  // Realtime events (pulse 条 + 历史表)
  pulseEvents,
  state: eventsStreamState,
  paused,
  toggleStream,
  historyEvents,
  historyTotal,
  historyLoading,
  page: eventsPage,
  pageSize: eventsPageSize,
  typeFilter,
  severityFilter,
  showSystemEvents,
  refreshHistory,
  openLinkedRun,
  // Trace flow
  flowLines,
  detailSpans,
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
  metrics: alertMetrics,
  metricsLoading: alertMetricsLoading,
  load: loadAlertRules,
  save: saveAlertRules,
  confirmRemoveRule: confirmRemoveAlertRule,
} = useMonitorAlertRules();
</script>
