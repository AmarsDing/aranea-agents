<template>
  <!--
    Harness › 运行与治理 › 实时运转（/monitor/live，2026-11-19 取代原「作战总台」）：回答「现在谁在干什么」。
    版式：四格状态 → 左：正在跑的事（运行 + 编码终端）、刚刚结束 ｜ 右：实时动态、在岗。
    原页的签批泳道与工作台 › 今日重复，改成「等人确认」一格直达今日；写死的业务线「待接入」占位已删。
  -->
  <q-page class="app-standard-page op-page">
    <AppPageHero :title="t('ops.live.title')" :subtitle="t('ops.live.hint')">
      <template #actions>
        <span class="op-live-ws" :class="{ 'is-off': !stream.connected.value }" role="status">
          <span class="op-dot" :class="stream.connected.value ? 'op-dot--run' : 'op-dot--alert'" aria-hidden="true" />
          {{ stream.connected.value ? t('ops.live.wsOn') : t('ops.live.wsOff') }}
          <span class="op-num">· {{ t('ops.live.rate', { n: stream.eventRatePerMin.value }) }}</span>
        </span>
        <q-btn flat round dense icon="refresh" :aria-label="t('ops.common.refresh')" @click="refreshAll" />
      </template>
    </AppPageHero>

    <div class="op-kpis" role="list">
      <component
        :is="k.to ? 'router-link' : 'div'"
        v-for="k in kpis"
        :key="k.key"
        :to="k.to"
        class="op-kpi"
        :class="[`op-kpi--${k.tone}`, { 'op-kpi--link': k.to }]"
        role="listitem"
      >
        <span class="op-kpi__label">{{ k.label }}</span>
        <span class="op-kpi__value op-num">{{ k.value }}</span>
        <span class="op-kpi__sub">{{ k.sub }}</span>
      </component>
    </div>

    <div class="op-grid op-grid--wide-left">
      <div class="op-col">
        <section class="op-panel" :aria-label="t('ops.live.running')">
          <header class="op-panel__head">
            <h2 class="op-panel__title">
              {{ t('ops.live.running') }}<span class="op-panel__count op-num">{{ runningTotal }}</span>
            </h2>
            <router-link class="op-link op-panel__meta" to="/monitor/logs?status=running">
              {{ t('ops.common.viewAll') }}
            </router-link>
          </header>
          <p v-if="runsError" class="op-note op-note--danger" role="alert">
            {{ t('ops.common.loadFailed', { msg: runsError }) }}
          </p>
          <p v-if="loaded && !running.length && !busyTerminals.length" class="op-empty">{{ t('ops.live.idle') }}</p>
          <ul v-if="running.length" class="op-list">
            <li v-for="r in running" :key="r.id">
              <router-link class="op-row" :to="traceLink(r.id)">
                <span class="op-dot op-dot--run" aria-hidden="true" />
                <span class="op-row__main">
                  <span class="op-row__title">{{ runTitle(r) }}</span>
                  <span class="op-row__sub">{{ runSub(r) }}</span>
                </span>
                <span class="op-row__num op-num">{{ elapsed(r.created_at) }}</span>
              </router-link>
            </li>
          </ul>
          <template v-if="busyTerminals.length">
            <h3 class="op-panel__sub">{{ t('ops.live.terminals', { n: busyTerminals.length }) }}</h3>
            <ul class="op-list">
              <li v-for="x in busyTerminals" :key="x.id">
                <router-link class="op-row" to="/fleet">
                  <span
                    class="op-dot"
                    :class="x.status === 'needs_input' ? 'op-dot--watch' : 'op-dot--run'"
                    aria-hidden="true"
                  />
                  <span class="op-row__main">
                    <span class="op-row__title">{{ x.title || x.agent }}</span>
                    <span class="op-row__sub">{{ x.preview || x.agent }}</span>
                  </span>
                  <span class="op-tag" :class="x.status === 'needs_input' ? 'op-tag--watch' : 'op-tag--run'">
                    {{ t(`ops.live.terminal.${x.status}`) }}
                  </span>
                </router-link>
              </li>
            </ul>
          </template>
          <p v-if="terminalsError" class="op-note op-note--danger">
            {{ t('ops.common.loadFailed', { msg: terminalsError }) }}
          </p>
        </section>

        <section class="op-panel" :aria-label="t('ops.live.recent')">
          <header class="op-panel__head">
            <h2 class="op-panel__title">{{ t('ops.live.recent') }}</h2>
          </header>
          <p v-if="loaded && !recent.length" class="op-empty">{{ t('ops.live.noRecent') }}</p>
          <ul v-else class="op-list">
            <li v-for="r in recent" :key="r.id">
              <router-link class="op-row" :to="traceLink(r.id)">
                <span class="op-dot" :class="`op-dot--${runTone(r.status)}`" aria-hidden="true" />
                <span class="op-row__main">
                  <span class="op-row__title">{{ runTitle(r) }}</span>
                  <span class="op-row__sub">{{ runSub(r) }}</span>
                </span>
                <span class="op-tag" :class="`op-tag--${runTone(r.status)}`">{{ runStatus(r.status) }}</span>
                <span class="op-row__time op-num">{{ relativeTime(t, r.updated_at || r.created_at) }}</span>
              </router-link>
            </li>
          </ul>
        </section>
      </div>

      <div class="op-col">
        <section class="op-panel op-feed" :aria-label="t('ops.live.feed')">
          <header class="op-panel__head">
            <h2 class="op-panel__title">{{ t('ops.live.feed') }}</h2>
            <span class="op-panel__meta">{{ t('ops.live.feedHint') }}</span>
          </header>
          <p v-if="!feed.length" class="op-empty">{{ t('ops.live.feedEmpty') }}</p>
          <ol v-else class="op-feed__list" aria-live="polite">
            <li v-for="line in feed" :key="line.id" class="op-feed__line" :class="`op-feed__line--${line.kind}`">
              <span class="op-feed__time op-num">{{ clock(line.at) }}</span>
              <span v-if="line.who" class="op-feed__who">{{ line.who }}</span>
              <span class="op-feed__text">{{ line.text }}</span>
            </li>
          </ol>
        </section>

        <section class="op-panel" :aria-label="t('ops.live.roster')">
          <header class="op-panel__head">
            <h2 class="op-panel__title">
              {{ t('ops.live.roster')
              }}<span class="op-panel__count op-num">{{ pageState.allAgents.value.length }}</span>
            </h2>
            <router-link class="op-link op-panel__meta" to="/settings/organization">{{
              t('ops.live.org')
            }}</router-link>
          </header>
          <p v-if="pageState.rosterLoaded.value && !depts.length" class="op-empty">{{ t('ops.live.noRoster') }}</p>
          <ul v-else class="op-list">
            <li v-for="d in depts" :key="d.id" class="op-row op-dept">
              <span class="op-row__main">
                <span class="op-row__title">{{ d.name }}</span>
                <span v-if="d.active.length" class="op-dept__agents">
                  <span v-for="a in d.active" :key="a.key" class="op-tag" :class="`op-tag--${a.tone}`" :title="a.word">
                    {{ a.name }}
                  </span>
                </span>
              </span>
              <span class="op-row__num op-num">{{
                t('ops.live.deptCount', { active: d.active.length, total: d.total })
              }}</span>
            </li>
          </ul>
        </section>
      </div>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import { useLiveOps } from '../features/monitor/useLiveOps';
