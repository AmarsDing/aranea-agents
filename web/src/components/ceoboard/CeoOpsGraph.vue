<script setup lang="ts">
// 公司运转图（经营版）：店主 → CEO Agent → 6 个业务模块 → 真实业务实体。
// 吸收参考截图的信息架构与有向语义，但所有数量/状态来自 /v1/ceoboard/company，
// 不做任何前端造数；未接入的经营域在底部显式声明。
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { CompanySnapshot, OpsCoverage, OpsEntity, OpsMetric, OpsModule } from '../../features/ceoboard/types';

const props = defineProps<{ snap: CompanySnapshot }>();
const emit = defineEmits<{ drill: [path: string] }>();
const { t } = useI18n();

const focus = ref('');

// 领域色只表达「是哪个部门」；健康状态由点、连线与边框表达，两者不共用色板。
const DOMAIN: Record<string, number> = { insight: 1, produce: 2, review: 3, publish: 4, commerce: 5, ops: 6 };
function toneOf(m: OpsModule): string {
  return `tone-domain-${DOMAIN[m.id] ?? 6}`;
}
function flowOf(m: OpsModule): 'active' | 'idle' | 'stalled' | 'error' | 'offline' {
  if (m.health === 'red') return 'error';
  if (m.health === 'yellow') return 'stalled';
  if (m.health === 'gray') return 'offline';
  return m.working ? 'active' : 'idle';
}

const workingCount = computed(() => props.snap.modules.filter((m) => m.working).length);
const unready = computed(() => props.snap.coverage.filter((c) => !c.ready));
const asOf = computed(() => {
  const d = new Date(props.snap.as_of);
  return Number.isNaN(d.getTime()) ? props.snap.as_of : d.toLocaleTimeString();
});

function tf(key: string, fallback: string): string {
  const text = t(key);
  return text === key ? fallback : text;
}
function moduleName(m: OpsModule): string {
  return tf(`ceoboard.ops.modules.${m.id}.name`, m.name);
}
function jobOf(m: OpsModule): string {
  return tf(`ceoboard.ops.modules.${m.id}.job`, m.job);
}
function scopeOf(m: OpsModule): string {
  return tf(`ceoboard.ops.modules.${m.id}.scope`, m.scope);
}
function cadenceOf(m: OpsModule): string {
  return tf(`ceoboard.ops.modules.${m.id}.cadence`, m.cadence);
}
function metricName(m: OpsMetric): string {
  return tf(`ceoboard.ops.kpis.${m.id}`, m.name);
}
function entityName(e: OpsEntity): string {
  return tf(`ceoboard.ops.entities.${e.id}`, e.name);
}
function statusLabel(value?: string): string {
  if (!value) return '';
  return tf(`ceoboard.ops.status.${value}`, value);
}
function coverageName(c: OpsCoverage): string {
  return tf(`ceoboard.ops.coverageNames.${c.id}`, c.name);
}
function coverageNote(c: OpsCoverage): string {
  return tf(`ceoboard.ops.coverageNotes.${c.note_key ?? c.id}`, c.note);
}

function toggle(id: string) {
  focus.value = focus.value === id ? '' : id;
}
function drillTo(path?: string) {
  if (path) emit('drill', path);
}
</script>

