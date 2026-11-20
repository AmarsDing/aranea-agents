<template>
  <!-- 工作台 › 邮件：三栏（视图与账户 · 列表 · 阅读），设计 docs/aranea-agents/design/2026-11-17-三大模块重设计/09-邮件.md §3 -->
  <q-page class="app-standard-page ml-page" :class="{ 'ml-page--reading': !!selectedId }">
    <Teleport defer to="#app-tab-actions">
      <q-btn
        flat
        dense
        round
        icon="refresh"
        :title="t('mailPage.refresh')"
        :aria-label="t('mailPage.refresh')"
        :loading="refreshing"
        @click="refreshAll"
      />
    </Teleport>

    <AppStatePanel v-if="forbidden" class="ml-forbidden" variant="forbidden" />

    <template v-else>
      <!-- ============ 左：写邮件 / 视图 / 账户 ============ -->
      <nav class="ml-panel ml-nav" :aria-label="t('mailPage.navAria')">
        <q-btn
          unelevated
          no-caps
          color="primary"
          icon="edit"
          class="ml-nav__compose"
          :label="t('mailPage.compose')"
          @click="startCompose('new')"
        />
        <button
          v-for="v in VIEWS"
          :key="v"
          type="button"
          class="ml-nv"
          :class="{ 'ml-nv--on': view === v && !accountFilter }"
          :aria-current="view === v && !accountFilter ? 'page' : undefined"
          @click="setView(v)"
        >
          <q-icon :name="VIEW_ICON[v]" size="18px" aria-hidden="true" />
          <span>{{ t(`mailPage.view.${v}`) }}</span>
          <span v-if="viewCount(v)" class="ml-nv__n ml-num">{{ viewCount(v) }}</span>
        </button>

        <div class="ml-sect">{{ t('mailPage.accounts') }}</div>
        <div v-for="a in accounts" :key="a.id" class="ml-acct">
          <button
            type="button"
            class="ml-nv"
            :class="{ 'ml-nv--on': accountFilter === a.id }"
            :title="a.email || a.username"
            @click="setAccount(a.id)"
          >
            <span class="ml-dot" :class="`ml-dot--${dotOf(a)}`" aria-hidden="true" />
            <span class="ml-nv__name">{{ a.name || a.email || a.username }}</span>
            <span v-if="a.status === 'auth_failed' || a.status === 'unreachable'" class="ml-nv__n ml-nv__n--bad">
              {{ t(`mailPage.status.${a.status}`) }}
            </span>
            <span v-else-if="summary?.accounts[a.id]" class="ml-nv__n ml-num">{{ summary?.accounts[a.id] }}</span>
          </button>
          <q-btn
            flat
            dense
            round
            size="sm"
            icon="more_horiz"
            class="ml-acct__more"
            :aria-label="t('mailPage.accountMenu', { name: a.name })"
          >
            <q-menu anchor="bottom right" self="top right">
              <q-list dense>
                <q-item v-close-popup clickable @click="syncAccount(a)">
                  <q-item-section>{{ t('mailPage.syncNow') }}</q-item-section>
                </q-item>
                <q-item v-close-popup clickable @click="testAccount(a)">
                  <q-item-section>{{ t('mailPage.testConn') }}</q-item-section>
                </q-item>
                <q-item v-close-popup clickable @click="editAccount(a)">
                  <q-item-section>{{ t('mailPage.editAccount') }}</q-item-section>
                </q-item>
                <q-item v-close-popup clickable class="text-negative" @click="removeAccount(a)">
                  <q-item-section>{{ t('mailPage.deleteAccount') }}</q-item-section>
                </q-item>
              </q-list>
            </q-menu>
          </q-btn>
        </div>
        <button type="button" class="ml-nv ml-nv--add" @click="addAccount">
          <q-icon name="add" size="18px" aria-hidden="true" />
          <span>{{ t('mailPage.addAccount') }}</span>
        </button>

        <div class="ml-sect">{{ t('mailPage.alsoCan') }}</div>
        <p class="ml-hint ml-nav__hint">{{ t('mailPage.dropHint') }}</p>
        <q-btn
          flat
          dense
          no-caps
          icon="upload_file"
          class="ml-nav__import"
          :label="t('mailPage.importEml')"
          @click="emlPicker?.click()"
        />
        <input ref="emlPicker" type="file" accept=".eml,message/rfc822" multiple class="ml-sr" @change="onEmlPicked" />
      </nav>

      <!-- ============ 中：搜索 + 列表 ============ -->
      <section
        class="ml-panel ml-list"
        :class="{ 'ml-list--drop': dragging }"
        :aria-label="t('mailPage.listAria')"
        @dragover.prevent="onDragOver"
        @dragleave="dragging = false"
        @drop.prevent="onDrop"
      >
        <div class="ml-list__head">
          <div class="ml-list__bar">
            <label class="ml-search">
              <q-icon name="search" size="18px" aria-hidden="true" />
              <span class="ml-sr">{{ t('mailPage.search') }}</span>
              <input
                ref="searchInput"
                v-model="keyword"
                type="search"
                :placeholder="t('mailPage.searchPlaceholder')"
                @keydown.esc="keyword = ''"
              />
            </label>
            <q-btn
              flat
              dense
              round
              icon="edit"
              class="ml-list__compose"
              :aria-label="t('mailPage.compose')"
              @click="startCompose('new')"
            />
          </div>
          <div class="ml-list__bar">
            <q-select
              v-model="narrowView"
              class="ml-list__viewsel"
              dense
              borderless
              emit-value
              map-options
              :options="narrowOptions"
              :aria-label="t('mailPage.navAria')"
            />
            <q-btn-toggle
              v-if="view === 'inbox' || view === 'unread'"
              :model-value="view"
              dense
              no-caps
              unelevated
              toggle-color="primary"
              :options="segOptions"
              :aria-label="t('mailPage.view.inbox')"
              @update:model-value="setView"
            />
            <span class="ml-sp" />
            <span class="ml-hint ml-num">{{ t('mailPage.totalN', { n: total }) }}</span>
          </div>
        </div>

        <div ref="listBody" class="ml-list__body" :aria-busy="listLoading" @scroll="onListScroll">
          <q-linear-progress
            v-if="listLoading && !rows.length"
            indeterminate
            color="primary"
            class="ml-list__progress"
          />
          <AppStatePanel
            v-if="listError"
            dense
            variant="partial"
            :description="listError"
            :action-label="t('mailPage.retry')"
            action-icon="refresh"
            @action="loadList(true)"
          />
          <div v-if="!accounts.length && !rows.length && !listLoading" class="ml-empty">
            <q-icon name="alternate_email" size="32px" aria-hidden="true" />
            <p class="ml-empty__title">{{ t('mailPage.noAccountTitle') }}</p>
            <p class="ml-hint">{{ t('mailPage.noAccountDesc') }}</p>
            <q-btn
              unelevated
              no-caps
              color="primary"
              icon="add"
              :label="t('mailPage.addAccount')"
              @click="addAccount"
            />
          </div>
          <div v-else-if="!rows.length && !listLoading && !listError" class="ml-empty">
            <q-icon :name="keyword ? 'search_off' : 'drafts'" size="32px" aria-hidden="true" />
            <p class="ml-empty__title">{{ keyword ? t('mailPage.noMatch') : t(`mailPage.empty.${view}`) }}</p>
          </div>

          <template v-for="g in groups" :key="g.key">
            <div class="ml-group">{{ t(`mailPage.group.${g.key}`) }}</div>
            <button
              v-for="m in g.rows"
              :key="m.id"
              type="button"
              class="ml-msg"
              :class="{ 'ml-msg--unread': !m.seen, 'ml-msg--on': m.id === selectedId }"
              :aria-current="m.id === selectedId ? 'true' : undefined"
              @click="openMessage(m)"
            >
              <span class="ml-av" :class="`ml-av--${avatarTone(counterpart(m))}`" aria-hidden="true">{{
                initial(counterpart(m))
              }}</span>
              <span class="ml-msg__main">
                <span class="ml-msg__top">
                  <span class="ml-msg__from">{{ listWho(m) }}</span>
                  <q-icon
                    v-if="m.flagged"
                    name="star"
                    size="14px"
                    class="ml-star"
                    :aria-label="t('mailPage.flagged')"
                  />
                  <span class="ml-msg__time ml-num">{{ listTime(m.received_at, now) }}</span>
                </span>
                <span class="ml-msg__subj">{{ m.subject || t('mailPage.noSubject') }}</span>
                <span class="ml-msg__snip">{{ m.snippet }}</span>
                <span v-if="m.has_attachments || m.task_ref || accounts.length > 1" class="ml-msg__chips">
                  <span v-if="m.has_attachments" class="ml-chip"
                    ><q-icon name="attach_file" size="13px" aria-hidden="true" />{{
                      t('mailPage.hasAttachments')
                    }}</span
                  >
                  <span v-if="m.task_ref" class="ml-chip">{{ t('mailPage.taskLinked') }}</span>
                  <span v-if="accounts.length > 1 && accountName(m.account_id)" class="ml-msg__acct">{{
                    accountName(m.account_id)
                  }}</span>
                </span>
              </span>
            </button>
          </template>
          <div v-if="loadingMore" class="ml-hint ml-list__more">{{ t('mailPage.loading') }}</div>
        </div>
      </section>

      <!-- ============ 右：阅读 ============ -->
      <section class="ml-panel ml-read" :aria-label="t('mailPage.readAria')">
        <div v-if="!detail" class="ml-empty ml-empty--read">
          <q-icon name="mark_email_read" size="36px" aria-hidden="true" />
          <p class="ml-hint">{{ detailLoading ? t('mailPage.loading') : t('mailPage.pickMessage') }}</p>
        </div>
        <template v-else>
          <header class="ml-read__head">
            <q-btn
              flat
              dense
              round
              icon="arrow_back"
              class="ml-read__back"
              :aria-label="t('mailPage.back')"
              @click="closeReader"
            />
            <h2 class="ml-read__subj">{{ detail.message.subject || t('mailPage.noSubject') }}</h2>
            <div class="ml-read__who">
              <span class="ml-av ml-av--lg" :class="`ml-av--${avatarTone(detail.message.sender)}`" aria-hidden="true">
                {{ initial(detail.message.sender) }}
              </span>
              <span class="ml-read__name">
                <b>{{ displayName(detail.message.sender) || t('mailPage.unknownSender') }}</b>
                <span class="ml-read__addr ml-num">{{ parseAddr(detail.message.sender).address }}</span>
              </span>
              <span class="ml-read__time ml-num">{{ fullTime(detail.message.received_at) }}</span>
              <span class="ml-read__to">
                <template v-if="detail.message.to">{{ t('mailPage.sentTo', { to: detail.message.to }) }}</template>
                <template v-if="detail.message.cc"> · {{ t('mailPage.ccTo', { cc: detail.message.cc }) }}</template>
                <template v-if="accountName(detail.message.account_id)">
                  · {{ accountName(detail.message.account_id) }}</template
                >
              </span>
            </div>
          </header>

          <div class="ml-read__body">
            <div class="ml-read__in">
              <MailHtmlView v-if="detail.body_html" :html="detail.body_html" :title="detail.message.subject" />
              <div v-else class="ml-letter">{{ detail.body || t('mailPage.emptyBody') }}</div>

              <div v-if="attachments.length" class="ml-atts">
                <div v-for="a in attachments" :key="a.id" class="ml-att">
                  <span
                    class="ml-att__ic ml-num"
                    :class="`ml-att__ic--${fileBadge(a.filename, a.content_type).toLowerCase()}`"
                  >
                    {{ fileBadge(a.filename, a.content_type) }}
                  </span>
                  <button
                    type="button"
                    class="ml-att__n"
                    :aria-label="t('mailPage.previewName', { name: a.filename })"
                    @click="openPreview(a)"
                  >
                    <b :title="a.filename">{{ a.filename }}</b>
                    <span class="ml-num">{{ fileSize(a.size) }}</span>
                  </button>
                  <q-btn
                    flat
                    dense
                    round
                    size="sm"
                    icon="download"
                    :aria-label="t('mailPage.downloadName', { name: a.filename })"
                    @click="downloadAtt(a)"
                  />
                  <q-btn
                    flat
                    dense
                    round
                    size="sm"
                    :icon="a.knowledge_doc_id ? 'library_add_check' : 'library_add'"
                    :disable="!!a.knowledge_doc_id"
                    :aria-label="
                      a.knowledge_doc_id
                        ? t('mailPage.inKnowledge')
                        : t('mailPage.toKnowledgeName', { name: a.filename })
                    "
                    @click="askClassification(a)"
                  />
                </div>
              </div>
            </div>
          </div>

          <footer class="ml-read__bar">
            <template v-if="detail.message.folder !== 'drafts'">
              <q-btn
                unelevated
                no-caps
                color="primary"
                icon="reply"
                :label="t('mailPage.reply')"
                @click="startCompose('reply')"
              />
              <q-btn flat no-caps icon="reply_all" :label="t('mailPage.replyAll')" @click="startCompose('replyAll')" />
              <q-btn flat no-caps icon="forward" :label="t('mailPage.forward')" @click="startCompose('forward')" />
            </template>
            <span class="ml-sp" />
            <q-btn
              flat
              no-caps
              icon="task_alt"
              :label="detail.message.task_ref ? t('mailPage.taskLinked') : t('mailPage.toTask')"
              :disable="!!detail.message.task_ref || acting"
              @click="toTask"
            />
            <q-btn
              flat
              dense
              round
              :icon="detail.message.flagged ? 'star' : 'star_border'"
              :class="{ 'ml-star': detail.message.flagged }"
              :aria-pressed="detail.message.flagged"
              :aria-label="t('mailPage.flag')"
              :title="t('mailPage.flag')"
              @click="toggleFlag"
            />
            <q-btn
              flat
              dense
              round
              :icon="detail.message.archived ? 'unarchive' : 'archive'"
              :aria-label="detail.message.archived ? t('mailPage.unarchive') : t('mailPage.archive')"
              :title="detail.message.archived ? t('mailPage.unarchive') : t('mailPage.archive')"
              @click="toggleArchive"
            />
            <q-btn flat dense round icon="more_horiz" :aria-label="t('mailPage.more')">
              <q-menu anchor="top right" self="bottom right">
                <q-list dense>
                  <q-item v-close-popup clickable @click="markUnread">
                    <q-item-section>{{ t('mailPage.markUnread') }}</q-item-section>
                  </q-item>
                  <q-item v-close-popup clickable @click="exportEml">
                    <q-item-section>{{ t('mailPage.exportEml') }}</q-item-section>
                  </q-item>
                </q-list>
              </q-menu>
            </q-btn>
            <q-btn
              flat
              dense
              round
              icon="delete_outline"
              color="negative"
              :aria-label="t('mailPage.delete')"
              :title="t('mailPage.delete')"
              @click="removeMessage"
            />
          </footer>
        </template>

        <MailComposer
          v-if="compose"
          :key="composeKey"
          :seed="compose"
          :accounts="accounts"
          :default-account-id="accountFilter"
          @close="compose = null"
          @changed="onComposeChanged"
        />
      </section>
    </template>

    <MailAccountWizard v-model="wizardOpen" :account="wizardAccount" @saved="onAccountSaved" />

    <AppDialog v-model="previewOpen" size="xl" position="standard" :title="preview?.att.filename ?? ''">
      <div class="ml-pv" aria-live="polite">
        <q-inner-loading :showing="previewLoading" />
        <template v-if="preview">
          <img v-if="preview.kind === 'image'" class="ml-pv__img" :src="preview.url" :alt="preview.att.filename" />
          <iframe
            v-else-if="preview.kind === 'pdf'"
            class="ml-pv__frame"
            :src="preview.url"
            :title="preview.att.filename"
          />
          <!-- 空 sandbox：附件里的脚本、表单、跳转一律不动 -->
          <iframe
            v-else-if="preview.kind === 'html'"
            class="ml-pv__frame ml-pv__frame--paper"
            sandbox=""
            :srcdoc="preview.html"
            :title="preview.att.filename"
          />
          <pre v-else-if="preview.kind === 'text'" class="ml-pv__text">{{ preview.text }}</pre>
          <p v-else class="ml-hint">{{ t('mailPage.previewNone') }}</p>
        </template>
      </div>
      <template #actions>
        <q-btn
          v-if="preview"
          unelevated
          no-caps
          color="primary"
          icon="download"
          :label="t('mailPage.download')"
          @click="downloadAtt(preview.att)"
        />
      </template>
    </AppDialog>

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
  </q-page>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useRoute } from 'vue-router';
