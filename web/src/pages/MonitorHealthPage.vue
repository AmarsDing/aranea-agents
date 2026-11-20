<template>
  <!--
    Harness › 运行与治理 › 健康与告警（2026-11-19 从原监控页的 Usage / Alerts 两个页签拆出）。
    版式：四格状态 → 需要处理（自检失败项 + 正在触发的告警 + 失败原因，每条带去处）→ 自检 | 运行指标 →
    最近告警 | 运行时自愈 → 告警规则。设计见 04-Harness系统.md §12。
  -->
  <q-page class="app-standard-page op-page">
    <AppPageHero :title="t('ops.health.title')" :subtitle="t('ops.health.hint')">
      <template #actions>
        <q-btn
          unelevated
          no-caps
          color="primary"
          icon="play_arrow"
          :label="t('ops.health.runCheck')"
          :loading="selfCheckTriggering"
          @click="triggerSelfCheck"
        />
        <q-btn flat round dense icon="refresh" :aria-label="t('ops.common.refresh')" @click="refreshAll" />
      </template>
    </AppPageHero>

    <div class="op-kpis" role="list">
      <div v-for="k in kpis" :key="k.key" class="op-kpi" :class="`op-kpi--${k.tone}`" role="listitem">
        <span class="op-kpi__label">{{ k.label }}</span>
        <span class="op-kpi__value op-num">{{ k.value }}</span>
        <span class="op-kpi__sub">{{ k.sub }}</span>
      </div>
    </div>

    <section class="op-panel" :aria-label="t('ops.health.todo.title')">
      <header class="op-panel__head">
        <h2 class="op-panel__title">
          {{ t('ops.health.todo.title') }}<span class="op-panel__count op-num">{{ todos.length }}</span>
        </h2>
        <span class="op-panel__meta">{{ t('ops.health.todo.hint') }}</span>
      </header>
      <p v-if="!todos.length" class="op-empty">{{ t('ops.health.todo.none') }}</p>
      <ul v-else class="op-list">
        <li v-for="item in todos" :key="item.key">
          <component :is="item.to ? 'router-link' : 'div'" :to="item.to" class="op-row">
            <span class="op-dot" :class="`op-dot--${item.tone}`" aria-hidden="true" />
            <span class="op-row__main">
              <span class="op-row__title">{{ item.title }}</span>
              <span class="op-row__sub" :title="item.sub">{{ item.sub }}</span>
            </span>
            <span v-if="item.action" class="op-link">{{ item.action }}</span>
          </component>
        </li>
      </ul>
    </section>

    <div class="op-grid op-grid--health">
      <section class="op-panel" :aria-label="t('ops.health.selfCheck.title')">
        <header class="op-panel__head">
          <h2 class="op-panel__title">{{ t('ops.health.selfCheck.title') }}</h2>
          <span v-if="latest" class="op-tag" :class="`op-tag--${checkTone(latest.overall_status)}`">
            {{ checkLabel(latest.overall_status) }}
          </span>
          <span v-if="latest" class="op-panel__meta op-num">
            {{ t('ops.health.selfCheck.at', { time: relativeTime(t, latest.finished_at), ms: latest.duration_ms }) }}
          </span>
        </header>
        <p v-if="selfCheckError" class="op-note op-note--danger" role="alert">
          {{ t('ops.common.loadFailed', { msg: selfCheckError }) }}
        </p>
        <p v-if="!latest && !selfCheckLoading" class="op-empty">{{ t('ops.health.selfCheck.empty') }}</p>
        <ul v-else class="op-list">
          <li v-for="c in checks" :key="c.check_id" class="op-row">
            <span class="op-dot" :class="`op-dot--${checkTone(c.status)}`" aria-hidden="true" />
            <span class="op-row__main">
              <span class="op-row__title">{{ checkerLabel(c.checker) }}</span>
              <span class="op-row__sub">{{ c.message }}</span>
            </span>
            <span class="op-tag" :class="`op-tag--${checkTone(c.status)}`">{{ checkLabel(c.status) }}</span>
          </li>
        </ul>
        <template v-if="latest?.repair_actions.length">
          <h3 class="op-panel__sub">{{ t('ops.health.selfCheck.repairs') }}</h3>
          <ul class="op-list">
            <li v-for="(a, i) in latest.repair_actions" :key="i" class="op-row">
              <span class="op-dot" :class="a.success ? 'op-dot--ok' : 'op-dot--alert'" aria-hidden="true" />
              <span class="op-row__main">
                <span class="op-row__title">{{ a.action }}</span>
                <span class="op-row__sub">{{ a.message }}</span>
              </span>
            </li>
          </ul>
        </template>
      </section>

      <MetricsMatrix
        class="op-panel op-matrix"
        :slices="matrixSlices"
        :active-window="60"
        @retry="reloadMatrixOne"
        @drill="onMatrixDrill"
      />

      <section class="op-panel" :aria-label="t('ops.health.alerts.title')">
        <header class="op-panel__head">
          <h2 class="op-panel__title">
            {{ t('ops.health.alerts.title') }}<span class="op-panel__count op-num">{{ alertsTotal }}</span>
          </h2>
        </header>
        <p v-if="alertsError" class="op-note op-note--danger" role="alert">
          {{ t('ops.common.loadFailed', { msg: alertsError }) }}
        </p>
        <p v-if="!alerts.length && !alertsLoading" class="op-empty">{{ t('ops.health.alerts.empty') }}</p>
        <ul v-else class="op-list">
          <li v-for="g in alertGroups" :key="g.key" class="op-row">
            <span class="op-dot" :class="g.critical ? 'op-dot--alert' : 'op-dot--watch'" aria-hidden="true" />
            <span class="op-row__main">
              <span class="op-row__title">{{ g.name }}</span>
              <span class="op-row__sub">{{ g.detail }}</span>
            </span>
            <span class="op-tag op-tag--watch op-num">{{ t('ops.health.alerts.times', { n: g.count }) }}</span>
            <span class="op-row__time op-num">{{ relativeTime(t, g.at) }}</span>
            <router-link v-if="g.link" class="op-link" :to="g.link.to">{{ g.link.label }}</router-link>
          </li>
        </ul>
      </section>

      <section class="op-panel" :aria-label="t('ops.health.heal.title')">
        <header class="op-panel__head">
          <h2 class="op-panel__title">{{ t('ops.health.heal.title') }}</h2>
        </header>
        <p v-if="healError" class="op-note op-note--danger" role="alert">
          {{ t('ops.common.loadFailed', { msg: healError }) }}
        </p>
        <p v-if="!heal?.total && !healRecords.length" class="op-empty">{{ t('ops.health.heal.empty') }}</p>
        <template v-else>
          <h3 v-if="heal?.top_fail_rules.length" class="op-panel__sub">{{ t('ops.health.heal.topFail') }}</h3>
          <ul v-if="heal?.top_fail_rules.length" class="op-chips">
            <li v-for="r in heal.top_fail_rules" :key="r.rule_id" class="op-tag op-tag--watch">
              {{ r.rule_id }} <span class="op-num">×{{ r.count }}</span>
            </li>
          </ul>
          <h3 class="op-panel__sub">{{ t('ops.health.heal.records') }}</h3>
          <ul class="op-list">
            <li v-for="r in healRecords" :key="r.id" class="op-row">
              <span
                class="op-dot"
                :class="r.status === 'success' ? 'op-dot--ok' : 'op-dot--alert'"
                aria-hidden="true"
              />
              <span class="op-row__main">
                <span class="op-row__title">{{ r.rule_id }} · {{ r.fix_action_type }}</span>
                <span class="op-row__sub">{{ r.reason || r.trigger_type }}</span>
              </span>
              <span class="op-row__time op-num">{{ relativeTime(t, r.created_at) }}</span>
            </li>
          </ul>
        </template>
      </section>
    </div>

    <section class="op-panel" :aria-label="t('ops.health.rulesTitle')">
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
    </section>
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import AppPageHero from '../components/layout/AppPageHero.vue';
import MetricsMatrix from '../components/monitor/MetricsMatrix.vue';
import MonitorAlertRules from '../components/monitor/MonitorAlertRules.vue';
import { useMonitorHealth } from '../features/monitor/useMonitorHealth';
import { useMonitorAlertRules } from '../features/monitor/useMonitorAlertRules';
import { useRunnerMetricsMatrix } from '../features/monitor/useRunnerMetricsMatrix';
import { useFailureReasons } from '../features/usage/useFailureReasons';
import { relativeTime } from '../features/monitor/eventView';
import {
  alertRuleStateOf,
  formatMetricValue,
  formatPercent,
  metricI18nKey,
  parseJSON,
} from '../features/monitor/utils';
import type { MatrixMetricKey } from '../features/monitor/metricsMatrix';
import type { PlatformResource } from '../features/monitor/types';
import type { FailureReason } from '../features/usage/useFailureReasons';

