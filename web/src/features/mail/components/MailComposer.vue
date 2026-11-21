<template>
  <!-- 写信 / 回复：从阅读区右下角升起，不离开当前邮件，可以对照着写（09-邮件.md §3.1） -->
  <section
    class="ml-cmp"
    role="dialog"
    :aria-label="heading"
    @keydown.esc.stop="close"
    @keydown.ctrl.enter.prevent="send"
    @keydown.meta.enter.prevent="send"
  >
    <header class="ml-cmp__head">
      <q-icon :name="kindIcon" size="18px" aria-hidden="true" />
      <span class="ml-cmp__title">{{ heading }}</span>
      <span v-if="savedAt" class="ml-cmp__saved ml-num">{{ t('mailPage.draftSavedAt', { time: savedAt }) }}</span>
      <span class="ml-sp" />
      <q-btn flat dense round icon="close" :aria-label="t('mailPage.close')" @click="close" />
    </header>

    <q-banner v-if="!sendable.length" rounded class="app-page-error-banner q-ma-sm" role="alert">
      {{ t('mailPage.noSendAccount') }}
    </q-banner>
    <q-banner v-if="error" rounded class="app-page-error-banner q-ma-sm" role="alert">{{ error }}</q-banner>

    <div class="ml-cmp__rows">
      <label class="ml-cmp__row">
        <span class="ml-cmp__lab">{{ t('mailPage.from') }}</span>
        <q-select
          v-model="accountId"
          class="ml-cmp__field"
          borderless
          dense
          emit-value
          map-options
          :options="accountOptions"
          :aria-label="t('mailPage.from')"
        />
      </label>
      <label class="ml-cmp__row">
        <span class="ml-cmp__lab">{{ t('mailPage.to') }}</span>
        <MailAddressInput
          ref="toInput"
          v-model="to"
          :placeholder="t('mailPage.toPlaceholder')"
          :label="t('mailPage.to')"
        />
        <q-btn v-if="!showCc" flat dense no-caps size="sm" :label="t('mailPage.addCc')" @click="showCc = true" />
      </label>
      <label v-if="showCc" class="ml-cmp__row">
        <span class="ml-cmp__lab">{{ t('mailPage.cc') }}</span>
        <MailAddressInput v-model="cc" :label="t('mailPage.cc')" />
      </label>
      <label class="ml-cmp__row">
        <span class="ml-cmp__lab">{{ t('mailPage.subject') }}</span>
        <q-input v-model="subject" class="ml-cmp__field" borderless dense :aria-label="t('mailPage.subject')" />
      </label>
    </div>

    <q-editor
      v-if="rich"
      v-model="html"
      class="ml-cmp__rich"
      flat
      min-height="12rem"
      :toolbar="[
        ['bold', 'italic', 'underline', 'strike'],
        ['unordered', 'ordered', 'quote'],
        ['link', 'removeFormat'],
      ]"
      :placeholder="t('mailPage.bodyPlaceholder')"
    />
    <textarea
      v-else
      ref="bodyInput"
      v-model="body"
      class="ml-cmp__body"
      :aria-label="t('mailPage.body')"
      :placeholder="t('mailPage.bodyPlaceholder')"
    />

    <div v-if="carried.length || files.length" class="ml-cmp__atts">
      <span v-for="a in carried" :key="a.id" class="ml-chip" :title="t('mailPage.carriedHint')">
        <q-icon name="attach_file" size="14px" aria-hidden="true" />{{ a.filename }}
        <span class="ml-num">{{ fileSize(a.size) }}</span>
      </span>
      <span v-for="(f, i) in files" :key="`${f.name}-${i}`" class="ml-chip">
        <q-icon name="attach_file" size="14px" aria-hidden="true" />{{ f.name }}
        <span class="ml-num">{{ fileSize(f.size) }}</span>
        <q-btn
          flat
          dense
          round
          size="xs"
          icon="close"
          :aria-label="t('mailPage.removeFile', { name: f.name })"
          @click="files.splice(i, 1)"
        />
      </span>
    </div>

    <!-- 撤销发送：点了发送先等几秒，期间可以撤回（Gmail 的做法，代替原来的二次确认框） -->
    <div v-if="undoLeft > 0" class="ml-cmp__undo" role="status" aria-live="polite">
      <q-icon name="schedule_send" size="18px" aria-hidden="true" />
      <span>{{ t('mailPage.sendingIn', { n: undoLeft }) }}</span>
      <span class="ml-sp" />
      <q-btn flat dense no-caps color="primary" :label="t('mailPage.undoSend')" @click="cancelSend" />
    </div>
    <div v-if="scheduleOpen" class="ml-cmp__sched">
      <span class="ml-hint">{{ t('mailPage.scheduleAt') }}</span>
      <q-btn
        v-for="p in schedulePresets"
        :key="p.label"
        flat
        dense
        no-caps
        size="sm"
        :label="p.label"
        @click="scheduleAt = p.value"
      />
      <input v-model="scheduleAt" type="datetime-local" class="ml-cmp__dt" :aria-label="t('mailPage.scheduleAt')" />
      <q-btn
        unelevated
        dense
        no-caps
        color="primary"
        :label="t('mailPage.scheduleConfirm')"
        :disable="!scheduleValid || !canSend"
        :loading="sending"
        @click="sendScheduled"
      />
      <q-btn flat dense round size="sm" icon="close" :aria-label="t('mailPage.close')" @click="scheduleOpen = false" />
    </div>

    <footer class="ml-cmp__foot">
      <q-btn-dropdown
        split
        unelevated
        no-caps
        color="primary"
        icon="send"
        :label="t('mailPage.send')"
        :loading="sending && undoLeft === 0"
        :disable="!canSend || undoLeft > 0"
        @click="send"
      >
        <q-list dense>
          <q-item v-close-popup clickable @click="openSchedule">
            <q-item-section avatar><q-icon name="schedule_send" size="18px" /></q-item-section>
            <q-item-section>{{ t('mailPage.scheduleSend') }}</q-item-section>
          </q-item>
        </q-list>
      </q-btn-dropdown>
      <q-btn flat no-caps :label="t('mailPage.saveDraft')" :loading="saving" :disable="!accountId" @click="saveDraft" />
      <q-btn flat dense round icon="attach_file" :aria-label="t('mailPage.attach')" @click="filePicker?.click()">
        <q-tooltip>{{ t('mailPage.attach') }}</q-tooltip>
      </q-btn>
      <q-btn
        flat
        dense
        round
        icon="folder_open"
        :aria-label="t('mailPage.attachLibrary')"
        @click="libPickerOpen = true"
      >
        <q-tooltip>{{ t('mailPage.attachLibrary') }}</q-tooltip>
      </q-btn>
      <q-btn
        flat
        dense
        round
        icon="format_color_text"
        :color="rich ? 'primary' : undefined"
        :aria-pressed="rich"
        :aria-label="t('mailPage.richText')"
        @click="toggleRich"
      >
        <q-tooltip>{{ t('mailPage.richText') }}</q-tooltip>
      </q-btn>
      <input ref="filePicker" type="file" multiple class="ml-sr" @change="onFiles" />
      <span class="ml-sp" />
      <span class="ml-hint">{{ t('mailPage.sendHint') }}</span>
    </footer>
    <LibraryFilePicker v-model="libPickerOpen" @picked="addFiles" />
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { confirm } from '../../../composables/useAppConfirm';
import { saveMailDraft, sendMail } from '../../office/api';
import LibraryFilePicker from '../../library/components/LibraryFilePicker.vue';
import MailAddressInput from './MailAddressInput.vue';
import type { MailAccountRow, MailAttachmentRow } from '../../office/types';
import {
  fileSize,
  fullTime,
  parseAddr,
  prefixedSubject,
  quotedOriginal,
  replyRecipients,
  splitAddrs,
  type ComposeSeed,
} from '../model';

