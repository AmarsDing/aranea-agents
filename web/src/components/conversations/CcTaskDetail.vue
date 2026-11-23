<template>
  <!--
    任务详情：标题栏（任务名、工作目录、开始时间、轮次、进展、状态灯、继续）
    → 执行卡（谁做的、按什么顺序；并行的一组装在虚线框里）
    → 选中那次执行的栏（产品头像、名字、轮数 / Token / 花费 / 时长、对话 / 资料）→ 对话。
  -->
  <section class="cc-detail" :aria-label="task.title">
    <header class="cc-th">
      <div class="cc-th__row">
        <h2 class="cc-th__title" :title="task.title">{{ task.title }}</h2>
        <div class="cc-th__facts">
          <span v-if="task.project" :title="t('cc.meta.project')"
            ><q-icon name="folder_open" size="14px" aria-hidden="true" /><b>{{ task.project }}</b></span
          >
          <span v-if="start" :title="t('cc.meta.start')"
            ><q-icon name="schedule" size="14px" aria-hidden="true" /><b class="cc-num">{{ when(start) }}</b></span
          >
          <span v-if="turns" :title="t('cc.info.turns')">{{ t('cc.meta.turns', { n: turns }) }}</span>
          <span v-if="progress" class="cc-th__prog" :title="progress">{{ progress }}</span>
        </div>
        <!-- 状态灯：做完（蓝）点一下就确认，确认后变灰 -->
        <button
          v-if="light === 'done' && bucket === 'active'"
          type="button"
          class="cc-light-btn"
          :title="t('cc.light.confirmTip')"
          :aria-label="t('cc.light.confirmTip')"
          :disabled="busy"
          @click="emit('approve')"
        >
          <span class="cc-light cc-light--done" aria-hidden="true" />
        </button>
        <span
          v-else
          class="cc-light"
          :class="`cc-light--${light}`"
          :title="t(`cc.light.${light}`)"
          role="img"
          :aria-label="t(`cc.light.${light}`)"
        />
        <div class="cc-th__acts">
          <q-btn-dropdown
            v-if="bucket === 'active'"
            flat
            dense
            no-caps
            icon="play_arrow"
            :label="t('cc.action.next')"
            class="cc-btn"
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
    </header>

    <section class="cc-chain" :aria-label="t('cc.chain.title')">
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
              <span class="cc-light" :class="`cc-light--${execLight(e)}`" :title="t(`cc.light.${execLight(e)}`)" />
            </button>
          </div>
          <!-- 执行卡：头像 + 名字 + 状态灯（不写字）/ 做的事 / 开始时间 · 用时 -->
          <button
            v-else
            type="button"
            class="cc-node"
            :class="{ 'cc-node--on': step.id === execId }"
            :aria-pressed="step.id === execId"
            :aria-label="`${step.executorName} · ${t(`cc.light.${execLight(step)}`)}`"
            @click="emit('select-exec', step.id)"
          >
            <CcGlyph :executor="step.executor" :name="step.executorName" />
            <span class="cc-node__who"
              ><span>{{ step.executorName }}</span
              ><span class="cc-light" :class="`cc-light--${execLight(step)}`" :title="t(`cc.light.${execLight(step)}`)"
            /></span>
            <span class="cc-node__what" :title="step.what">{{ short(step.what) }}</span>
            <span class="cc-node__meta cc-num"
              >{{ when(step.start) }}<template v-if="took(step)"> · {{ took(step) }}</template></span
            >
          </button>
        </template>
      </div>
    </section>

    <div v-if="exec" class="cc-xbar">
      <div class="cc-xbar__who">
        <CcGlyph :executor="exec.executor" :name="exec.executorName" size="sm" />
        <b>{{ exec.executorName }}</b>
        <span v-if="exec.model && exec.model !== exec.executorName" class="cc-num">{{ exec.model }}</span>
      </div>
      <!-- 这次执行的用量（原「信息」页签的四个数） -->
      <dl class="cc-xbar__kpis">
        <div>
          <dt>{{ t('cc.info.turns') }}</dt>
          <dd class="cc-num">{{ exec.turns || '—' }}</dd>
        </div>
        <div>
          <dt>{{ t('cc.info.tokens') }}</dt>
          <dd class="cc-num">{{ exec.tokens ? fmtTok(exec.tokens) : '—' }}</dd>
        </div>
        <div>
          <dt>{{ t('cc.info.cost') }}</dt>
          <dd class="cc-num">{{ exec.costUsd ? `$${exec.costUsd.toFixed(2)}` : '—' }}</dd>
        </div>
        <div>
          <dt>{{ t('cc.info.duration') }}</dt>
          <dd class="cc-num">{{ took(exec) || '—' }}</dd>
        </div>
      </dl>
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
  execDuration,
  execLight,
  flat,
  sumOf,
  taskBucket,
  taskLight,
  taskPinned,
  taskStart,
  type ConvTask,
  type Execution,
} from '../../features/conversations/model';
import { dayOrClock } from '../../features/today/logic';

export type DetailTab = 'talk' | 'files';
const TABS: DetailTab[] = ['talk', 'files'];

const props = defineProps<{ task: ConvTask; execId: string; tab: DetailTab; busy?: boolean }>();
const emit = defineEmits<{
  'select-exec': [id: string];
  tab: [tab: DetailTab];
  handover: [to: 'spirit' | 'agent' | 'tool'];
  approve: [];
  pin: [];
  archive: [];
  restore: [];
  delete: [];
}>();
const { t } = useI18n();

// 交接单只挂在单个执行前面（并行的一组由编排拆出来，不是交接）
const handOf = (step: Execution | Execution[]) => (!Array.isArray(step) && step.hand ? step : undefined);
const matCount = (hand: string) => (hand.match(/^\d+\. /gm) ?? []).length;
const handExec = ref<Execution>();

const execs = computed(() => flat(props.task));
const exec = computed(() => execs.value.find((e) => e.id === props.execId));
const light = computed(() => taskLight(props.task));
const bucket = computed(() => taskBucket(props.task));
const pinned = computed(() => taskPinned(props.task));
const hasSession = computed(() => execs.value.some((e) => e.kind === 'session'));
const start = computed(() => taskStart(props.task));
const turns = computed(() => sumOf(props.task, 'turns'));
// 进展：最近一次执行的那句话（失败原因 / 最近一句 / 摘要），没有就写它在做什么
const progress = computed(() => {
  const latest = [...execs.value].sort((a, b) => a.last.localeCompare(b.last)).pop();
  const text = (latest ? latest.detail || latest.what : '').split('\n')[0]?.trim() ?? '';
  return text === props.task.title ? '' : text;
});

const when = (iso: string) => (iso ? dayOrClock(iso, new Date()) : '—');
// 执行卡上的任务名：去掉交接单的「【任务】」前缀，只留一行
const short = (what: string) => what.replace(/^【[^】]*】\s*/, '').split('\n')[0] ?? '';
const fmtTok = (n: number) =>
  n >= 1e6 ? `${(n / 1e6).toFixed(1)}M` : n >= 1e3 ? `${Math.round(n / 1e3)}k` : String(n);
function took(e: Execution) {
  const ms = execDuration(e);
  if (!ms) return '';
  const m = Math.round(ms / 60000);
  if (m < 1) return t('cc.info.seconds', { n: Math.max(1, Math.round(ms / 1000)) });
  return m < 60 ? t('cc.info.minutes', { n: m }) : t('cc.info.hours', { n: (m / 60).toFixed(1) });
}
</script>
