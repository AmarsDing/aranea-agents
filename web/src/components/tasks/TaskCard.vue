<template>
  <!--
    一条任务（今日页的两块列表和任务页看板共用）。点标题打开详情；右侧放当前阶段最常用的一两个动作。
    AI 在做的显示执行者、当前步骤和状态点；卡住 / 等你回复的红边排最前。
  -->
  <article class="tk-card" :class="[`tk-card--${state}`, { 'tk-card--busy': busy }]">
    <q-checkbox
      v-if="checkable"
      :model-value="false"
      dense
      class="tk-card__check"
      :disable="busy"
      :aria-label="t('today.tasks.complete', { title })"
      @update:model-value="emit('complete')"
    />
    <component
      :is="mainTag"
      class="tk-card__main"
      v-bind="mainAttrs"
      :aria-label="t('taskBoard.actions.open', { title })"
      @click="onMain"
    >
      <span class="tk-card__title">{{ title }}</span>
      <span class="tk-card__meta">
        <!-- 待办不再挂「待办」小标：栏名 / 面板名已经说了 -->
        <span v-if="state !== 'todo'" class="today-state" :class="`today-state--${badge}`">
          <span v-if="state === 'running'" class="tk-live" aria-hidden="true" />
          {{ t(`taskBoard.state.${state}`) }}
        </span>
        <span v-if="run?.agentKey" class="tk-card__who">
          <q-icon :name="run.assigneeKind === 'team' ? 'groups' : 'smart_toy'" size="12px" aria-hidden="true" />
          {{ run.agentKey }}
        </span>
        <span v-if="due" class="tk-due today-mono" :class="`tk-due--${due}`">
          <q-icon name="event" size="12px" aria-hidden="true" />{{ dueLabel }}
        </span>
        <span v-if="task?.file_count" class="tk-card__files today-mono">
          <q-icon name="attach_file" size="12px" aria-hidden="true" />{{ task.file_count }}
        </span>
        <span v-if="task?.source === 'mail'" class="today-tag">{{ t('today.tasks.fromMail') }}</span>
        <span v-if="!task && run" class="today-tag">{{ t('taskBoard.coding') }}</span>
        <span v-if="stamp" class="today-mono">{{ stamp }}</span>
      </span>
      <span v-if="run?.detail" class="tk-card__detail">{{ run.detail }}</span>
    </component>

    <div class="tk-card__acts">
      <q-btn
        v-if="state === 'todo'"
        flat
        dense
        round
        size="sm"
        icon="play_arrow"
        :disable="busy"
        :aria-label="t('taskBoard.actions.startLabel', { title })"
        @click="emit('move', 'doing')"
      >
        <q-tooltip>{{ t('taskBoard.actions.start') }}</q-tooltip>
      </q-btn>
      <q-btn
        v-if="state === 'todo' || state === 'doing'"
        flat
        dense
        round
        size="sm"
        icon="smart_toy"
        :disable="busy"
        :aria-label="t('taskBoard.actions.dispatchLabel', { title })"
      >
        <q-tooltip>{{ t('taskBoard.actions.dispatch') }}</q-tooltip>
        <TaskRecipientMenu @pick="emit('dispatch', $event)" />
      </q-btn>
      <q-btn
        v-if="state === 'doing'"
        flat
        dense
        no-caps
        size="sm"
        class="tk-card__act"
        :label="t('taskBoard.actions.complete')"
        :disable="busy"
        :aria-label="t('today.tasks.complete', { title })"
        @click="emit('complete')"
      />
      <q-btn
        v-if="state === 'review' && task"
        flat
        dense
        no-caps
        size="sm"
        class="tk-card__act tk-card__act--primary"
        :label="t('today.delegated.accept')"
        :disable="busy"
        :aria-label="t('today.delegated.acceptLabel', { title })"
        @click="emit('accept')"
      />
      <q-btn
        v-if="state === 'done'"
        flat
        dense
        no-caps
        size="sm"
        class="tk-card__act"
        :label="t('tasksPage.reopen')"
        :disable="busy"
        :aria-label="t('tasksPage.reopenLabel', { title })"
        @click="emit('move', 'todo')"
      />
      <q-btn
        v-if="moves.length"
        flat
        dense
        round
        size="sm"
        icon="more_horiz"
        :disable="busy"
        :aria-label="t('taskBoard.actions.more', { title })"
      >
        <q-menu anchor="bottom right" self="top right">
          <q-list dense class="tk-moves">
            <q-item-label header>{{ t('taskBoard.actions.moveTo') }}</q-item-label>
            <q-item v-for="col in moves" :key="col" v-close-popup clickable @click="emit('move', col)">
              <q-item-section>{{ t(`taskBoard.col.${col}`) }}</q-item-section>
            </q-item>
          </q-list>
        </q-menu>
      </q-btn>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { OfficeTaskRow } from '../../features/office/types';
