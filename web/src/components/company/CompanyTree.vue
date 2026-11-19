<template>
  <!--
    公司树（照设计稿的画法）：我 → 总经理 Agent → 部门 → 岗位，直接画在页面上。
    连线按卡片的实际位置计算：总经理到各部门走直角圆弧，外侧的线先拐弯、内侧的后拐，所以互不交叉。
  -->
  <section ref="treeEl" class="co-tree" :aria-label="t('company.treeLabel')">
    <svg class="co-wires" aria-hidden="true">
      <template v-for="(wire, i) in wires" :key="wire.key">
        <path :d="wire.d" :style="{ stroke: wire.color }" />
        <circle v-if="!calm" r="2.4" :style="{ fill: wire.color }">
          <animateMotion
            :dur="`${3.2 + (i % 3) * 0.5}s`"
            :begin="`-${i * 0.7}s`"
            repeatCount="indefinite"
            :path="wire.d"
          />
        </circle>
      </template>
    </svg>

    <button type="button" class="co-node co-me" :aria-pressed="selected === 'me'" @click="emit('select', 'me')">
      <span class="co-me__av" aria-hidden="true">{{ t('company.me.title') }}</span>
      <span class="co-node__main">
        <span class="co-node__name">{{ t('company.me.title') }}</span>
        <span class="co-node__sub">{{ t('company.me.sub') }}</span>
      </span>
      <span v-if="tree.me.waiting > 0" class="co-me__badge co-mono">
        {{ t('company.me.chip', { n: tree.me.waiting }) }}
      </span>
    </button>

    <div class="co-lens co-mono">
      <div class="co-lab co-lab--down">
        <b>↓ {{ t('company.flow.decide') }}</b>
        <span>{{ meLine }}</span>
      </div>
      <div class="co-lab co-lab--up">
        <b>↑ {{ t('company.flow.report') }}</b>
        <span>{{ ceoLine }}</span>
      </div>
    </div>

    <button
      type="button"
      class="co-node co-ceo"
      :class="{ 'co-node--unset': !tree.ceo }"
      :aria-pressed="selected === 'ceo'"
      @click="emit('select', 'ceo')"
    >
      <span class="co-radar" :class="{ 'co-radar--on': tree.ceo?.ranToday }" aria-hidden="true" />
      <span class="co-node__main">
        <span class="co-node__name">{{ tree.ceo?.name ?? t('company.ceo.title') }}</span>
        <span class="co-node__sub">
          <template v-if="tree.ceo">
            {{ t('company.ceo.role') }} ·
            <span class="co-mono">{{ shortTime(tree.ceo.lastRunAt, now) || t('company.neverRan') }}</span>
          </template>
          <template v-else>{{ t(tree.ceoConfigured ? 'company.ceo.missing' : 'company.ceo.unset') }}</template>
        </span>
      </span>
      <span v-if="tree.ceo" class="co-state" :class="`co-state--${ceoState}`">{{
        t(`company.state.${ceoState}`)
      }}</span>
    </button>

    <p v-if="tree.depts.length === 0" class="co-note">{{ t('company.noDepts') }}</p>
    <template v-else>
      <div class="co-fan co-cols co-mono" :style="{ '--co-cols': tree.depts.length }">
        <div v-for="dept in tree.depts" :key="dept.id" class="co-lab">
          <b>↓ {{ t('company.edge.delegated', { n: dept.delegated }) }}</b>
          <span :class="{ 'co-lab__hot': dept.waiting > 0 }"
            >↑
            {{ dept.waiting > 0 ? t('company.edge.waiting', { n: dept.waiting }) : t('company.edge.noWaiting') }}</span
          >
        </div>
      </div>
      <div class="co-depts co-cols" :style="{ '--co-cols': tree.depts.length }">
        <div v-for="(dept, index) in tree.depts" :key="dept.id" class="co-dept" :style="{ '--co-c': lineColor(index) }">
          <button
            type="button"
            class="co-node co-node--dept"
            :aria-pressed="selected === `dept:${dept.id}`"
            @click="emit('select', `dept:${dept.id}`)"
          >
            <span class="co-node__row">
              <span class="co-dept__mark" aria-hidden="true" />
              <span class="co-node__name">{{ dept.name }}</span>
              <span class="co-state" :class="`co-state--${dept.state}`">{{ t(`company.state.${dept.state}`) }}</span>
            </span>
            <span class="co-node__row co-node__row--figure">
              <span class="co-figure co-mono"
                >{{ dept.activeToday }}<small>/{{ dept.agentCount }}</small></span
              >
              <span class="co-node__sub">{{ t('company.dept.activeLabel') }}</span>
            </span>
            <span class="co-node__sub co-mono">
              {{
                t('company.dept.rhythm', {
                  last: shortTime(dept.lastRunAt, now) || '—',
                  next: shortTime(dept.nextRunAt, now) || '—',
                })
              }}
            </span>
          </button>

          <ul class="co-leaves">
            <li v-for="pos in visiblePositions(dept)" :key="pos.id">
              <button
                type="button"
                class="co-leaf"
                :class="{ 'co-leaf--hot': pos.state === 'attention', 'co-leaf--empty': pos.state === 'empty' }"
                :aria-pressed="selected === `pos:${dept.id}:${pos.id}`"
                @click="emit('select', `pos:${dept.id}:${pos.id}`)"
              >
                <span class="co-leaf__name">{{ pos.name }}</span>
                <span class="co-leaf__val co-mono">{{ leafValue(pos) }}</span>
              </button>
            </li>
            <li v-if="dept.positions.length > LEAF_LIMIT">
              <button type="button" class="co-link" :aria-expanded="expanded.has(dept.id)" @click="toggle(dept.id)">
                {{
                  expanded.has(dept.id)
                    ? t('company.dept.collapse')
                    : t('company.dept.more', { n: dept.positions.length - LEAF_LIMIT })
                }}
              </button>
            </li>
            <li v-if="dept.positions.length === 0" class="co-node__sub">{{ t('company.dept.noPositions') }}</li>
          </ul>
        </div>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { shortTime, type CompanyTree, type DeptNode, type PositionLeaf } from '../../features/company/tree';