import AppDialog from '../components/common/AppDialog.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import { confirm } from '../composables/useAppConfirm';
import { isForbiddenApiError } from '../utils/kratosError';
import {
  attachmentToKnowledge,
  deleteMailAccount,
  deleteMailMessage,
  downloadMailAttachment,
  exportMailEml,
  fetchMailAttachment,
  getMailboxSummary,
  getMailMessage,
  importMailEml,
  listMailAccounts,
  listMailAttachments,
  listMailMessages,
  mailToTask,
  markMailMessages,
  syncMailAccount,
  testMailAccount,
} from '../features/office/api';
import type {
  MailAccountRow,
  MailAttachmentRow,
  MailboxSummaryRow,
  MailMessageRow,
  MailView,
} from '../features/office/types';
import MailAccountWizard from '../features/mail/components/MailAccountWizard.vue';
import MailComposer from '../features/mail/components/MailComposer.vue';
import MailHtmlView from '../features/mail/components/MailHtmlView.vue';
import {
  attachmentSrcdoc,
  avatarTone,
  displayName,
  fileBadge,
  fileSize,
  fullTime,
  groupByDate,
  initial,
  listTime,
  parseAddr,
  previewKind,
  splitAddrs,
  type AttachmentPreviewKind,
  type ComposeKind,
  type ComposeSeed,
} from '../features/mail/model';

