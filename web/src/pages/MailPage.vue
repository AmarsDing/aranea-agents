<template>
  <q-page class="app-standard-page mail-page">
    <AppPageHero :kicker="t('mailPage.kicker')" :title="t('mailPage.title')" :subtitle="t('mailPage.subtitle')">
      <template #actions>
        <q-btn
          flat
          rounded
          no-caps
          icon="refresh"
          :label="t('mailPage.btnRefresh')"
          :loading="listLoading"
          @click="loadAll"
        />
        <q-btn flat rounded no-caps icon="upload_file" :label="t('mailPage.btnImport')" @click="importEmlFile" />
        <q-btn
          flat
          rounded
          no-caps
          icon="alternate_email"
          :label="t('mailPage.btnAccounts')"
          @click="accountsOpen = true"
        />
      </template>
    </AppPageHero>

    <q-banner v-if="pageError" rounded class="app-page-error-banner q-mb-md" role="alert">
      {{ pageError }}
      <template #action>
        <q-btn flat dense :label="t('mailPage.retry')" @click="loadAll" />
      </template>
    </q-banner>

    <!-- 账号过滤条 -->
    <div v-if="accounts.length" class="mail-filter row items-center q-gutter-xs">
      <q-chip
        clickable
        :outline="filterAccount !== ''"
        :color="filterAccount === '' ? 'primary' : void 0"
        :text-color="filterAccount === '' ? 'white' : undefined"
        @click="setFilter('')"
      >
        {{ t('mailPage.filterAll') }}
      </q-chip>
      <q-chip
        v-for="acc in accounts"
        :key="acc.id"
        clickable
        :outline="filterAccount !== acc.id"
        :color="filterAccount === acc.id ? 'primary' : void 0"
        :text-color="filterAccount === acc.id ? 'white' : undefined"
        @click="setFilter(acc.id)"
      >
        {{ acc.name || acc.username }}
      </q-chip>
    </div>

    <div class="mail-main">
      <!-- 列表 -->
      <section class="mail-list" :class="{ 'mail-list--hidden': Boolean(detail) && $q.screen.lt.md }">
        <div v-if="!listLoading && !messages.length" class="mail-empty text-caption">
          {{ t('mailPage.empty') }}
        </div>
        <q-card
          v-for="m in messages"
          :key="m.id"
          flat
          bordered
          class="mail-item"
          :class="{ 'mail-item--active': detail?.message.id === m.id }"
          clickable
          @click="openMessage(m.id)"
        >
          <q-card-section class="mail-item__inner">
            <div class="mail-item__top">
              <span class="mail-item__sender ellipsis">{{ m.sender || t('mailPage.unknownSender') }}</span>
              <span class="mail-item__time mono">{{ fmtTime(m.received_at) }}</span>
            </div>
            <div class="mail-item__subject ellipsis">{{ m.subject || t('mailPage.noSubject') }}</div>
            <div class="mail-item__snippet ellipsis-2-lines">{{ m.snippet }}</div>
            <div class="mail-item__meta row items-center q-gutter-xs">
              <q-icon v-if="m.has_attachments" name="attach_file" size="16px" class="mail-item__meta-icon" />
              <q-chip v-if="m.task_ref" dense size="sm" color="primary" text-color="white" class="mail-item__taskref">
                {{ t('mailPage.taskLinked') }}
              </q-chip>
            </div>
          </q-card-section>
        </q-card>

        <div v-if="total > pageSize" class="mail-pager row items-center justify-between">
          <q-btn
            flat
            dense
            no-caps
            icon="chevron_left"
            :label="t('mailPage.prevPage')"
            :disable="page <= 1"
            @click="gotoPage(page - 1)"
          />
          <span class="mail-pager__info mono">{{ page }} / {{ totalPages }}</span>
          <q-btn
            flat
            dense
            no-caps
            icon-right="chevron_right"
            :label="t('mailPage.nextPage')"
            :disable="page >= totalPages"
            @click="gotoPage(page + 1)"
          />
        </div>
      </section>

      <!-- 阅读窗 -->
      <section v-if="detail" class="mail-reader">
        <q-card flat bordered class="mail-reader__card">
          <q-card-section class="mail-reader__head">
            <div class="row items-start justify-between no-wrap">
              <div class="col">
                <div class="mail-reader__subject">{{ detail.message.subject || t('mailPage.noSubject') }}</div>
                <div class="mail-reader__from">
                  <span class="ellipsis">{{ detail.message.sender }}</span>
                  <span class="mono mail-reader__time">{{ fmtTime(detail.message.received_at) }}</span>
                </div>
              </div>
              <q-btn flat dense round icon="close" :aria-label="t('mailPage.closeReader')" @click="detail = null" />
            </div>
          </q-card-section>
          <q-separator />
          <q-card-section class="mail-reader__body">{{ detail.body || t('mailPage.emptyBody') }}</q-card-section>

          <template v-if="attachments.length">
            <q-separator />
            <q-card-section class="mail-reader__atts">
              <div class="mail-reader__atts-title">{{ t('mailPage.attachments', { n: attachments.length }) }}</div>
              <div v-for="a in attachments" :key="a.id" class="mail-att row items-center no-wrap">
                <q-icon name="insert_drive_file" size="20px" class="mail-att__icon" />
                <div class="mail-att__meta col">
                  <div class="mail-att__name ellipsis">{{ a.filename }}</div>
                  <div class="mail-att__size mono">{{ a.content_type }} · {{ fmtSize(a.size) }}</div>
                </div>
                <q-btn
                  v-if="a.knowledge_doc_id"
                  flat
                  dense
                  no-caps
                  disable
                  icon="inventory_2"
                  :label="t('mailPage.inKnowledge')"
                  class="mail-att__btn"
                />
                <q-btn
                  v-else
                  flat
                  dense
                  no-caps
                  icon="inventory_2"
                  :label="t('mailPage.toKnowledge')"
                  class="mail-att__btn"
                  @click="askClassification(a)"
                />
                <q-btn flat dense round icon="download" :aria-label="t('mailPage.download')" @click="downloadAtt(a)" />
              </div>
            </q-card-section>
          </template>

          <q-separator />
          <q-card-actions align="right" class="mail-reader__actions">
            <q-btn flat no-caps icon="task_alt" :label="t('mailPage.toTask')" :loading="acting" @click="toTask" />
            <q-btn flat no-caps icon="download" :label="t('mailPage.exportEml')" @click="exportEml" />
            <q-btn
              flat
              no-caps
              color="negative"
              icon="delete_outline"
              :label="t('mailPage.delete')"
              @click="confirmDeleteMessage"
            />
          </q-card-actions>
        </q-card>
      </section>

      <section
        v-else-if="!listLoading && messages.length && $q.screen.gt.sm"
        class="mail-reader mail-reader--placeholder"
      >
        <div class="mail-empty text-caption">{{ t('mailPage.pickMessage') }}</div>
      </section>
    </div>

    <!-- ============ 账号管理 ============ -->
    <AppDialog v-model="accountsOpen" size="md" :title="t('mailPage.accountsTitle')">
      <div v-if="!accounts.length" class="text-caption mail-empty">
        {{ t('mailPage.accountsEmpty') }}
      </div>
      <q-list v-else separator>
        <q-item v-for="acc in accounts" :key="acc.id">
          <q-item-section>
            <q-item-label>{{ acc.name || acc.username }}</q-item-label>
            <q-item-label caption class="mono"
              >{{ acc.username }}@{{ acc.host }}:{{ acc.port }} · {{ acc.tls_mode }}</q-item-label
            >
            <q-item-label caption>
              {{ t('mailPage.lastSync') }}:
              <span class="mono">{{ acc.last_sync_at ? fmtTime(acc.last_sync_at) : t('mailPage.neverSynced') }}</span>
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <div class="row no-wrap items-center">
              <q-btn
                flat
                dense
                no-caps
                icon="sync"
                :label="t('mailPage.syncNow')"
                :loading="syncingId === acc.id"
                @click="syncAccount(acc)"
              />
              <q-btn flat dense round icon="edit" :aria-label="t('mailPage.edit')" @click="openAccountEdit(acc)" />
              <q-btn
                flat
                dense
                round
                icon="delete_outline"
                :aria-label="t('mailPage.delete')"
                @click="confirmDeleteAccount(acc)"
              />
            </div>
          </q-item-section>
        </q-item>
      </q-list>
      <template #actions>
        <q-btn
          unelevated
          dense
          rounded
          no-caps
          color="primary"
          icon="add"
          :label="t('mailPage.btnNewAccount')"
          @click="openAccountCreate"
        />
        <q-btn flat no-caps :label="t('mailPage.close')" @click="accountsOpen = false" />
      </template>
    </AppDialog>

    <!-- ============ 账号新建/编辑 ============ -->
    <AppDialog
      v-model="accountEditorOpen"
      persistent
      size="sm"
      :title="editingAccountId ? t('mailPage.accountEditTitle') : t('mailPage.accountCreateTitle')"
    >
      <div class="q-gutter-md">
        <q-banner v-if="accountEditorError" rounded class="app-page-error-banner" role="alert">
          {{ accountEditorError }}
        </q-banner>
        <q-input v-model="accountForm.name" outlined dense :label="t('mailPage.fieldName')" autofocus />
        <div class="row q-gutter-md">
          <q-input
            v-model="accountForm.host"
            class="col"
            outlined
            dense
            :label="t('mailPage.fieldHost')"
            hint="imap.example.com"
          />
          <q-input
            v-model.number="accountForm.port"
            class="col-4"
            outlined
            dense
            type="number"
            :label="t('mailPage.fieldPort')"
          />
        </div>
        <q-select
          v-model="accountForm.tls_mode"
          outlined
          dense
          :options="tlsOptions"
          :label="t('mailPage.fieldTls')"
          emit-value
          map-options
        />
        <q-input
          v-model="accountForm.username"
          outlined
          dense
          :label="t('mailPage.fieldUsername')"
          autocomplete="off"
        />
        <q-input
          v-model="accountForm.password"
          outlined
          dense
          type="password"
          :label="t('mailPage.fieldPassword')"
          :hint="editingAccountId ? t('mailPage.fieldPasswordEditHint') : t('mailPage.fieldPasswordHint')"
          autocomplete="new-password"
        />
      </div>
      <template #actions>
        <q-btn flat no-caps :label="t('mailPage.cancel')" @click="accountEditorOpen = false" />
        <q-btn
          unelevated
          no-caps
          color="primary"
          :label="t('mailPage.save')"
          :loading="savingAccount"
          :disable="!accountFormValid"
          @click="saveAccount"
        />
      </template>
    </AppDialog>

    <!-- ============ 附件入知识库（密级选择） ============ -->
    <AppDialog v-model="classificationOpen" persistent size="sm" :title="t('mailPage.classifyTitle')">
      <q-option-group v-model="classification" :options="classificationOptions" color="primary" type="radio" />
      <template #actions>
        <q-btn flat no-caps :label="t('mailPage.cancel')" @click="classificationOpen = false" />
        <q-btn
          unelevated
          no-caps
          color="primary"
          :label="t('mailPage.classifyConfirm')"
          :loading="acting"
          @click="doToKnowledge"
        />
      </template>
    </AppDialog>

    <input ref="emlInput" type="file" accept=".eml,message/rfc822" class="mail-hidden-input" @change="onEmlPicked" />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { confirm } from '../composables/useAppConfirm';
