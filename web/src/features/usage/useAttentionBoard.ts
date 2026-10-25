/**
 * 概览页注意力队列（attention board）跨域聚合 composable。
 *
 * 数据源（均为现有 API 直读快照，30s 轮询）：
 *   激活告警   → monitor alert-rules × alert-metrics（current_value ≥ threshold 即触发）
 *   失败 runs  → monitor runner-metrics（24h 窗口聚合 error_runs）+ cron-task-runs failure
 *   调用异常   → usage overview anomalies（排除用户主动取消）
 *   积压待办   → team task-dead-letters(pending) + decision hitl_approval(outcome=pending)
 *
 * TECH-DEBT: 跨域直读各 features/<domain>/api（未过 Store）；
 * 后续后端上线 GET /v1/monitor/attention-summary 聚合端点后切换（见 62-overview-command-center.design.md）。
 */
import { computed, onMounted, onUnmounted, ref, type Ref } from 'vue';
import { listMonitorAlertRules, listAlertMetrics, getRunnerMetrics } from '../monitor/api';
import { listTaskDeadLetters } from '../teams/api';
import { listDecisionRecords } from '../decision/api';
import { listCronTaskRuns } from '../cron/api';
import type { ModelTokenUsageEvent } from './types';
import {
  ATTENTION_ERROR_STATUSES,
  type AttentionBoard,
  type AttentionItem,
  type AttentionLane,
  type AttentionLaneState,
  type AttentionSeverity,
} from './types';

const POLL_INTERVAL_MS = 30_000;
const MAX_ROWS_PER_LANE = 3;

function severityOf(rule: { severity?: string }): AttentionSeverity {
  const s = (rule.severity ?? '').toLowerCase();
  if (s === 'critical' || s === 'high' || s === 'p0' || s === 'p1') return 'critical';
  if (s === 'warning' || s === 'warn' || s === 'medium' || s === 'p2') return 'warning';
  return 'info';
}

function truncate(text: string, max = 80): string {
  const t = (text ?? '').trim();
  return t.length > max ? `${t.slice(0, max)}…` : t;
}

function formatMetricValue(value: number, unit: string): string {
  if (unit === 'ratio') return `${(value * 100).toFixed(1)}%`;
  return value.toFixed(2).replace(/\.?0+$/, '') || '0';
}

/** outcome 为 pending/空 的 hitl 决策视为待审批（DecisionRecord 无 status 字段）。 */
function isPendingDecision(outcome: string): boolean {
  const o = (outcome ?? '').toLowerCase();
  return o === '' || o === 'pending' || o === 'awaiting';
}

