<template>
  <!--
    任务详情（右侧抽屉，今日页与任务页共用；2026-11-20 第二版版面）：
      顶部   标题（直接改）+ 一行状态（阶段、执行者、到期、附件、更新时间）+ 生命周期条
      执行卡 最先看到「现在怎么样、我能做什么」：
               我的   开始 / 退回待办、派给 AI（可先补一句）、完成
               AI 的  状态与原因、当前步骤；打开会话、停止 / 重试、验收、改派、收回自己做、不再跟进；追加指令 / 退回意见
               已完成 重开
      详情   说明、到期
      附件   列表 + 添加；整块抽屉都能拖文件进来、粘贴截图；派给 AI 后新加的附件同时转给执行者
      关联   来源邮件、排过的日程、排进日程
    底部只放「删除任务」（关闭用右上角 ×，不和右下角的浮窗头像挤在一起）。改动后通知父级重读列表。
  -->
  <AppDialog
    :model-value="open"
    position="right"
    size="md"
    icon="task_alt"
    :title="t('taskBoard.detail.title')"
    :subtitle="t(`taskBoard.state.${state}`)"
    @update:model-value="!$event && emit('close')"
  >
    <div
      v-if="task"
      class="tk-detail"
      :class="{ 'tk-detail--drag': dragging }"
      @dragenter.prevent="onDragEnter"
      @dragover.prevent
      @dragleave="onDragLeave"
      @drop.prevent="onDrop"
      @paste="onPaste"
    >
      <!-- 顶部：标题 + 一行状态 + 生命周期 -->
      <section class="tk-detail__hero">
        <label class="tk-sr" for="tk-detail-title">{{ t('taskBoard.detail.titleLabel') }}</label>
        <input
          id="tk-detail-title"
          v-model="form.title"
          class="tk-detail__title"
          maxlength="200"
          @blur="saveField('title')"
          @keydown.enter.prevent="($event.target as HTMLInputElement).blur()"
        />
        <div class="tk-detail__meta">
          <span class="today-state" :class="`today-state--${badge}`">
            <span v-if="state === 'running'" class="tk-live" aria-hidden="true" />
            {{ t(`taskBoard.state.${state}`) }}
          </span>
          <span v-if="assigned" class="tk-detail__who">
            <q-icon :name="task.assignee_kind === 'team' ? 'groups' : 'smart_toy'" size="14px" aria-hidden="true" />
            {{ task.assignee_name || task.assignee_id }}
          </span>
          <span v-if="due" class="tk-due today-mono" :class="`tk-due--${due}`">
            <q-icon name="event" size="12px" aria-hidden="true" />{{ dueLabel }}
          </span>
          <span v-if="files.length" class="tk-detail__count today-mono">
            <q-icon name="attach_file" size="12px" aria-hidden="true" />{{ files.length }}
          </span>
          <span v-if="updatedAt" class="tk-detail__updated today-mono">
            {{ t('taskBoard.detail.updatedAt', { time: updatedAt }) }}
          </span>
        </div>
        <ol class="tk-steps" :aria-label="t('taskBoard.detail.lifecycle')">
          <li
            v-for="(col, index) in BOARD_COLUMNS"
            :key="col"
            class="tk-steps__item"
            :class="{ 'tk-steps__item--done': index < stepIndex, 'tk-steps__item--on': index === stepIndex }"
            :aria-current="index === stepIndex ? 'step' : undefined"
          >
            {{ t(`taskBoard.col.${col}`) }}
          </li>
        </ol>
      </section>

      <!-- 执行卡 -->
      <section class="tk-exec" :class="`tk-exec--${badge}`" aria-labelledby="tk-detail-exec">
        <h3 id="tk-detail-exec" class="tk-detail__h">
          <q-icon name="play_circle" size="16px" aria-hidden="true" />{{ t('taskBoard.detail.exec') }}
        </h3>

        <!-- 已完成 -->
        <template v-if="state === 'done'">
          <p class="tk-exec__line">{{ t('taskBoard.detail.doneLine', { time: updatedAt }) }}</p>
          <div class="tk-exec__acts">
            <q-btn
              outline
              dense
              no-caps
              icon="replay"
              :label="t('tasksPage.reopen')"
              :loading="busy === 'status'"
              @click="setStatus('open')"
            />
          </div>
        </template>

        <!-- 我的：待办 / 我在做 -->
        <template v-else-if="!assigned">
          <p class="tk-exec__line">
            {{ t(state === 'doing' ? 'taskBoard.detail.mineDoing' : 'taskBoard.detail.notAssigned') }}
          </p>
          <div class="tk-exec__acts">
            <q-btn
              unelevated
              dense
              no-caps
              color="primary"
              text-color="dark"
              icon="smart_toy"
              :label="t('taskBoard.actions.dispatch')"
              :loading="busy === 'dispatch'"
            >
              <TaskRecipientMenu @pick="onDispatch" />
            </q-btn>
            <q-btn
              v-if="state === 'todo'"
              outline
              dense
              no-caps
              icon="play_arrow"
              :label="t('taskBoard.actions.start')"
              :loading="busy === 'status'"
              @click="setStatus('doing')"
            />
            <q-btn
              v-else
              flat
              dense
              no-caps
              icon="undo"
              :label="t('taskBoard.actions.toTodo')"
              :loading="busy === 'status'"
              @click="setStatus('open')"
            />
            <q-btn
              outline
              dense
              no-caps
              icon="check"
              :label="t('taskBoard.actions.complete')"
              :loading="busy === 'status'"
              @click="setStatus('done')"
            />
          </div>
          <q-input
            v-model="note"
            dense
            outlined
            autogrow
            type="textarea"
            class="tk-exec__note"
            :label="t('taskBoard.detail.note')"
          />
        </template>

        <!-- AI 的：在做 / 卡住 / 等你回复 / 待验收 -->
        <template v-else>
          <p class="tk-exec__line">
            <strong>{{ t(`taskBoard.state.${state}`) }}</strong>
            <span v-if="reason"> · {{ reason }}</span>
          </p>
          <blockquote v-if="run?.detail" class="tk-exec__step" aria-live="polite">
            <span class="tk-exec__step-label">{{ t('taskBoard.detail.step') }}</span>
            {{ run.detail }}
          </blockquote>
          <div class="tk-exec__acts">
            <q-btn
              v-if="state === 'review'"
              unelevated
              dense
              no-caps
              color="primary"
              text-color="dark"
              icon="verified"
              :label="t('today.delegated.accept')"
              :loading="busy === 'status'"
              @click="setStatus('done')"
            />
            <q-btn
              :unelevated="state !== 'review'"
              :outline="state === 'review'"
              dense
              no-caps
              :color="state === 'review' ? undefined : 'primary'"
              :text-color="state === 'review' ? undefined : 'dark'"
              icon="forum"
              :label="t('taskBoard.detail.openSession')"
              :to="sessionTo"
            />
            <q-btn
              v-if="state === 'running'"
              outline
              dense
              no-caps
              icon="stop_circle"
              :label="t('taskBoard.detail.stop')"
              :loading="busy === 'stop'"
              @click="stop"
            />
            <q-btn
              v-if="canRetry"
              outline
              dense
              no-caps
              icon="refresh"
              :label="t('taskBoard.detail.retry')"
              :loading="busy === 'retry'"
              @click="retry"
            />
            <q-btn flat dense no-caps icon="more_horiz" :label="t('taskBoard.detail.more')">
              <q-menu anchor="bottom right" self="top right">
                <q-list dense class="tk-moves">
                  <q-item clickable :disable="busy !== ''">
                    <q-item-section avatar><q-icon name="swap_horiz" size="18px" /></q-item-section>
                    <q-item-section>{{ t('taskBoard.detail.reassign') }}</q-item-section>
                    <TaskRecipientMenu @pick="onReassign" />
                  </q-item>
                  <q-item v-close-popup clickable :disable="busy !== ''" @click="takeBack">
                    <q-item-section avatar><q-icon name="person" size="18px" /></q-item-section>
                    <q-item-section>{{ t('taskBoard.detail.takeBack') }}</q-item-section>
                  </q-item>
                  <q-item v-if="state !== 'review'" v-close-popup clickable :disable="busy !== ''" @click="drop">
                    <q-item-section avatar><q-icon name="task_alt" size="18px" /></q-item-section>
                    <q-item-section>{{ t('taskBoard.detail.drop') }}</q-item-section>
                  </q-item>
                </q-list>
              </q-menu>
            </q-btn>
          </div>
          <form class="tk-exec__follow" @submit.prevent="sendFollow">
            <q-input
              v-model="follow"
              dense
              outlined
              autogrow
              type="textarea"
              :label="t(state === 'review' ? 'taskBoard.detail.rework' : 'taskBoard.detail.followUp')"
              :placeholder="
                t(state === 'review' ? 'taskBoard.detail.reworkPlaceholder' : 'taskBoard.detail.followUpPlaceholder')
              "
              @keydown.enter.ctrl.prevent="sendFollow"
            >
              <template #append>
                <!-- 在 q-input 的 append 里原生 submit 不会触发表单提交，直接接 click -->
                <q-btn
                  flat
                  dense
                  round
                  size="sm"
                  icon="send"
                  :aria-label="t(state === 'review' ? 'taskBoard.detail.reworkSend' : 'taskBoard.detail.send')"
                  :loading="busy === 'follow'"
                  :disable="!follow.trim()"
                  @click="sendFollow"
                />
              </template>
            </q-input>
          </form>
        </template>
      </section>

      <!-- 详情：说明、到期 -->
      <section class="tk-detail__block" aria-labelledby="tk-detail-basic">
        <h3 id="tk-detail-basic" class="tk-detail__h">
          <q-icon name="notes" size="16px" aria-hidden="true" />{{ t('taskBoard.detail.basic') }}
        </h3>
        <q-input
          v-model="form.body"
          dense
          outlined
          autogrow
          type="textarea"
          maxlength="8000"
          :label="t('taskBoard.detail.body')"
          :placeholder="t('taskBoard.detail.bodyPlaceholder')"
          @blur="saveField('body')"
        />
        <div class="tk-detail__row">
          <label class="tk-detail__label" for="tk-detail-due">{{ t('taskBoard.detail.due') }}</label>
          <input id="tk-detail-due" v-model="form.due" class="tk-input" type="datetime-local" @change="saveDue" />
          <q-btn
            v-if="form.due"
            flat
            dense
            no-caps
            size="sm"
            :label="t('taskBoard.detail.clearDue')"
            @click="clearDue"
          />
        </div>
      </section>

      <!-- 附件 -->
      <section class="tk-detail__block" aria-labelledby="tk-detail-files">
        <h3 id="tk-detail-files" class="tk-detail__h">
          <q-icon name="attach_file" size="16px" aria-hidden="true" />{{ t('taskBoard.detail.files') }}
          <span class="today-panel__count">{{ files.length }}</span>
          <q-btn
            flat
            dense
            no-caps
            size="sm"
            icon="add"
            class="tk-detail__h-act"
            :label="t('taskBoard.detail.upload')"
            @click="fileEl?.click()"
          />
        </h3>
        <button
          v-if="files.length === 0 && uploads.length === 0"
          type="button"
          class="tk-drop"
          :class="{ 'tk-drop--on': dragging }"
          @click="fileEl?.click()"
        >
          <q-icon name="cloud_upload" size="18px" aria-hidden="true" />
          <span>{{ t('taskBoard.detail.filesHint') }}</span>
        </button>
        <ul v-else class="tk-files">
          <li v-for="f in files" :key="f.id" class="tk-file">
            <img v-if="thumbs[f.id]" class="tk-file__thumb" :src="thumbs[f.id]" alt="" />
            <q-icon
              v-else
              :name="fileIcon(f.filename, f.content_type)"
              size="22px"
              class="tk-file__icon"
              aria-hidden="true"
            />
            <span class="tk-file__name" :title="f.filename">{{ f.filename }}</span>
            <span class="tk-file__meta today-mono">{{ fileSize(f.size) }}</span>
            <q-btn
              flat
              dense
              round
              size="sm"
              icon="download"
              :aria-label="t('taskBoard.detail.download', { name: f.filename })"
              @click="download(f)"
            />
            <q-btn
              flat
              dense
              round
              size="sm"
              icon="delete_outline"
              :aria-label="t('taskBoard.detail.removeFile', { name: f.filename })"
              @click="removeFile(f)"
            />
          </li>
          <li v-for="u in uploads" :key="u.key" class="tk-file tk-file--pending">
            <q-icon name="upload" size="22px" class="tk-file__icon" aria-hidden="true" />
            <span class="tk-file__name">{{ u.name }}</span>
            <span class="tk-file__meta today-mono" aria-live="polite">
              {{ t('taskBoard.detail.uploading', { pct: u.pct }) }}
            </span>
          </li>
        </ul>
        <p v-if="assigned && state !== 'done'" class="tk-detail__muted">{{ t('taskBoard.detail.filesForward') }}</p>
        <input
          ref="fileEl"
          type="file"
          multiple
          class="tk-hidden-input"
          tabindex="-1"
          aria-hidden="true"
          @change="onPick"
        />
        <p v-if="fileError" class="today-note today-note--danger" role="alert">{{ fileError }}</p>
      </section>

      <!-- 关联 -->
      <section class="tk-detail__block" aria-labelledby="tk-detail-links">
        <h3 id="tk-detail-links" class="tk-detail__h">
          <q-icon name="link" size="16px" aria-hidden="true" />{{ t('taskBoard.detail.links') }}
        </h3>
        <p v-if="task.source === 'mail' && task.source_ref" class="tk-detail__row">
          <q-icon name="mail" size="16px" aria-hidden="true" />
          {{ t('taskBoard.detail.fromMail') }}
          <router-link class="today-link" :to="{ path: '/mail', query: { message: task.source_ref } }">
            {{ t('taskBoard.detail.openMail') }}
          </router-link>
        </p>
        <div class="tk-detail__row tk-detail__row--between">
          <span class="tk-detail__sub">
            <q-icon name="event_note" size="16px" aria-hidden="true" />{{ t('taskBoard.detail.schedule') }}
          </span>
          <q-btn
            v-if="!schedOpen"
            flat
            dense
            no-caps
            size="sm"
            icon="event_available"
            :label="t('taskBoard.detail.scheduleAdd')"
            @click="schedOpen = true"
          />
        </div>
        <p v-if="eventsFailed" class="today-note">{{ t('taskBoard.detail.scheduleFailed') }}</p>
        <p v-else-if="events.length === 0 && !schedOpen" class="tk-detail__muted">
          {{ t('taskBoard.detail.scheduleNone') }}
        </p>
        <ul v-if="events.length" class="tk-events">
          <li v-for="ev in events" :key="ev.id" class="today-mono">
            {{ dayOrClock(ev.start_at, now) }} – {{ clock(ev.end_at) }}
            <span class="tk-events__title">{{ ev.title }}</span>
          </li>
        </ul>
        <form v-if="schedOpen" class="tk-detail__row tk-detail__row--wrap" @submit.prevent="addSchedule">
          <label class="tk-sr" for="tk-sched-start">{{ t('taskBoard.detail.scheduleStart') }}</label>
          <input id="tk-sched-start" v-model="sched.start" class="tk-input" type="datetime-local" required />
          <span aria-hidden="true">–</span>
          <label class="tk-sr" for="tk-sched-end">{{ t('taskBoard.detail.scheduleEnd') }}</label>
          <input id="tk-sched-end" v-model="sched.end" class="tk-input" type="datetime-local" required />
          <q-btn
            type="submit"
            unelevated
            dense
            no-caps
            size="sm"
            color="primary"
            text-color="dark"
            :label="t('taskBoard.detail.scheduleOk')"
            :loading="busy === 'schedule'"
          />
          <q-btn flat dense no-caps size="sm" :label="t('taskBoard.detail.cancel')" @click="schedOpen = false" />
        </form>
      </section>

      <p v-if="dragging" class="tk-detail__drop" aria-hidden="true">{{ t('taskBoard.composer.dropHint') }}</p>
    </div>

    <template #actions>
      <q-btn
        flat
        no-caps
        color="negative"
        icon="delete_outline"
        class="tk-detail__delete"
        :label="t('taskBoard.detail.delete')"
        :loading="busy === 'delete'"
        @click="remove"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppDialog from '../../../components/common/AppDialog.vue';
