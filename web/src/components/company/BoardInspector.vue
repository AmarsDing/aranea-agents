<template>
  <!--
    右侧检视：点图上任何节点，这里说清「它现在怎么样、等我什么、交出去的事到哪了」，并给出下一步去处。
    只读：拍板和验收仍在工作台收件箱 / 任务页里做（同一份数据）。
  -->
  <aside class="kb-insp" role="region" :aria-labelledby="headId" @keydown.esc.stop="emit('close')">
    <header class="kb-insp__head">
      <div class="kb-insp__titles">
        <span class="kb-insp__kicker">{{ detail.kicker }}</span>
        <h2 :id="headId" ref="headEl" class="kb-insp__title" tabindex="-1">{{ detail.title }}</h2>
      </div>
      <span class="kb-state" :class="`kb-state--${detail.state}`">{{ t(`company.state.${detail.state}`) }}</span>
      <q-btn flat dense round icon="close" size="sm" :aria-label="t('company.close')" @click="emit('close')" />
    </header>

    <div class="kb-insp__body">
      <p class="kb-insp__now">{{ detail.now }}</p>
      <BoardLoop :loop="detail.loop" />

      <section v-if="detail.report" class="kb-insp__sec kb-insp__report">
        <h3>{{ t('company.gm.reportTitle', { at: time(detail.report.at) }) }}</h3>
        <p class="kb-insp__report-conclusion">{{ detail.report.conclusion }}</p>
        <ul v-if="detail.report.reasons.length" class="kb-insp__reasons">
          <li v-for="reason in detail.report.reasons" :key="reason">{{ reason }}</li>
        </ul>
        <p class="kb-insp__report-meta">
          {{ t(detail.report.writer === 'model' ? 'company.gm.byModel' : 'company.gm.byRule') }}
          <router-link v-if="!detail.report.acked" class="kb-link" to="/workbench">{{
            t('company.gm.unread')
          }}</router-link>
        </p>
      </section>

      <dl class="kb-insp__facts">
        <template v-for="fact in detail.facts" :key="fact.label">
          <dt>{{ fact.label }}</dt>
          <dd :class="{ 'kb-mono': fact.mono }">{{ fact.value }}</dd>
        </template>
      </dl>

      <div class="kb-insp__actions">
        <q-btn
          v-for="(action, index) in detail.actions"
          :key="action.to"
          no-caps
          dense
          :unelevated="index === 0"
          :outline="index > 0"
          color="primary"
          :label="action.label"
          :to="action.to"
        />
      </div>

      <section v-if="detail.waiting.length" class="kb-insp__sec">
        <h3>{{ t('company.sec.waiting', { n: detail.waiting.length }) }}</h3>
        <ul class="kb-insp__list">
          <li v-for="item in detail.waiting.slice(0, LIST_LIMIT)" :key="item.id">
            <router-link class="kb-insp__item" :to="itemLink(item.link)">
              <span v-if="item.risk_level === 'high'" class="kb-badge kb-badge--stuck">{{
                t('company.highRisk')
              }}</span>
              <span class="kb-insp__item-title">{{ item.title }}</span>
              <span v-if="item.due_at" class="kb-mono kb-insp__item-meta">{{ time(item.due_at) }}</span>
            </router-link>
          </li>
        </ul>
        <router-link v-if="detail.waiting.length > LIST_LIMIT" class="kb-link" to="/workbench">
          {{ t('company.more', { n: detail.waiting.length - LIST_LIMIT }) }}
        </router-link>
      </section>

      <section v-if="detail.work.length" class="kb-insp__sec">
        <h3>{{ t('company.sec.work', { n: detail.work.length }) }}</h3>
        <ul class="kb-insp__list">
          <li v-for="item in detail.work.slice(0, LIST_LIMIT)" :key="item.id">
            <router-link class="kb-insp__item" :to="item.to">
              <span class="kb-badge" :class="`kb-badge--${item.state}`">{{ t(`company.work.${item.state}`) }}</span>
              <span class="kb-insp__item-title">{{ item.title }}</span>
              <span class="kb-mono kb-insp__item-meta">{{ time(item.at) }}</span>
            </router-link>
          </li>
        </ul>
        <router-link v-if="detail.work.length > LIST_LIMIT" class="kb-link" to="/tasks">
          {{ t('company.more', { n: detail.work.length - LIST_LIMIT }) }}
        </router-link>
      </section>

      <section v-if="detail.signals.length" class="kb-insp__sec">
        <h3>{{ t('company.sec.signals', { n: detail.signals.length }) }}</h3>
        <ul class="kb-insp__list">
          <li v-for="signal in detail.signals" :key="signal.id">
            <router-link class="kb-insp__item" :to="signal.to || '/ceo/overview'">
              <span class="kb-badge" :class="signal.level === 'red' ? 'kb-badge--stuck' : 'kb-badge--waiting'">
                {{ t(`company.level.${signal.level === 'red' ? 'red' : 'yellow'}`) }}
              </span>
              <span class="kb-insp__item-title">{{ signal.text }}</span>
            </router-link>
          </li>
        </ul>
      </section>

      <section v-if="detail.children.length" class="kb-insp__sec">
        <h3>{{ detail.childrenTitle }}</h3>
        <ul class="kb-insp__list">
          <li v-for="child in detail.children" :key="child.id">
            <button type="button" class="kb-insp__item" @click="emit('select', child.id)">
              <span class="kb-dot" :class="`kb-dot--${child.state}`" aria-hidden="true" />
              <span class="kb-insp__item-title">{{ child.name }}</span>
              <span class="kb-sr">{{ t(`company.state.${child.state}`) }}</span>
              <span
                class="kb-mono kb-insp__item-meta"
                :class="{ 'kb-insp__item-meta--hot': child.state === 'attention' }"
              >
                {{ child.value }}
              </span>
            </button>
          </li>
        </ul>
      </section>

      <p v-if="partial.length" class="kb-insp__foot">
        {{ t('company.partial', { sources: partial.map((s) => t(`company.source.${s}`)).join('、') }) }}
      </p>
      <p class="kb-insp__foot">{{ t('company.footnote') }}</p>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import BoardLoop from './BoardLoop.vue';