const { t, te } = useI18n();
const router = useRouter();

const {
  latest,
  checks,
  failedChecks,
  selfCheckLoading,
  selfCheckTriggering,
  selfCheckError,
  triggerSelfCheck,
  alerts,
  alertsTotal,
  alerts24h,
  alertsLoading,
  alertsError,
  heal,
  healRecords,
  healError,
  refresh,
} = useMonitorHealth();

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

const { slices: matrixSlices, reloadOne: reloadMatrixOne, reloadAll: reloadMatrixAll } = useRunnerMetricsMatrix();

// 近 7 天失败的模型调用按原因归并：内置 Agent 模型丢失这类配置问题会持续失败，放进「需要处理」
const failures = useFailureReasons();
const FAILURE_RANGE = '7d';
void failures.load({ range: FAILURE_RANGE });

/** 告警该去哪看：失败率类 → 失败的运行；死信 → 今日收件箱；缓存命中 → 用量成本 */
function alertLink(metricKey: string): { to: string; label: string } | null {
  if (metricKey.startsWith('runner.'))
    return { to: '/monitor/logs?status=error', label: t('ops.health.alerts.goRuns') };
  if (metricKey.startsWith('sequencer.')) return { to: '/workbench', label: t('ops.health.alerts.goInbox') };
  if (metricKey.startsWith('llm.')) return { to: '/usage/events', label: t('ops.health.alerts.goUsage') };
  return null;
}

