<template>
  <!--
    任务详情（右侧抽屉，两页共用）：生命周期条 → 基本信息（标题 / 说明 / 到期，失焦即存）→ 附件 →
    执行（派给 AI / 当前步骤 / 停止 / 追加指令 / 收回 / 验收 / 退回）→ 关联（来源邮件、日程）。
    改动后通知父级刷新列表；Esc 关闭、焦点回到打开它的地方由 AppDialog 负责。
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
    <div v-if="task" class="tk-detail">
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

      <!-- 基本信息 -->
      <section class="tk-detail__block" :aria-label="t('taskBoard.detail.basic')">
        <q-input
          v-model="form.title"
          dense
          outlined
          maxlength="200"
          :label="t('taskBoard.detail.titleLabel')"
          @blur="saveField('title')"
          @keydown.enter.prevent="saveField('title')"
        />
        <q-input
          v-model="form.body"
          dense
          outlined
          autogrow
          type="textarea"
          maxlength="8000"
          class="tk-detail__body"
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
          <span v-if="due === 'overdue'" class="tk-due tk-due--overdue">{{ t('taskBoard.overdueShort') }}</span>
        </div>
      </section>

      <!-- 附件 -->
      <section class="tk-detail__block" aria-labelledby="tk-detail-files">
        <h3 id="tk-detail-files" class="tk-detail__h">
          {{ t('taskBoard.detail.files') }}
          <span class="today-panel__count">{{ files.length }}</span>
        </h3>
        <div
          class="tk-drop"
          :class="{ 'tk-drop--on': dragging }"
          role="button"
          tabindex="0"
          :aria-label="t('taskBoard.detail.upload')"
          @click="fileEl?.click()"
          @keydown.enter.prevent="fileEl?.click()"
          @keydown.space.prevent="fileEl?.click()"
          @dragenter.prevent="dragging = true"
          @dragover.prevent
          @dragleave="dragging = false"
          @drop.prevent="onDrop"
          @paste="onPaste"
        >
          <q-icon name="cloud_upload" size="20px" aria-hidden="true" />
          <span>{{ t('taskBoard.detail.filesHint') }}</span>
        </div>
        <input
          ref="fileEl"
          type="file"
          multiple
          class="tk-composer__file"
          tabindex="-1"
          aria-hidden="true"
          @change="onPick"
        />
        <ul v-if="files.length || uploads.length" class="tk-files">
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
            <span class="tk-file__meta today-mono" aria-live="polite">{{
              t('taskBoard.detail.uploading', { pct: u.pct })
            }}</span>
          </li>
        </ul>
        <p v-if="fileError" class="today-note today-note--danger" role="alert">{{ fileError }}</p>
      </section>

      <!-- 执行 -->
      <section class="tk-detail__block" aria-labelledby="tk-detail-exec">
        <h3 id="tk-detail-exec" class="tk-detail__h">{{ t('taskBoard.detail.exec') }}</h3>
        <template v-if="!assigned">
          <p class="tk-detail__muted">{{ t('taskBoard.detail.notAssigned') }}</p>
          <q-input
            v-if="state !== 'done'"
            v-model="note"
            dense
            outlined
            autogrow
            type="textarea"
            :label="t('taskBoard.detail.note')"
          />
          <div class="tk-detail__acts">
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
              v-if="state === 'doing'"
              outline
              dense
              no-caps
              icon="undo"
              :label="t('taskBoard.actions.toTodo')"
              :loading="busy === 'status'"
              @click="setStatus('open')"
            />
            <q-btn
              v-if="state !== 'done'"
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
              v-if="state !== 'done'"
              outline
              dense
              no-caps
              icon="check"
              :label="t('taskBoard.actions.complete')"
              :loading="busy === 'status'"
              @click="setStatus('done')"
            />
            <q-btn
              v-else
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
        <template v-else>
          <dl class="tk-detail__facts">
            <div>
              <dt>{{ t('taskBoard.detail.assignee') }}</dt>
              <dd>
                <q-icon :name="task.assignee_kind === 'team' ? 'groups' : 'smart_toy'" size="16px" aria-hidden="true" />
                {{ task.assignee_name || task.assignee_id }}
              </dd>
            </div>
            <div>
              <dt>{{ t('taskBoard.detail.status') }}</dt>
              <dd>
                <span class="today-state" :class="`today-state--${badge}`">
                  <span v-if="state === 'running'" class="tk-live" aria-hidden="true" />
                  {{ t(`taskBoard.state.${state}`) }}
                </span>
              </dd>
            </div>
            <div v-if="run?.detail" class="tk-detail__fact--wide">
              <dt>{{ t('taskBoard.detail.step') }}</dt>
              <dd aria-live="polite">{{ run.detail }}</dd>
            </div>
            <div v-if="run?.at">
              <dt>{{ t('taskBoard.detail.updated') }}</dt>
              <dd class="today-mono">{{ dayOrClock(run.at, now) }}</dd>
            </div>
          </dl>
          <div class="tk-detail__acts">
            <q-btn outline dense no-caps icon="forum" :label="t('taskBoard.detail.openSession')" :to="sessionTo" />
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
              outline
              dense
              no-caps
              icon="person"
              :label="t('taskBoard.detail.takeBack')"
              :loading="busy === 'assign'"
              @click="takeBack"
            />
          </div>
          <form v-if="state !== 'done'" class="tk-detail__follow" @submit.prevent="sendFollow">
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
            />
            <q-btn
              type="submit"
              outline
              dense
              no-caps
              icon="send"
              :label="t(state === 'review' ? 'taskBoard.detail.reworkSend' : 'taskBoard.detail.send')"
              :loading="busy === 'follow'"
              :disable="!follow.trim()"
            />
          </form>
        </template>
      </section>

      <!-- 关联 -->
      <section class="tk-detail__block" aria-labelledby="tk-detail-links">
        <h3 id="tk-detail-links" class="tk-detail__h">{{ t('taskBoard.detail.links') }}</h3>
        <p v-if="task.source === 'mail' && task.source_ref" class="tk-detail__row">
          <q-icon name="mail" size="16px" aria-hidden="true" />
          {{ t('taskBoard.detail.fromMail') }}
          <router-link class="today-link" :to="{ path: '/mail', query: { message: task.source_ref } }">
            {{ t('taskBoard.detail.openMail') }}
          </router-link>
        </p>
        <div class="tk-detail__sub">{{ t('taskBoard.detail.schedule') }}</div>
        <p v-if="eventsFailed" class="today-note">{{ t('taskBoard.detail.scheduleFailed') }}</p>
        <p v-else-if="events.length === 0" class="tk-detail__muted">{{ t('taskBoard.detail.scheduleNone') }}</p>
        <ul v-else class="tk-events">
          <li v-for="ev in events" :key="ev.id" class="today-mono">
            {{ dayOrClock(ev.start_at, now) }} – {{ clock(ev.end_at) }}
            <span class="tk-events__title">{{ ev.title }}</span>
          </li>
        </ul>
        <form class="tk-detail__row tk-detail__row--wrap" @submit.prevent="addSchedule">
          <label class="tk-sr" for="tk-sched-start">{{ t('taskBoard.detail.scheduleStart') }}</label>
          <input id="tk-sched-start" v-model="sched.start" class="tk-input" type="datetime-local" required />
          <span aria-hidden="true">–</span>
          <label class="tk-sr" for="tk-sched-end">{{ t('taskBoard.detail.scheduleEnd') }}</label>
          <input id="tk-sched-end" v-model="sched.end" class="tk-input" type="datetime-local" required />
          <q-btn
            type="submit"
            outline
            dense
            no-caps
            icon="event_available"
            :label="t('taskBoard.detail.scheduleAdd')"
            :loading="busy === 'schedule'"
          />
        </form>
      </section>
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
      <q-btn flat no-caps :label="t('taskBoard.detail.close')" @click="emit('close')" />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppDialog from '../common/AppDialog.vue';