import TaskRecipientMenu from '../../../components/tasks/TaskRecipientMenu.vue';
import { confirm } from '../../../composables/useAppConfirm';
import { parseKratosApiError } from '../../../utils/kratosError';
import { retrySession, stopGeneration } from '../../chat/api';
import {
  assignOfficeTask,
  deleteOfficeTask,
  deleteTaskFile,
  downloadTaskFile,
  fetchTaskFile,
  listTaskEvents,
  listTaskFiles,
  scheduleTask,
  setOfficeTaskStatus,
  TASK_FILE_MAX_BYTES,
  TASK_FILES_MAX,
  updateOfficeTask,
  uploadTaskFile,
} from '../../office/api';
import type { OfficeEventRow, OfficeTaskFileRow, OfficeTaskRow, OfficeTaskStatus } from '../../office/types';
import type { Recipient } from '../../today/command';
import { clock, dayOrClock, type DelegatedRow } from '../../today/logic';
import { BOARD_COLUMNS, dueState, fileIcon, fileSize } from '../board';
import { useTaskDispatch } from '../useTaskDispatch';

const props = defineProps<{
  open: boolean;
  task: OfficeTaskRow | null;
  /** AI 执行进度（派给 AI 的任务才有） */
  run: DelegatedRow | null;
  now: Date;
}>();

