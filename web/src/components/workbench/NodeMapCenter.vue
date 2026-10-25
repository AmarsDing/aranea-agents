<template>
  <div ref="root" class="wb-nmc">
    <svg
      class="wb-nmc__corridor"
      :viewBox="`0 0 ${size.w} ${size.h}`"
      :width="size.w"
      :height="size.h"
      aria-hidden="true"
    >
      <g v-for="c in corridors" :key="c.id">
        <path
          v-for="(seg, si) in c.paths"
          :key="si"
          :d="seg"
          class="wb-nmc__hair"
          :class="{ active: c.active, await: c.awaiting }"
        />
        <template v-if="c.active && !reducedMotion">
          <circle
            v-for="(seg, si) in c.paths"
            :key="`d${si}`"
            r="2"
            class="wb-nmc__carrier"
            :class="{ gold: c.awaiting }"
          >
            <animateMotion dur="0.26s" repeatCount="indefinite" :path="seg" />
          </circle>
        </template>
        <text v-if="c.merged > 1" :x="c.label[0]" :y="c.label[1]" class="wb-nmc__merge">×{{ c.merged }}</text>
      </g>
    </svg>

    <div class="wb-nmc__sources">
      <div class="wb-nmc__col-label">{{ t('workbench.map.source') }}</div>
      <div
        v-for="line in sources"
        :key="line.id"
        class="wb-nmc__source"
        :class="{ dim: !line.connected }"
        :data-source-port="line.id"
      >
        <i class="wb-nmc__src-dot" :class="{ on: line.connected }" />
        <span class="wb-nmc__src-name">{{ t(`workbench.lines.${line.id}`) }}</span>
        <em class="wb-nmc__src-note">{{
          line.connected ? (line.pending > 0 ? String(line.pending) : '·') : t('workbench.sources.pending')
        }}</em>
      </div>
    </div>

    <div class="wb-nmc__bus" @scroll.passive="scheduleMeasure">
      <div class="wb-nmc__col-label">{{ t('workbench.map.dotcore') }}</div>
      <div
        v-for="dept in roster.depts"
        :key="dept.id"
        class="wb-nmc__bus-entry"
        :class="{ on: deptLive(dept.id), gold: deptAwait(dept.id) }"
        :data-bus-port="dept.id"
      >
        <i class="wb-nmc__bus-pip" />
        <span class="wb-nmc__bus-name">{{ dept.name }}</span>
        <em class="wb-nmc__bus-count">{{ deptAgentCount(dept.id) }}</em>
      </div>
      <div v-if="roster.unassigned.length" class="wb-nmc__bus-entry" :data-bus-port="MISC_BUS">
        <i class="wb-nmc__bus-pip" />
        <span class="wb-nmc__bus-name">{{ t('workbench.roster.unassigned') }}</span>
        <em class="wb-nmc__bus-count">{{ roster.unassigned.length }}</em>
      </div>
    </div>

    <div ref="rowsPane" class="wb-nmc__rows" @scroll.passive="scheduleMeasure">
      <div class="wb-nmc__col-label">{{ t('workbench.roster.rowsLabel') }}</div>
      <RoleRow
        v-for="a in agents"
        :key="a.key"
        :agent="a"
        :live="live.get(a.key)"
        :active-now="isActive(a.key)"
        :waiting="isWaiting(a.key)"
        @drill="(k, card) => $emit('drill', k, card)"
      />
      <div v-if="!agents.length" class="wb-nmc__empty">
        <p>{{ t('workbench.roster.empty') }}</p>
        <q-btn
          flat
          dense
          no-caps
          color="primary"
          :label="t('workbench.roster.configure')"
          @click="$emit('open-organization')"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * 主角：DOTS Node Map 中枢 · 黄金走廊（设计 §2.5.1）。
 * 三列同构 SOURCE 权限卡 → DOTCORE 花名册总线 → 每专项一行四卡；
 * 走廊 = 列间空白带的 1px 虚线贝塞尔：静态发丝灰桥；真实事件点亮跑 220–300ms 载波；
 * 等批专项走廊转金（仪式形态 C）；同屏汇聚 >8 条按业务线合一为 ×N。
 * 诚实口径：只画 源→总线 与 总线→专项 两段真实结构；无 join key 的归因连线不画。
 */
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { CORRIDOR_ACTIVE_WINDOW_MS, CORRIDOR_MERGE_THRESHOLD } from '../../features/workbench/logic';
import type { AgentLiveState, BizLineCard, BizLineId, RosterAgent, RosterModel } from '../../features/workbench/types';
import RoleRow from './RoleRow.vue';