// 每个部门默认露出几个岗位；3 个时整棵树在 900 高的屏幕上不用滚
const LEAF_LIMIT = 3;
const LINE_COLORS = 6;

const props = defineProps<{
  tree: CompanyTree;
  now: Date;
  /** me / ceo / dept:<id> / pos:<deptId>:<posId> */
  selected: string;
}>();
const emit = defineEmits<{ select: [id: string] }>();

const { t } = useI18n();
const expanded = ref<Set<string>>(new Set());

function toggle(deptId: string) {
  const next = new Set(expanded.value);
  if (next.has(deptId)) next.delete(deptId);
  else next.add(deptId);
  expanded.value = next;
}

function visiblePositions(dept: DeptNode): PositionLeaf[] {
  return expanded.value.has(dept.id) ? dept.positions : dept.positions.slice(0, LEAF_LIMIT);
}

function leafValue(pos: PositionLeaf): string {
  if (pos.failed > 0) return t('company.leaf.failed', { n: pos.failed });
  if (pos.waiting > 0) return t('company.leaf.waiting', { n: pos.waiting });
  if (pos.agents.length === 0) return t('company.leaf.empty');
  return t('company.leaf.agents', { n: pos.agents.length });
}

/** 部门识别色：只用于连线、色点和岗位竖线，状态仍然用文字标签表达。 */
function lineColor(index: number): string {
  return `var(--nt-line-${(index % LINE_COLORS) + 1})`;
}

const ceoState = computed(() => (props.tree.ceo?.failed ? 'attention' : props.tree.ceo?.ranToday ? 'ok' : 'idle'));
// 「我」和总经理之间两侧的说明：左边是等我拍板的事，右边是总经理最近一次运行
const meLine = computed(() => {
  const { waiting, earliestDue } = props.tree.me;
  if (waiting === 0) return t('company.lens.clear');
  const due = shortTime(earliestDue, props.now);
  return due ? t('company.lens.waitingDue', { n: waiting, time: due }) : t('company.me.chip', { n: waiting });
});
const ceoLine = computed(() => {
  if (!props.tree.ceo) return t('company.lens.ceoUnset');
  const last = shortTime(props.tree.ceo.lastRunAt, props.now);
  return last ? t('company.lens.ceoLast', { time: last }) : t('company.neverRan');
});

// ——— 连线 ———
type Wire = { key: string; d: string; color: string };
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
    return {
      cx: (r.left + r.right) / 2 - box.left + el.scrollLeft,
      t: r.top - box.top + el.scrollTop,
      b: r.bottom - box.top + el.scrollTop,
    };
  };
  const me = at(el.querySelector('.co-me'));
  const ceo = at(el.querySelector('.co-ceo'));
  if (!me || !ceo) return;
  const out: Wire[] = [];
  // 我 ↔ 总经理：左边往下是拍板，右边往上是汇报
  const x = me.cx;
  const m = ceo.t - me.b;
  out.push({
    key: 'down',
    d: `M${x - 10},${me.b} C${x - 40},${me.b + m * 0.3} ${x - 40},${me.b + m * 0.7} ${x - 10},${ceo.t}`,
    color: 'var(--nt-warn)',
  });
  out.push({
    key: 'up',
    d: `M${x + 10},${ceo.t} C${x + 40},${me.b + m * 0.7} ${x + 40},${me.b + m * 0.3} ${x + 10},${me.b}`,
    color: 'var(--nt-run)',
  });
  // 总经理 → 各部门：每条线先垂直下到自己那一层，再横向走到部门正上方，最后垂直落到卡片
  const labels = el.querySelectorAll('.co-fan .co-lab');
  const cards = el.querySelectorAll('.co-node--dept');
  const count = cards.length;
  const mid = (count - 1) / 2;
  const levels = Math.ceil(count / 2);
  const gap = Math.min(7, 200 / Math.max(count, 1));
  for (let i = 0; i < count; i += 1) {
    const lab = at(labels[i]);
    const card = at(cards[i]);
    if (!lab || !card) continue;
    const sx = ceo.cx + (i - mid) * gap;
    const dir = Math.sign(lab.cx - sx) || 1;
    // k = 0 是最外侧的线，层最高（最先拐弯）
    const k = Math.floor(mid - Math.abs(i - mid));
    const level = ceo.b + (lab.t - ceo.b) * (0.24 + (levels > 1 ? (0.5 * k) / (levels - 1) : 0));
    const r = Math.max(0, Math.min(12, Math.abs(lab.cx - sx) / 2, level - ceo.b, lab.t - level));
    out.push({
      key: `dept-${i}`,
      d: `M${sx},${ceo.b} L${sx},${level - r} Q${sx},${level} ${sx + dir * r},${level} L${lab.cx - dir * r},${level} Q${lab.cx},${level} ${lab.cx},${level + r} L${lab.cx},${card.t}`,
      color: lineColor(i),
    });
  }
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
  () => props.tree,
  () => void nextTick(draw),
);
</script>
