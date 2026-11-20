<template>
  <!--
    今日页的两块任务（数据与任务页同源，按生命周期分）：
      part=tasks    我的任务 = 待办（列表在上，输入框沉底；随手记录收在列表下面）
      part=delegated 进行中 = 我在做的 + AI 在做 / 卡住 / 做完待验收的（有在跑的就每 10 秒对一次账）
  -->
  <section class="today-tasks" :class="{ 'today-tasks--compose': part === 'tasks' }">
    <template v-if="part === 'tasks'">
      <header class="today-panel__head">
        <h2 class="today-panel__title">
          {{ t('today.tasks.title') }}
          <span class="today-panel__count">{{ todo.length }}</span>
        </h2>
        <router-link class="today-link" to="/tasks">{{ t('today.delegated.viewAll') }}</router-link>
      </header>

      <div class="today-tasks__scroll">
        <p v-if="tasksStatus === 'error'" class="today-note today-note--danger" role="alert">
          {{ t('today.tasks.failed') }}
        </p>
        <p v-else-if="tasksStatus === 'ok' && todo.length === 0" class="today-empty">{{ t('today.tasks.empty') }}</p>
        <ul v-else class="tk-list">
          <li v-for="task in todo" :key="task.id">
            <TaskCard
              :task="task"
              :busy="taskBusy.has(task.id)"
              :now="now"
              @open="emit('open', task, null)"
              @complete="emit('complete', task)"
              @move="emit('move', task, $event)"
              @dispatch="emit('dispatch', task, $event)"
            />
          </li>
        </ul>
        <p v-if="lastDone" class="today-tasks__undo" role="status" aria-live="polite">
          {{ t('today.tasks.completed', { title: lastDone.title }) }}
          <button type="button" class="today-link" @click="emit('undo', lastDone)">{{ t('today.tasks.undo') }}</button>
        </p>

        <details v-if="notes.length > 0" class="today-notes">
          <summary class="today-notes__summary">{{ t('today.notes.title', { n: notes.length }) }}</summary>
          <ul class="today-tasks__list">
            <li v-for="item in notes" :key="item.id" class="today-tasks__row">
              <span class="today-tasks__title">{{ item.title }}</span>
              <button type="button" class="today-link" @click="emit('dismissNote', item)">
                {{ t('today.notes.dismiss') }}
              </button>
            </li>
          </ul>
        </details>
      </div>

      <TaskComposer
        :add="add"
        :add-note="addNote"
        @created="emit('created', $event)"
        @say="(text, error, link) => emit('say', text, error, link)"
      />
    </template>

    <template v-else>
      <header class="today-panel__head">
        <h2 class="today-panel__title">
          {{ t('today.delegated.title') }}
          <span class="today-panel__count">{{ doing.length + delegated.length }}</span>
          <span v-if="running" class="tk-live-tag" :title="t('taskBoard.liveHint')">
            <span class="tk-live" aria-hidden="true" />{{ t('taskBoard.live') }}
          </span>
        </h2>
        <router-link class="today-link" to="/tasks">{{ t('today.delegated.viewAll') }}</router-link>
      </header>
      <p v-if="delegatedStatus === 'error'" class="today-note today-note--danger" role="alert">
        {{ t('today.delegated.failed') }}
      </p>
      <p
        v-else-if="delegatedStatus === 'ok' && tasksStatus !== 'loading' && doing.length + delegated.length === 0"
        class="today-empty"
      >
        {{ t('today.delegated.empty') }}
      </p>
      <ul v-else class="tk-list" aria-live="polite">
        <li v-for="row in visibleRuns" :key="row.id">
          <TaskCard
            :task="aiTask(row)"
            :run="row"
            :busy="row.officeTaskId ? taskBusy.has(row.officeTaskId) : false"
            :now="now"
            @open="emit('open', aiTask(row), row)"
            @accept="emit('accept', row)"
          />
        </li>
        <li v-for="task in doing" :key="task.id">
          <TaskCard
            :task="task"
            :busy="taskBusy.has(task.id)"
            :now="now"
            @open="emit('open', task, null)"
            @complete="emit('complete', task)"
            @move="emit('move', task, $event)"
            @dispatch="emit('dispatch', task, $event)"
          />
        </li>
      </ul>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import TaskCard from '../tasks/TaskCard.vue';
import TaskComposer from '../tasks/TaskComposer.vue';
import type { OfficeTaskRow } from '../../features/office/types';
import { sortMyTasks, type BoardColumn } from '../../features/tasks/board';
import type { Recipient } from '../../features/today/command';
import type { DelegatedRow } from '../../features/today/logic';
import type { BlockStatus } from '../../features/today/useTodaySide';

const MAX_RUN_ROWS = 8;

const props = defineProps<{
  /** tasks = 我的任务（待办）；delegated = 进行中 */
  part: 'tasks' | 'delegated';
  /** 我的任务：待办 + 我在做 */
  tasks: OfficeTaskRow[];
  tasksStatus: BlockStatus;
  taskBusy: Set<string>;
  /** 最近一次勾选完成的任务（提供撤销）；没有时为 null */
  lastDone: OfficeTaskRow | null;
  /** 最近几条随手记录 */
  notes: OfficeTaskRow[];
  /** AI 执行（派给 AI 的任务 + 编码任务） */
  delegated: DelegatedRow[];
  /** 派给 AI 的工作台任务原始行（按 id 配对） */
  aiTasks: OfficeTaskRow[];
  delegatedStatus: BlockStatus;
  now: Date;
  add: (title: string, dueAt?: string) => Promise<{ error: string; task: OfficeTaskRow | null }>;
  addNote: (text: string) => Promise<string>;
}>();

const emit = defineEmits<{
  open: [task: OfficeTaskRow | null, run: DelegatedRow | null];
  complete: [task: OfficeTaskRow];
  move: [task: OfficeTaskRow, to: BoardColumn];
  dispatch: [task: OfficeTaskRow, recipient: Recipient];
  undo: [task: OfficeTaskRow];
  dismissNote: [note: OfficeTaskRow];
  /** 待验收的验收通过 */
  accept: [row: DelegatedRow];
  created: [task: OfficeTaskRow];
  say: [text: string, error: boolean, link?: string];
}>();

const { t } = useI18n();

const sorted = computed(() => sortMyTasks(props.tasks));
const todo = computed(() => sorted.value.filter((task) => task.status !== 'doing'));
const doing = computed(() => sorted.value.filter((task) => task.status === 'doing'));
const visibleRuns = computed(() => props.delegated.slice(0, MAX_RUN_ROWS));
const running = computed(() => props.delegated.some((row) => row.state === 'running'));

const byId = computed(() => new Map(props.aiTasks.map((task) => [task.id, task])));
function aiTask(row: DelegatedRow): OfficeTaskRow | null {
  return (row.officeTaskId && byId.value.get(row.officeTaskId)) || null;
}
</script>