import type { OpsAnomaly } from '../../features/ceoboard/types';
import {
  money,
  shortTime,
  type PartialSource,
  type Board,
  type BoardAgent,
  type CompanyNode,
  type DeptNode,
  type GmStatus,
  type InboxInput,
  type Loop,
  type NodeState,
  type WorkInput,
} from '../../features/company/tree';

const LIST_LIMIT = 6;

type Fact = { label: string; value: string; mono?: boolean };
type Child = { id: string; name: string; state: NodeState; value: string };
type Signal = { id: string; level: string; text: string; to: string };
type Detail = {
  kicker: string;
  title: string;
  state: NodeState;
  now: string;
  loop: Loop;
  facts: Fact[];
  actions: { label: string; to: string }[];
  waiting: InboxInput[];
  work: WorkInput[];
  signals: Signal[];
  childrenTitle: string;
  children: Child[];
  /** 公司：总经理最近一次汇报 */
  report?: NonNullable<GmStatus['lastReport']>;
};

const props = defineProps<{
  board: Board;
  /** me / co:<id> / dept:<coId>:<deptId> / agent:<coId>:<deptId>:<agentId> */
  selected: string;
  now: Date;
  signals: readonly OpsAnomaly[];
  partial: readonly PartialSource[];
}>();
const emit = defineEmits<{ close: []; select: [id: string] }>();

const { t } = useI18n();
const headId = 'kb-insp-title';
const headEl = ref<HTMLElement | null>(null);

const time = (iso: string) => shortTime(iso, props.now) || t('company.none');
const chat = (agentId: string) => `/chat?agent=${encodeURIComponent(agentId)}`;
// 收件箱条目自带去处；外链或空的回到工作台
const itemLink = (link: string | undefined) => (link?.startsWith('/') ? link : '/workbench');

function anomalyText(a: OpsAnomaly): string {
  const key = `ceoboard.ops.anomalies.${a.code}`;
  const translated = t(key, a.params ?? {});
  return translated === key ? a.text : translated;
}

/** 一句话说清这个节点现在怎么样 */
function nowLine(state: NodeState, loop: Loop, failed: number, active: number, agents: number): string {
  if (state === 'attention') {
    return t('company.now.attention', {
      waiting: loop.waiting,
      review: loop.review,
      stuck: loop.stuck,
      failed,
    });
  }
  if (state === 'ok') return t('company.now.ok', { active, running: loop.running });
  if (state === 'idle') return t('company.now.idle', { n: agents });
  return t('company.now.empty');
}

function deptValue(d: DeptNode): string {
  if (d.agentCount === 0) return t('company.leaf.empty');
  const hot = d.loop.waiting + d.loop.review + d.loop.stuck + d.failed;
  return hot > 0 ? t('company.needsYou', { n: hot }) : `${d.activeToday}/${d.agentCount}`;
}

