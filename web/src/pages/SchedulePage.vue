<template>
  <q-page class="app-standard-page schedule-page">
    <AppPageHero
      :kicker="t('schedulePage.kicker')"
      :title="t('schedulePage.title')"
      :subtitle="t('schedulePage.subtitle')"
    >
      <template #actions>
        <q-btn
          flat
          rounded
          no-caps
          icon="refresh"
          :label="t('schedulePage.btnRefresh')"
          :loading="loading"
          @click="loadEvents"
        />
        <q-btn flat rounded no-caps icon="rss_feed" :label="t('schedulePage.btnSubscribe')" @click="openIcalDialog" />
        <q-btn
          unelevated
          rounded
          no-caps
          color="primary"
          icon="add"
          :label="t('schedulePage.btnNew')"
          @click="openCreate"
        />
      </template>
    </AppPageHero>

    <AppStatePanel
      v-if="pageState === 'error'"
      class="q-mb-md"
      variant="error"
      :description="pageError"
      :action-label="t('schedulePage.retry')"
      action-icon="refresh"
      @action="loadEvents"
    />
    <AppStatePanel v-else-if="pageState === 'forbidden'" class="q-mb-md" variant="forbidden" />
    <AppStatePanel v-else-if="pageState === 'loading'" variant="loading" :rows="5" />

    <template v-else>
      <AppStatePanel
        v-if="pageState === 'partial'"
        class="q-mb-md"
        variant="partial"
        dense
        :description="pageError"
        :action-label="t('schedulePage.retry')"
        action-icon="refresh"
        @action="loadEvents"
      />

    <div class="schedule-layout">
      <!-- ============ 月历（一次范围拉取驱动整月渲染） ============ -->
      <section class="schedule-calendar" :aria-label="t('schedulePage.title')">
        <div class="schedule-monthbar">
          <q-btn
            flat
            dense
            round
            icon="chevron_left"
            :aria-label="t('schedulePage.prevMonth')"
            @click="shiftMonth(-1)"
          />
          <div class="schedule-monthbar__label mono">{{ monthLabel }}</div>
          <q-btn
            flat
            dense
            round
            icon="chevron_right"
            :aria-label="t('schedulePage.nextMonth')"
            @click="shiftMonth(1)"
          />
          <q-btn flat dense no-caps :label="t('schedulePage.today')" @click="goToday" />
        </div>

        <div class="schedule-grid schedule-grid--head">
          <div v-for="w in 7" :key="w" class="schedule-grid__weekday">
            {{ t(`schedulePage.weekdays.${w - 1}`) }}
          </div>
        </div>
        <div class="schedule-grid schedule-grid--body" role="grid">
          <button
            v-for="cell in monthCells"
            :key="cell.key"
            type="button"
            class="schedule-cell"
            :class="{
              'schedule-cell--other': !cell.inMonth,
              'schedule-cell--today': cell.isToday,
              'schedule-cell--selected': cell.key === selectedDay,
            }"
            :aria-label="cell.key"
            @click="selectedDay = cell.key"
          >
            <span class="schedule-cell__day mono">{{ cell.day }}</span>
            <span class="schedule-cell__events">
              <span
                v-for="ev in cell.events.slice(0, 3)"
                :key="ev.id"
                class="schedule-chip"
                :class="`schedule-chip--${ev.source || 'manual'}`"
              >
                {{ ev.title }}
              </span>
              <span v-if="cell.events.length > 3" class="schedule-cell__more mono">
                {{ t('schedulePage.more', { n: cell.events.length - 3 }) }}
              </span>
            </span>
          </button>
        </div>
      </section>

      <!-- ============ 当日时间线 ============ -->
      <aside class="schedule-day" :aria-label="t('schedulePage.dayPanelAria')">
        <div class="schedule-day__title">
          <span class="mono">{{ selectedDay }}</span>
          <q-btn flat dense round icon="add" :aria-label="t('schedulePage.btnNew')" @click="openCreate" />
        </div>
        <AppStatePanel
          v-if="!dayEvents.length"
          class="q-mt-sm"
          variant="empty"
          dense
          :title="t('schedulePage.emptyDay')"
          icon="event_available"
        />
        <q-list v-else separator class="schedule-day__list">
          <q-item v-for="ev in dayEvents" :key="ev.id" class="schedule-day__item">
            <q-item-section>
              <q-item-label class="schedule-day__item-title">
                {{ ev.title }}
                <q-badge
                  v-if="ev.source && ev.source !== 'manual'"
                  outline
                  color="secondary"
                  :label="t(`schedulePage.source.${ev.source}`)"
                />
              </q-item-label>
              <q-item-label caption class="mono">
                {{ fmtTimeRange(ev) }}
                <template v-if="ev.reminder_minutes > 0">
                  · {{ t('schedulePage.reminderLabel', { n: ev.reminder_minutes }) }}
                </template>
              </q-item-label>
              <q-item-label v-if="ev.note" caption lines="2">{{ ev.note }}</q-item-label>
              <q-item-label v-if="ev.task_ref" caption class="mono">{{ ev.task_ref }}</q-item-label>
            </q-item-section>
            <q-item-section side top>
              <div class="row no-wrap">
                <q-btn
                  flat
                  dense
                  round
                  size="sm"
                  icon="edit"
                  :aria-label="t('schedulePage.edit')"
                  @click="openEdit(ev)"
                />
                <q-btn
                  flat
                  dense
                  round
                  size="sm"
                  icon="delete_outline"
                  :aria-label="t('schedulePage.delete')"
                  @click="confirmDelete(ev)"
                />
              </div>
            </q-item-section>
          </q-item>
        </q-list>
      </aside>
    </div>
    </template>

    <!-- ============ 新建/编辑事件 ============ -->
    <AppDialog
      v-model="editorOpen"
      persistent
      size="sm"
      :title="editingId ? t('schedulePage.editTitle') : t('schedulePage.createTitle')"
    >
      <div class="q-gutter-md">
        <q-banner v-if="editorError" rounded class="app-page-error-banner" role="alert">
          {{ editorError }}
        </q-banner>
        <q-input v-model="form.title" outlined dense :label="t('schedulePage.fieldTitle')" autofocus />
        <q-input v-model="form.note" outlined dense type="textarea" autogrow :label="t('schedulePage.fieldNote')" />
        <div class="row q-gutter-md">
          <q-input
            v-model="form.start"
            class="col"
            outlined
            dense
            type="datetime-local"
            :label="t('schedulePage.fieldStart')"
          />
          <q-input
            v-model="form.end"
            class="col"
            outlined
            dense
            type="datetime-local"
            :label="t('schedulePage.fieldEnd')"
          />
        </div>
        <q-select
          v-model="form.reminder_minutes"
          outlined
          dense
          emit-value
          map-options
          :options="reminderOptions"
          :label="t('schedulePage.fieldReminder')"
        />
      </div>
      <template #actions>
        <q-btn flat no-caps :label="t('schedulePage.cancel')" @click="editorOpen = false" />
        <q-btn
          unelevated
          no-caps
          color="primary"
          :label="t('schedulePage.save')"
          :loading="saving"
          :disable="!form.title || !form.start || !form.end"
          @click="saveEvent"
        />
      </template>
    </AppDialog>

    <!-- ============ ics 订阅 ============ -->
    <AppDialog v-model="icalOpen" size="md" :title="t('schedulePage.icalTitle')">
      <div class="q-gutter-md">
        <q-banner v-if="icalError" rounded class="app-page-error-banner" role="alert">{{ icalError }}</q-banner>
        <div class="row q-gutter-sm items-center">
          <q-input v-model="icalName" class="col" outlined dense :label="t('schedulePage.icalNameLabel')" />
          <q-btn
            unelevated
            no-caps
            color="primary"
            :label="t('schedulePage.icalCreate')"
            :loading="icalSaving"
            :disable="!icalName.trim()"
            @click="createToken"
          />
        </div>
        <q-banner v-if="newFeedUrl" rounded class="schedule-ical__once">
          <div class="text-caption">{{ t('schedulePage.icalOnceHint') }}</div>
          <div class="row items-center no-wrap q-gutter-sm q-mt-xs">
            <code class="mono schedule-ical__url">{{ icalFeedAbsolute(newFeedUrl) }}</code>
            <q-btn
              flat
              dense
              round
              size="sm"
              icon="content_copy"
              :aria-label="t('schedulePage.icalCopy')"
              @click="copyFeedUrl"
            />
          </div>
        </q-banner>
        <q-list v-if="icalTokens.length" separator dense>
          <q-item v-for="tk in icalTokens" :key="tk.id">
            <q-item-section>
              <q-item-label>{{ tk.name }}</q-item-label>
              <q-item-label caption class="mono">{{ fmtDay(tk.created_at) }}</q-item-label>
            </q-item-section>
            <q-item-section side>
              <q-badge v-if="tk.revoked_at" outline color="grey" :label="t('schedulePage.icalRevoked')" />
              <q-btn
                v-else
                flat
                dense
                no-caps
                size="sm"
                color="negative"
                :label="t('schedulePage.icalRevoke')"
                @click="revokeToken(tk)"
              />
            </q-item-section>
          </q-item>
        </q-list>
        <div v-else class="text-caption text-grey">{{ t('schedulePage.icalEmpty') }}</div>
      </div>
      <template #actions>
        <q-btn flat no-caps :label="t('schedulePage.cancel')" @click="icalOpen = false" />
      </template>
    </AppDialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { confirm } from '../composables/useAppConfirm';
