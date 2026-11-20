<template>
  <!--
    看板总览：一张图管全部公司。我 → 各公司（总经理 Agent 并在公司卡里）→ 部门 → Agent。
    我到各公司的连线按卡片实际位置计算（直角圆弧、外侧先拐，互不交叉）；公司以下用竖线串起来。
    公司可收起部门，部门可展开 Agent；点任何节点在右侧看详情。
  -->
  <section ref="treeEl" class="kb-tree" :aria-label="t('company.treeLabel')">
    <svg class="kb-wires" aria-hidden="true">
      <template v-for="wire in wires" :key="wire.key">
        <path :d="wire.d" :style="{ stroke: wire.color }" />
        <circle v-for="dot in wire.dots" :key="dot.key" r="2.6" :style="{ fill: dot.color }">
          <animateMotion
            :dur="`${dot.dur}s`"
            :begin="`-${dot.delay}s`"
            repeatCount="indefinite"
            :keyPoints="dot.up ? '1;0' : '0;1'"
            keyTimes="0;1"
            calcMode="linear"
            :path="wire.d"
          />
        </circle>
      </template>
    </svg>

    <button type="button" class="kb-node kb-me" :aria-pressed="selected === 'me'" @click="emit('select', 'me')">
      <span class="kb-me__av" aria-hidden="true">{{ t('company.me.title') }}</span>
      <span class="kb-node__main">
        <span class="kb-node__name">{{ t('company.me.title') }}</span>
        <span class="kb-node__sub">
          {{ t('company.me.sub', { companies: board.companies.length, agents: board.me.agentCount }) }}
        </span>
      </span>
      <BoardLoop :loop="board.me.loop" />
      <span class="kb-me__cost kb-mono">
        <b>{{ money(board.me.costMicro) }}</b>
        <small>{{ t('company.costToday') }}</small>
      </span>
    </button>

    <p v-if="board.companies.length === 0" class="kb-note">
      {{ t('company.noCompany') }}
      <router-link class="kb-link" to="/settings/organization">{{ t('company.goOrg') }}</router-link>
    </p>

    <div v-else class="kb-cols" :style="{ '--kb-cols': board.companies.length }">
      <div
        v-for="(co, ci) in board.companies"
        :key="co.id"
        class="kb-col"
        :style="{ '--kb-c': lineColor(ci) }"
        :data-company="co.id"
      >
        <div class="kb-lab kb-mono">
          <span>↓ {{ t('company.edge.work', { n: inFlight(co.loop) }) }}</span>
          <span :class="{ 'kb-lab__hot': co.loop.waiting > 0 }">
            ↑
            {{ co.loop.waiting > 0 ? t('company.edge.waiting', { n: co.loop.waiting }) : t('company.edge.noWaiting') }}
          </span>
        </div>

        <article class="kb-co" :class="[`kb-co--${co.state}`, { 'kb-co--folded': folded.has(co.id) }]">
          <button
            type="button"
            class="kb-node kb-co__body"
            :aria-pressed="selected === `co:${co.id}`"
            @click="emit('select', `co:${co.id}`)"
          >
            <span class="kb-row">
              <span class="kb-co__mark" aria-hidden="true" />
              <span class="kb-node__name" :title="co.name">{{ co.name }}</span>
              <span class="kb-state" :class="`kb-state--${co.state}`">{{ t(`company.state.${co.state}`) }}</span>
            </span>
            <span class="kb-row kb-ceo" :class="{ 'kb-ceo--unset': !co.ceo }">
              <span class="kb-radar" :class="{ 'kb-radar--on': radarOn(co) }" aria-hidden="true" />
              <template v-if="co.ceo">
                <span class="kb-node__sub kb-ceo__name" :title="co.ceo.name">{{ co.ceo.name }}</span>
                <span class="kb-node__sub kb-mono kb-ceo__time" :title="ceoLine(co)">{{ ceoLine(co) }}</span>
              </template>
              <span v-else class="kb-node__sub">{{
                t(co.ceoConfigured ? 'company.ceo.missing' : 'company.ceo.unset')
              }}</span>
            </span>
            <span class="kb-row kb-row--figure">
              <span class="kb-figure kb-mono"
                >{{ co.activeToday }}<small>/{{ co.agentCount }}</small></span
              >
              <span class="kb-node__sub">{{ t('company.activeLabel') }}</span>
              <span class="kb-co__cost kb-mono">
                {{ money(co.costMicro) }} · {{ t('company.calls', { n: co.calls }) }}
              </span>
            </span>
            <BoardLoop :loop="co.loop" small />
          </button>
          <button
            type="button"
            class="kb-fold"
            :aria-expanded="!folded.has(co.id)"
            :aria-label="t(folded.has(co.id) ? 'company.unfoldCompany' : 'company.foldCompany', { name: co.name })"
            :title="t(folded.has(co.id) ? 'company.unfoldCompany' : 'company.foldCompany', { name: co.name })"
            @click="emit('toggleCompany', co.id)"
          >
            <q-icon :name="folded.has(co.id) ? 'unfold_more' : 'unfold_less'" size="18px" />
          </button>
        </article>

        <button
          v-if="folded.has(co.id)"
          type="button"
          class="kb-folded"
          :aria-label="t('company.unfoldCompany', { name: co.name })"
          @click="emit('toggleCompany', co.id)"
        >
          {{
            t('company.foldedNote', { n: co.depts.length, hot: co.depts.filter((d) => d.state === 'attention').length })
          }}
        </button>
        <ul v-else class="kb-spine">
          <li v-if="co.depts.length === 0" class="kb-branch kb-empty">{{ t('company.noDepts') }}</li>
          <li v-for="dept in co.depts" :key="dept.id" class="kb-branch">
            <div class="kb-dept" :class="`kb-dept--${dept.state}`">
              <button
                type="button"
                class="kb-dept__body"
                :aria-pressed="selected === `dept:${co.id}:${dept.id}`"
                @click="emit('select', `dept:${co.id}:${dept.id}`)"
              >
                <span class="kb-dot" :class="`kb-dot--${dept.state}`" aria-hidden="true" />
                <span class="kb-dept__name">{{ dept.name }}</span>
                <span class="kb-sr">{{ t(`company.state.${dept.state}`) }}</span>
                <span v-for="badge in badges(dept)" :key="badge.key" class="kb-badge" :class="`kb-badge--${badge.key}`">
                  {{ t(`company.badge.${badge.key}`, { n: badge.n }) }}
                </span>
                <span class="kb-dept__fig kb-mono" :title="t('company.activeLabel')">
                  {{ dept.agentCount ? `${dept.activeToday}/${dept.agentCount}` : t('company.leaf.empty') }}
                </span>
              </button>
              <button
                v-if="dept.agents.length"
                type="button"
                class="kb-fold kb-fold--sm"
                :aria-expanded="opened.has(dept.id)"
                :aria-label="t(opened.has(dept.id) ? 'company.foldDept' : 'company.unfoldDept', { name: dept.name })"
                @click="emit('toggleDept', dept.id)"
              >
                <q-icon :name="opened.has(dept.id) ? 'expand_less' : 'expand_more'" size="18px" />
              </button>
            </div>
            <ul v-if="opened.has(dept.id)" class="kb-spine kb-spine--agents">
              <li v-for="agent in dept.agents" :key="agent.id" class="kb-branch">
                <button
                  type="button"
                  class="kb-agent"
                  :class="`kb-agent--${agent.state}`"
                  :aria-pressed="selected === `agent:${co.id}:${dept.id}:${agent.id}`"
                  @click="emit('select', `agent:${co.id}:${dept.id}:${agent.id}`)"
                >
                  <span class="kb-dot" :class="`kb-dot--${agent.state}`" aria-hidden="true" />
                  <span class="kb-agent__main">
                    <span class="kb-agent__name" :title="agent.name">{{ agent.name }}</span>
                    <!-- 多数 Agent 以岗位命名，岗位名与名字相同时不重复写 -->
                    <span v-if="agent.id === dept.lead?.id" class="kb-agent__pos">{{ t('company.deptLead') }}</span>
                    <span v-else-if="agent.position && agent.position !== agent.name" class="kb-agent__pos">{{
                      agent.position
                    }}</span>
                  </span>
                  <span class="kb-agent__val kb-mono" :class="{ 'kb-agent__val--hot': agent.state === 'attention' }">
                    {{ agentValue(agent) }}
                  </span>
                </button>
              </li>
            </ul>
          </li>
        </ul>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import BoardLoop from './BoardLoop.vue';