import AppDialog from '../components/common/AppDialog.vue';
import AppPageHero from '../components/layout/AppPageHero.vue';
import {
  attachmentToKnowledge,
  createMailAccount,
  deleteMailAccount,
  deleteMailMessage,
  downloadMailAttachment,
  exportMailEml,
  getMailMessage,
  importMailEml,
  listMailAccounts,
  listMailAttachments,
  listMailMessages,
  mailToTask,
  syncMailAccount,
  updateMailAccount,
} from '../features/office/api';
import type { MailAccountRow, MailAttachmentRow, MailMessageRow, MailTlsMode } from '../features/office/types';

const { t } = useI18n();
const $q = useQuasar();

// ---------- 账号 ----------

const accounts = ref<MailAccountRow[]>([]);
const accountsOpen = ref(false);
const syncingId = ref('');

// ---------- 列表 ----------

const messages = ref<MailMessageRow[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = 20;
const listLoading = ref(false);
const pageError = ref('');
const filterAccount = ref('');

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)));

async function loadAccounts() {
  accounts.value = await listMailAccounts();
}

async function loadMessages() {
  listLoading.value = true;
  pageError.value = '';
  try {
    const res = await listMailMessages({ accountId: filterAccount.value, page: page.value, pageSize });
    messages.value = res.items;
    total.value = res.total;
  } catch (e) {
    pageError.value = e instanceof Error ? e.message : String(e);
  } finally {
    listLoading.value = false;
  }
}