import { useWorkbenchStream } from '../features/workbench/useWorkbenchStream';
import { useWorkbenchPage } from '../features/workbench/useWorkbenchPage';
import { relativeTime } from '../features/monitor/eventView';
import { formatCompactCount } from '../features/usage/moneyFormat';
import type { MonitorTraceRow } from '../features/monitor/types';
import type { RosterAgent } from '../features/workbench/types';

const { t, te } = useI18n();

const { running, runningTotal, recent, runsError, busyTerminals, terminalsError, inbox, hour, loaded, refresh } =
  useLiveOps();
const stream = useWorkbenchStream();
const pageState = useWorkbenchPage();

// 秒表：正在跑的运行显示已跑多久
const now = ref(Date.now());
let tick: ReturnType<typeof setInterval> | null = null;

onMounted(async () => {
  tick = setInterval(() => (now.value = Date.now()), 5000);
  void pageState.loadRoster();
  // 冷开时用今天的决策记录先垫上实时动态，避免空白
  const backfill = await pageState.fetchDecisionBackfill();
  stream.backfill(backfill.lines, backfill.humanDoneCount);
});
onUnmounted(() => {
  if (tick) clearInterval(tick);
});

function refreshAll() {
  void refresh();
  void pageState.loadRoster();
}

const nameOf = computed(() => new Map(pageState.allAgents.value.map((a) => [a.key, a.name])));

type Tone = 'ok' | 'watch' | 'alert' | 'off' | 'run';

/**
 * 服务端正在运行的运行所属的 Agent（ID 与显示名都记，花名册按 key 或名字认）。
 * 只靠 WS 实时事件的话，刷新页面后正在干活的 Agent 会显示成 0。
 */
const runningAgents = computed(() => {
  const ids = new Set<string>();
  const names = new Set<string>();
  for (const r of running.value) {
    if (r.agent_id) ids.add(r.agent_id);
    if (r.agent_name) names.add(r.agent_name);
  }
  return { ids, names, count: new Set(running.value.map((r) => r.agent_id || r.agent_name).filter(Boolean)).size };
});

