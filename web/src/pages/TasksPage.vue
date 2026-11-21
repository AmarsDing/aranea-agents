<template>
  <!--
    工作台 · 任务：按生命周期四栏——待办 → 进行中 → 待验收 → 已完成（设计 2026-11-20-工作台任务生命周期.md）。
    卡片可拖动换栏（键盘用卡片上的「移到…」）；点卡片打开右侧详情，?task=<id> 可直达。
    数据与今日页同源；AI 在跑时进行中每 10 秒对一次账。
  -->
  <q-page class="app-standard-page today-page tk-page">
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

    <div class="tk-board">
      <section
        v-for="col in BOARD_COLUMNS"
        :key="col"
        class="today-panel tk-col"
        :class="`tk-col--${col}`"
        :aria-labelledby="`tk-col-${col}`"
      >
        <header class="today-panel__head">
          <h2 :id="`tk-col-${col}`" class="today-panel__title">
            <span class="tk-col__dot" aria-hidden="true" />
            {{ t(`taskBoard.col.${col}`) }}
            <span class="today-panel__count">{{ lists[col].length }}</span>
            <span v-if="col === 'doing' && running" class="tk-live-tag" :title="t('taskBoard.liveHint')">
              <span class="tk-live" aria-hidden="true" />{{ t('taskBoard.live') }}
            </span>
          </h2>
          <button v-if="col === 'doing'" type="button" class="today-link" @click="openAssign({ to: 'tool' })">
            {{ t('tasksPage.newCoding') }}
          </button>
          <q-btn
            v-if="col === 'todo'"
            flat
            dense
            round
            size="sm"
            icon="add"
            :aria-label="t('taskBoard.newTask')"
            @click="openAssign()"
          >
            <q-tooltip>{{ t('taskBoard.newTask') }}</q-tooltip>
          </q-btn>
        </header>

        <p v-if="colError(col)" class="today-note today-note--danger" role="alert">{{ colError(col) }}</p>
        <draggable
          :list="lists[col]"
          item-key="key"
          group="tk-board"
          tag="ul"
          class="tk-col__list"
          :data-col="col"
          :move="canMove"
          :animation="150"
          :delay="150"
          :delay-on-touch-only="true"
          ghost-class="tk-card--ghost"
          @change="(evt: DragChange) => onDragChange(col, evt)"
        >
          <template #item="{ element }">
            <li :class="{ 'tk-col__item--fixed': !isDraggable(element) }">
              <TaskCard
                :task="element.task"
                :run="element.run"
                :column="col"
                :busy="isBusy(element)"
                :now="now"
                @open="openItem(element)"
                @complete="moveItem(element, 'done')"
                @accept="moveItem(element, 'done')"
                @move="moveItem(element, $event)"
                @dispatch="onDispatch(element, $event)"
              />
            </li>
          </template>
          <template #footer>
            <li v-if="lists[col].length === 0 && loaded(col)" class="today-empty tk-col__empty">
              {{ t(`taskBoard.colEmpty.${col}`) }}
            </li>
          </template>
        </draggable>
      </section>
    </div>

    <TaskDetailDrawer
      :open="Boolean(detailTask)"
      :task="detailTask"
      :run="detailRun"
      :now="now"
      @close="closeDetail"
      @changed="refreshAll"
      @updated="lastDetail = $event"
      @say="say"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRoute, useRouter } from 'vue-router';
import draggable from 'vuedraggable';
import TaskCard from '../components/tasks/TaskCard.vue';
import TaskDetailDrawer from '../features/tasks/components/TaskDetailDrawer.vue';
import { listOfficeTasks, setOfficeTaskStatus } from '../features/office/api';
import type { OfficeTaskRow } from '../features/office/types';
import {
  BOARD_COLUMNS,
  buildBoard,
  isDraggable,
  moveAction,
  type BoardColumn,
  type BoardItem,
} from '../features/tasks/board';
import { openAssign } from '../features/conversations/assign';
import { useTaskDispatch } from '../features/tasks/useTaskDispatch';
import type { Recipient } from '../features/today/command';
import { COMMAND_DONE_EVENT, type CommandDoneDetail } from '../features/today/useCommandRunner';
import { useTodaySide, type BlockStatus } from '../features/today/useTodaySide';
import { parseKratosApiError } from '../utils/kratosError';