async function loadAll() {
  try {
    await loadAccounts();
  } catch (e) {
    pageError.value = e instanceof Error ? e.message : String(e);
  }
  await loadMessages();
}

onMounted(() => void loadAll());

function setFilter(id: string) {
  if (filterAccount.value === id) return;
  filterAccount.value = id;
  page.value = 1;
  void loadMessages();
}

function gotoPage(p: number) {
  if (p < 1 || p > totalPages.value) return;
  page.value = p;
  void loadMessages();
}

// ---------- 阅读窗 ----------

const detail = ref<{ message: MailMessageRow; body: string } | null>(null);
const attachments = ref<MailAttachmentRow[]>([]);
const acting = ref(false);

async function openMessage(id: string) {
  try {
    detail.value = await getMailMessage(id);
    attachments.value = await listMailAttachments(id);
  } catch (e) {
    pageError.value = e instanceof Error ? e.message : String(e);
  }
}

async function toTask() {
  if (!detail.value) return;
  acting.value = true;
  try {
    const taskRef = await mailToTask(detail.value.message.id, '');
    detail.value = { ...detail.value, message: { ...detail.value.message, task_ref: taskRef } };
    const m = messages.value.find((x) => x.id === detail.value?.message.id);
    if (m) m.task_ref = taskRef;
    $q.notify({ type: 'positive', message: t('mailPage.toTaskDone', { ref: taskRef }) });
  } catch (e) {
    pageError.value = e instanceof Error ? e.message : String(e);
  } finally {
    acting.value = false;
  }
}