const { t } = useI18n();
const $q = useQuasar();
const route = useRoute();

const VIEWS: MailView[] = ['inbox', 'flagged', 'task', 'attachments', 'sent', 'drafts', 'archived'];
const VIEW_ICON: Record<MailView, string> = {
  inbox: 'inbox',
  unread: 'mark_email_unread',
  flagged: 'star_border',
  task: 'task_alt',
  attachments: 'attach_file',
  sent: 'send',
  drafts: 'drafts',
  archived: 'archive',
};
const PAGE = 50;

const accounts = ref<MailAccountRow[]>([]);
const summary = ref<MailboxSummaryRow | null>(null);
const view = ref<MailView>('inbox');
const accountFilter = ref('');
const keyword = ref('');
const rows = ref<MailMessageRow[]>([]);
const total = ref(0);
const page = ref(1);
const listLoading = ref(false);
const loadingMore = ref(false);
const listError = ref('');
const forbidden = ref(false);
const refreshing = ref(false);
const now = ref(new Date());

// ---------- 加载 ----------

async function loadAccounts() {
  accounts.value = await listMailAccounts();
}

async function loadSummary() {
  try {
    summary.value = await getMailboxSummary();
  } catch {
    summary.value = null;
  }
}

let listSeq = 0;
async function loadList(reset: boolean) {
  const seq = ++listSeq;
  if (reset) {
    page.value = 1;
    listLoading.value = true;
  } else {
    loadingMore.value = true;
  }
  listError.value = '';
  try {
    const res = await listMailMessages({
      view: view.value,
      accountId: accountFilter.value,
      keyword: keyword.value.trim(),
      page: page.value,
      pageSize: PAGE,
    });
    if (seq !== listSeq) return;
    rows.value = reset ? res.items : [...rows.value, ...res.items];
    total.value = res.total;
    now.value = new Date();
  } catch (e) {
    if (seq !== listSeq) return;
    forbidden.value = isForbiddenApiError(e);
    listError.value = e instanceof Error ? e.message : String(e);
  } finally {
    if (seq === listSeq) {
      listLoading.value = false;
      loadingMore.value = false;
    }
  }
}