import { BOARD_COLUMNS, dueState, moveAction, type BoardColumn, type BoardItem } from '../../features/tasks/board';
import type { Recipient } from '../../features/today/command';
import { clock, dayOrClock, type DelegatedRow } from '../../features/today/logic';
import TaskRecipientMenu from './TaskRecipientMenu.vue';

const props = defineProps<{
  task: OfficeTaskRow | null;
  run?: DelegatedRow | null;
  /** 看板上的所在栏（给「移到…」菜单算可去的栏）；今日页不传 */
  column?: BoardColumn;
  busy?: boolean;
  now: Date;
}>();

const emit = defineEmits<{
  open: [];
  complete: [];
  accept: [];
  move: [to: BoardColumn];
  dispatch: [recipient: Recipient];
}>();

const { t } = useI18n();

const run = computed(() => props.run ?? null);
const title = computed(() => props.task?.title || run.value?.title || '');

/** todo 待办 | doing 我在做 | running 运行中 | waiting 等你回复 | stuck 卡住了 | review 待验收 | done 已完成 */
const state = computed(() => {
  if (run.value) {
    if (run.value.state === 'review') return 'review';
    if (run.value.state === 'stuck') return run.value.waiting ? 'waiting' : 'stuck';
    return 'running';
  }
  const status = props.task?.status;
  return status === 'done' ? 'done' : status === 'doing' ? 'doing' : 'todo';
});

// 状态小标沿用今日页的三色：卡住 / 待验收 / 进行中；待办和已完成用中性色
const badge = computed(() => {
  switch (state.value) {
    case 'stuck':
    case 'waiting':
      return 'stuck';
    case 'review':
      return 'review';
    case 'running':
    case 'doing':
      return 'running';
    default:
      return 'idle';
  }
});

const checkable = computed(() => state.value === 'todo');

const due = computed(() => (props.task && state.value !== 'done' ? dueState(props.task.due_at, props.now) : ''));
const dueLabel = computed(() => {
  const iso = props.task?.due_at ?? '';
  if (due.value === 'today') return t('taskBoard.dueToday', { time: clock(iso) });
  if (due.value === 'overdue') return t('taskBoard.overdue', { when: dayOrClock(iso, props.now) });
  return dayOrClock(iso, props.now);
});

const stamp = computed(() => {
  if (run.value?.at) return dayOrClock(run.value.at, props.now);
  if (state.value === 'done' && props.task) return dayOrClock(props.task.updated_at, props.now);
  return '';
});

// 编码任务没有工作台任务：点了去它自己的页面
const mainTag = computed(() => (!props.task && run.value ? 'router-link' : 'button'));
const mainAttrs = computed(() => (!props.task && run.value ? { to: run.value.to } : { type: 'button' }));

function onMain() {
  if (props.task) emit('open');
}

const moves = computed<BoardColumn[]>(() => {
  if (!props.column || !props.task) return [];
  const item: BoardItem = { key: '', column: props.column, task: props.task, run: run.value };
  return BOARD_COLUMNS.filter((col) => moveAction(item, col) !== null);
});
</script>
