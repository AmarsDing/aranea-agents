/** Read-only projection of the attention snapshot; polling belongs to the page lifecycle. */
import { computed, onMounted, onUnmounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { useAttentionStore, type AttentionSource } from '../../stores/usage/attention';
import { useUsageStore } from '../../stores/usage';
import {
  ATTENTION_ERROR_STATUSES,
  type AttentionBoard,
  type AttentionItem,
  type AttentionLane,
  type AttentionLaneState,
  type AttentionSeverity,
} from './types';

const POLL_INTERVAL_MS = 30_000;
const MAX_ROWS = 3;

function severityOf(rule: { severity?: string }): AttentionSeverity {
  const severity = (rule.severity ?? '').toLowerCase();
  if (['critical', 'high', 'p0', 'p1'].includes(severity)) return 'critical';
  return ['warning', 'warn', 'medium', 'p2'].includes(severity) ? 'warning' : 'info';
}

function truncate(text: string, max = 80): string {
  const value = (text ?? '').trim();
  return value.length > max ? `${value.slice(0, max)}…` : value;
}

function formatMetric(value: number, unit: string): string {
  return unit === 'ratio' ? `${(value * 100).toFixed(1)}%` : value.toFixed(2).replace(/\.?0+$/, '') || '0';
}

export function useAttentionBoard() {
  const { t } = useI18n();
  const store = useAttentionStore();
  const usage = useUsageStore();
  let timer: ReturnType<typeof setInterval> | undefined;

  function statusOf(sources: AttentionSource[]): AttentionLaneState['status'] {
    const missing = sources.filter((key) => store.snapshot[key] === undefined).length;
    if (missing === sources.length) return store.initialized ? 'unavailable' : 'loading';
    return missing || sources.some((key) => store.failedSources.includes(key)) ? 'partial' : 'ready';
  }

  const board = computed<AttentionBoard>(() => {
    const data = store.snapshot;
    const metrics = new Map((data.metrics ?? []).map((metric) => [metric.key, metric]));
    const alerts: AttentionItem[] = (data.alerts ?? []).flatMap((rule) => {
      const metric = metrics.get(rule.metric_key);
      if (!rule.enabled || !metric || metric.current_value < rule.threshold) return [];
      return [
        {
          id: `alert:${rule.id}`,
          lane: 'alerts',
          severity: severityOf(rule),
          title: rule.name || rule.metric_key,
          detail: t('uiSystem.overview.metricThreshold', {
            name: metric.name || rule.metric_key,
            current: formatMetric(metric.current_value, metric.unit),
            threshold: formatMetric(rule.threshold, metric.unit),
          }),
          occurred_at: metric.evaluated_at ?? '',
          to: { path: '/monitor/logs', query: { tab: 'alerts' } },
        },
      ];
    });
    const cron = data.cron?.items ?? [];
    const runs: AttentionItem[] = cron.map((run) => ({
      id: `cron:${run.id}`,
      lane: 'failed_runs',
      severity: 'warning',
      title: run.task_name || t('uiSystem.overview.scheduledTask'),
      detail: truncate(run.error_message || t('uiSystem.overview.runFailed')),
      occurred_at: run.started_at ?? run.created_at ?? '',
      to: { path: '/cron' },
    }));
    const anomalies = (usage.overview?.anomalies ?? []).filter((event) => ATTENTION_ERROR_STATUSES.has(event.status));
    const calls: AttentionItem[] = anomalies.map((event) => ({
      id: `call:${event.id}`,
      lane: 'call_errors',
      severity: 'warning',
      title: event.agent_name || event.model_display_name || event.model_api_id,
      detail: truncate(event.error_message || `${event.status} · ${event.provider_code}/${event.model_api_id}`),
      occurred_at: event.occurred_at ?? '',
      to: { path: '/usage/events', query: { status: 'error' } },
    }));
    const dead = data.dead ?? [];
    const pending = (data.decisions?.items ?? []).filter((decision) =>
      ['', 'pending', 'awaiting'].includes((decision.outcome ?? '').toLowerCase()),
    );
    const backlog: AttentionItem[] = [
      ...dead.map(
        (item): AttentionItem => ({
          id: `dead:${item.id}`,
          lane: 'backlog',
          severity: 'critical',
          title: t('uiSystem.overview.deadLetter', { source: item.source_type || 'task' }),
          detail: truncate(item.error_message || t('uiSystem.overview.awaitingReplay')),
          occurred_at: item.created_at ?? '',
          to: { path: '/team' },
        }),
      ),
      ...pending.map(
        (item): AttentionItem => ({
          id: `hitl:${item.decision_key}`,
          lane: 'backlog',
          severity: 'warning',
          title: t('uiSystem.overview.awaitingApproval', { scenario: item.scenario || item.category }),
          detail: truncate(item.reasoning || item.actor_key || t('uiSystem.overview.awaitingHuman')),
          occurred_at: item.created_at ?? '',
        }),
      ),
    ].sort((a, b) => Number(b.severity === 'critical') - Number(a.severity === 'critical'));
    const usageStatus = !usage.overview ? (usage.error ? 'unavailable' : 'loading') : usage.error ? 'partial' : 'ready';
    const alertStatus = statusOf(['alerts', 'metrics']);
    const missingMetric = data.alerts?.some((rule) => rule.enabled && !metrics.has(rule.metric_key));
    const lanes: AttentionLaneState[] = [
      {
        lane: 'alerts',
        status: alertStatus === 'ready' && missingMetric ? 'partial' : alertStatus,
        items: alerts.slice(0, MAX_ROWS),
        count: alerts.length,
      },
      {
        lane: 'failed_runs',
        status: statusOf(['runner', 'cron']),
        items: runs.slice(0, MAX_ROWS),
        count: (data.runner?.error_runs ?? 0) + cron.length,
      },
      { lane: 'call_errors', status: usageStatus, items: calls.slice(0, MAX_ROWS), count: calls.length },
      {
        lane: 'backlog',
        status: statusOf(['dead', 'decisions']),
        items: backlog.slice(0, MAX_ROWS),
        count: backlog.length,
      },
    ];
    const counts = Object.fromEntries(lanes.map((lane) => [lane.lane, lane.count])) as Record<AttentionLane, number>;
    const incomplete = lanes.some((lane) => lane.status !== 'ready');
    return {
      lanes,
      counts,
      incomplete,
      updatedAt: store.updatedAt,
      worstSeverity:
        backlog.some((item) => item.severity === 'critical') || alerts.some((item) => item.severity === 'critical')
          ? 'critical'
          : Object.values(counts).some(Boolean)
            ? 'warning'
            : incomplete
              ? 'unknown'
              : 'ok',
    };
  });

  function refresh() {
    return store.refresh();
  }
  function onVisibility() {
    if (!document.hidden) void refresh();
  }
  onMounted(() => {
    void refresh();
    timer = setInterval(onVisibility, POLL_INTERVAL_MS);
    document.addEventListener('visibilitychange', onVisibility);
  });
  onUnmounted(() => {
    clearInterval(timer);
    document.removeEventListener('visibilitychange', onVisibility);
  });
  return { board, loading: computed(() => store.loading), refresh };
}