async function refreshAll() {
  refreshing.value = true;
  try {
    await Promise.all([loadAccounts().catch(() => undefined), loadSummary(), loadList(true)]);
  } finally {
    refreshing.value = false;
  }
}

function onListScroll(ev: Event) {
  const el = ev.target as HTMLElement;
  if (loadingMore.value || listLoading.value || rows.value.length >= total.value) return;
  if (el.scrollTop + el.clientHeight > el.scrollHeight - 240) {
    page.value += 1;
    void loadList(false);
  }
}

let searchTimer: ReturnType<typeof setTimeout> | undefined;
watch(keyword, () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => void loadList(true), 300);
});

// 新信由后台 IDLE 同步进库；页面开着时每分钟刷新一次计数和第一页（只在页面可见、没在搜索时）。
let pollTimer: ReturnType<typeof setInterval> | undefined;
onMounted(() => {
  void refreshAll();
  // ?message=<id>：从任务「来自邮件」跳过来，直接打开那封信
  const linked = route.query.message;
  if (typeof linked === 'string' && linked)
    void openMessage({ id: linked, folder: 'inbox', has_attachments: true } as MailMessageRow);
  pollTimer = setInterval(() => {
    if (document.visibilityState !== 'visible' || keyword.value || page.value > 1) return;
    void loadSummary();
    void loadList(true);
  }, 60_000);
});
onBeforeUnmount(() => {
  clearInterval(pollTimer);
  clearTimeout(searchTimer);
});