async function exportEml() {
  if (!detail.value) return;
  try {
    await exportMailEml(detail.value.message);
  } catch (e) {
    pageError.value = e instanceof Error ? e.message : String(e);
  }
}

async function confirmDeleteMessage() {
  if (!detail.value) return;
  const id = detail.value.message.id;
  const ok = await confirm({
    title: t('mailPage.deleteMessageTitle'),
    message: t('mailPage.deleteMessageBody'),
    tone: 'danger',
  });
  if (!ok) return;
  try {
    await deleteMailMessage(id);
    detail.value = null;
    attachments.value = [];
    await loadMessages();
  } catch (e) {
    pageError.value = e instanceof Error ? e.message : String(e);
  }
}

// ---------- 附件 ----------

async function downloadAtt(a: MailAttachmentRow) {
  try {
    await downloadMailAttachment(a);
  } catch (e) {
    pageError.value = e instanceof Error ? e.message : String(e);
  }
}

const classificationOpen = ref(false);
const classification = ref(1);
const classifyingAtt = ref<MailAttachmentRow | null>(null);

const classificationOptions = computed(() => [
  { label: t('mailPage.classifyL1'), value: 1 },
  { label: t('mailPage.classifyL2'), value: 2 },
  { label: t('mailPage.classifyL3'), value: 3 },
]);

