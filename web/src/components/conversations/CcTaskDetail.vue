<template>
  <!--
    任务详情：这件事是什么、做到哪了 → 执行链（谁做的、按什么顺序；并行的一组装在虚线框里）
    → 当前选中那次执行的对话（插槽，由页面放进对应的执行视图）或信息。
  -->
  <section class="cc-detail" :aria-label="task.title">
    <header class="cc-th">
      <div class="cc-th__row">
        <h2 class="cc-th__title" :title="task.title">{{ task.title }}</h2>
        <span class="cc-pill" :class="`cc-pill--${status}`">{{
          t(review ? 'cc.status.review' : `cc.status.${status}`)
        }}</span>
        <div class="cc-th__acts">
          <q-btn-dropdown
            v-if="bucket === 'active'"
            unelevated
            dense
            no-caps
            color="primary"
            icon="play_arrow"
            :label="t('cc.action.next')"
            class="cc-th__next"
          >
            <q-list dense>
              <q-item v-close-popup clickable @click="emit('handover', 'spirit')">
                <q-item-section>{{ t('cc.handover.spirit') }}</q-item-section>
              </q-item>
              <q-item v-close-popup clickable @click="emit('handover', 'agent')">
                <q-item-section>{{ t('cc.handover.agent') }}</q-item-section>
              </q-item>
              <q-item v-close-popup clickable @click="emit('handover', 'tool')">
                <q-item-section>{{ t('cc.handover.tool') }}</q-item-section>
              </q-item>
            </q-list>
          </q-btn-dropdown>
          <q-btn
            v-if="hasSession && bucket !== 'deleted'"
            flat
            dense
            round
            :icon="pinned ? 'star' : 'star_border'"
            :aria-label="t(pinned ? 'cc.action.unpin' : 'cc.action.pin')"
            :title="t(pinned ? 'cc.action.unpin' : 'cc.action.pin')"
            :disable="busy"
            @click="emit('pin')"
          />
          <q-btn
            v-if="bucket === 'active'"
            flat
            dense
            round
            icon="archive"
            :aria-label="t('cc.action.archive')"
            :title="t('cc.action.archiveKey')"
            :disable="busy"
            @click="emit('archive')"
          />
          <q-btn
            v-else
            flat
            dense
            no-caps
            :icon="bucket === 'deleted' ? 'restore_from_trash' : 'unarchive'"
            :label="t(bucket === 'deleted' ? 'cc.action.restore' : 'cc.action.unarchive')"
            :disable="busy"
            @click="emit('restore')"
          />
          <q-btn
            v-if="bucket !== 'deleted'"
            flat
            dense
            round
            icon="delete_outline"
            class="cc-th__del"
            :aria-label="t('cc.action.delete')"
            :title="t('cc.action.deleteKey')"
            :disable="busy"
            @click="emit('delete')"
          />
        </div>
      </div>
      <div class="cc-th__meta">
        <span class="cc-origin">{{ t(`cc.origin.${task.origin}`) }}</span>
        <span v-if="task.project"
          ><q-icon name="folder_open" size="14px" aria-hidden="true" /><b>{{ task.project }}</b></span
        >
        <span v-if="start"
          ><q-icon name="schedule" size="14px" aria-hidden="true" />{{ t('cc.meta.start') }}
          <b class="cc-num">{{ when(start) }}</b></span
        >
        <span>{{
          sumOf(task, 'turns')
            ? t('cc.meta.amount', { n: execs.length, turns: sumOf(task, 'turns') })
            : t('cc.meta.execs', { n: execs.length })
        }}</span>
        <span v-if="tokens">{{ t('cc.meta.tokens', { n: fmtTok(tokens) }) }}</span>
        <span v-if="cost"
          ><q-icon name="payments" size="14px" aria-hidden="true" /><b class="cc-num">${{ cost.toFixed(2) }}</b></span
        >
      </div>
      <div v-if="progress" class="cc-prog">
        <span class="cc-prog__k">{{ t('cc.progress') }}</span>
        <span class="cc-prog__v">{{ progress }}</span>
      </div>
      <!-- 待你验收：选了「交回什么」、最后一位做完了 -->
      <div v-if="review" class="cc-review" role="group" :aria-label="t('cc.status.review')">
        <p>
          {{ t('cc.review.done', { who: lastExec?.executorName ?? '', what: deliverText }) }}
          <span>{{ t('cc.review.hint') }}</span>
        </p>
        <div class="cc-review__acts">
          <q-btn
            unelevated
            dense
            no-caps
            color="primary"
            :label="t('cc.review.approve')"
            :disable="busy"
            @click="emit('approve')"
          />
          <q-btn outline dense no-caps :label="t('cc.review.reject')" :disable="busy" @click="emit('reject')" />
          <q-btn flat dense no-caps :label="t('cc.review.next')" @click="emit('handover', 'spirit')" />
        </div>
      </div>
    </header>

    <section class="cc-chain" :aria-label="t('cc.chain.title')">
      <div class="cc-chain__head">
        <b>{{ t('cc.chain.title') }}</b
        ><span>{{ t('cc.chain.hint') }}</span>
      </div>
      <div class="cc-track">
        <template v-for="(step, i) in task.chain" :key="i">
          <span v-if="i > 0 && !handOf(step)" class="cc-link" aria-hidden="true"
            ><q-icon name="east" size="16px"
          /></span>
          <!-- 两次执行之间：有交接单就挂一枚小标签，点开看当时发过去的原文 -->
          <span v-else-if="i > 0" class="cc-link cc-link--hand">
            <q-icon name="east" size="16px" aria-hidden="true" />
            <button
              type="button"
              class="cc-hand"
              :title="t('cc.chain.handTitle', { who: handOf(step)!.executorName })"
              @click="handExec = handOf(step)"
            >
              {{ t('cc.chain.hand', { n: matCount(handOf(step)!.hand) }) }}
            </button>
          </span>
          <div v-if="Array.isArray(step)" class="cc-par">
            <span class="cc-par__label">{{ t('cc.chain.parallel', { n: step.length }) }}</span>
            <button
              v-for="e in step"
              :key="e.id"
              type="button"
              class="cc-node cc-node--mini"
              :class="{ 'cc-node--on': e.id === execId }"
              :aria-pressed="e.id === execId"
              @click="emit('select-exec', e.id)"
            >
              <CcGlyph :executor="e.executor" :name="e.executorName" size="xs" />
              <span class="cc-node__who">{{ e.executorName }}</span>
              <span class="cc-sd" :class="`cc-sd--${e.status}`" :title="t(`cc.status.${e.status}`)" />
            </button>
          </div>
          <button
            v-else
            type="button"
            class="cc-node"
            :class="{ 'cc-node--on': step.id === execId }"
            :aria-pressed="step.id === execId"
            @click="emit('select-exec', step.id)"
          >
            <CcGlyph :executor="step.executor" :name="step.executorName" />
            <span class="cc-node__who"
              ><span>{{ step.executorName }}</span
              ><span class="cc-sd" :class="`cc-sd--${step.status}`" :title="t(`cc.status.${step.status}`)"
            /></span>
            <span class="cc-node__what">{{ step.what }}</span>
            <span class="cc-node__meta"
              >{{ t(`cc.status.${step.status}`)
              }}<template v-if="step.turns"> · {{ t('cc.meta.turns', { n: step.turns }) }}</template> ·
              {{ when(step.last) }}</span
            >
          </button>
        </template>
      </div>
    </section>

    <div v-if="exec" class="cc-xbar">
      <div class="cc-xbar__who">
        <CcGlyph :executor="exec.executor" :name="exec.executorName" size="xs" />
        <span
          ><b>{{ exec.executorName }}</b> · {{ exec.what
          }}<template v-if="exec.model">
            · <span class="cc-num">{{ exec.model }}</span></template
          ></span
        >
      </div>
      <nav class="cc-dtabs" role="tablist">
        <button
          v-for="k in TABS"
          :key="k"
          type="button"
          role="tab"
          :aria-selected="tab === k"
          :class="{ 'cc-dtabs--on': tab === k }"
          @click="emit('tab', k)"
        >
          {{ t(`cc.tab.${k}`) }}
        </button>
      </nav>
    </div>

    <div v-show="tab === 'talk'" class="cc-detail__talk"><slot /></div>
    <slot v-if="tab === 'files'" name="files" />
    <div v-if="tab === 'info' && exec" class="cc-info">
      <div class="cc-kpis">
        <div class="cc-kpi">
          <span>{{ t('cc.info.turns') }}</span
          ><b>{{ exec.turns || '—' }}</b>
        </div>
        <div class="cc-kpi">
          <span>{{ t('cc.info.tokens') }}</span
          ><b>{{ exec.tokens ? fmtTok(exec.tokens) : '—' }}</b>
        </div>
        <div class="cc-kpi">
          <span>{{ t('cc.info.cost') }}</span
          ><b>{{ exec.costUsd ? `$${exec.costUsd.toFixed(2)}` : '—' }}</b>
        </div>
        <div class="cc-kpi">
          <span>{{ t('cc.info.duration') }}</span
          ><b>{{ duration(exec) }}</b>
        </div>
      </div>
      <dl class="cc-dl">
        <dt>{{ t('cc.info.executor') }}</dt>
        <dd>{{ exec.executorName }}</dd>
        <dt>{{ t('cc.info.model') }}</dt>
        <dd class="cc-num">{{ exec.model || '—' }}</dd>
        <dt>{{ t('cc.info.source') }}</dt>
        <dd>{{ t(`cc.source.${exec.kind}`) }}</dd>
        <dt>{{ t('cc.info.id') }}</dt>
        <dd class="cc-num">{{ exec.refId }}</dd>
        <dt>{{ t('cc.info.task') }}</dt>
        <dd>{{ t(`cc.origin.${task.origin}`) }}</dd>
        <dt>{{ t('cc.info.deleteMeans') }}</dt>
        <dd>{{ t(`cc.deleteMeans.${exec.kind}`) }}</dd>
      </dl>
    </div>

    <q-dialog :model-value="Boolean(handExec)" @update:model-value="(v: boolean) => !v && (handExec = undefined)">
      <q-card v-if="handExec" class="cc-hand-dlg">
        <q-card-section>
          <h3>{{ t('cc.chain.handTitle', { who: handExec.executorName }) }}</h3>
          <p>{{ t('cc.chain.handSent', { at: when(handExec.start) }) }}</p>
          <pre>{{ handExec.hand }}</pre>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup flat no-caps :label="t('cc.compose.close')" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import CcGlyph from './CcGlyph.vue';