<template>
  <section class="ceo-ops" :aria-label="t('ceoboard.ops.title')">
    <header class="ceo-ops__head">
      <div class="ceo-ops__title">
        <p class="ceo-ops__kicker">{{ t('ceoboard.ops.kicker') }}</p>
        <h2>{{ t('ceoboard.ops.title') }}</h2>
        <p class="ceo-ops__scope">{{ tf('ceoboard.ops.scopeNote', snap.scope_note) }}</p>
      </div>
      <div class="ceo-ops__legend">
        <span class="flow-active">{{ t('ceoboard.ops.legendActive') }}</span>
        <span class="flow-stalled">{{ t('ceoboard.ops.legendStalled') }}</span>
        <span class="flow-error">{{ t('ceoboard.ops.legendError') }}</span>
        <span class="flow-off">{{ t('ceoboard.ops.legendOff') }}</span>
        <span class="dim">{{ t('ceoboard.ops.asOf', { at: asOf }) }}</span>
      </div>
    </header>

    <div class="ceo-ops__owner">
      <div class="ceo-ops__owner-card">
        <span class="role">{{ t('ceoboard.ops.owner') }}</span>
        <span class="sub">{{ t('ceoboard.ops.ownerSub') }}</span>
      </div>
      <div class="ceo-ops__ceo" :class="snap.company.health">
        <span class="ceo-dot" :class="snap.company.health"></span>
        <span class="name">CEO Agent</span>
        <span class="sub">
          {{ t('ceoboard.ops.ceoSummary', { modules: snap.modules.length, agents: snap.company.agents, working: workingCount }) }}
        </span>
      </div>
      <div class="ceo-ops__owner-card report">
        <span class="role">{{ t('ceoboard.ops.reportRole') }}</span>
        <span class="sub">{{ t('ceoboard.ops.reportSub', { anomalies: snap.anomalies.length }) }}</span>
      </div>
    </div>

    <div class="ceo-ops__bus" aria-hidden="true">
      <span class="down">{{ t('ceoboard.ops.downShort') }}</span>
      <span class="up">{{ t('ceoboard.ops.upShort') }}</span>
    </div>

    <div class="ceo-ops__grid" :class="{ 'has-focus': !!focus }">
      <article
        v-for="m in snap.modules"
        :key="m.id"
        class="ceo-ops__col"
        :class="[toneOf(m), { focused: focus === m.id, dimmed: !!focus && focus !== m.id }]"
        :data-flow="flowOf(m)"
      >
        <button
          type="button"
          class="ceo-ops__module"
          :class="m.health"
          :aria-pressed="focus === m.id"
          :title="`${jobOf(m)} · ${scopeOf(m)}`"
          @click="toggle(m.id)"
        >
          <span class="ceo-ops__module-top">
            <span class="ceo-dot" :class="m.health"></span>
            <span class="name">{{ moduleName(m) }}</span>
            <span v-if="m.working" class="live">{{ t('ceoboard.ops.live') }}</span>
          </span>
          <span class="job">{{ jobOf(m) }}</span>
          <span class="scope">{{ scopeOf(m) }}</span>
          <span class="cadence">{{ cadenceOf(m) }}</span>
          <span class="metrics">
            <span v-for="mm in m.metrics.slice(0, 2)" :key="mm.id" class="metric">
              <b>{{ mm.display }}</b>
              <i>{{ metricName(mm) }}</i>
            </span>
          </span>
        </button>

        <div class="ceo-ops__entities">
          <button
            v-for="e in m.entities"
            :key="e.id"
            type="button"
            class="ceo-ops__entity"
            :class="[e.health, { link: !!e.drill?.path, zero: !e.count }]"
            :disabled="!e.drill?.path"
            :title="e.drill?.path ? `${entityName(e)} · ${t('ceoboard.ops.openDrill')}` : entityName(e)"
            @click="drillTo(e.drill?.path)"
          >
            <span class="ceo-dot" :class="e.health"></span>
            <span class="ename">{{ entityName(e) }}</span>
            <strong class="ecount">{{ e.count }}</strong>
            <span v-if="e.status" class="estatus">{{ statusLabel(e.status) }}</span>
          </button>
        </div>
      </article>
    </div>

    <footer class="ceo-ops__foot">
      <p class="ceo-ops__coverage-label">
        {{ t('ceoboard.ops.coverage', { n: unready.length }) }}
        <span class="hint">{{ t('ceoboard.ops.coverageHint') }}</span>
      </p>
      <ul class="ceo-ops__coverage-chips">
        <li v-for="c in unready" :key="c.id" class="ceo-ops__ghost" :title="coverageNote(c)">
          <b>{{ coverageName(c) }}</b>
          <span class="ceo-sr">{{ coverageNote(c) }}</span>
        </li>
      </ul>
      <p class="ceo-ops__notice">{{ tf('ceoboard.ops.dataNotice', snap.notice ?? '') }}</p>
    </footer>
  </section>
</template>
