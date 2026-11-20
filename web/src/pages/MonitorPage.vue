<template>
  <!--
    Harness › 运行与治理 › 追踪与日志（2026-11-19 重排）：回答「某一次运行发生了什么」。
    页签：运行追踪 / 事件 / 日志 / 审计 / 桌面；原 Usage、Alerts 页签挪到「健康与告警」(/monitor/health)。
  -->
  <q-page :class="['app-standard-page', 'op-page', 'monitor-page', { 'monitor-page--logs-fill': tab === 'logs' }]">
    <AppPageHero :title="t('ops.traces.title')" :subtitle="t('ops.traces.hint')">
      <template #actions>
        <q-btn
          flat
          round
          dense
          icon="refresh"
          :aria-label="t('ops.common.refresh')"
          :loading="loading"
          @click="loadAll"
        />
      </template>
    </AppPageHero>

    <MonitorErrorBanner v-if="error" :message="error" @retry="loadAll" />
    <MonitorErrorBanner
      v-if="backpressureMessage"
      :message="backpressureMessage"
      :action-label="t('ops.traces.gotIt')"
      @retry="clearBackpressure"
    />

    <q-tabs v-model="tab" class="op-tabs" align="left" no-caps dense inline-label outside-arrows mobile-arrows>
      <q-tab v-for="item in tabItems" :key="item.name" :name="item.name" :icon="item.icon" :label="item.label" />
    </q-tabs>

    <q-tab-panels
      v-model="tab"
      animated
      class="op-tab-panels monitor-panels"
      :class="{ 'monitor-panels--logs-fill': tab === 'logs' }"
    >
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
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import MonitorErrorBanner from '../components/monitor/MonitorErrorBanner.vue';
import AuditTable from '../components/monitor/AuditTable.vue';
import LogStreamPanel from '../components/monitor/LogStreamPanel.vue';
import RealtimeEvents from '../components/monitor/RealtimeEvents.vue';
import TraceList from '../components/monitor/TraceList.vue';
import TraceDetailDialog from '../components/monitor/TraceDetailDialog.vue';
import MonitorDesktopPanel from '../features/computeruse/MonitorDesktopPanel.vue';
import { useMonitorPage } from '../features/monitor/useMonitorPage';

const { t } = useI18n();

const tabItems = computed(() => [
  { name: 'traces', icon: 'account_tree', label: t('ops.traces.tab.traces') },
  { name: 'events', icon: 'sensors', label: t('ops.traces.tab.events') },
  { name: 'logs', icon: 'terminal', label: t('ops.traces.tab.logs') },
  { name: 'audit', icon: 'fact_check', label: t('ops.traces.tab.audit') },
  { name: 'desktop', icon: 'desktop_windows', label: t('ops.traces.tab.desktop') },
]);

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
  // Notify & confirm
  notify,
  confirmClearEvents,
  confirmClearFlow,
} = useMonitorPage();
</script>