export function useAttentionBoard(anomalies: Ref<ModelTokenUsageEvent[]>) {
  const lanes = ref<AttentionLaneState[]>([]);
  const loading = ref(false);
  const updatedAt = ref('');
  let timer: ReturnType<typeof setInterval> | null = null;

  async function refresh(): Promise<void> {
    loading.value = true;
    const [alertsRes, metricsRes, runnerRes, deadRes, decisionRes, cronRes] = await Promise.allSettled([
      listMonitorAlertRules(),
      listAlertMetrics(),
      getRunnerMetrics(24 * 60),
      listTaskDeadLetters({ status: 'pending', limit: 20 }),
      listDecisionRecords({ category: 'hitl_approval', pageSize: 50 }),
      listCronTaskRuns({ status: 'failure', limit: 20 }),
    ]);
    try {
      const next: AttentionLaneState[] = [];

      // —— 分栏 1：激活告警（enabled 规则 × 当前值超阈值） ——
      if (alertsRes.status === 'fulfilled' && metricsRes.status === 'fulfilled') {
        const metricByKey = new Map(metricsRes.value.map((m) => [m.key, m]));
        const triggered: AttentionItem[] = [];
        for (const rule of alertsRes.value) {
          if (!rule.enabled) continue;
          const metric = metricByKey.get(rule.metric_key);
          if (!metric) continue;
          if (metric.current_value < rule.threshold) continue;
          triggered.push({
            id: `alert:${rule.id}`,
            lane: 'alerts',
            severity: severityOf(rule),
            title: rule.name || rule.metric_key,
            detail: `${metric.name || rule.metric_key} 当前 ${formatMetricValue(metric.current_value, metric.unit)} ≥ 阈值 ${formatMetricValue(rule.threshold, metric.unit)}`,
            occurred_at: metric.evaluated_at ?? '',
            to: { path: '/monitor/logs', query: { tab: 'alerts' } },
          });
        }
        next.push({ lane: 'alerts', items: triggered.slice(0, MAX_ROWS_PER_LANE), count: triggered.length });
      }

      // —— 分栏 2：失败 runs（24h runner 聚合 + cron 失败明细） ——
      {
        const errorRuns = runnerRes.status === 'fulfilled' ? (runnerRes.value.error_runs ?? 0) : 0;
        const cronFailures = cronRes.status === 'fulfilled' ? cronRes.value.items : [];
        const items: AttentionItem[] = cronFailures.slice(0, MAX_ROWS_PER_LANE).map((run) => ({
          id: `cron:${run.id}`,
          lane: 'failed_runs',
          severity: 'warning',
          title: run.task_name || '定时任务',
          detail: truncate(run.error_message || '执行失败'),
          occurred_at: run.started_at ?? run.created_at ?? '',
          to: { path: '/cron' },
        }));
        next.push({ lane: 'failed_runs', items, count: errorRuns + cronFailures.length });
      }

      // —— 分栏 3：模型调用异常（usage anomalies，排除用户主动取消） ——
      {
        const rows = anomalies.value.filter((e) => ATTENTION_ERROR_STATUSES.has(e.status));
        const items: AttentionItem[] = rows.slice(0, MAX_ROWS_PER_LANE).map((e) => ({
          id: `call:${e.id}`,
          lane: 'call_errors',
          severity: 'warning',
          title: e.agent_name || e.model_display_name || e.model_api_id,
          detail: truncate(e.error_message || `${e.status} · ${e.provider_code}/${e.model_api_id}`),
          occurred_at: e.occurred_at ?? '',
          to: { path: '/usage/events', query: { status: 'error' } },
        }));
        next.push({ lane: 'call_errors', items, count: rows.length });
      }

      // —— 分栏 4：积压与待办（死信 + HITL 待审批） ——
      {
        const dead = deadRes.status === 'fulfilled' ? deadRes.value : [];
        const pendingDecisions =
          decisionRes.status === 'fulfilled' ? decisionRes.value.items.filter((d) => isPendingDecision(d.outcome)) : [];
        const items: AttentionItem[] = [
          ...dead.slice(0, MAX_ROWS_PER_LANE).map((d) => ({
            id: `dead:${d.id}`,
            lane: 'backlog' as AttentionLane,
            severity: 'critical' as AttentionSeverity,
            title: `死信 · ${d.source_type || 'task'}`,
            detail: truncate(d.error_message || '待重放'),
            occurred_at: d.created_at ?? '',
            to: { path: '/team' },
          })),
          ...pendingDecisions.slice(0, MAX_ROWS_PER_LANE).map((d) => ({
            id: `hitl:${d.decision_key}`,
            lane: 'backlog' as AttentionLane,
            severity: 'warning' as AttentionSeverity,
            title: `待审批 · ${d.scenario || d.category}`,
            detail: truncate(d.reasoning || d.actor_key || '等待人工处理'),
            occurred_at: d.created_at ?? '',
          })),
        ]
          .sort((a, b) => (a.severity === 'critical' ? -1 : 1) - (b.severity === 'critical' ? -1 : 1))
          .slice(0, MAX_ROWS_PER_LANE);
        next.push({ lane: 'backlog', items, count: dead.length + pendingDecisions.length });
      }

      lanes.value = next;
      updatedAt.value = new Date().toISOString();
    } finally {
      loading.value = false;
    }
  }

  const board = computed<AttentionBoard>(() => {
    const counts = lanes.value.reduce((acc, l) => ({ ...acc, [l.lane]: l.count }), {
      alerts: 0,
      failed_runs: 0,
      call_errors: 0,
      backlog: 0,
    } as Record<AttentionLane, number>);
    const hasCritical = lanes.value.some((l) => l.items.some((i) => i.severity === 'critical'));
    const anyCount = Object.values(counts).some((c) => c > 0);
    return {
      lanes: lanes.value,
      counts,
      worstSeverity: hasCritical ? 'critical' : anyCount ? 'warning' : 'ok',
      updatedAt: updatedAt.value,
    };
  });

  function onVisibility() {
    if (!document.hidden) void refresh();
  }

  onMounted(() => {
    void refresh();
    timer = setInterval(() => {
      if (!document.hidden) void refresh();
    }, POLL_INTERVAL_MS);
    document.addEventListener('visibilitychange', onVisibility);
  });

  onUnmounted(() => {
    if (timer) clearInterval(timer);
    document.removeEventListener('visibilitychange', onVisibility);
  });

  return { board, loading, refresh };
}
