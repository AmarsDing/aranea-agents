<template>
  <!--
    工作台 · 今日（照设计稿）：一屏三列。上面四张摘要卡，下面左列待我处理（列表 + 处理台），
    中列今日时间线和我的任务，右列最新邮件和交办中。日期在顶栏，指令入口是右下角的指令窗。
  -->
  <q-page class="app-standard-page today-page">
    <div class="today-statusline">
      <p class="today-live" :role="feedback.error ? 'alert' : 'status'" aria-live="polite">
        {{ feedback.text }}
        <router-link v-if="feedback.link" class="today-link" :to="feedback.link">{{
          t('today.command.openSession')
        }}</router-link>
      </p>
      <span v-if="inbox.updatedAt.value" class="today-fresh today-mono">
        {{ t('today.updatedAt', { time: clock(inbox.updatedAt.value) }) }}
      </span>
      <q-btn flat dense no-caps icon="refresh" :label="t('today.refresh')" :loading="refreshing" @click="refreshAll" />
    </div>

    <div class="today-sum">
      <div v-for="card in summary" :key="card.key" class="today-sum__card">
        <span class="today-sum__icon"><q-icon :name="card.icon" size="20px" aria-hidden="true" /></span>
        <span class="today-sum__top">
          {{ card.label }}
          <strong class="today-sum__num today-mono" :class="{ 'today-sum__num--warn': card.warn }">{{
            card.value
          }}</strong>
        </span>
        <span class="today-sum__sub">{{ card.sub }}</span>
      </div>
    </div>

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
          part="tasks"
          :tasks="side.tasks.value"
          :tasks-status="side.tasksStatus.value"
          :task-busy="side.taskBusy.value"
          :last-done="lastDone"
          :notes="side.notes.value"
          :delegated="side.delegated.value"
          :delegated-status="side.delegatedStatus.value"
          :add="side.addTask"
          @complete="onComplete"
          @undo="onUndo"
          @dismiss-note="side.dismissNote($event.id)"
        />
      </div>
      <div class="today-side">
        <TodayMail
          class="today-panel"
          :mails="side.mails.value"
          :total="side.mailTotal.value"
          :status="side.mailsStatus.value"
        />
        <TodayTasks
          class="today-panel"
          part="delegated"
          :tasks="side.tasks.value"
          :tasks-status="side.tasksStatus.value"
          :task-busy="side.taskBusy.value"
          :last-done="null"
          :notes="[]"
          :delegated="side.delegated.value"
          :delegated-status="side.delegatedStatus.value"
          :add="side.addTask"
          @accept="onAccept"
        />
      </div>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import TodayInbox from '../components/today/TodayInbox.vue';
import TodayMail from '../components/today/TodayMail.vue';
import TodayTasks from '../components/today/TodayTasks.vue';
import TodayTimeline from '../components/today/TodayTimeline.vue';
import type { InboxAction, InboxItemRow } from '../features/approval/api';
import type { OfficeTaskRow } from '../features/office/types';
import {
  batchableLowRisk,
  buildTimeline,
  clock,
  dayBounds,
  inboxHeadline,
  type DelegatedRow,
} from '../features/today/logic';
import { COMMAND_DONE_EVENT, type CommandDoneDetail } from '../features/today/useCommandRunner';
import { useInbox } from '../features/today/useInbox';
import { useTodaySide } from '../features/today/useTodaySide';

const { t } = useI18n();
const inbox = useInbox();
const side = useTodaySide();

const inboxEl = ref<{ focusList: () => void } | null>(null);
const selectedId = ref('');
const refreshing = ref(false);
const feedback = reactive({ text: '', error: false, link: '' });
const lastDone = ref<OfficeTaskRow | null>(null);

// 每分钟推进一次「现在」，驱动时间线分隔线与逾期判断。
const nowIso = ref(new Date().toISOString());
let clockTimer: ReturnType<typeof setInterval> | null = null;
onMounted(() => {
  clockTimer = setInterval(() => (nowIso.value = new Date().toISOString()), 60_000);
  window.addEventListener(COMMAND_DONE_EVENT, onCommandDone);
});
onUnmounted(() => {
  if (clockTimer) clearInterval(clockTimer);
  window.removeEventListener(COMMAND_DONE_EVENT, onCommandDone);
});

const now = computed(() => new Date(nowIso.value));

const headline = computed(() => inboxHeadline(inbox.rows.value, inbox.total.value, now.value));

// 四张摘要卡：数字都来自下面几块已经取回的数据，取不到的写明，不显示成 0
const summary = computed(() => {
  const { total, dueToday, highRisk } = headline.value;
  const end = dayBounds(now.value).end.getTime();
  const nextDue = inbox.rows.value
    .filter((row) => row.due_at && new Date(row.due_at).getTime() < end)
    .sort((a, b) => new Date(a.due_at).getTime() - new Date(b.due_at).getTime())[0];
  const counts = side.delegatedCounts.value;
  const latestMail = side.mails.value[0];
  const mailReady = side.mailsStatus.value === 'ok';
  return [
    {
      key: 'inbox',
      icon: 'inbox',
      label: t('today.inbox.title'),
      value: inbox.loaded.value ? String(total) : '—',
      warn: total > 0,
      sub: total === 0 ? t('today.summary.inboxNone') : t('today.summary.inboxSub', { high: highRisk }),
    },
    {
      key: 'due',
      icon: 'schedule',
      label: t('today.summary.due'),
      value: inbox.loaded.value ? String(dueToday) : '—',
      warn: dueToday > 0,
      sub: nextDue
        ? t('today.summary.dueSub', { time: clock(nextDue.due_at), title: nextDue.title || nextDue.summary })
        : t('today.summary.dueNone'),
    },
    {
      key: 'delegated',
      icon: 'send',
      label: t('today.delegated.title'),
      value: side.delegatedStatus.value === 'ok' ? String(side.delegated.value.length) : '—',
      warn: false,
      sub: t('today.summary.delegatedSub', { stuck: counts.stuck, review: counts.review }),
    },
    {
      key: 'mail',
      icon: 'mail',
      label: t('today.summary.mail'),
      value: mailReady ? String(side.mailTotal.value) : '—',
      warn: false,
      sub: !mailReady
        ? t(side.mailsStatus.value === 'denied' ? 'today.mail.denied' : 'today.mail.failed')
        : latestMail
          ? t('today.summary.mailSub', { subject: latestMail.subject || t('today.mail.noSubject') })
          : t('today.mail.empty'),
    },
  ];
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

function say(text: string, error = false, link = '') {
  feedback.text = text;
  feedback.error = error;
  feedback.link = link;
}

// 指令窗 / Ctrl+K 指令面板执行完指令都会广播，这里统一更新列表
function onCommandDone(event: Event) {
  const detail = (event as CustomEvent<CommandDoneDetail>).detail;
  if (detail) side.ingest(detail.intent, detail.task);
}

async function onAccept(row: DelegatedRow) {
  const error = await side.acceptDelegated(row.officeTaskId);
  if (error) say(t('today.tasks.saveFailed', { message: error }), true);
  else say(t(row.state === 'review' ? 'today.delegated.accepted' : 'today.delegated.dropped', { title: row.title }));
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