import { AxiosError } from 'axios';
import AppDialog from '../components/common/AppDialog.vue';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import { resolvePageState } from './pageState';
import { isForbiddenApiError } from '../utils/kratosError';
import {
  createEvent,
  createICalToken,
  deleteEvent,
  listEventsRange,
  listICalTokens,
  revokeICalToken,
  updateEvent,
} from '../features/office/api';
import type { ICalTokenRow, OfficeEventRow } from '../features/office/types';

const { t } = useI18n();
const $q = useQuasar();

// ---------- 月历窗口（K-1 验收③：每个可见月仅一次范围请求） ----------

const now = new Date();
const visibleMonth = ref(new Date(now.getFullYear(), now.getMonth(), 1));
const events = ref<OfficeEventRow[]>([]);
const loading = ref(false);
const pageError = ref('');
/** 403 / PERMISSION_DENIED：页面渲染 forbidden 态，不提供重试。 */
const pageForbidden = ref(false);

/** 五态判定：错误优先于空态；有数据时失败保留月历并加 partial 横幅。 */
const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: pageError.value,
    count: events.value.length,
    forbidden: pageForbidden.value,
  }),
);

const DAY_MS = 86_400_000;

function dayKey(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** 可见网格范围 [gridStart, gridStart+42d)：周一起始，覆盖整月。 */
const gridStart = computed(() => {
  const first = visibleMonth.value;
  const dow = (first.getDay() + 6) % 7; // 周一=0
  return new Date(first.getFullYear(), first.getMonth(), first.getDate() - dow);
});

type DayCell = { key: string; day: number; inMonth: boolean; isToday: boolean; events: OfficeEventRow[] };

const eventsByDay = computed(() => {
  const map = new Map<string, OfficeEventRow[]>();
  for (const ev of events.value) {
    // 与当天相交即落桶（跨天事件多天可见）。
    const start = new Date(ev.start_at);
    const end = new Date(ev.end_at);
    if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) continue;
    let cur = new Date(start.getFullYear(), start.getMonth(), start.getDate());
    const last = new Date(end.getFullYear(), end.getMonth(), end.getDate());
    while (cur.getTime() <= last.getTime()) {
      const k = dayKey(cur);
      const arr = map.get(k);
      if (arr) arr.push(ev);
      else map.set(k, [ev]);
      cur = new Date(cur.getTime() + DAY_MS);
    }
  }
  return map;
});