function agentValue(a: BoardAgent): string {
  if (a.failed) return t('company.agentFailed');
  const hot = a.loop.waiting + a.loop.review + a.loop.stuck;
  if (hot > 0) return t('company.needsYou', { n: hot });
  if (a.loop.running > 0) return t('company.badge.running', { n: a.loop.running });
  return a.calls > 0 ? t('company.calls', { n: a.calls }) : time(a.lastRunAt);
}

/** 总经理的巡视 / 汇报：上次 → 下次；定时任务停了写「没在巡视」。 */
function gmFacts(gm: GmStatus | undefined): Fact[] {
  if (!gm) return [];
  if (!gm.enabled) return [{ label: t('company.fact.patrol'), value: t('company.gm.off') }];
  return [
    { label: t('company.fact.patrol'), value: `${time(gm.lastPatrolAt)} → ${time(gm.nextPatrolAt)}`, mono: true },
    {
      label: t('company.fact.report'),
      value: `${time(gm.lastReport?.at ?? '')} → ${time(gm.nextReportAt)}`,
      mono: true,
    },
  ];
}

function companyDetail(co: CompanyNode): Detail {
  return {
    kicker: t('company.kind.company'),
    title: co.name,
    state: co.state,
    now: nowLine(co.state, co.loop, co.failed, co.activeToday, co.agentCount),
    loop: co.loop,
    facts: [
      {
        label: t('company.fact.ceo'),
        value: co.ceo ? co.ceo.name : t(co.ceoConfigured ? 'company.leadMissing' : 'company.leadUnset'),
      },
      { label: t('company.fact.depts'), value: String(co.depts.length), mono: true },
      { label: t('company.fact.agents'), value: `${co.activeToday} / ${co.agentCount}`, mono: true },
      {
        label: t('company.fact.cost'),
        value: `${money(co.costMicro)} · ${t('company.calls', { n: co.calls })}`,
        mono: true,
      },
      ...gmFacts(co.gm),
      { label: t('company.fact.lastRun'), value: time(co.lastRunAt), mono: true },
      { label: t('company.fact.nextRun'), value: time(co.nextRunAt), mono: true },
    ],
    report: co.gm?.lastReport,
    actions: [
      ...(co.loop.waiting > 0 ? [{ label: t('company.goDecide'), to: '/workbench' }] : []),
      ...(co.ceo ? [{ label: t('company.askCeo'), to: chat(co.ceo.id) }] : []),
      { label: t('company.goOrg'), to: '/settings/organization' },
    ],
    waiting: co.waitingItems,
    work: co.workItems,
    signals: [],
    childrenTitle: t('company.sec.depts', { n: co.depts.length }),
    children: co.depts.map((d) => ({ id: `dept:${co.id}:${d.id}`, name: d.name, state: d.state, value: deptValue(d) })),
  };
}

function deptDetail(co: CompanyNode, d: DeptNode): Detail {
  return {
    kicker: `${t('company.kind.dept')} · ${co.name}`,
    title: d.name,
    state: d.state,
    now: nowLine(d.state, d.loop, d.failed, d.activeToday, d.agentCount),
    loop: d.loop,
    facts: [
      {
        label: t('company.fact.lead'),
        value: d.lead?.name ?? t(d.leadMissing ? 'company.leadMissing' : 'company.leadUnset'),
      },
      {
        label: t('company.fact.positions'),
        value: t('company.positionsValue', { n: d.positions, empty: d.emptyPositions }),
        mono: true,
      },
      { label: t('company.fact.agents'), value: `${d.activeToday} / ${d.agentCount}`, mono: true },
      {
        label: t('company.fact.cost'),
        value: `${money(d.costMicro)} · ${t('company.calls', { n: d.calls })}`,
        mono: true,
      },
      { label: t('company.fact.lastRun'), value: time(d.lastRunAt), mono: true },
      { label: t('company.fact.nextRun'), value: time(d.nextRunAt), mono: true },
    ],
    actions: [
      ...(d.loop.waiting > 0 ? [{ label: t('company.goDecide'), to: '/workbench' }] : []),
      ...(d.lead ? [{ label: t('company.askLead'), to: chat(d.lead.id) }] : []),
    ],
    waiting: d.waitingItems,
    work: d.workItems,
    signals: [],
    childrenTitle: t('company.sec.agents', { n: d.agents.length }),
    children: d.agents.map((a) => ({
      id: `agent:${co.id}:${d.id}:${a.id}`,
      name: a.name,
      state: a.state,
      value: agentValue(a),
    })),
  };
}