const emit = defineEmits<{
  close: [];
  /** 任务变了（或被删），父级重读列表 */
  changed: [];
  say: [text: string, error: boolean, link?: string];
  /** 状态变了的那一行（页面列表里可能已经没有它了，比如今日页不读已完成的） */
  updated: [task: OfficeTaskRow];
}>();

const { t } = useI18n();
const { dispatch, sendFollowUp, sendFiles, ensureRecipients } = useTaskDispatch();

type Busy = '' | 'status' | 'dispatch' | 'assign' | 'stop' | 'retry' | 'follow' | 'schedule' | 'delete';

const form = reactive({ title: '', body: '', due: '' });
const files = ref<OfficeTaskFileRow[]>([]);
const thumbs = reactive<Record<string, string>>({});
const uploads = ref<{ key: string; name: string; pct: number }[]>([]);
const fileError = ref('');
const events = ref<OfficeEventRow[]>([]);
const eventsFailed = ref(false);
const note = ref('');
const follow = ref('');
const busy = ref<Busy>('');
const dragging = ref(false);
const fileEl = ref<HTMLInputElement | null>(null);
const sched = reactive({ start: '', end: '' });
const schedOpen = ref(false);
let dragDepth = 0;

const assigned = computed(() => Boolean(props.task?.assignee_kind));