type Tone = 'ok' | 'watch' | 'alert' | 'off';

function checkTone(status: string): Tone {
  if (status === 'passed') return 'ok';
  if (status === 'warning') return 'watch';
  return 'alert';
}
function checkLabel(status: string) {
  return t(`ops.status.${checkTone(status)}`);
}
function checkerLabel(name: string) {
  const key = `ops.health.selfCheck.checker.${name}`;
  return te(key) ? t(key) : name;
}

function alertName(a: PlatformResource) {
  const metric = String(parseJSON(a.metadata_json).metric_key ?? '');
  const key = metric ? metricI18nKey(metric, 'name') : '';
  return key && te(key) ? t(key) : a.name;
}
/** 数值按指标单位显示（比率显示成百分比），单位取告警指标目录 */
function alertDetail(a: PlatformResource) {
  const m = parseJSON(a.metadata_json);
  if (m.value === undefined) return a.description;
  const unit = alertMetrics.value.find((x) => x.key === m.metric_key)?.unit ?? '';
  const text = t('ops.health.alerts.value', {
    value: formatMetricValue(unit, Number(m.value)),
    threshold: formatMetricValue(unit, Number(m.threshold)),
  });
  return m.reminder ? `${text} · ${t('ops.health.alerts.reminder')}` : text;
}

/** 同一条规则反复触发只占一行：次数 + 最近一次（列表已按时间倒序） */
const alertGroups = computed(() => {
  type Group = {
    key: string;
    name: string;
    detail: string;
    count: number;
    at: string;
    critical: boolean;
    link: { to: string; label: string } | null;
  };
  const groups = new Map<string, Group>();
  for (const a of alerts.value) {
    const key = String(parseJSON(a.metadata_json).rule_id ?? a.name);
    const g = groups.get(key);
    if (g) {
      g.count += 1;
      g.critical ||= a.status === 'critical';
    } else {
      groups.set(key, {
        key,
        name: alertName(a),
        detail: alertDetail(a),
        count: 1,
        at: a.created_at,
        critical: a.status === 'critical',
        link: alertLink(String(parseJSON(a.metadata_json).metric_key ?? '')),
      });
    }
  }
  return [...groups.values()];
});

const hour = computed(() => matrixSlices.value.find((s) => s.windowMinutes === 60)?.metrics ?? null);

type Todo = { key: string; tone: Tone; title: string; sub: string; to?: string; action?: string };

// 只涉及一个 Agent 时直达它的设置；涉及多个时去用量成本的「失败原因」，那里每个 Agent 一个修复链接
function failureLink(r: FailureReason): string {
  if (r.fix === 'agent' && r.agents.length === 1) return `/agents/${r.agents[0].id}/settings`;
  if (r.fix === 'credentials') return '/credentials';
  return '/usage/events';
}