const props = defineProps<{ seed: ComposeSeed; accounts: MailAccountRow[]; defaultAccountId: string }>();
const emit = defineEmits<{ close: []; changed: [kind: 'sent' | 'saved'] }>();

const { t } = useI18n();
const $q = useQuasar();

const MAX_TOTAL = 25 * 1024 * 1024;

const sendable = computed(() => props.accounts.filter((a) => a.smtp_host));
const accountOptions = computed(() =>
  props.accounts.map((a) => ({
    value: a.id,
    label: a.email ? `${a.name || a.email} · ${a.email}` : a.name || a.username,
    disable: !a.smtp_host,
  })),
);

const msg = props.seed.message;
const kind = props.seed.kind;
const mine = props.accounts.map((a) => (a.email || a.username).toLowerCase());
const pickAccount = () => {
  const preferred = msg?.account_id && props.accounts.find((a) => a.id === msg.account_id && a.smtp_host);
  return (
    (preferred ? preferred.id : '') ||
    sendable.value.find((a) => a.id === props.defaultAccountId)?.id ||
    sendable.value[0]?.id ||
    ''
  );
};

const accountId = ref(pickAccount());
const to = ref('');
const cc = ref('');
const subject = ref('');
const body = ref('');
const files = ref<File[]>([]);
const carried = ref<MailAttachmentRow[]>(props.seed.carried ?? []);
const draftId = ref(kind === 'draft' && msg ? msg.id : '');
const sending = ref(false);
const saving = ref(false);
const error = ref('');
const savedAt = ref('');
const showCc = ref(false);
const rich = ref(false);
const html = ref('');
const libPickerOpen = ref(false);
let savedSnapshot = '';