const kpis = computed(() => {
  const h = hour.value;
  const failed = h?.error_runs ?? 0;
  const liveKeys = [...stream.liveAgents.values()].filter((s) => s.statusKind !== 'idle').map((s) => s.agentKey);
  // shortcut: WS 事件用 agent_key、运行记录用 agent_id，取两者较大值而非并集；ID 统一后改成并集
  const activeAgents = Math.max(new Set(liveKeys).size, runningAgents.value.count);
  return [
    {
      key: 'running',
      label: t('ops.live.kpi.running'),
      value: String(runningTotal.value),
      sub: t('ops.live.kpi.runningSub', { n: busyTerminals.value.length }),
      tone: (runningTotal.value || busyTerminals.value.length ? 'run' : 'off') as Tone,
      to: '/monitor/logs?status=running',
    },
    {
      key: 'waiting',
      label: t('ops.live.kpi.waiting'),
      value: inbox.value ? String(inbox.value.total) : t('ops.common.none'),
      sub: t('ops.live.kpi.waitingSub', { n: inbox.value?.highRisk ?? 0 }),
      tone: (inbox.value?.highRisk ? 'alert' : inbox.value?.total ? 'watch' : 'ok') as Tone,
      to: '/workbench',
    },
    {
      key: 'hour',
      label: t('ops.live.kpi.hour'),
      value: h ? String(h.total_runs - failed) : t('ops.common.none'),
      sub: t('ops.live.kpi.hourSub', { failed }),
      tone: (failed ? 'watch' : 'ok') as Tone,
      to: failed ? '/monitor/logs?status=error' : undefined,
    },
    {
      key: 'agents',
      label: t('ops.live.kpi.agents'),
      value: String(activeAgents),
      sub: t('ops.live.kpi.agentsSub', { n: pageState.allAgents.value.length }),
      tone: (activeAgents ? 'run' : 'off') as Tone,
    },
  ];
});

function traceLink(id: string) {
  return { path: '/monitor/logs', query: { trace: id } };
}

function runTitle(r: MonitorTraceRow) {
  return r.agent_name || r.team_name || r.description || r.name;
}

function runSub(r: MonitorTraceRow) {
  const domain = r.domain || r.name;
  const kind = te(`ops.live.domain.${domain}`) ? t(`ops.live.domain.${domain}`) : domain;
  const parts = [kind === runTitle(r) ? '' : kind, r.model];
  if (r.total_tokens) parts.push(`${formatCompactCount(r.total_tokens)} Token`);
  return parts.filter(Boolean).join(' · ');
}

function runTone(status: string): Tone {
  if (status === 'ok') return 'ok';
  if (status === 'running') return 'run';
  if (status === 'cancelled' || status === 'interrupted') return 'off';
  return 'alert';
}

function runStatus(status: string) {
  return te(`ops.live.status.${status}`) ? t(`ops.live.status.${status}`) : status;
}

/** 已跑多久：按开始时间到现在，秒级精度足够 */
function elapsed(iso: string) {
  const ms = now.value - Date.parse(iso);
  if (!Number.isFinite(ms) || ms < 0) return t('ops.common.none');
  const s = Math.floor(ms / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m${String(s % 60).padStart(2, '0')}s`;
  return `${Math.floor(m / 60)}h${String(m % 60).padStart(2, '0')}m`;
}

function clock(iso: string) {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleTimeString([], { hour12: false });
}

const feed = computed(() =>
  stream.logLines.value.slice(0, 40).map((line) => ({
    ...line,
    who: line.agentKey ? (nameOf.value.get(line.agentKey) ?? line.agentKey) : '',
  })),
);

/** 在岗：每个部门一行，只把正在干活 / 等人的 Agent 列出来，空闲的只计数（几百个 Agent 时不刷屏） */
const depts = computed(() => {
  const roster = pageState.roster.value;
  const groups = [
    ...roster.depts.map((d) => ({
      id: d.id,
      name: d.name,
      agents: [...d.positions.flatMap((p) => p.agents), ...d.floatAgents],
    })),
    ...(roster.unassigned.length ? [{ id: '_none', name: t('ops.live.unassigned'), agents: roster.unassigned }] : []),
  ];
  return groups
    .map((g) => {
      const active = g.agents.flatMap((a: RosterAgent) => {
        const live = stream.liveAgents.get(a.key);
        const waiting = stream.waitingAgents.has(a.key);
        const runningNow = runningAgents.value.ids.has(a.key) || runningAgents.value.names.has(a.name);
        if (!waiting && !runningNow && (!live || live.statusKind === 'idle')) return [];
        const tone: Tone =
          waiting || live?.statusKind === 'waiting' ? 'watch' : live?.statusKind === 'error' ? 'alert' : 'run';
        return [{ key: a.key, name: a.name, tone, word: live?.statusWord ?? '' }];
      });
      return { id: g.id, name: g.name, total: g.agents.length, active };
    })
    .sort((a, b) => b.active.length - a.active.length || b.total - a.total);
});
</script>
