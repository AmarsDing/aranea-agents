<template>
  <!--
    任务输入框（沉在「我的任务」底部）：写一句话回车添加；Shift+Enter 换行。
    文件三种加法：回形针点选、拖进框里、粘贴截图。写「@某 Agent …」或「让某某…」= 建好立刻派给它；
    「记：…」= 随手记录。附件跟着任务存，派给 AI 时一起带过去。
  -->
  <form
    class="tk-composer"
    :class="{ 'tk-composer--drag': dragging, 'tk-composer--busy': busy }"
    @submit.prevent="submit"
    @dragenter.prevent="onDragEnter"
    @dragover.prevent
    @dragleave="onDragLeave"
    @drop.prevent="onDrop"
  >
    <textarea
      ref="inputEl"
      v-model="text"
      class="tk-composer__input"
      rows="1"
      maxlength="2000"
      :placeholder="t('taskBoard.composer.placeholder')"
      :aria-label="t('taskBoard.composer.label')"
      :disabled="busy"
      @keydown.enter="onEnter"
      @paste="onPaste"
      @focus="ensureRecipients"
      @input="autosize"
    />

    <ul
      v-if="files.length"
      class="tk-composer__files"
      :aria-label="t('taskBoard.composer.pending', { n: files.length })"
    >
      <li v-for="(f, index) in files" :key="`${f.name}:${index}`" class="tk-chip">
        <q-icon :name="fileIcon(f.name, f.type)" size="14px" aria-hidden="true" />
        <span class="tk-chip__name">{{ f.name }}</span>
        <span class="tk-chip__meta today-mono">{{ fileSize(f.size) }}</span>
        <button
          type="button"
          class="tk-chip__x"
          :aria-label="t('taskBoard.composer.remove', { name: f.name })"
          @click="files.splice(index, 1)"
        >
          <q-icon name="close" size="14px" aria-hidden="true" />
        </button>
      </li>
    </ul>

    <div class="tk-composer__bar">
      <q-btn
        flat
        dense
        round
        size="sm"
        icon="attach_file"
        class="tk-composer__tool"
        :aria-label="t('taskBoard.composer.attach')"
        :disable="busy"
        @click="fileEl?.click()"
      >
        <q-tooltip>{{ t('taskBoard.composer.attach') }}</q-tooltip>
      </q-btn>
      <label class="tk-composer__due" :class="{ 'tk-composer__due--set': due }">
        <q-icon name="event" size="16px" aria-hidden="true" />
        <span class="tk-sr">{{ t('taskBoard.composer.due') }}</span>
        <input v-model="due" type="datetime-local" :disabled="busy" :aria-label="t('taskBoard.composer.due')" />
      </label>
      <span class="tk-composer__hint" aria-hidden="true">{{ hint }}</span>
      <q-btn
        type="submit"
        unelevated
        dense
        no-caps
        color="primary"
        text-color="dark"
        class="tk-composer__send"
        :icon="intent === 'delegate' ? 'smart_toy' : 'add'"
        :label="t(intent === 'delegate' ? 'taskBoard.composer.sendDelegate' : 'taskBoard.composer.send')"
        :loading="busy"
        :disable="!text.trim()"
      />
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
    <p v-if="error" class="today-note today-note--danger" role="alert">{{ error }}</p>
    <p v-if="dragging" class="tk-composer__drop" aria-hidden="true">{{ t('taskBoard.composer.dropHint') }}</p>
  </form>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { TASK_FILE_MAX_BYTES, TASK_FILES_MAX, uploadTaskFile } from '../../features/office/api';
import type { OfficeTaskRow } from '../../features/office/types';
import { fileIcon, fileSize } from '../../features/tasks/board';
import { useTaskDispatch } from '../../features/tasks/useTaskDispatch';
import { parseCommand, recipientRef } from '../../features/today/command';
import { parseKratosApiError } from '../../utils/kratosError';

const props = defineProps<{
  /** 新建一条我的任务 */
  add: (title: string, dueAt?: string) => Promise<{ error: string; task: OfficeTaskRow | null }>;
  /** 记一笔随手记录；返回错误文案，成功返回空串 */
  addNote: (text: string) => Promise<string>;
}>();

const emit = defineEmits<{
  /** 建好（或派出）了一条任务，附件也传完了：父级据此刷新列表 */
  created: [task: OfficeTaskRow];
  /** 一句话的结果（成功 / 部分失败），由父级浮在右上角 */
  say: [text: string, error: boolean, link?: string];
}>();