const monthCells = computed<DayCell[]>(() => {
  const start = gridStart.value;
  const todayKey = dayKey(new Date());
  const cells: DayCell[] = [];
  for (let i = 0; i < 42; i++) {
    const d = new Date(start.getTime() + i * DAY_MS);
    const key = dayKey(d);
    cells.push({
      key,
      day: d.getDate(),
      inMonth: d.getMonth() === visibleMonth.value.getMonth(),
      isToday: key === todayKey,
      events: eventsByDay.value.get(key) ?? [],
    });
  }
  return cells;
});

const monthLabel = computed(() => {
  const m = visibleMonth.value;
  return `${m.getFullYear()}-${String(m.getMonth() + 1).padStart(2, '0')}`;
});

const selectedDay = ref(dayKey(now));

const dayEvents = computed(() =>
  (eventsByDay.value.get(selectedDay.value) ?? []).slice().sort((a, b) => a.start_at.localeCompare(b.start_at)),
);

function shiftMonth(delta: number) {
  const m = visibleMonth.value;
  visibleMonth.value = new Date(m.getFullYear(), m.getMonth() + delta, 1);
}

function goToday() {
  const d = new Date();
  visibleMonth.value = new Date(d.getFullYear(), d.getMonth(), 1);
  selectedDay.value = dayKey(d);
}