if (msg && kind === 'draft') {
  to.value = msg.to;
  cc.value = msg.cc;
  subject.value = msg.subject;
  body.value = props.seed.body ?? '';
} else if (msg) {
  const r = replyRecipients(msg, kind, mine);
  to.value = r.to;
  cc.value = r.cc;
  subject.value = prefixedSubject(msg.subject, kind);
  body.value = quotedOriginal(msg, props.seed.body ?? '', {
    header: t('mailPage.quoteHeader'),
    from: t('mailPage.from'),
    to: t('mailPage.to'),
    time: t('mailPage.quoteTime'),
    subject: t('mailPage.subject'),
  });
}
showCc.value = !!cc.value;
if (kind !== 'draft') {
  const sig = props.accounts.find((a) => a.id === accountId.value)?.signature?.trim();
  // 新写的放最后；回复 / 转发放在引用原文前面
  if (sig) body.value = msg ? `\n\n-- \n${sig}${body.value}` : `${body.value}\n\n-- \n${sig}`;
}
// 草稿是带格式的：用格式编辑器接着写
if (kind === 'draft' && props.seed.bodyHtml) {
  rich.value = true;
  html.value = props.seed.bodyHtml;
}

const snapshot = () =>
  JSON.stringify([accountId.value, to.value, cc.value, subject.value, body.value, html.value, files.value.length]);
savedSnapshot = snapshot();

const heading = computed(
  () =>
    ({
      new: t('mailPage.composeNew'),
      reply: t('mailPage.composeReply', { subject: msg?.subject ?? '' }),
      replyAll: t('mailPage.composeReply', { subject: msg?.subject ?? '' }),
      forward: t('mailPage.composeForward', { subject: msg?.subject ?? '' }),
      draft: t('mailPage.composeDraft'),
    })[kind],
);
const kindIcon = computed(
  () => ({ new: 'edit', reply: 'reply', replyAll: 'reply_all', forward: 'forward', draft: 'drafts' })[kind],
);

const recipients = computed(() => [...splitAddrs(to.value), ...splitAddrs(cc.value)]);
const canSend = computed(() => !!accountId.value && splitAddrs(to.value).length > 0 && !sending.value);

const toInput = ref<InstanceType<typeof MailAddressInput> | null>(null);
const bodyInput = ref<HTMLTextAreaElement | null>(null);
const filePicker = ref<HTMLInputElement | null>(null);

onMounted(() => {
  void nextTick(() => {
    if (!to.value) toInput.value?.focus();
    else {
      bodyInput.value?.focus();
      bodyInput.value?.setSelectionRange(0, 0);
    }
  });
});

function onFiles(ev: Event) {
  const input = ev.target as HTMLInputElement;
  const picked = Array.from(input.files ?? []);
  input.value = '';
  addFiles(picked);
}

function addFiles(picked: File[]) {
  const total =
    [...files.value, ...picked].reduce((n, f) => n + f.size, 0) + carried.value.reduce((n, a) => n + a.size, 0);
  if (total > MAX_TOTAL) {
    error.value = t('mailPage.tooLarge');
    return;
  }
  files.value.push(...picked);
}

/** 纯文本 ↔ 带格式切换：内容跟着转过去。 */
function toggleRich() {
  if (rich.value) {
    body.value = htmlToText(html.value);
  } else {
    html.value = textToHtml(body.value);
  }
  rich.value = !rich.value;
}

function textToHtml(text: string): string {
  const d = document.createElement('div');
  d.textContent = text;
  return d.innerHTML.replace(/\n/g, '<br>');
}

function htmlToText(h: string): string {
  const d = document.createElement('div');
  d.innerHTML = h.replace(/<br\s*\/?>/gi, '\n').replace(/<\/(p|div|li|blockquote)>/gi, '\n');
  return (d.textContent ?? '').replace(/\n{3,}/g, '\n\n').trim();
}