// ---------- 视图 / 账户 ----------

function setView(v: MailView) {
  view.value = v;
  accountFilter.value = '';
  void loadList(true);
}

function setAccount(id: string) {
  accountFilter.value = accountFilter.value === id ? '' : id;
  view.value = 'inbox';
  void loadList(true);
}

function viewCount(v: MailView): number {
  const s = summary.value;
  if (!s) return 0;
  return {
    inbox: s.inbox_unread,
    unread: s.inbox_unread,
    flagged: s.flagged,
    task: s.task,
    attachments: s.attachments,
    sent: 0,
    drafts: s.drafts,
    archived: 0,
  }[v];
}

const segOptions = computed(() => [
  { value: 'inbox', label: t('mailPage.segAll') },
  {
    value: 'unread',
    label: summary.value?.inbox_unread
      ? t('mailPage.segUnreadN', { n: summary.value.inbox_unread })
      : t('mailPage.segUnread'),
  },
]);

// 窄屏（左栏收起）时，视图改成列表头上的下拉
const narrowOptions = computed(() => [
  ...VIEWS.map((v) => ({ value: `view:${v}`, label: t(`mailPage.view.${v}`) })),
  ...accounts.value.map((a) => ({ value: `acct:${a.id}`, label: a.name || a.email })),
]);
const narrowView = computed({
  get: () =>
    accountFilter.value ? `acct:${accountFilter.value}` : `view:${view.value === 'unread' ? 'inbox' : view.value}`,
  set: (v: string) => (v.startsWith('acct:') ? setAccount(v.slice(5)) : setView(v.slice(5) as MailView)),
});