function failureAction(r: FailureReason): string {
  if (r.fix === 'agent' && r.agents.length === 1) return t('ops.health.todo.fixAgent');
  if (r.fix === 'credentials') return t('ops.failures.fixCredentials');
  return t('ops.health.todo.viewUsage');
}

/** 需要处理：自检失败项、正在触发的告警、失败最多的几类原因；每条说清是什么、去哪处理 */
const todos = computed<Todo[]>(() => {
  const out: Todo[] = [];
  for (const c of checks.value) {
    if (c.status === 'passed') continue;
    out.push({
      key: `check:${c.checker}`,
      tone: checkTone(c.status),
      title: t('ops.health.todo.check', { name: checkerLabel(c.checker) }),
      sub: c.message,
    });
  }
  for (const rule of alertRules.value) {
    const metric = alertMetrics.value.find((m) => m.key === rule.metric_key);
    if (alertRuleStateOf(rule, metric) !== 'firing') continue;
    const link = alertLink(rule.metric_key);
    const nameKey = metricI18nKey(rule.metric_key, 'name');
    out.push({
      key: `alert:${rule.id}`,
      tone: rule.severity === 'critical' ? 'alert' : 'watch',
      title: t('ops.health.todo.alert', { name: te(nameKey) ? t(nameKey) : rule.name }),
      sub: t('ops.health.alerts.value', {
        value: formatMetricValue(metric?.unit ?? '', metric?.current_value),
        threshold: formatMetricValue(metric?.unit ?? '', rule.threshold),
      }),
      to: link?.to,
      action: link?.label,
    });
  }
  for (const r of failures.reasons.value.slice(0, 3)) {
    out.push({
      key: `fail:${r.key}`,
      tone: 'alert',
      title: t('ops.health.todo.failure', {
        n: r.count,
        cause: r.cause ? t(`ops.failures.cause.${r.cause}`) : r.message,
      }),
      sub: r.agents.map((a) => a.name).join('、') || r.message,
      to: failureLink(r),
      action: failureAction(r),
    });
  }
  return out;
});

const kpis = computed(() => {
  const total = latest.value?.check_results.length ?? 0;
  const healTotal = heal.value?.total ?? 0;
  const errorRate = hour.value?.error_rate ?? 0;
  return [
    {
      key: 'selfCheck',
      label: t('ops.health.kpi.selfCheck'),
      value: latest.value ? checkLabel(latest.value.overall_status) : t('ops.common.none'),
      sub: latest.value
        ? t('ops.health.kpi.selfCheckSub', { failed: failedChecks.value, total })
        : t('ops.health.kpi.selfCheckNone'),
      tone: (latest.value ? checkTone(latest.value.overall_status) : 'off') as Tone,
    },
    {
      key: 'alerts',
      label: t('ops.health.kpi.alerts'),
      value: String(alerts24h.value),
      sub: t('ops.health.kpi.alertsSub', { total: alertsTotal.value }),
      tone: (alerts24h.value > 0 ? 'watch' : 'ok') as Tone,
    },
    {
      key: 'errorRate',
      label: t('ops.health.kpi.errorRate'),
      value: hour.value ? formatPercent(errorRate) : t('ops.common.none'),
      sub: t('ops.health.kpi.errorRateSub', { runs: hour.value?.total_runs ?? 0 }),
      tone: (!hour.value ? 'off' : errorRate >= 0.25 ? 'alert' : errorRate > 0 ? 'watch' : 'ok') as Tone,
    },
    {
      key: 'heal',
      label: t('ops.health.kpi.heal'),
      value: String(healTotal),
      sub: healTotal
        ? t('ops.health.kpi.healSub', { rate: formatPercent(heal.value?.success_rate) })
        : t('ops.health.kpi.healNone'),
      tone: (healTotal ? 'ok' : 'off') as Tone,
    },
  ];
});

/** 矩阵下钻：去「追踪与日志」看那一段时间的运行，失败类指标只看失败的。 */
function onMatrixDrill(payload: { windowMinutes: number; metric: MatrixMetricKey }) {
  const failed = payload.metric === 'error_rate' || payload.metric === 'error_runs';
  void router.push({ path: '/monitor/logs', query: { tab: 'traces', ...(failed ? { status: 'error' } : {}) } });
}

function refreshAll() {
  void refresh();
  void failures.load({ range: FAILURE_RANGE });
  void reloadMatrixAll();
  void loadAlertRules();
}
</script>