function input(sendAt = '') {
  return {
    account_id: accountId.value,
    to: to.value,
    cc: cc.value,
    subject: subject.value,
    body: rich.value ? htmlToText(html.value) : body.value,
    body_html: rich.value ? html.value : '',
    send_at: sendAt,
    reply_to_id: msg && (kind === 'reply' || kind === 'replyAll') ? msg.id : '',
    forward_of_id: msg && kind === 'forward' && !draftId.value ? msg.id : '',
    draft_id: draftId.value,
    files: files.value,
  };
}

function errText(e: unknown): string {
  const status = (e as { response?: { status?: number } })?.response?.status;
  if (status === 502) return t('mailPage.sendUpstream');
  return e instanceof Error ? e.message : String(e);
}

async function send() {
  if (!canSend.value) return;
  const bad = recipients.value.find((r) => !/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(parseAddr(r).address));
  if (bad) {
    error.value = t('mailPage.badRecipient', { addr: bad });
    return;
  }
  sending.value = true;
  error.value = '';
  undoLeft.value = UNDO_SECONDS;
  undoTimer = setInterval(() => {
    undoLeft.value -= 1;
    if (undoLeft.value <= 0) {
      clearInterval(undoTimer);
      void reallySend('');
    }
  }, 1000);
}

const UNDO_SECONDS = 5;
const undoLeft = ref(0);
let undoTimer: ReturnType<typeof setInterval> | undefined;
onBeforeUnmount(() => clearInterval(undoTimer));

function cancelSend() {
  clearInterval(undoTimer);
  undoLeft.value = 0;
  sending.value = false;
  $q.notify({ message: t('mailPage.sendUndone') });
}

async function reallySend(sendAt: string) {
  undoLeft.value = 0;
  sending.value = true;
  try {
    await sendMail(input(sendAt));
    $q.notify({
      type: 'positive',
      message: sendAt ? t('mailPage.scheduled', { at: fullTime(sendAt) }) : t('mailPage.sent'),
    });
    emit('changed', sendAt ? 'saved' : 'sent');
    emit('close');
  } catch (e) {
    error.value = errText(e);
  } finally {
    sending.value = false;
  }
}

// ---------- 定时发送 ----------

const scheduleOpen = ref(false);
const scheduleAt = ref('');

/** datetime-local 的值（本地时间，到分钟）。 */
function localValue(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}

const schedulePresets = computed(() => {
  const now = new Date();
  const at = (days: number, h: number) => new Date(now.getFullYear(), now.getMonth(), now.getDate() + days, h, 0);
  const monday = (8 - now.getDay()) % 7 || 7;
  return [
    { label: t('mailPage.schedTomorrow8'), value: localValue(at(1, 8)) },
    { label: t('mailPage.schedTomorrow14'), value: localValue(at(1, 14)) },
    { label: t('mailPage.schedMonday8'), value: localValue(at(monday, 8)) },
  ];
});
const scheduleValid = computed(() => !!scheduleAt.value && new Date(scheduleAt.value).getTime() > Date.now() + 60_000);

function openSchedule() {
  scheduleAt.value = schedulePresets.value[0]?.value ?? '';
  scheduleOpen.value = true;
}

async function sendScheduled() {
  if (!scheduleValid.value || !canSend.value) return;
  await reallySend(new Date(scheduleAt.value).toISOString());
}

async function saveDraft() {
  saving.value = true;
  error.value = '';
  try {
    draftId.value = await saveMailDraft(input());
    // 新加的附件已经存进草稿，之后由服务端从草稿带上
    carried.value = [
      ...carried.value,
      ...files.value.map((f, i) => ({
        id: `new-${i}-${f.name}`,
        message_id: draftId.value,
        filename: f.name,
        content_type: f.type,
        size: f.size,
        knowledge_doc_id: '',
      })),
    ];
    files.value = [];
    savedSnapshot = snapshot();
    const d = new Date();
    savedAt.value = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
    emit('changed', 'saved');
  } catch (e) {
    error.value = errText(e);
  } finally {
    saving.value = false;
  }
}

async function close() {
  if (snapshot() !== savedSnapshot && (body.value.trim() || to.value.trim() || subject.value.trim())) {
    const ok = await confirm({
      title: t('mailPage.discardTitle'),
      message: t('mailPage.discardBody'),
      tone: 'danger',
      confirmLabel: t('mailPage.discard'),
    });
    if (!ok) return;
  }
  emit('close');
}
</script>