import { confirm } from '../../composables/useAppConfirm';
import { stopGeneration } from '../../features/chat/api';
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
} from '../../features/office/api';
import type { OfficeEventRow, OfficeTaskFileRow, OfficeTaskRow, OfficeTaskStatus } from '../../features/office/types';
import { BOARD_COLUMNS, dueState, fileIcon, fileSize } from '../../features/tasks/board';
import { useTaskDispatch } from '../../features/tasks/useTaskDispatch';
import type { Recipient } from '../../features/today/command';
import { clock, dayOrClock, type DelegatedRow } from '../../features/today/logic';
import { parseKratosApiError } from '../../utils/kratosError';
import TaskRecipientMenu from './TaskRecipientMenu.vue';

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
}>();

const { t } = useI18n();
const { dispatch, sendFollowUp, ensureRecipients } = useTaskDispatch();

const form = reactive({ title: '', body: '', due: '' });
const files = ref<OfficeTaskFileRow[]>([]);
const thumbs = reactive<Record<string, string>>({});
const uploads = ref<{ key: string; name: string; pct: number }[]>([]);
const fileError = ref('');
const events = ref<OfficeEventRow[]>([]);
const eventsFailed = ref(false);
const note = ref('');
const follow = ref('');
const busy = ref<'' | 'status' | 'dispatch' | 'assign' | 'stop' | 'follow' | 'schedule' | 'delete'>('');
const dragging = ref(false);
const fileEl = ref<HTMLInputElement | null>(null);
const sched = reactive({ start: '', end: '' });