const MISC_BUS = '__misc';

type Anchor = { x: number; y: number };

type CorridorItem = {
  id: string;
  paths: string[];
  active: boolean;
  awaiting: boolean;
  merged: number;
  label: [number, number];
};

const props = defineProps<{
  roster: RosterModel;
  agents: RosterAgent[];
  live: Map<string, AgentLiveState>;
  waiting: Set<string>;
  sources: BizLineCard[];
  /** 钻取抽屉打开→布局回落过场；过场后重测沟槽锚点 */
  retreat: boolean;
}>();

defineEmits<{
  drill: [agentKey: string, card: 'dot' | 'job' | 'rule' | 'activity'];
  'open-organization': [];
}>();

const { t } = useI18n();

const root = ref<HTMLElement | null>(null);
const rowsPane = ref<HTMLElement | null>(null);
const size = ref({ w: 10, h: 10 });
const busAnchors = ref(new Map<string, { out: Anchor; in: Anchor }>());
const rowAnchors = ref(new Map<string, Anchor>());
const srcAnchors = ref(new Map<string, Anchor>());
const nowTick = ref(Date.now());
/** prefers-reduced-motion：跑电载波与 kpi 辉光同受事件白名单约束 */
const reducedMotion = ref(false);

let resizeObserver: ResizeObserver | null = null;
let tickTimer: ReturnType<typeof setInterval> | null = null;
let rafId = 0;
let motionQuery: MediaQueryList | null = null;
const onMotionChange = (e: MediaQueryListEvent): void => {
  reducedMotion.value = e.matches;
};

function scheduleMeasure(): void {
  if (rafId) return;
  rafId = requestAnimationFrame(() => {
    rafId = 0;
    measure();
  });
}

function measure(): void {
  const el = root.value;
  if (!el) return;
  const rect = el.getBoundingClientRect();
  size.value = { w: Math.max(10, rect.width), h: Math.max(10, rect.height) };
  // 边到边锚点（设计 §2.5.1：走廊画在列间沟槽、连线永不压字）：
  // 源出口=源卡右缘中、总线出口=总线条右缘中、总线入口=左缘中、行入口=行左缘中
  const edgeRight = (target: Element): Anchor => {
    const r = target.getBoundingClientRect();
    return { x: r.right - rect.left, y: r.top - rect.top + r.height / 2 };
  };
  const edgeLeft = (target: Element): Anchor => {
    const r = target.getBoundingClientRect();
    return { x: r.left - rect.left, y: r.top - rect.top + r.height / 2 };
  };
  const bus = new Map<string, { out: Anchor; in: Anchor }>();
  el.querySelectorAll('[data-bus-port]').forEach((node) => {
    const key = node.getAttribute('data-bus-port');
    if (key) bus.set(key, { out: edgeRight(node), in: edgeLeft(node) });
  });
  const rows = new Map<string, Anchor>();
  el.querySelectorAll('[data-agent-row]').forEach((node) => {
    const key = node.getAttribute('data-agent-row');
    if (key) rows.set(key, edgeLeft(node));
  });
  const src = new Map<string, Anchor>();
  el.querySelectorAll('[data-source-port]').forEach((node) => {
    const key = node.getAttribute('data-source-port');
    if (key) src.set(key, edgeRight(node));
  });
  busAnchors.value = bus;
  rowAnchors.value = rows;
  srcAnchors.value = src;
}

const agentByKey = computed(() => new Map(props.agents.map((a) => [a.key, a])));

const activeKeys = computed(() => {
  const now = nowTick.value;
  const keys = new Set<string>();
  for (const [key, st] of props.live) {
    if (!st.lastEventAt) continue;
    const at = Date.parse(st.lastEventAt);
    if (!Number.isNaN(at) && now - at < CORRIDOR_ACTIVE_WINDOW_MS) keys.add(key);
  }
  return keys;
});

function isActive(key: string): boolean {
  return activeKeys.value.has(key);
}