/** todo 待办 | doing 我在做 | running AI 在做 | waiting 等你回复 | stuck 卡住了 | review 待验收 | done 已完成 */
const state = computed(() => {
  const task = props.task;
  if (!task) return 'todo';
  if (task.status === 'done') return 'done';
  if (assigned.value) {
    if (props.run?.state === 'review') return 'review';
    if (props.run?.state === 'stuck') return props.run.waiting ? 'waiting' : 'stuck';
    return 'running';
  }
  return task.status === 'doing' ? 'doing' : 'todo';
});

const badge = computed(() => {
  switch (state.value) {
    case 'stuck':
    case 'waiting':
      return 'stuck';
    case 'review':
      return 'review';
    case 'todo':
    case 'done':
      return 'idle';
    default:
      return 'running';
  }
});

const stepIndex = computed(() => ({ todo: 0, review: 2, done: 3 })[state.value as 'todo'] ?? 1);

// 卡住的原因：失败 / 被取消 / 等你回复；刚派出去还没有态势的写「刚派出」
const reason = computed(() => {
  const situation = props.run?.situation ?? '';
  if (state.value === 'running' && !situation) return t('taskBoard.detail.reasonFresh');
  if (['failed', 'cancelled', 'waiting', 'queued'].includes(situation))
    return t(`taskBoard.detail.reason.${situation}`);
  return '';
});