const assigned = computed(() => Boolean(props.task?.assignee_kind));

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

const badge = computed(() =>
  state.value === 'stuck' || state.value === 'waiting' ? 'stuck' : state.value === 'review' ? 'review' : 'running',
);

const stepIndex = computed(() => {
  switch (state.value) {
    case 'todo':
      return 0;
    case 'review':
      return 2;
    case 'done':
      return 3;
    default:
      return 1;
  }
});

const due = computed(() => (props.task ? dueState(props.task.due_at, props.now) : ''));

const sessionTo = computed(() => {
  const task = props.task;
  if (!task?.session_id) return '/chat';
  const owner = task.assignee_kind === 'team' ? 'team' : 'agent';
  return { path: '/chat', query: { session: task.session_id, [owner]: task.assignee_id } };
});

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
      const start = new Date(Date.now() + 60 * 60 * 1000);
      start.setMinutes(0, 0, 0);
      sched.start = toLocalInput(start.toISOString());
      sched.end = toLocalInput(new Date(start.getTime() + 60 * 60 * 1000).toISOString());
    }
    void loadFiles(id);
    void loadEvents(id);
    if (task.assignee_kind) void ensureRecipients();
  },
  { immediate: true },
);

// 列表刷新带来新数据时，没在编辑的字段跟着更新
watch(
  () => props.task,
  (task) => {
    if (!task) return;
    const active = document.activeElement as HTMLElement | null;
    if (!active?.closest('.tk-detail__block')) {
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

// ——— 附件 ———

async function upload(list: FileList | File[] | null | undefined) {
  const task = props.task;
  if (!task) return;
  fileError.value = '';
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
    } catch (err) {
      fileError.value = t('taskBoard.composer.uploadFailed', { names: `${file.name}（${errText(err)}）` });
    } finally {
      uploads.value = uploads.value.filter((u) => u !== entry);
    }
  }
  void loadFiles(task.id);
  emit('changed');
}

function onPick(event: Event) {
  const input = event.target as HTMLInputElement;
  void upload(input.files);
  input.value = '';
}

function onDrop(event: DragEvent) {
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

async function setStatus(status: OfficeTaskStatus) {
  const task = props.task;
  if (!task) return;
  busy.value = 'status';
  try {
    await setOfficeTaskStatus(task.id, status);
    if (status === 'done')
      emit(
        'say',
        t(assigned.value ? 'today.delegated.accepted' : 'today.tasks.completed', { title: task.title }),
        false,
      );
    emit('changed');
  } catch (err) {
    emit('say', t('today.tasks.saveFailed', { message: errText(err) }), true);
  } finally {
    busy.value = '';
  }
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

async function takeBack() {
  const task = props.task;
  if (!task) return;
  busy.value = 'assign';
  try {
    await assignOfficeTask(task.id, null);
    emit('say', t('taskBoard.detail.takenBack', { title: task.title }), false);
    emit('changed');
  } catch (err) {
    emit('say', t('today.tasks.saveFailed', { message: errText(err) }), true);
  } finally {
    busy.value = '';
  }
}

async function stop() {
  const sessionId = props.task?.session_id;
  if (!sessionId) return;
  busy.value = 'stop';
  try {
    await stopGeneration(sessionId);
    emit('say', t('taskBoard.detail.stopped'), false);
    emit('changed');
  } catch (err) {
    emit('say', t('taskBoard.detail.stopFailed', { message: errText(err) }), true);
  } finally {
    busy.value = '';
  }
}

async function sendFollow() {
  const task = props.task;
  if (!task || !follow.value.trim()) return;
  const rework = state.value === 'review';
  busy.value = 'follow';
  const error = await sendFollowUp(
    { sessionId: task.session_id, assigneeKind: task.assignee_kind, assigneeId: task.assignee_id },
    rework ? t('taskBoard.detail.reworkPrefix', { text: follow.value.trim() }) : follow.value,
  );
  busy.value = '';
  if (error) return emit('say', t('taskBoard.detail.sendFailed', { message: error }), true);
  follow.value = '';
  emit('say', t(rework ? 'taskBoard.detail.reworkSent' : 'taskBoard.detail.sent', { title: task.title }), false);
  emit('changed');
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