function dotOf(a: MailAccountRow): string {
  if (a.status === 'ok') return 'ok';
  if (a.status === 'auth_failed' || a.status === 'unreachable') return 'bad';
  return 'unknown';
}

function accountName(id: string): string {
  const a = accounts.value.find((x) => x.id === id);
  return a ? a.name || a.email : '';
}

const groups = computed(() => groupByDate(rows.value, now.value));

/** 列表里那一位：收件箱看发件人，已发送 / 草稿看收件人。 */
function counterpart(m: MailMessageRow): string {
  return m.folder === 'sent' || m.folder === 'drafts' ? (splitAddrs(m.to)[0] ?? '') : m.sender;
}

function listWho(m: MailMessageRow): string {
  if (m.folder === 'drafts')
    return m.to ? t('mailPage.draftTo', { to: displayName(counterpart(m)) }) : t('mailPage.draftNoTo');
  if (m.folder === 'sent') return t('mailPage.sentToShort', { to: displayName(counterpart(m)) });
  return displayName(m.sender) || t('mailPage.unknownSender');
}

// ---------- 阅读 ----------

const selectedId = ref('');
const detail = ref<{ message: MailMessageRow; body: string; body_html: string } | null>(null);
const detailLoading = ref(false);
const attachments = ref<MailAttachmentRow[]>([]);
const acting = ref(false);

function patchRow(id: string, patch: Partial<MailMessageRow>) {
  const r = rows.value.find((x) => x.id === id);
  if (r) Object.assign(r, patch);
  if (detail.value?.message.id === id) Object.assign(detail.value.message, patch);
}

async function openMessage(m: MailMessageRow) {
  if (m.folder === 'drafts') {
    await openDraft(m);
    return;
  }
  selectedId.value = m.id;
  detailLoading.value = true;
  detail.value = null;
  attachments.value = [];
  try {
    const [d, atts] = await Promise.all([
      getMailMessage(m.id),
      m.has_attachments ? listMailAttachments(m.id) : Promise.resolve([]),
    ]);
    if (selectedId.value !== m.id) return;
    detail.value = d;
    attachments.value = atts;
    if (!d.message.seen) {
      patchRow(m.id, { seen: true });
      if (summary.value && d.message.folder === 'inbox' && !d.message.archived) {
        summary.value.inbox_unread = Math.max(0, summary.value.inbox_unread - 1);
        const acc = summary.value.accounts[d.message.account_id];
        if (acc) summary.value.accounts[d.message.account_id] = acc - 1;
      }
      void markMailMessages([m.id], { seen: true }).catch(() => undefined);
    }
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  } finally {
    detailLoading.value = false;
  }
}

function closeReader() {
  selectedId.value = '';
  detail.value = null;
}