type DragChange = { added?: { element: BoardItem; newIndex: number } };

const DONE_SHOWN = 30;

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const side = useTodaySide('tasks');
const { dispatch } = useTaskDispatch();

const feedback = reactive({ text: '', error: false, link: '' });
const done = ref<OfficeTaskRow[]>([]);
const doneStatus = ref<BlockStatus>('loading');
const busyIds = ref<Set<string>>(new Set());

const nowIso = ref(new Date().toISOString());
const now = computed(() => new Date(nowIso.value));
let clockTimer: ReturnType<typeof setInterval> | undefined;

const board = computed(() =>
  buildBoard({ mine: side.tasks.value, runs: side.delegated.value, aiTasks: side.aiTasks.value, done: done.value }),
);

// 拖动要改的是可变数组：每次数据变了就按归类重置一份
const lists = reactive<Record<BoardColumn, BoardItem[]>>({ todo: [], doing: [], review: [], done: [] });
watch(
  board,
  (next) => {
    for (const col of BOARD_COLUMNS) lists[col] = [...next[col]];
  },
  { immediate: true },
);

const running = computed(() => side.delegated.value.some((row) => row.state === 'running'));

function loaded(col: BoardColumn): boolean {
  if (col === 'done') return doneStatus.value === 'ok';
  if (col === 'todo') return side.tasksStatus.value === 'ok';
  return side.delegatedStatus.value === 'ok' && side.tasksStatus.value !== 'loading';
}

function colError(col: BoardColumn): string {
  if (col === 'todo' && side.tasksStatus.value === 'error') return t('today.tasks.failed');
  if ((col === 'doing' || col === 'review') && side.delegatedStatus.value === 'error')
    return t('today.delegated.failed');
  if (col === 'done' && doneStatus.value === 'error') return t('tasksPage.doneFailed');
  return '';
}

// 操作结果浮在页面右上角，过一会儿自己收起（出错的多留一会儿）
let sayTimer: ReturnType<typeof setTimeout> | undefined;
function say(text: string, error = false, link = '') {
  feedback.text = text;
  feedback.error = error;
  feedback.link = link;
  clearTimeout(sayTimer);
  sayTimer = setTimeout(() => (feedback.text = ''), error ? 10_000 : 5_000);
}

// 最近完成 = 我自己做完的 + 验收过的 AI 任务，按完成时间倒排
async function loadDone() {
  const results = await Promise.allSettled([
    listOfficeTasks('done', DONE_SHOWN),
    listOfficeTasks('done', DONE_SHOWN, 'delegate'),
  ]);
  if (results.every((res) => res.status === 'rejected')) {
    console.warn('[tasks] done load failed', results[0].status === 'rejected' ? results[0].reason : '');
    doneStatus.value = 'error';
    return;
  }
  const rows = results.flatMap((res) => (res.status === 'fulfilled' ? res.value : []));
  done.value = rows.sort((a, b) => b.updated_at.localeCompare(a.updated_at)).slice(0, DONE_SHOWN);
  doneStatus.value = 'ok';
}

async function refreshAll() {
  await Promise.all([side.refresh(), loadDone()]);
}

function isBusy(item: BoardItem): boolean {
  const id = item.task?.id;
  return Boolean(id && (busyIds.value.has(id) || side.taskBusy.value.has(id)));
}

/** Sortable 的 move 回调：不能拖的卡、不允许去的栏直接拦下（落点高亮也就不出现） */
function canMove(evt: { draggedContext: { element: BoardItem }; to: HTMLElement }): boolean {
  const to = evt.to.dataset.col as BoardColumn | undefined;
  const item = evt.draggedContext.element;
  if (!to || to === item.column) return to === item.column; // 栏内挪动不改数据，放行即可
  return moveAction(item, to) !== null;
}