import {
  money,
  shortTime,
  type Board,
  type BoardAgent,
  type CompanyNode,
  type DeptNode,
  type Loop,
} from '../../features/company/tree';

const LINE_COLORS = 6;

const props = defineProps<{
  board: Board;
  now: Date;
  /** '' / me / co:<id> / dept:<coId>:<deptId> / agent:<coId>:<deptId>:<agentId> */
  selected: string;
  /** 收起了部门的公司 id */
  folded: ReadonlySet<string>;
  /** 展开了 Agent 的部门 id */
  opened: ReadonlySet<string>;
}>();
const emit = defineEmits<{ select: [id: string]; toggleCompany: [id: string]; toggleDept: [id: string] }>();

const { t } = useI18n();

/** 公司识别色：只用于连线、竖线和色点，状态仍然用文字表达。 */
function lineColor(index: number): string {
  return `var(--nt-line-${(index % LINE_COLORS) + 1})`;
}

const inFlight = (loop: Loop) => loop.running + loop.stuck + loop.review;
const lastRun = (iso: string) => shortTime(iso, props.now) || t('company.neverRan');

// 总经理巡视过才转雷达：两小时内巡视过（每小时一次，留一次容错）；没有巡视数据时退回「今天跑过」
const RADAR_WINDOW_MS = 2 * 60 * 60 * 1000;
function radarOn(co: CompanyNode): boolean {
  if (!co.gm) return Boolean(co.ceo?.ranToday);
  const at = Date.parse(co.gm.lastPatrolAt);
  return co.gm.enabled && Number.isFinite(at) && props.now.getTime() - at < RADAR_WINDOW_MS;
}