async function loadEvents() {
  loading.value = true;
  pageError.value = '';
  pageForbidden.value = false;
  try {
    const start = gridStart.value;
    const end = new Date(start.getTime() + 42 * DAY_MS);
    events.value = await listEventsRange(start.toISOString(), end.toISOString());
  } catch (e) {
    pageError.value = e instanceof Error ? e.message : String(e);
    pageForbidden.value = isForbiddenApiError(e);
  } finally {
    loading.value = false;
  }
}

// 月切换 = 唯一拉取触发点（每可见月一次；手动刷新走 loadEvents 直调）。
watch(visibleMonth, () => void loadEvents());
onMounted(() => void loadEvents());

// ---------- 新建/编辑 ----------

const editorOpen = ref(false);
const editorError = ref('');
const saving = ref(false);
const editingId = ref('');

const form = ref({
  title: '',
  note: '',
  start: '',
  end: '',
  reminder_minutes: 0,
});

const reminderOptions = computed(() => [
  { value: 0, label: t('schedulePage.reminderNone') },
  { value: 5, label: t('schedulePage.reminderMinutes', { n: 5 }) },
  { value: 15, label: t('schedulePage.reminderMinutes', { n: 15 }) },
  { value: 30, label: t('schedulePage.reminderMinutes', { n: 30 }) },
  { value: 60, label: t('schedulePage.reminderMinutes', { n: 60 }) },
]);

function toLocalInput(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}

function fromLocalInput(v: string): Date | null {
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? null : d;
}

function openCreate() {
  editingId.value = '';
  editorError.value = '';
  const base = selectedDay.value ? new Date(`${selectedDay.value}T09:00`) : new Date();
  const start = Number.isNaN(base.getTime()) ? new Date() : base;
  form.value = {
    title: '',
    note: '',
    start: toLocalInput(start),
    end: toLocalInput(new Date(start.getTime() + 3_600_000)),
    reminder_minutes: 0,
  };
  editorOpen.value = true;
}

function openEdit(ev: OfficeEventRow) {
  editingId.value = ev.id;
  editorError.value = '';
  form.value = {
    title: ev.title,
    note: ev.note,
    start: toLocalInput(new Date(ev.start_at)),
    end: toLocalInput(new Date(ev.end_at)),
    reminder_minutes: ev.reminder_minutes,
  };
  editorOpen.value = true;
}

async function saveEvent() {
  const start = fromLocalInput(form.value.start);
  const end = fromLocalInput(form.value.end);
  if (!start || !end) return;
  saving.value = true;
  editorError.value = '';
  const input = {
    title: form.value.title,
    note: form.value.note,
    start_at: start.toISOString(),
    end_at: end.toISOString(),
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Shanghai',
    reminder_minutes: form.value.reminder_minutes,
  };
  try {
    if (editingId.value) {
      await updateEvent(editingId.value, input);
    } else {
      await createEvent(input);
    }
    editorOpen.value = false;
    await loadEvents();
  } catch (e) {
    // 409 = 时间重叠（K-2 硬冲突）；弹层保持打开，错误就地呈现。
    editorError.value =
      e instanceof AxiosError && e.response?.status === 409
        ? t('schedulePage.conflictError')
        : e instanceof Error
          ? e.message
          : String(e);
  } finally {
    saving.value = false;
  }
}

async function confirmDelete(ev: OfficeEventRow) {
  const ok = await confirm({
    title: t('schedulePage.deleteConfirmTitle'),
    message: t('schedulePage.deleteConfirmBody', { title: ev.title }),
    tone: 'danger',
  });
  if (!ok) return;
  try {
    await deleteEvent(ev.id);
    await loadEvents();
  } catch (e) {
    pageError.value = e instanceof Error ? e.message : String(e);
  }
}

// ---------- ics 订阅 ----------

const icalOpen = ref(false);
const icalTokens = ref<ICalTokenRow[]>([]);
const icalError = ref('');
const icalSaving = ref(false);
const icalName = ref('');
const newFeedUrl = ref('');

function openIcalDialog() {
  icalOpen.value = true;
  icalError.value = '';
  newFeedUrl.value = '';
  void refreshTokens();
}

async function refreshTokens() {
  try {
    icalTokens.value = await listICalTokens();
  } catch (e) {
    icalError.value = e instanceof Error ? e.message : String(e);
  }
}