import {
  flat,
  needsReview,
  sumOf,
  taskBucket,
  taskPinned,
  taskStart,
  taskStatus,
  type ConvTask,
  type Execution,
} from '../../features/conversations/model';
import { dayOrClock } from '../../features/today/logic';

export type DetailTab = 'talk' | 'files' | 'info';
const TABS: DetailTab[] = ['talk', 'files', 'info'];

const props = defineProps<{ task: ConvTask; execId: string; tab: DetailTab; busy?: boolean }>();
const emit = defineEmits<{
  'select-exec': [id: string];
  tab: [tab: DetailTab];
  handover: [to: 'spirit' | 'agent' | 'tool'];
  approve: [];
  reject: [];
  pin: [];
  archive: [];
  restore: [];
  delete: [];
}>();
const { t } = useI18n();

const review = computed(() => needsReview(props.task));
const lastExec = computed(() => [...flat(props.task)].sort((a, b) => a.last.localeCompare(b.last)).pop());
const deliverText = computed(() => (props.task.deliver ?? []).map((d) => t(`cc.deliver.${d}`)).join(t('cc.hand.sep')));
// 交接单只挂在单个执行前面（并行的一组由编排拆出来，不是交接）
const handOf = (step: Execution | Execution[]) => (!Array.isArray(step) && step.hand ? step : undefined);
const matCount = (hand: string) => (hand.match(/^\d+\. /gm) ?? []).length;
const handExec = ref<Execution>();

