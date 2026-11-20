<template>
  <!--
    工作台 · 今日（照设计稿）：一屏三列。上面四张摘要卡，下面左列待我处理（列表 + 处理台），
    中列今日时间线和我的任务，右列最新邮件和交办中。日期在顶栏，指令入口是底部正中的半圆按钮。
  -->
  <q-page class="app-standard-page today-page">
    <!-- 不再有「更新于 / 刷新」一行：数据每 30 秒和切回页面时自动刷新；操作结果浮在右上角 -->
    <p
      class="today-live"
      :class="{ 'today-live--on': feedback.text }"
      :role="feedback.error ? 'alert' : 'status'"
      aria-live="polite"
    >
      {{ feedback.text }}
      <router-link v-if="feedback.link" class="today-link" :to="feedback.link">{{
        t('today.command.openSession')
      }}</router-link>
    </p>

    <div class="today-sum">
      <router-link v-for="card in summary" :key="card.key" class="today-sum__card" :to="card.to">
        <span class="today-sum__icon"><q-icon :name="card.icon" size="20px" aria-hidden="true" /></span>
        <span class="today-sum__top">
          {{ card.label }}
          <strong class="today-sum__num today-mono" :class="{ 'today-sum__num--warn': card.warn }">{{
            card.value
          }}</strong>
        </span>
        <span class="today-sum__sub">{{ card.sub }}</span>
      </router-link>
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
          v-bind="taskProps"
          @open="openTask"
          @complete="onComplete"
          @move="onMove"
          @dispatch="onDispatch"
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
          v-bind="taskProps"
          @open="openTask"
          @complete="onComplete"
          @move="onMove"
          @dispatch="onDispatch"
          @accept="onAccept"
        />
      </div>
    </div>

    <TaskDetailDrawer
      :open="Boolean(detailTask)"
      :task="detailTask"
      :run="detailRun"
      :now="now"
      @close="detailId = ''"
      @changed="side.refresh()"
      @updated="lastDetail = $event"
      @say="say"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import TodayInbox from '../components/today/TodayInbox.vue';
import TodayMail from '../components/today/TodayMail.vue';
import TodayTasks from '../components/today/TodayTasks.vue';
import TodayTimeline from '../components/today/TodayTimeline.vue';
import TaskDetailDrawer from '../features/tasks/components/TaskDetailDrawer.vue';
import type { InboxAction, InboxItemRow } from '../features/approval/api';
import type { OfficeTaskRow } from '../features/office/types';
import type { BoardColumn } from '../features/tasks/board';
import { useTaskDispatch } from '../features/tasks/useTaskDispatch';
import type { Recipient } from '../features/today/command';
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
const { dispatch } = useTaskDispatch();

const inboxEl = ref<{ focusList: () => void } | null>(null);
const selectedId = ref('');
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

// 四张摘要卡（点了去对应的页）：数字都来自下面几块已经取回的数据，取不到的写明，不显示成 0
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
      to: '/approvals',
      icon: 'inbox',
      label: t('today.inbox.title'),
      value: inbox.loaded.value ? String(total) : '—',
      warn: total > 0,
      sub: total === 0 ? t('today.summary.inboxNone') : t('today.summary.inboxSub', { high: highRisk }),
    },
    {
      key: 'due',
      to: '/schedule',
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
      to: '/tasks',
      icon: 'sync',
      label: t('today.delegated.title'),
      value:
        side.delegatedStatus.value === 'ok'
          ? String(side.delegated.value.length + side.tasks.value.filter((task) => task.status === 'doing').length)
          : '—',
      warn: counts.stuck + counts.review > 0,
      sub: t('today.summary.delegatedSub', { stuck: counts.stuck, review: counts.review }),
    },
    {
      key: 'mail',
      to: '/mail',
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
  buildTimeline({
    events: side.events.value,
    inbox: inbox.rows.value,
    cron: side.cron.value,
    tasks: side.tasks.value,
    now: now.value,
  }),
);

// 两块任务列表共用的入参
const taskProps = computed(() => ({
  tasks: side.tasks.value,
  tasksStatus: side.tasksStatus.value,
  taskBusy: side.taskBusy.value,
  lastDone: lastDone.value,
  notes: side.notes.value,
  delegated: side.delegated.value,
  aiTasks: side.aiTasks.value,
  delegatedStatus: side.delegatedStatus.value,
  now: now.value,
}));

// ——— 任务详情抽屉：按 id 记，列表刷新后跟着拿最新那一行 ———
const detailId = ref('');
// 刷新间隙里（任务刚从「我的」挪到「AI 的」，两张表还没都读回来）先用上一次的那行，抽屉不会闪关
const lastDetail = ref<OfficeTaskRow | null>(null);
const detailTask = computed(() => {
  const hit = [...side.tasks.value, ...side.aiTasks.value].find((task) => task.id === detailId.value);
  return hit ?? (lastDetail.value?.id === detailId.value ? lastDetail.value : null);
});
watch(detailTask, (task) => {
  if (task) lastDetail.value = task;
});
const detailRun = computed(
  () => side.delegated.value.find((row) => row.officeTaskId && row.officeTaskId === detailId.value) ?? null,
);
function openTask(task: OfficeTaskRow | null) {
  if (task) detailId.value = task.id;
}

async function onMove(task: OfficeTaskRow, to: BoardColumn) {
  const status = to === 'doing' ? 'doing' : to === 'done' ? 'done' : 'open';
  const error = await side.setTaskStatus(task.id, status);
  if (error) say(t('today.tasks.saveFailed', { message: error }), true);
  else if (status === 'done') lastDone.value = task;
}

async function onDispatch(task: OfficeTaskRow, recipient: Recipient) {
  say(t('taskBoard.dispatch.sending', { name: recipient.name }));
  const outcome = await dispatch(task, recipient);
  if (outcome.ok)
    say(
      outcome.task ? t('taskBoard.dispatch.ok', { name: recipient.name }) : t('taskBoard.dispatch.notRecorded'),
      !outcome.task,
      outcome.link,
    );
  else say(t('taskBoard.dispatch.failed', { message: outcome.message }), true, outcome.link);
  await side.refresh();
}

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

// 操作结果浮在页面右上角，过一会儿自己收起（出错的多留一会儿）
let sayTimer: ReturnType<typeof setTimeout> | undefined;
onUnmounted(() => clearTimeout(sayTimer));
function say(text: string, error = false, link = '') {
  feedback.text = text;
  feedback.error = error;
  feedback.link = link;
  clearTimeout(sayTimer);
  sayTimer = setTimeout(() => (feedback.text = ''), error ? 10_000 : 5_000);
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
</script>