function isWaiting(key: string): boolean {
  return props.waiting.has(key);
}

function busKeyOf(a: RosterAgent): string {
  return a.deptId || MISC_BUS;
}

function deptAgentCount(deptId: string): number {
  return props.agents.filter((a) => a.deptId === deptId).length;
}

function deptLive(deptId: string): boolean {
  return props.agents.some((a) => a.deptId === deptId && isActive(a.key));
}

function deptAwait(deptId: string): boolean {
  return props.agents.some((a) => a.deptId === deptId && isWaiting(a.key));
}

/** 贝塞尔走廊段：from（右侧出口）→ to（左侧入口），弯曲被约束在列间空白带 */
function bezier(from: Anchor, to: Anchor): string {
  const bend = Math.max(28, Math.abs(to.x - from.x) * 0.45);
  return `M ${from.x} ${from.y} C ${from.x + bend} ${from.y}, ${to.x - bend} ${to.y}, ${to.x} ${to.y}`;
}

const corridors = computed<CorridorItem[]>(() => {
  const bus = busAnchors.value;
  const rows = rowAnchors.value;
  const src = srcAnchors.value;
  const items: CorridorItem[] = [];

  // 段一：源→总线（join key = 部门业务线↔业务线卡；线级无精确归因，点亮按部门级事件聚合，不上金）
  for (const dept of props.roster.depts) {
    if (!dept.bizLine) continue;
    const from = src.get(dept.bizLine);
    const to = bus.get(dept.id)?.in;
    if (!from || !to) continue;
    items.push({
      id: `src:${dept.id}`,
      paths: [bezier(from, to)],
      active: deptLive(dept.id),
      awaiting: false,
      merged: 1,
      label: [(from.x + to.x) / 2, to.y - 8],
    });
  }

  // 段二：总线→专项
  if (!bus.size || !rows.size) return items;

  const activeRowKeys: string[] = [];

  for (const a of props.agents) {
    const from = bus.get(busKeyOf(a))?.out ?? bus.get(MISC_BUS)?.out;
    const to = rows.get(a.key);
    if (!from || !to) continue;
    const active = activeKeys.value.has(a.key);
    const awaiting = props.waiting.has(a.key);
    if (active) activeRowKeys.push(a.key);
    items.push({
      id: a.key,
      paths: [bezier(from, to)],
      active,
      awaiting,
      merged: 1,
      label: [(from.x + to.x) / 2, to.y - 8],
    });
  }

  // 同屏汇聚 >8 条 active → 按业务线合一为 ×N 载波（诚实合流口径）
  if (activeRowKeys.length > CORRIDOR_MERGE_THRESHOLD) {
    const byLine = new Map<string, CorridorItem[]>();
    for (const key of activeRowKeys) {
      const line: BizLineId | '' = agentByKey.value.get(key)?.bizLine ?? '';
      const list = byLine.get(line) ?? [];
      const item = items.find((c) => c.id === key);
      if (item) list.push(item);
      byLine.set(line, list);
    }
    for (const [line, members] of byLine) {
      if (members.length <= 1) continue;
      const head = members[0];
      const midY = members.reduce((acc, m) => acc + m.label[1] + 8, 0) / members.length;
      const mergedItem: CorridorItem = {
        id: `line:${line || 'misc'}`,
        paths: members.flatMap((m) => m.paths),
        active: true,
        awaiting: members.some((m) => m.awaiting),
        merged: members.length,
        label: [head.label[0], midY + 8],
      };
      for (const m of members) {
        m.active = false; // 成员并入合流，单条不再跑载波
      }
      items.push(mergedItem);
    }
  }
  return items;
});

watch(
  () => [props.agents.length, props.roster.depts.length, props.sources.length],
  () => nextTick(scheduleMeasure),
);

watch(
  () => props.retreat,
  () => {
    // 钻取开合伴随布局回落过场（约 320ms），过场后重测沟槽锚点
    window.setTimeout(scheduleMeasure, 340);
  },
);