async function toggleFlag() {
  const m = detail.value?.message;
  if (!m) return;
  const flagged = !m.flagged;
  patchRow(m.id, { flagged });
  try {
    await markMailMessages([m.id], { flagged });
    void loadSummary();
  } catch (e) {
    patchRow(m.id, { flagged: !flagged });
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

async function toggleArchive() {
  const m = detail.value?.message;
  if (!m) return;
  const archived = !m.archived;
  try {
    await markMailMessages([m.id], { archived });
    $q.notify({ message: archived ? t('mailPage.archived') : t('mailPage.unarchived') });
    nextAfterRemove(m.id);
    void loadSummary();
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

async function markUnread() {
  const m = detail.value?.message;
  if (!m) return;
  await markMailMessages([m.id], { seen: false });
  patchRow(m.id, { seen: false });
  closeReader();
  void loadSummary();
}

/** 归档 / 删除后从列表拿掉，并顺手打开下一封。 */
function nextAfterRemove(id: string) {
  const i = rows.value.findIndex((x) => x.id === id);
  if (i >= 0) {
    rows.value.splice(i, 1);
    total.value = Math.max(0, total.value - 1);
  }
  const next = rows.value[i] ?? rows.value[i - 1];
  if (next) void openMessage(next);
  else closeReader();
}

async function removeMessage() {
  const m = detail.value?.message;
  if (!m) return;
  const ok = await confirm({
    title: t('mailPage.deleteTitle'),
    message: t('mailPage.deleteBody', { subject: m.subject || t('mailPage.noSubject') }),
    tone: 'danger',
    confirmLabel: t('mailPage.delete'),
  });
  if (!ok) return;
  try {
    await deleteMailMessage(m.id);
    nextAfterRemove(m.id);
    void loadSummary();
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

async function toTask() {
  const m = detail.value?.message;
  if (!m) return;
  acting.value = true;
  try {
    const taskRef = await mailToTask(m.id, '');
    patchRow(m.id, { task_ref: taskRef });
    $q.notify({ type: 'positive', message: t('mailPage.toTaskDone') });
    void loadSummary();
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  } finally {
    acting.value = false;
  }
}

async function exportEml() {
  if (detail.value) await exportMailEml(detail.value.message);
}

async function downloadAtt(a: MailAttachmentRow) {
  try {
    await downloadMailAttachment(a);
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

type AttachmentPreview = {
  att: MailAttachmentRow;
  kind: AttachmentPreviewKind;
  url: string;
  html: string;
  text: string;
};
const previewOpen = ref(false);
const previewLoading = ref(false);
const preview = ref<AttachmentPreview | null>(null);
const PREVIEW_TEXT_MAX = 1 << 20;

/** 在页面里打开附件：图片 / PDF / HTML / 文本直接看，其他类型给下载。 */
async function openPreview(a: MailAttachmentRow) {
  const kind = previewKind(a.filename, a.content_type);
  preview.value = { att: a, kind, url: '', html: '', text: '' };
  previewOpen.value = true;
  if (kind === 'none') return;
  previewLoading.value = true;
  try {
    let blob = await fetchMailAttachment(a);
    if (preview.value?.att.id !== a.id) return;
    if (kind === 'pdf' && blob.type !== 'application/pdf') blob = new Blob([blob], { type: 'application/pdf' });
    if (kind === 'image' || kind === 'pdf') {
      preview.value = { ...preview.value, url: URL.createObjectURL(blob) };
    } else {
      const text = new TextDecoder('utf-8').decode(await blob.slice(0, PREVIEW_TEXT_MAX).arrayBuffer());
      preview.value = kind === 'html' ? { ...preview.value, html: attachmentSrcdoc(text) } : { ...preview.value, text };
    }
  } catch (e) {
    previewOpen.value = false;
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  } finally {
    previewLoading.value = false;
  }
}

watch(previewOpen, (open) => {
  if (open) return;
  if (preview.value?.url) URL.revokeObjectURL(preview.value.url);
  preview.value = null;
});

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
    classifyingAtt.value.knowledge_doc_id = await attachmentToKnowledge(classifyingAtt.value.id, classification.value);
    classificationOpen.value = false;
    $q.notify({ type: 'positive', message: t('mailPage.toKnowledgeDone') });
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  } finally {
    acting.value = false;
  }
}

// ---------- 写信 ----------

const compose = ref<ComposeSeed | null>(null);
const composeKey = ref(0);

function startCompose(kind: ComposeKind) {
  const d = detail.value;
  if (kind !== 'new' && !d) return;
  composeKey.value += 1;
  compose.value =
    kind === 'new' || !d
      ? { kind: 'new' }
      : { kind, message: d.message, body: d.body, carried: kind === 'forward' ? attachments.value : [] };
}

async function openDraft(m: MailMessageRow) {
  try {
    const [d, atts] = await Promise.all([
      getMailMessage(m.id),
      m.has_attachments ? listMailAttachments(m.id) : Promise.resolve([]),
    ]);
    composeKey.value += 1;
    compose.value = { kind: 'draft', message: d.message, body: d.body, carried: atts };
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

function onComposeChanged() {
  void loadSummary();
  if (view.value === 'sent' || view.value === 'drafts') void loadList(true);
}

// ---------- 账户 ----------

const wizardOpen = ref(false);
const wizardAccount = ref<MailAccountRow | null>(null);

function addAccount() {
  wizardAccount.value = null;
  wizardOpen.value = true;
}

function editAccount(a: MailAccountRow) {
  wizardAccount.value = a;
  wizardOpen.value = true;
}

async function onAccountSaved(a: MailAccountRow, isNew: boolean) {
  await loadAccounts();
  if (isNew) {
    $q.notify({ message: t('mailPage.syncStarted', { name: a.name }) });
    await syncAccount(a);
  }
}

async function syncAccount(a: MailAccountRow) {
  try {
    const r = await syncMailAccount(a.id);
    $q.notify({ type: 'positive', message: t('mailPage.syncDone', { n: r.imported }) });
  } catch (e) {
    // 超时不算失败：服务端不随请求取消，会在后台同步完
    if ((e as { code?: string }).code === 'ECONNABORTED') {
      $q.notify({ type: 'info', message: t('mailPage.syncSlow', { name: a.name }), timeout: 6000 });
    } else {
      $q.notify({ type: 'negative', message: t('mailPage.syncFailed', { name: a.name }) });
      console.warn('mail sync failed', e);
    }
  } finally {
    await Promise.all([loadAccounts().catch(() => undefined), loadSummary(), loadList(true)]);
  }
}

async function testAccount(a: MailAccountRow) {
  try {
    const r = await testMailAccount({ id: a.id });
    const parts = [
      r.imap_ok
        ? t('mailPage.testImapOk', { n: r.inbox_count })
        : t('mailPage.testImapBad', { why: t(`mailPage.err.${r.imap_error || 'protocol'}`) }),
      r.smtp_skipped
        ? t('mailPage.wizSmtpSkipped')
        : r.smtp_ok
          ? t('mailPage.testSmtpOk')
          : t('mailPage.testSmtpBad', { why: t(`mailPage.err.${r.smtp_error || 'protocol'}`) }),
    ];
    $q.notify({
      type: r.imap_ok && (r.smtp_ok || r.smtp_skipped) ? 'positive' : 'warning',
      message: parts.join('；'),
      timeout: 6000,
    });
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  } finally {
    void loadAccounts().catch(() => undefined);
  }
}

async function removeAccount(a: MailAccountRow) {
  const ok = await confirm({
    title: t('mailPage.deleteAccountTitle'),
    message: t('mailPage.deleteAccountBody', { name: a.name || a.email }),
    tone: 'danger',
    confirmLabel: t('mailPage.deleteAccount'),
  });
  if (!ok) return;
  try {
    await deleteMailAccount(a.id);
    if (accountFilter.value === a.id) accountFilter.value = '';
    await refreshAll();
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

// ---------- .eml 导入（拖进列表 / 选文件） ----------

const dragging = ref(false);
const emlPicker = ref<HTMLInputElement | null>(null);

function onDragOver(ev: DragEvent) {
  if (ev.dataTransfer?.types.includes('Files')) dragging.value = true;
}

async function importFiles(files: File[]) {
  const emls = files.filter((f) => /\.eml$/i.test(f.name) || f.type === 'message/rfc822');
  if (!emls.length) {
    $q.notify({ type: 'warning', message: t('mailPage.dropOnlyEml') });
    return;
  }
  let ok = 0;
  for (const f of emls) {
    try {
      await importMailEml(f);
      ok += 1;
    } catch (e) {
      $q.notify({ type: 'negative', message: `${f.name}: ${e instanceof Error ? e.message : String(e)}` });
    }
  }
  if (ok) $q.notify({ type: 'positive', message: t('mailPage.imported', { n: ok }) });
  await Promise.all([loadSummary(), loadList(true)]);
}

function onDrop(ev: DragEvent) {
  dragging.value = false;
  void importFiles(Array.from(ev.dataTransfer?.files ?? []));
}

function onEmlPicked(ev: Event) {
  const input = ev.target as HTMLInputElement;
  const files = Array.from(input.files ?? []);
  input.value = '';
  void importFiles(files);
}

// ---------- 键盘：J/K 上下、R 回复、A 全部回复、F 转发、E 归档、# 删除、/ 搜索、C 写邮件 ----------

const searchInput = ref<HTMLInputElement | null>(null);
const listBody = ref<HTMLElement | null>(null);

function onKey(e: KeyboardEvent) {
  if (e.ctrlKey || e.metaKey || e.altKey) return;
  const el = e.target as HTMLElement | null;
  if (el && (el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName))) return;
  if (compose.value || document.querySelector('.q-dialog')) return;
  const i = rows.value.findIndex((x) => x.id === selectedId.value);
  const go = (n: number) => {
    const r = rows.value[n];
    if (r) {
      void openMessage(r);
      listBody.value?.querySelectorAll('.ml-msg')[n]?.scrollIntoView({ block: 'nearest' });
    }
  };
  const act: Record<string, () => void> = {
    j: () => go(i + 1),
    k: () => go(Math.max(0, i - 1)),
    r: () => startCompose('reply'),
    a: () => startCompose('replyAll'),
    f: () => startCompose('forward'),
    e: () => void toggleArchive(),
    '#': () => void removeMessage(),
    '/': () => searchInput.value?.focus(),
    c: () => startCompose('new'),
  };
  const fn = act[e.key.toLowerCase()];
  if (!fn) return;
  e.preventDefault();
  fn();
}
onMounted(() => window.addEventListener('keydown', onKey));
onBeforeUnmount(() => window.removeEventListener('keydown', onKey));
</script>