/** 总经理一行：巡视 11:10 · 汇报 12:20 → 20:20；没有巡视数据时写上次运行时间。 */
function ceoLine(co: CompanyNode): string {
  const gm = co.gm;
  if (!gm) return lastRun(co.ceo?.lastRunAt ?? '');
  if (!gm.enabled) return t('company.gm.off');
  const short = (iso: string) => shortTime(iso, props.now);
  // 还没汇报过时只有下次，也带箭头，免得把「下次 23:20」读成「上次 23:20」
  const last = short(gm.lastReport?.at ?? '');
  const next = short(gm.nextReportAt);
  const report = last && next ? `${last} → ${next}` : last || (next ? `→ ${next}` : '');
  const patrol = t('company.gm.patrol', { at: short(gm.lastPatrolAt) || t('company.gm.notYet') });
  return report ? `${patrol} · ${t('company.gm.report', { at: report })}` : patrol;
}

function badges(dept: DeptNode): { key: string; n: number }[] {
  const list = [
    { key: 'waiting', n: dept.loop.waiting },
    { key: 'review', n: dept.loop.review },
    { key: 'stuck', n: dept.loop.stuck },
    { key: 'failed', n: dept.failed },
    { key: 'running', n: dept.loop.running },
  ];
  return list.filter((b) => b.n > 0);
}