function onDragChange(col: BoardColumn, evt: DragChange) {
  if (evt.added) void moveItem(evt.added.element, col);
}

/** 把一张卡移到某栏：开始 / 退回待办 / 完成 / 验收 / 重开；退回返工打开详情写意见。 */
async function moveItem(item: BoardItem, to: BoardColumn) {
  const task = item.task;
  const action = moveAction(item, to);
  if (!task || !action) {
    for (const c of BOARD_COLUMNS) lists[c] = [...board.value[c]];
    return;
  }
  if (action === 'rework') {
    detailId.value = task.id;
    say(t('taskBoard.reworkHint'));
    for (const c of BOARD_COLUMNS) lists[c] = [...board.value[c]];
    return;
  }
  const status = action === 'accept' || action === 'done' ? 'done' : action;
  busyIds.value = new Set(busyIds.value).add(task.id);
  try {
    await setOfficeTaskStatus(task.id, status);
    if (status === 'done')
      say(t(action === 'accept' ? 'today.delegated.accepted' : 'today.tasks.completed', { title: task.title }));
    else if (item.column === 'done') say(t('tasksPage.reopened', { title: task.title }));
  } catch (err) {
    say(t('today.tasks.saveFailed', { message: parseKratosApiError(err).message || 'error' }), true);
  } finally {
    const next = new Set(busyIds.value);
    next.delete(task.id);
    busyIds.value = next;
  }
  await refreshAll();
}

async function onDispatch(item: BoardItem, recipient: Recipient) {
  const task = item.task;
  if (!task) return;
  busyIds.value = new Set(busyIds.value).add(task.id);
  say(t('taskBoard.dispatch.sending', { name: recipient.name }));
  const outcome = await dispatch(task, recipient);
  const next = new Set(busyIds.value);
  next.delete(task.id);
  busyIds.value = next;
  if (outcome.ok)
    say(
      outcome.task ? t('taskBoard.dispatch.ok', { name: recipient.name }) : t('taskBoard.dispatch.notRecorded'),
      !outcome.task,
      outcome.link,
    );
  else say(t('taskBoard.dispatch.failed', { message: outcome.message }), true, outcome.link);
  await refreshAll();
}

// ——— 详情抽屉（?task=<id> 直达；按 id 记，列表刷新后拿最新那一行）———
const detailId = ref(typeof route.query.task === 'string' ? route.query.task : '');
// 刷新间隙里（任务刚从「我的」挪到「AI 的」，两张表还没都读回来）先用上一次的那行，抽屉不会闪关
const lastDetail = ref<OfficeTaskRow | null>(null);
const detailTask = computed(() => {
  const hit = [...side.tasks.value, ...side.aiTasks.value, ...done.value].find((task) => task.id === detailId.value);
  return hit ?? (lastDetail.value?.id === detailId.value ? lastDetail.value : null);
});
watch(detailTask, (task) => {
  if (task) lastDetail.value = task;
});
const detailRun = computed(
  () => side.delegated.value.find((row) => row.officeTaskId && row.officeTaskId === detailId.value) ?? null,
);

function openItem(item: BoardItem) {
  if (item.task) detailId.value = item.task.id;
}

function closeDetail() {
  detailId.value = '';
  if (route.query.task) void router.replace({ query: { ...route.query, task: undefined } });
}

watch(
  () => route.query.task,
  (id) => {
    if (typeof id === 'string' && id) detailId.value = id;
  },
);

function onCommandDone(event: Event) {
  const detail = (event as CustomEvent<CommandDoneDetail>).detail;
  if (detail) side.ingest(detail.intent, detail.task);
}

onMounted(() => {
  void loadDone();
  window.addEventListener(COMMAND_DONE_EVENT, onCommandDone);
  clockTimer = setInterval(() => (nowIso.value = new Date().toISOString()), 60_000);
});
onUnmounted(() => {
  window.removeEventListener(COMMAND_DONE_EVENT, onCommandDone);
  clearTimeout(sayTimer);
  clearInterval(clockTimer);
});
</script>