function askClassification(a: MailAttachmentRow) {
  classifyingAtt.value = a;
  classification.value = 1;
  classificationOpen.value = true;
}

async function doToKnowledge() {
  if (!classifyingAtt.value) return;
  acting.value = true;
  try {
    const docId = await attachmentToKnowledge(classifyingAtt.value.id, classification.value);
    classifyingAtt.value.knowledge_doc_id = docId;
    classificationOpen.value = false;
    $q.notify({ type: 'positive', message: t('mailPage.toKnowledgeDone') });
  } catch (e) {
    pageError.value = e instanceof Error ? e.message : String(e);
  } finally {
    acting.value = false;
  }
}

// ---------- 账号编辑 ----------

const accountEditorOpen = ref(false);
const accountEditorError = ref('');
const savingAccount = ref(false);
const editingAccountId = ref('');

const accountForm = ref({
  name: '',
  host: '',
  port: 993,
  tls_mode: 'TLS_MODE_TLS' as MailTlsMode,
  username: '',
  password: '',
});

const tlsOptions = computed(() => [
  { label: 'TLS', value: 'TLS_MODE_TLS' },
  { label: 'STARTTLS', value: 'TLS_MODE_STARTTLS' },
  { label: t('mailPage.tlsNone'), value: 'TLS_MODE_NONE' },
]);

const accountFormValid = computed(() => {
  const f = accountForm.value;
  if (!f.host.trim() || !f.username.trim() || !(f.port > 0)) return false;
  // 创建必须带密码；编辑留空 = 不轮换。
  if (!editingAccountId.value && !f.password) return false;
  return true;
});

function openAccountCreate() {
  editingAccountId.value = '';
  accountEditorError.value = '';
  accountForm.value = { name: '', host: '', port: 993, tls_mode: 'TLS_MODE_TLS', username: '', password: '' };
  accountEditorOpen.value = true;
}

function openAccountEdit(acc: MailAccountRow) {
  editingAccountId.value = acc.id;
  accountEditorError.value = '';
  accountForm.value = {
    name: acc.name,
    host: acc.host,
    port: acc.port || 993,
    tls_mode: acc.tls_mode,
    username: acc.username,
    password: '',
  };
  accountEditorOpen.value = true;
}

async function saveAccount() {
  savingAccount.value = true;
  accountEditorError.value = '';
  const input = {
    name: accountForm.value.name.trim(),
    host: accountForm.value.host.trim(),
    port: accountForm.value.port,
    tls_mode: accountForm.value.tls_mode,
    username: accountForm.value.username.trim(),
    password: accountForm.value.password,
  };
  try {
    if (editingAccountId.value) {
      await updateMailAccount(editingAccountId.value, input);
    } else {
      await createMailAccount(input);
    }
    accountEditorOpen.value = false;
    accountForm.value.password = '';
    await loadAccounts();
  } catch (e) {
    accountEditorError.value = e instanceof Error ? e.message : String(e);
  } finally {
    savingAccount.value = false;
  }
}

async function syncAccount(acc: MailAccountRow) {
  syncingId.value = acc.id;
  pageError.value = '';
  try {
    const r = await syncMailAccount(acc.id);
    $q.notify({ type: 'positive', message: t('mailPage.syncDone', { imported: r.imported, skipped: r.skipped }) });
    await loadAccounts();
    await loadMessages();
  } catch (e) {
    // 上游 IMAP 失败服务端已审计；此处仅呈现通用错误。
    pageError.value = e instanceof Error ? e.message : String(e);
  } finally {
    syncingId.value = '';
  }
}