const { t } = useI18n();
const { recipients, ensureRecipients, dispatch } = useTaskDispatch();

const text = ref('');
const due = ref('');
const files = ref<File[]>([]);
const busy = ref(false);
const error = ref('');
const dragging = ref(false);
const inputEl = ref<HTMLTextAreaElement | null>(null);
const fileEl = ref<HTMLInputElement | null>(null);
let dragDepth = 0;

const parsed = computed(() => parseCommand(text.value, recipients.value));
const intent = computed(() =>
  parsed.value.recipientRef ? 'delegate' : parsed.value.intent === 'note' ? 'note' : 'task',
);
const hint = computed(() => {
  if (intent.value === 'delegate') {
    const r = recipients.value.find((x) => recipientRef(x) === parsed.value.recipientRef);
    return t('taskBoard.composer.willDelegate', { name: r?.name ?? '' });
  }
  if (intent.value === 'note') return t('taskBoard.composer.willNote');
  return t('taskBoard.composer.keys');
});

function autosize() {
  const el = inputEl.value;
  if (!el) return;
  el.style.height = 'auto';
  el.style.height = `${Math.min(el.scrollHeight, 120)}px`;
}

function addFiles(list: FileList | File[] | null | undefined) {
  error.value = '';
  for (const f of Array.from(list ?? [])) {
    if (files.value.length >= TASK_FILES_MAX) {
      error.value = t('taskBoard.composer.tooMany', { n: TASK_FILES_MAX });
      break;
    }
    if (f.size > TASK_FILE_MAX_BYTES) {
      error.value = t('taskBoard.composer.tooLarge', { name: f.name });
      continue;
    }
    files.value.push(f);
  }
}

function onPick(event: Event) {
  const input = event.target as HTMLInputElement;
  addFiles(input.files);
  input.value = '';
}

function onPaste(event: ClipboardEvent) {
  const pasted = Array.from(event.clipboardData?.files ?? []);
  if (!pasted.length) return;
  event.preventDefault();
  // 截图粘贴进来没有像样的文件名：按时间起一个
  addFiles(
    pasted.map((f) =>
      f.name && f.name !== 'image.png'
        ? f
        : new File([f], `paste-${Date.now()}.${f.type.split('/')[1] || 'png'}`, { type: f.type }),
    ),
  );
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
  addFiles(event.dataTransfer?.files);
  inputEl.value?.focus();
}

function onEnter(event: KeyboardEvent) {
  if (event.shiftKey || event.isComposing) return;
  event.preventDefault();
  void submit();
}

async function submit() {
  const raw = text.value.trim();
  if (!raw || busy.value) return;
  busy.value = true;
  error.value = '';
  try {
    if (intent.value === 'note') {
      const err = await props.addNote(parsed.value.text || raw);
      if (err) error.value = t('today.tasks.saveFailed', { message: err });
      else reset();
      return;
    }
    const recipient = recipients.value.find((x) => recipientRef(x) === parsed.value.recipientRef);
    const title = (recipient ? parsed.value.text : raw) || raw;
    const dueISO = due.value ? new Date(due.value).toISOString() : '';
    const { error: addError, task } = await props.add(title, dueISO);
    if (!task) {
      error.value = t('today.tasks.saveFailed', { message: addError || 'error' });
      return;
    }
    const failed: string[] = [];
    for (const f of files.value) {
      try {
        await uploadTaskFile(task.id, f);
      } catch (err) {
        failed.push(`${f.name}（${parseKratosApiError(err).message || 'error'}）`);
      }
    }
    const created = { ...task, file_count: files.value.length - failed.length };
    reset();
    if (failed.length) emit('say', t('taskBoard.composer.uploadFailed', { names: failed.join('、') }), true);
    if (recipient) {
      const outcome = await dispatch(created, recipient);
      if (outcome.ok) emit('say', t('taskBoard.dispatch.ok', { name: recipient.name }), false, outcome.link);
      else emit('say', t('taskBoard.dispatch.failed', { message: outcome.message }), true, outcome.link);
    }
    emit('created', created);
  } finally {
    busy.value = false;
  }
}

function reset() {
  text.value = '';
  due.value = '';
  files.value = [];
  requestAnimationFrame(autosize);
}

defineExpose({ focus: () => inputEl.value?.focus(), addFiles });
</script>