function agentValue(agent: BoardAgent): string {
  if (agent.loop.waiting > 0) return t('company.badge.waiting', { n: agent.loop.waiting });
  if (agent.loop.stuck > 0) return t('company.badge.stuck', { n: agent.loop.stuck });
  if (agent.failed) return t('company.agentFailed');
  if (agent.loop.review > 0) return t('company.badge.review', { n: agent.loop.review });
  if (agent.loop.running > 0) return t('company.badge.running', { n: agent.loop.running });
  if (agent.calls > 0) return t('company.calls', { n: agent.calls });
  return shortTime(agent.lastRunAt, props.now) || t('company.neverRan');
}

// ——— 我 → 各公司的连线 ———
type Dot = { key: string; color: string; up: boolean; dur: number; delay: number };
type Wire = { key: string; d: string; color: string; dots: Dot[] };
const treeEl = ref<HTMLElement | null>(null);
const wires = ref<Wire[]>([]);
// 系统设置了「减少动效」时不画沿线移动的小圆点
const calm = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;

function draw() {
  const el = treeEl.value;
  if (!el || !el.offsetParent) return;
  const box = el.getBoundingClientRect();
  const at = (node: Element | null | undefined) => {
    if (!node) return null;
    const r = node.getBoundingClientRect();
    return { cx: (r.left + r.right) / 2 - box.left, t: r.top - box.top, b: r.bottom - box.top };
  };
  const me = at(el.querySelector('.kb-me'));
  if (!me) return;
  const cols = Array.from(el.querySelectorAll<HTMLElement>('.kb-col'));
  const count = cols.length;
  const mid = (count - 1) / 2;
  const levels = Math.ceil(count / 2);
  const gap = Math.min(14, 280 / Math.max(count, 1));
  const out: Wire[] = [];
  cols.forEach((col, i) => {
    const lab = at(col.querySelector('.kb-lab'));
    const card = at(col.querySelector('.kb-co'));
    const co = props.board.companies[i];
    if (!lab || !card || !co) return;
    const color = lineColor(i);
    const sx = me.cx + (i - mid) * gap;
    const dir = Math.sign(lab.cx - sx) || 1;
    // k = 0 是最外侧的线，层最高（最先拐弯），所以线与线不交叉
    const k = Math.floor(mid - Math.abs(i - mid));
    const span = lab.t - me.b;
    const level = me.b + span * (0.28 + (levels > 1 ? (0.44 * k) / (levels - 1) : 0));
    const r = Math.max(0, Math.min(14, Math.abs(lab.cx - sx) / 2, level - me.b, lab.t - level));
    const d =
      Math.abs(lab.cx - sx) < 1
        ? `M${sx},${me.b} L${lab.cx},${lab.t}`
        : `M${sx},${me.b} L${sx},${level - r} Q${sx},${level} ${sx + dir * r},${level} L${lab.cx - dir * r},${level} Q${lab.cx},${level} ${lab.cx},${level + r} L${lab.cx},${lab.t}`;
    // 有事在转才有流动：往下是交出去的事，往上是等我拍板的事
    const dots: Dot[] = [];
    if (!calm && inFlight(co.loop) > 0) dots.push({ key: 'down', color, up: false, dur: 3.4, delay: i * 0.7 });
    if (!calm && co.loop.waiting > 0)
      dots.push({ key: 'up', color: 'var(--nt-warn)', up: true, dur: 3.8, delay: i * 0.9 });
    out.push({ key: `co-${co.id}`, d, color, dots });
    out.push({ key: `lab-${co.id}`, d: `M${lab.cx},${lab.b} L${lab.cx},${card.t}`, color, dots: [] });
  });
  wires.value = out;
}

let observer: ResizeObserver | null = null;
onMounted(() => {
  void nextTick(draw);
  if (typeof ResizeObserver !== 'undefined' && treeEl.value) {
    observer = new ResizeObserver(draw);
    observer.observe(treeEl.value);
  }
  // 字体加载完后卡片宽度会变，再画一次
  void document.fonts?.ready.then(draw);
});
onBeforeUnmount(() => observer?.disconnect());
watch(
  () => [props.board, props.folded, props.opened],
  () => void nextTick(draw),
);
</script>
