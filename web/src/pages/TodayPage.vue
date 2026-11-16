<template>
  <q-page class="app-standard-page today-page">
    <header class="today-head">
      <div>
        <p class="today-head__date today-mono">{{ dateLabel }}</p>
        <h1 class="today-head__line">{{ greeting }}。{{ headlineText }}</h1>
        <p v-if="headline.highRisk > 0" class="today-head__risk">
          {{ t('today.headline.highRisk', { n: headline.highRisk }) }}
        </p>
      </div>
      <div class="today-head__tools">
        <span v-if="inbox.updatedAt.value" class="today-head__fresh today-mono">
          {{ t('today.updatedAt', { time: clock(inbox.updatedAt.value) }) }}
        </span>
        <q-btn flat no-caps icon="refresh" :label="t('today.refresh')" :loading="refreshing" @click="refreshAll" />
      </div>
    </header>

    <p class="today-live" :role="feedback.error ? 'alert' : 'status'" aria-live="polite">{{ feedback.text }}</p>

    <div class="today-grid">
      <TodayInbox
        ref="inboxEl"
        class="today-panel"
        :rows="inbox.rows.value"
        :total="inbox.total.value"
        :loaded="inbox.loaded.value"
        :failed="inbox.failed.value"
        :degraded="inbox.degraded.value"
        :busy-ids="inbox.busyIds.value"
        :selected-id="selectedId"
        @select="selectedId = $event"
        @decide="onDecide"
        @batch="onBatch"
      />
      <div class="today-side">
        <TodayTimeline
          class="today-panel"
          :entries="timeline"
          :status="side.eventsStatus.value"
          :now="nowIso"
          @pick="pickFromTimeline"
        />
        <TodayTasks
          class="today-panel"
          :tasks="side.tasks.value"
          :tasks-status="side.tasksStatus.value"
          :task-busy="side.taskBusy.value"
          :last-done="lastDone"
          :delegated="side.delegated.value"
          :delegated-status="side.delegatedStatus.value"
          :add="side.addTask"
          @complete="onComplete"
          @undo="onUndo"
        />
      </div>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import TodayInbox from '../components/today/TodayInbox.vue';
import TodayTasks from '../components/today/TodayTasks.vue';
import TodayTimeline from '../components/today/TodayTimeline.vue';
import type { InboxAction, InboxItemRow } from '../features/approval/api';
import type { OfficeTaskRow } from '../features/office/types';
import { batchableLowRisk, buildTimeline, clock, inboxHeadline } from '../features/today/logic';
import { useInbox } from '../features/today/useInbox';
import { useTodaySide } from '../features/today/useTodaySide';

const { t, locale } = useI18n();
const inbox = useInbox();
const side = useTodaySide();

const inboxEl = ref<{ focusList: () => void } | null>(null);
const selectedId = ref('');
const refreshing = ref(false);
const feedback = reactive({ text: '', error: false });
const lastDone = ref<OfficeTaskRow | null>(null);

// 每分钟推进一次「现在」，驱动问候语、时间线分隔线与逾期判断。
const nowIso = ref(new Date().toISOString());
let clockTimer: ReturnType<typeof setInterval> | null = null;
onMounted(() => {
  clockTimer = setInterval(() => (nowIso.value = new Date().toISOString()), 60_000);
});
onUnmounted(() => {
  if (clockTimer) clearInterval(clockTimer);
});
const now = computed(() => new Date(nowIso.value));

const dateLabel = computed(() =>
  new Intl.DateTimeFormat(locale.value, { weekday: 'long', month: 'long', day: 'numeric' }).format(now.value),
);
const greeting = computed(() => {
  const hour = now.value.getHours();
  if (hour < 12) return t('today.greeting.morning');
  return hour < 18 ? t('today.greeting.afternoon') : t('today.greeting.evening');
});
const headline = computed(() => inboxHeadline(inbox.rows.value, inbox.total.value, now.value));
const headlineText = computed(() => {
  const { total, dueToday } = headline.value;
  if (!inbox.loaded.value) return '';
  if (total === 0) return t('today.headline.none');
  return dueToday > 0 ? t('today.headline.countDue', { total, due: dueToday }) : t('today.headline.count', { total });
});

const timeline = computed(() =>
  buildTimeline({ events: side.events.value, inbox: inbox.rows.value, cron: side.cron.value, now: now.value }),
);

// 保持始终有一条被选中：首次加载选第一条；选中的条目被处理后落到原位置的下一条。
let lastIndex = 0;
watch(
  () => inbox.rows.value,
  (rows) => {
    const index = rows.findIndex((row) => row.id === selectedId.value);
    if (index >= 0) {
      lastIndex = index;
      return;
    }
    selectedId.value = rows[Math.min(lastIndex, rows.length - 1)]?.id ?? '';
  },
  { immediate: true },
);

function say(text: string, error = false) {
  feedback.text = text;
  feedback.error = error;
}

async function onDecide(row: InboxItemRow, action: InboxAction, note: string) {
  const outcome = await inbox.decide(row, action, note);
  if (outcome.ok) {
    say(t(`today.result.${action}`, { title: row.title || row.summary }));
    inboxEl.value?.focusList();
  } else if (outcome.gone) {
    say(t('today.result.gone'));
  } else if (outcome.message) {
    say(t('today.result.failed', { message: outcome.message }), true);
  }
}

/** 同来源的低风险条目逐条批准（服务端逐条校验；任一失败不影响其余）。 */
async function onBatch(source: string) {
  const targets = batchableLowRisk(inbox.rows.value, source);
  let ok = 0;
  for (const row of targets) {
    const outcome = await inbox.decide(row, 'approve');
    if (outcome.ok) ok += 1;
  }
  say(t('today.result.batch', { ok, fail: targets.length - ok }), ok < targets.length);
  inboxEl.value?.focusList();
}

function pickFromTimeline(inboxId: string) {
  selectedId.value = inboxId;
  inboxEl.value?.focusList();
}

async function onComplete(task: OfficeTaskRow) {
  const error = await side.completeTask(task.id);
  if (error) say(t('today.tasks.saveFailed', { message: error }), true);
  else lastDone.value = task;
}

async function onUndo(task: OfficeTaskRow) {
  lastDone.value = null;
  await side.reopenTask(task.id);
}

async function refreshAll() {
  refreshing.value = true;
  await Promise.all([inbox.refresh(), side.refresh()]);
  refreshing.value = false;
}
</script>