onMounted(() => {
  measure();
  resizeObserver = new ResizeObserver(scheduleMeasure);
  if (root.value) resizeObserver.observe(root.value);
  if (rowsPane.value) resizeObserver.observe(rowsPane.value);
  window.addEventListener('resize', scheduleMeasure);
  tickTimer = setInterval(() => {
    nowTick.value = Date.now();
  }, 1000);
  motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
  reducedMotion.value = motionQuery.matches;
  motionQuery.addEventListener('change', onMotionChange);
  nextTick(scheduleMeasure);
});

onUnmounted(() => {
  resizeObserver?.disconnect();
  window.removeEventListener('resize', scheduleMeasure);
  motionQuery?.removeEventListener('change', onMotionChange);
  if (tickTimer) clearInterval(tickTimer);
  if (rafId) cancelAnimationFrame(rafId);
});
</script>

<style scoped lang="sass">
.wb-nmc
  position: relative
  display: grid
  grid-template-columns: 15% 21% minmax(0, 1fr)
  gap: 12px
  height: 100%
  min-height: 0
.wb-nmc__corridor
  position: absolute
  inset: 0
  pointer-events: none
  // 沟槽画层沉底：z-index 低于三列（连线永不压字，设计 §2.5.1）
  z-index: 0
.wb-nmc__hair
  fill: none
  stroke: var(--wb-line-hairline)
  stroke-width: 1
  stroke-dasharray: 3 4
  &.active
    stroke: color-mix(in srgb, var(--wb-flow) 55%, transparent)
    animation: wb-carrier-dash 0.9s linear infinite
  &.await
    stroke: var(--wb-await-line)
    stroke-dasharray: 3 4
    &.active
      stroke: var(--wb-await)
      animation: wb-carrier-dash 0.9s linear infinite
.wb-nmc__carrier
  fill: var(--wb-flow)
  &.gold
    fill: var(--wb-await)
.wb-nmc__merge
  font-size: 10px
  fill: var(--color-text-secondary)
  font-variant-numeric: tabular-nums
.wb-nmc__col-label
  flex: none
  padding: 2px 2px 6px
  font-size: var(--wb-micro-label)
  letter-spacing: 0.16em
  color: var(--color-text-tertiary, var(--color-text-secondary))
.wb-nmc__sources
  position: relative
  z-index: 1
  display: flex
  flex-direction: column
  gap: 6px
  min-height: 0
  overflow: hidden
.wb-nmc__source
  display: flex
  align-items: center
  gap: 6px
  padding: 8px
  border: 1px solid var(--wb-line-hairline)
  border-radius: var(--wb-radius)
  background: color-mix(in srgb, var(--color-text-primary) 3%, transparent)
  font-size: var(--wb-caption)
  &.dim
    opacity: 0.45
.wb-nmc__src-dot
  width: 6px
  height: 6px
  border-radius: 50%
  background: var(--color-text-icon-muted)
  &.on
    background: var(--wb-ok)
.wb-nmc__src-name
  flex: 1
  font-weight: 600
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis
.wb-nmc__src-note
  font-style: normal
  color: var(--color-text-secondary)
.wb-nmc__bus
  position: relative
  z-index: 1
  display: flex
  flex-direction: column
  gap: 4px
  min-height: 0
  overflow-y: auto
.wb-nmc__bus-entry
  display: flex
  align-items: center
  gap: 6px
  padding: 6px 8px
  border-radius: var(--wb-radius)
  background: color-mix(in srgb, var(--color-text-primary) 4%, transparent)
  font-size: var(--wb-caption)
.wb-nmc__bus-pip
  width: 6px
  height: 6px
  border-radius: 50%
  background: var(--color-text-icon-muted)
.wb-nmc__bus-entry.on .wb-nmc__bus-pip
  background: var(--wb-flow)
.wb-nmc__bus-entry.gold .wb-nmc__bus-pip
  background: var(--wb-await)
.wb-nmc__bus-name
  flex: 1
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis
.wb-nmc__bus-count
  font-style: normal
  color: var(--color-text-secondary)
  font-variant-numeric: tabular-nums
.wb-nmc__rows
  position: relative
  z-index: 1
  min-height: 0
  overflow-y: auto
  padding-right: 2px
.wb-nmc__empty
  padding: 16px 8px
  font-size: var(--wb-caption)
  color: var(--color-text-secondary)

@media (prefers-reduced-motion: reduce)
  .wb-nmc__hair.active
    animation: none
  .wb-nmc__hair.await.active
    animation: none
</style>