const execs = computed(() => flat(props.task));
const exec = computed(() => execs.value.find((e) => e.id === props.execId));
const status = computed(() => taskStatus(props.task));
const bucket = computed(() => taskBucket(props.task));
const pinned = computed(() => taskPinned(props.task));
const hasSession = computed(() => execs.value.some((e) => e.kind === 'session'));
const start = computed(() => taskStart(props.task));
const tokens = computed(() => sumOf(props.task, 'tokens'));
const cost = computed(() => sumOf(props.task, 'costUsd'));
// 进展：最近一次执行的那句话（失败原因 / 最近一句 / 摘要），没有就写它在做什么
const progress = computed(() => {
  const latest = [...execs.value].sort((a, b) => a.last.localeCompare(b.last)).pop();
  const text = latest ? latest.detail || latest.what : '';
  return text === props.task.title ? '' : text;
});

const when = (iso: string) => (iso ? dayOrClock(iso, new Date()) : '—');
const fmtTok = (n: number) =>
  n >= 1e6 ? `${(n / 1e6).toFixed(1)}M` : n >= 1e3 ? `${Math.round(n / 1e3)}k` : String(n);
function duration(e: Execution) {
  const ms = Date.parse(e.last) - Date.parse(e.start);
  if (!Number.isFinite(ms) || ms <= 0) return '—';
  const m = Math.round(ms / 60000);
  return m < 60 ? t('cc.info.minutes', { n: m }) : t('cc.info.hours', { n: (m / 60).toFixed(1) });
}
</script>