async function confirmDeleteAccount(acc: MailAccountRow) {
  const ok = await confirm({
    title: t('mailPage.deleteAccountTitle'),
    message: t('mailPage.deleteAccountBody', { name: acc.name || acc.username }),
    tone: 'danger',
  });
  if (!ok) return;
  try {
    await deleteMailAccount(acc.id);
    if (filterAccount.value === acc.id) {
      filterAccount.value = '';
      page.value = 1;
    }
    if (detail.value?.message.account_id === acc.id) detail.value = null;
    await loadAccounts();
    await loadMessages();
  } catch (e) {
    pageError.value = e instanceof Error ? e.message : String(e);
  }
}

// ---------- .eml 导入 ----------

const emlInput = ref<HTMLInputElement | null>(null);

function importEmlFile() {
  emlInput.value?.click();
}

async function onEmlPicked(ev: Event) {
  const input = ev.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = '';
  if (!file) return;
  try {
    const text = await file.text();
    await importMailEml(text);
    $q.notify({ type: 'positive', message: t('mailPage.importDone') });
    await loadMessages();
  } catch (e) {
    pageError.value = e instanceof Error ? e.message : String(e);
  }
}

// ---------- 呈现辅助 ----------

function fmtTime(iso: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function fmtSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}
</script>

<style scoped lang="sass">
// 令牌只取 --nt-*（AGENTS.md §6）；时间/大小等数字一律等宽 + tabular-nums。
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums

.mail-filter
  margin-bottom: 12px

.mail-main
  display: flex
  gap: 16px
  align-items: flex-start

.mail-list
  flex: 0 0 360px
  min-width: 0

.mail-reader
  flex: 1
  min-width: 0

.mail-reader--placeholder
  display: flex
  align-items: center
  justify-content: center
  min-height: 240px
  border: 1px dashed var(--nt-border-1)
  border-radius: 12px

@media (max-width: 799px)
  .mail-main
    flex-direction: column
  .mail-list
    flex: 1 1 auto
    width: 100%
  .mail-list--hidden
    display: none

.mail-empty
  color: var(--nt-text-2)
  padding: 48px 0
  text-align: center

.mail-item
  background: var(--nt-bg-1)
  border-color: var(--nt-border-1)
  border-radius: 8px
  margin-bottom: 8px
  cursor: pointer

.mail-item--active
  border-color: var(--nt-accent)

.mail-item__inner
  padding: 12px 16px

.mail-item__top
  display: flex
  justify-content: space-between
  align-items: baseline
  gap: 8px

.mail-item__sender
  font-size: 13px
  color: var(--nt-text-1)
  min-width: 0

.mail-item__time
  font-size: 12px
  color: var(--nt-text-2)
  flex-shrink: 0

.mail-item__subject
  font-weight: 600
  margin-top: 2px

.mail-item__snippet
  font-size: 12px
  color: var(--nt-text-2)
  margin-top: 2px

.mail-item__meta
  margin-top: 6px
  min-height: 20px

.mail-item__meta-icon
  color: var(--nt-text-2)

.mail-pager
  padding: 8px 0

.mail-pager__info
  font-size: 12px
  color: var(--nt-text-2)

.mail-reader__card
  background: var(--nt-bg-1)
  border-color: var(--nt-border-1)
  border-radius: 12px

.mail-reader__subject
  font-size: 16px
  font-weight: 600

.mail-reader__from
  display: flex
  gap: 12px
  align-items: baseline
  font-size: 13px
  color: var(--nt-text-1)
  margin-top: 4px

.mail-reader__time
  font-size: 12px
  color: var(--nt-text-2)

.mail-reader__body
  white-space: pre-wrap
  word-break: break-word
  font-size: 13px
  line-height: 1.7
  min-height: 120px

.mail-reader__atts-title
  font-size: 13px
  font-weight: 600
  color: var(--nt-text-1)
  margin-bottom: 8px

.mail-att
  gap: 8px
  padding: 6px 0

.mail-att__icon
  color: var(--nt-text-2)

.mail-att__name
  font-size: 13px

.mail-att__size
  font-size: 12px
  color: var(--nt-text-2)

.mail-att__btn
  flex-shrink: 0

.mail-account-editor
  min-width: min(520px, 94vw)

.mail-hidden-input
  display: none
</style>