// 失败 / 取消的可以重试上一轮（会话说能重试，或明确失败了）
const canRetry = computed(
  () => state.value === 'stuck' && (props.run?.retryAvailable || props.run?.situation === 'failed'),
);

const due = computed(() => (props.task && state.value !== 'done' ? dueState(props.task.due_at, props.now) : ''));
const dueLabel = computed(() => {
  const iso = props.task?.due_at ?? '';
  if (due.value === 'today') return t('taskBoard.dueToday', { time: clock(iso) });
  if (due.value === 'overdue') return t('taskBoard.overdue', { when: dayOrClock(iso, props.now) });
  return dayOrClock(iso, props.now);
});

const updatedAt = computed(() => dayOrClock(props.run?.at || props.task?.updated_at || '', props.now));

const sessionTo = computed(() => {
  const task = props.task;
  if (!task?.session_id) return '/chat';
  const owner = task.assignee_kind === 'team' ? 'team' : 'agent';
  return { path: '/chat', query: { session: task.session_id, [owner]: task.assignee_id } };
});

const runRef = computed(() => ({
  sessionId: props.task?.session_id,
  assigneeKind: props.task?.assignee_kind,
  assigneeId: props.task?.assignee_id,
}));

/** ISO → datetime-local 输入框的本地时间串 */
function toLocalInput(iso: string): string {
  const d = new Date(iso);
  if (!iso || Number.isNaN(d.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function errText(err: unknown): string {
  return parseKratosApiError(err).message || (err instanceof Error ? err.message : '') || 'error';
}

function clearThumbs() {
  for (const key of Object.keys(thumbs)) {
    URL.revokeObjectURL(thumbs[key]!);
    delete thumbs[key];
  }
}
onUnmounted(clearThumbs);

async function loadFiles(taskId: string) {
  try {
    files.value = await listTaskFiles(taskId);
  } catch (err) {
    fileError.value = t('taskBoard.detail.filesFailed', { message: errText(err) });
    return;
  }
  // 图片附件给缩略图（只取 2MB 以内的，免得一打开就拉一堆大图）
  for (const f of files.value) {
    if (!f.content_type.startsWith('image/') || f.size > 2 * 1024 * 1024 || thumbs[f.id]) continue;
    fetchTaskFile(f.id)
      .then((blob) => {
        if (props.task?.id === taskId) thumbs[f.id] = URL.createObjectURL(blob);
      })
      .catch(() => undefined);
  }
}

async function loadEvents(taskId: string) {
  eventsFailed.value = false;
  try {
    events.value = await listTaskEvents(taskId);
  } catch {
    events.value = [];
    eventsFailed.value = true;
  }
}

function resetSchedule() {
  const start = new Date(Date.now() + 60 * 60 * 1000);
  start.setMinutes(0, 0, 0);
  sched.start = toLocalInput(start.toISOString());
  sched.end = toLocalInput(new Date(start.getTime() + 60 * 60 * 1000).toISOString());
}

// 换了任务（或打开）时重置表单并读附件、日程
watch(
  () => [props.open, props.task?.id] as const,
  ([open, id], prev) => {
    if (!open || !id) return;
    const task = props.task!;
    form.title = task.title;
    form.body = task.body;
    form.due = toLocalInput(task.due_at);
    if (prev?.[1] !== id) {
      clearThumbs();
      files.value = [];
      events.value = [];
      note.value = '';
      follow.value = '';
      fileError.value = '';
      schedOpen.value = false;
      resetSchedule();
    }
    void loadFiles(id);
    void loadEvents(id);
    void ensureRecipients();
  },
  { immediate: true },
);

// 列表刷新带来新数据时，没在编辑的字段跟着更新
watch(
  () => props.task,
  (task) => {
    if (!task) return;
    const active = document.activeElement as HTMLElement | null;
    if (!active?.closest('.tk-detail')) {
      form.title = task.title;
      form.body = task.body;
      form.due = toLocalInput(task.due_at);
    }
  },
);

async function saveField(field: 'title' | 'body') {
  const task = props.task;
  if (!task) return;
  const value = form[field].trim();
  if (value === task[field].trim()) return;
  if (field === 'title' && !value) {
    form.title = task.title;
    return;
  }
  try {
    await updateOfficeTask(task.id, { [field]: value });
    emit('say', t('taskBoard.detail.saved'), false);
    emit('changed');
  } catch (err) {
    emit('say', t('today.tasks.saveFailed', { message: errText(err) }), true);
  }
}

async function saveDue() {
  const task = props.task;
  if (!task || !form.due) return;
  try {
    await updateOfficeTask(task.id, { dueAt: new Date(form.due).toISOString() });
    emit('changed');
  } catch (err) {
    emit('say', t('today.tasks.saveFailed', { message: errText(err) }), true);
  }
}

async function clearDue() {
  const task = props.task;
  if (!task) return;
  form.due = '';
  try {
    await updateOfficeTask(task.id, { dueAt: null });
    emit('changed');
  } catch (err) {
    emit('say', t('today.tasks.saveFailed', { message: errText(err) }), true);
  }
}

// ——— 附件（派给 AI 后新加的，同时转给执行者）———

async function upload(list: FileList | File[] | null | undefined) {
  const task = props.task;
  if (!task) return;
  fileError.value = '';
  const done: File[] = [];
  for (const file of Array.from(list ?? [])) {
    if (files.value.length + uploads.value.length >= TASK_FILES_MAX) {
      fileError.value = t('taskBoard.composer.tooMany', { n: TASK_FILES_MAX });
      break;
    }
    if (file.size > TASK_FILE_MAX_BYTES) {
      fileError.value = t('taskBoard.composer.tooLarge', { name: file.name });
      continue;
    }
    const entry = reactive({ key: `${Date.now()}:${file.name}`, name: file.name, pct: 0 });
    uploads.value.push(entry);
    try {
      const row = await uploadTaskFile(task.id, file, (ratio) => (entry.pct = Math.round(ratio * 100)));
      files.value.push(row);
      done.push(file);
    } catch (err) {
      fileError.value = t('taskBoard.composer.uploadFailed', { names: `${file.name}（${errText(err)}）` });
    } finally {
      uploads.value = uploads.value.filter((u) => u !== entry);
    }
  }
  if (done.length && assigned.value && state.value !== 'done') {
    const error = await sendFiles(runRef.value, done);
    emit(
      'say',
      error ? t('taskBoard.detail.forwardFailed', { message: error }) : t('taskBoard.detail.forwarded'),
      !!error,
    );
  }
  void loadFiles(task.id);
  emit('changed');
}

function onPick(event: Event) {
  const input = event.target as HTMLInputElement;
  void upload(input.files);
  input.value = '';
}

function onDragEnter(event: DragEvent) {
  if (!event.dataTransfer?.types.includes('Files')) return;
  dragDepth += 1;
  dragging.value = true;
}

function onDragLeave() {
  dragDepth = Math.max(0, dragDepth - 1);
  if (!dragDepth) dragging.value = false;
}

function onDrop(event: DragEvent) {
  dragDepth = 0;
  dragging.value = false;
  void upload(event.dataTransfer?.files);
}

function onPaste(event: ClipboardEvent) {
  const pasted = Array.from(event.clipboardData?.files ?? []);
  if (pasted.length) {
    event.preventDefault();
    void upload(pasted);
  }
}

async function download(f: OfficeTaskFileRow) {
  try {
    await downloadTaskFile(f);
  } catch (err) {
    fileError.value = errText(err);
  }
}

async function removeFile(f: OfficeTaskFileRow) {
  const ok = await confirm({
    title: t('taskBoard.detail.removeFileTitle'),
    message: t('taskBoard.detail.removeFileConfirm', { name: f.filename }),
    tone: 'danger',
    confirmLabel: t('taskBoard.detail.removeFileOk'),
  });
  if (!ok) return;
  try {
    await deleteTaskFile(f.id);
    files.value = files.value.filter((x) => x.id !== f.id);
    emit('changed');
  } catch (err) {
    fileError.value = errText(err);
  }
}

// ——— 执行 ———

async function act(kind: Busy, action: () => Promise<string | void>, okText = '') {
  busy.value = kind;
  try {
    const error = await action();
    if (error) emit('say', error, true);
    else {
      if (okText) emit('say', okText, false);
      emit('changed');
    }
  } catch (err) {
    emit('say', t('today.tasks.saveFailed', { message: errText(err) }), true);
  } finally {
    busy.value = '';
  }
}

function setStatus(status: OfficeTaskStatus) {
  const task = props.task;
  if (!task) return;
  const text =
    status === 'done'
      ? t(assigned.value ? 'today.delegated.accepted' : 'today.tasks.completed', { title: task.title })
      : '';
  return act('status', async () => void emit('updated', await setOfficeTaskStatus(task.id, status)), text);
}

/** 不再跟进：卡住 / 在做的 AI 任务直接结掉（会话不动） */
function drop() {
  const task = props.task;
  if (!task) return;
  return act(
    'status',
    async () => void emit('updated', await setOfficeTaskStatus(task.id, 'done')),
    t('today.delegated.dropped', { title: task.title }),
  );
}

async function onDispatch(recipient: Recipient) {
  const task = props.task;
  if (!task) return;
  busy.value = 'dispatch';
  const outcome = await dispatch({ ...task, file_count: files.value.length }, recipient, note.value);
  busy.value = '';
  if (outcome.ok) {
    note.value = '';
    emit(
      'say',
      outcome.task ? t('taskBoard.dispatch.ok', { name: recipient.name }) : t('taskBoard.dispatch.notRecorded'),
      !outcome.task,
      outcome.link,
    );
    emit('changed');
  } else {
    emit('say', t('taskBoard.dispatch.failed', { message: outcome.message }), true, outcome.link);
  }
}

/** 改派：在跑的先停下，再开新会话派给另一个执行者（原会话留着可查） */
async function onReassign(recipient: Recipient) {
  if (state.value === 'running' && props.task?.session_id) {
    await stopGeneration(props.task.session_id).catch(() => false);
  }
  await onDispatch(recipient);
}

function takeBack() {
  const task = props.task;
  if (!task) return;
  return act(
    'assign',
    async () => void emit('updated', await assignOfficeTask(task.id, null)),
    t('taskBoard.detail.takenBack', { title: task.title }),
  );
}

function stop() {
  const sessionId = props.task?.session_id;
  if (!sessionId) return;
  return act(
    'stop',
    async () => ((await stopGeneration(sessionId)) ? '' : t('taskBoard.detail.stopNothing')),
    t('taskBoard.detail.stopped'),
  );
}

/**
 * 重试：先让会话重跑上一轮；会话里没有可重跑的回合（比如执行者还没接手就失败了）时，
 * 把任务说明在同一个会话里再发一遍，效果一样是「重新开始」。
 */
function retry() {
  const task = props.task;
  const sessionId = task?.session_id;
  if (!task || !sessionId) return;
  return act(
    'retry',
    async () => {
      const retried = await retrySession(sessionId).catch(() => false);
      if (retried) return;
      const error = await sendFollowUp(runRef.value, [task.title, task.body].filter((s) => s.trim()).join('\n\n'));
      if (error) return t('taskBoard.detail.retryFailed', { message: error });
    },
    t('taskBoard.detail.retried'),
  );
}

async function sendFollow() {
  const task = props.task;
  if (!task || !follow.value.trim() || busy.value) return;
  const rework = state.value === 'review';
  const text = rework ? t('taskBoard.detail.reworkPrefix', { text: follow.value.trim() }) : follow.value;
  await act(
    'follow',
    async () => {
      const error = await sendFollowUp(runRef.value, text);
      if (error) return t('taskBoard.detail.sendFailed', { message: error });
      follow.value = '';
    },
    t(rework ? 'taskBoard.detail.reworkSent' : 'taskBoard.detail.sent', { title: task.title }),
  );
}

// ——— 关联 ———

async function addSchedule() {
  const task = props.task;
  if (!task || !sched.start || !sched.end) return;
  const start = new Date(sched.start);
  const end = new Date(sched.end);
  if (!(end > start)) return emit('say', t('taskBoard.detail.scheduleOrder'), true);
  busy.value = 'schedule';
  try {
    await scheduleTask(task, start.toISOString(), end.toISOString());
    emit('say', t('taskBoard.detail.scheduled'), false);
    schedOpen.value = false;
    resetSchedule();
    void loadEvents(task.id);
  } catch (err) {
    const conflict = parseKratosApiError(err).status === 409;
    emit(
      'say',
      conflict ? t('taskBoard.detail.conflict') : t('today.tasks.saveFailed', { message: errText(err) }),
      true,
    );
  } finally {
    busy.value = '';
  }
}

async function remove() {
  const task = props.task;
  if (!task) return;
  const ok = await confirm({
    title: t('taskBoard.detail.delete'),
    message: t('taskBoard.detail.deleteConfirm', { title: task.title }),
    tone: 'danger',
    confirmLabel: t('taskBoard.detail.deleteOk'),
  });
  if (!ok) return;
  busy.value = 'delete';
  try {
    await deleteOfficeTask(task.id);
    emit('say', t('taskBoard.detail.deleted', { title: task.title }), false);
    emit('changed');
    emit('close');
  } catch (err) {
    emit('say', t('today.tasks.saveFailed', { message: errText(err) }), true);
  } finally {
    busy.value = '';
  }
}
</script>