async function createToken() {
  icalSaving.value = true;
  icalError.value = '';
  try {
    const tk = await createICalToken(icalName.value.trim());
    newFeedUrl.value = tk.feed_url; // 明文仅此一次（服务端契约）
    icalName.value = '';
    await refreshTokens();
  } catch (e) {
    icalError.value = e instanceof Error ? e.message : String(e);
  } finally {
    icalSaving.value = false;
  }
}

async function revokeToken(tk: ICalTokenRow) {
  try {
    await revokeICalToken(tk.id);
    await refreshTokens();
  } catch (e) {
    icalError.value = e instanceof Error ? e.message : String(e);
  }
}

function icalFeedAbsolute(path: string): string {
  if (!path) return '';
  return `${window.location.origin}${path}`;
}

async function copyFeedUrl() {
  try {
    await navigator.clipboard.writeText(icalFeedAbsolute(newFeedUrl.value));
    $q.notify({ type: 'positive', message: t('schedulePage.icalCopied') });
  } catch {
    $q.notify({ type: 'negative', message: t('schedulePage.icalCopyFailed') });
  }
}

// ---------- 呈现 ----------

function fmtTimeRange(ev: OfficeEventRow): string {
  const s = new Date(ev.start_at);
  const e = new Date(ev.end_at);
  const hm = (d: Date) => `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
  if (Number.isNaN(s.getTime()) || Number.isNaN(e.getTime())) return '';
  return `${hm(s)} ~ ${hm(e)}`;
}

function fmtDay(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? '' : dayKey(d);
}
</script>

<style scoped lang="sass">
// 令牌只取 --nt-*（AGENTS.md §6）；时间/日期一律等宽 + tabular-nums。
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
:deep(.mono)
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums

.schedule-layout
  display: grid
  grid-template-columns: minmax(0, 1fr) 320px
  gap: 16px
  align-items: start

.schedule-calendar
  background: var(--nt-bg-1)
  border: 1px solid var(--nt-border-1)
  border-radius: 12px
  padding: 12px

.schedule-monthbar
  display: flex
  align-items: center
  gap: 8px
  margin-bottom: 8px

.schedule-monthbar__label
  font-size: 16px
  font-weight: 600
  min-width: 96px
  text-align: center

.schedule-grid
  display: grid
  grid-template-columns: repeat(7, minmax(0, 1fr))

.schedule-grid__weekday
  text-align: center
  font-size: 12px
  color: var(--nt-text-2)
  padding: 4px 0

.schedule-cell
  display: flex
  flex-direction: column
  gap: 2px
  min-height: 72px
  min-width: 44px
  padding: 6px
  border: 1px solid var(--nt-border-1)
  border-radius: 6px
  background: transparent
  color: inherit
  text-align: left
  cursor: pointer

.schedule-cell:hover
  background: var(--nt-bg-2)

.schedule-cell:focus-visible
  outline: 2px solid var(--nt-accent)
  outline-offset: 2px

.schedule-cell--other
  opacity: 0.45

.schedule-cell--today .schedule-cell__day
  color: var(--nt-accent)
  font-weight: 700

.schedule-cell--selected
  background: var(--nt-bg-2)
  border-color: var(--nt-accent)

.schedule-cell__day
  font-size: 12px

.schedule-cell__events
  display: flex
  flex-direction: column
  gap: 2px
  overflow: hidden

.schedule-chip
  font-size: 11px
  line-height: 16px
  padding: 0 6px
  border-radius: 6px
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis
  background: var(--nt-bg-3)
  color: var(--nt-text-1)

.schedule-chip--task_schedule
  color: var(--nt-accent)

.schedule-chip--agent_draft
  color: var(--nt-ai)

.schedule-cell__more
  font-size: 11px
  color: var(--nt-text-2)

.schedule-day
  background: var(--nt-bg-1)
  border-radius: 12px
  padding: 12px

.schedule-day__title
  display: flex
  align-items: center
  justify-content: space-between
  font-weight: 600
  margin-bottom: 8px

.schedule-day__empty
  color: var(--nt-text-2)
  padding: 16px 0
  text-align: center

.schedule-day__item-title
  display: flex
  align-items: center
  gap: 6px

.schedule-ical
  min-width: min(560px, 94vw)

.schedule-ical__url
  word-break: break-all
  font-size: 12px

// 断点 1099：侧栏转下排（AGENTS.md §6-10）。
@media (max-width: 1099px)
  .schedule-layout
    grid-template-columns: minmax(0, 1fr)
</style>