function agentDetail(co: CompanyNode, d: DeptNode | undefined, a: BoardAgent): Detail {
  return {
    kicker: [t('company.kind.agent'), d?.name ?? t('company.ceo.short'), co.name].join(' · '),
    title: a.name,
    state: a.state,
    now: a.failed ? t('company.now.failed') : nowLine(a.state, a.loop, 0, a.ranToday ? 1 : 0, 1),
    loop: a.loop,
    facts: [
      { label: t('company.fact.position'), value: a.id === d?.lead?.id ? t('company.deptLead') : a.position || '—' },
      { label: t('company.fact.lastRun'), value: time(a.lastRunAt), mono: true },
      { label: t('company.fact.nextRun'), value: time(a.nextRunAt), mono: true },
      {
        label: t('company.fact.cost'),
        value: `${money(a.costMicro)} · ${t('company.calls', { n: a.calls })}`,
        mono: true,
      },
    ],
    actions: [
      { label: t('company.openChat'), to: chat(a.id) },
      ...(a.loop.waiting > 0 ? [{ label: t('company.goDecide'), to: '/workbench' }] : []),
      { label: t('company.agentSettings'), to: `/agents/${encodeURIComponent(a.id)}/settings` },
    ],
    waiting: a.waitingItems,
    work: a.workItems,
    signals: [],
    childrenTitle: '',
    children: [],
  };
}

function meDetail(): Detail {
  const me = props.board.me;
  const state: NodeState = me.loop.waiting + me.loop.review + me.loop.stuck > 0 ? 'attention' : 'ok';
  return {
    kicker: t('company.kind.me'),
    title: t('company.me.title'),
    state,
    now:
      state === 'attention'
        ? t('company.me.waiting', {
            n: me.loop.waiting,
            high: me.highRisk,
            review: me.loop.review,
            stuck: me.loop.stuck,
          })
        : t('company.me.clear'),
    loop: me.loop,
    facts: [
      { label: t('company.fact.companies'), value: String(props.board.companies.length), mono: true },
      { label: t('company.fact.agents'), value: `${me.activeToday} / ${me.agentCount}`, mono: true },
      {
        label: t('company.fact.cost'),
        value: `${money(me.costMicro)} · ${t('company.calls', { n: me.calls })}`,
        mono: true,
      },
      ...(me.earliestDue ? [{ label: t('company.fact.earliestDue'), value: time(me.earliestDue), mono: true }] : []),
      ...(me.unattributed
        ? [{ label: t('company.fact.unattributed'), value: String(me.unattributed), mono: true }]
        : []),
      ...(me.unassigned ? [{ label: t('company.fact.unassigned'), value: String(me.unassigned), mono: true }] : []),
    ],
    actions: [
      { label: t(me.loop.waiting > 0 ? 'company.goDecide' : 'company.goWorkbench'), to: '/workbench' },
      { label: t('company.goTasks'), to: '/tasks' },
    ],
    waiting: me.waitingItems,
    work: me.workItems,
    signals: props.signals.map((a) => ({ id: a.id, level: a.level, text: anomalyText(a), to: a.drill?.path ?? '' })),
    childrenTitle: t('company.sec.companies', { n: props.board.companies.length }),
    children: props.board.companies.map((co) => ({
      id: `co:${co.id}`,
      name: co.name,
      state: co.state,
      value: `${co.activeToday}/${co.agentCount}`,
    })),
  };
}

const detail = computed<Detail>(() => {
  const [kind, coId, deptId, agentId] = props.selected.split(':');
  const co = props.board.companies.find((c) => c.id === coId);
  const dept = co?.depts.find((d) => d.id === deptId);
  if (kind === 'co' && co) return companyDetail(co);
  if (kind === 'dept' && co && dept) return deptDetail(co, dept);
  if (kind === 'agent' && co) {
    const agent = dept ? dept.agents.find((a) => a.id === agentId) : co.ceo?.id === agentId ? co.ceo : undefined;
    if (agent) return agentDetail(co, dept, agent);
  }
  return meDetail();
});

// 打开或换节点时把焦点放到标题上，读屏和键盘都从这里接着走
const focusHead = () => void nextTick(() => headEl.value?.focus());
onMounted(focusHead);
watch(() => props.selected, focusHead);
</script>
