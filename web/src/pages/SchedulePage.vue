<template>
  <q-page class="app-standard-page sch-page">
    <!-- 操作放在页签行右边；日程不自动拉取，刷新只留图标 -->
    <Teleport defer to="#app-tab-actions">
      <q-btn
        flat
        dense
        round
        icon="refresh"
        :title="t('schedulePage.btnRefresh')"
        :aria-label="t('schedulePage.btnRefresh')"
        :loading="loading"
        @click="reload"
      />
      <q-btn flat dense no-caps icon="rss_feed" :label="t('schedulePage.btnSubscribe')" @click="icalOpen = true" />
      <q-btn
        unelevated
        dense
        no-caps
        color="primary"
        icon="add"
        :label="t('schedulePage.btnNew')"
        @click="startNew('plan')"
      />
    </Teleport>

    <AppStatePanel v-if="pageForbidden" class="sch-forbidden" variant="forbidden" />

    <template v-else>
      <!-- ============ 月历：一屏 6 × 7 ============ -->
      <section class="sch-panel sch-cal" :aria-label="t('schedulePage.title')" :aria-busy="loading">
        <div class="sch-bar">
          <q-btn
            flat
            dense
            round
            icon="chevron_left"
            :aria-label="t('schedulePage.prevMonth')"
            @click="shiftMonth(-1)"
          />
          <button type="button" class="sch-month" :class="{ 'sch-month--on': pickerOpen }" :aria-expanded="pickerOpen">
            <span class="sch-num">{{
              t('schedulePage.monthLabel', { y: month.getFullYear(), m: month.getMonth() + 1 })
            }}</span>
            <q-icon name="expand_more" size="18px" aria-hidden="true" />
            <q-menu
              v-model="pickerOpen"
              anchor="bottom left"
              self="top left"
              :offset="[0, 6]"
              class="sch-picker-menu"
              @before-show="openPicker"
            >
              <div class="sch-picker" role="dialog" :aria-label="t('schedulePage.pickMonth')">
                <div class="sch-picker__y">
                  <q-btn
                    flat
                    dense
                    round
                    icon="chevron_left"
                    :aria-label="t('schedulePage.prevYear')"
                    @click="pickerYear--"
                  />
                  <span class="sch-num">{{ t('schedulePage.pickerYear', { y: pickerYear }) }}</span>
                  <q-btn
                    flat
                    dense
                    round
                    icon="chevron_right"
                    :aria-label="t('schedulePage.nextYear')"
                    @click="pickerYear++"
                  />
                </div>
                <div class="sch-picker__grid">
                  <button
                    v-for="(n, i) in yearCounts"
                    :key="i"
                    v-close-popup
                    type="button"
                    :class="{
                      'sch-picker__m--on': pickerYear === month.getFullYear() && i === month.getMonth(),
                      'sch-picker__m--now': pickerYear === today.getFullYear() && i === today.getMonth(),
                    }"
                    class="sch-picker__m"
                    :aria-label="t('schedulePage.monthPlans', { m: i + 1, n })"
                    @click="goMonth(pickerYear, i)"
                  >
                    {{ t('schedulePage.monthShort', { m: i + 1 }) }}
                    <small v-if="n" class="sch-num">{{ n }}</small>
                  </button>
                </div>
                <p class="sch-hint">{{ t('schedulePage.pickerHint') }}</p>
              </div>
            </q-menu>
          </button>
          <q-btn
            flat
            dense
            round
            icon="chevron_right"
            :aria-label="t('schedulePage.nextMonth')"
            @click="shiftMonth(1)"
          />
          <q-btn flat dense no-caps :label="t('schedulePage.today')" @click="goToday" />

          <dl class="sch-stats">
            <div>
              <dt>{{ t('schedulePage.stats.plans') }}</dt>
              <dd class="sch-num">{{ stats.plans }}</dd>
            </div>
            <div>
              <dt>{{ t('schedulePage.stats.done') }}</dt>
              <dd class="sch-num">{{ stats.done }}</dd>
            </div>
            <div :class="{ 'sch-stats--bad': stats.overdue }">
              <dt>{{ t('schedulePage.stats.overdue') }}</dt>
              <dd class="sch-num">{{ stats.overdue }}</dd>
            </div>
            <div>
              <dt>{{ t('schedulePage.stats.logs') }}</dt>
              <dd class="sch-num">{{ stats.logs }}</dd>
            </div>
          </dl>

          <div class="sch-filters" role="group" :aria-label="t('schedulePage.filter.aria')">
            <button
              v-for="k in FILTERS"
              :key="k"
              type="button"
              class="sch-fchip"
              :class="[`sch-fchip--${k}`, { 'sch-fchip--off': !show[k] }]"
              :aria-pressed="show[k]"
              @click="show[k] = !show[k]"
            >
              <i aria-hidden="true" />{{ t(`schedulePage.filter.${k}`) }}
            </button>
          </div>
        </div>

        <AppStatePanel
          v-if="pageError"
          class="q-mb-sm"
          variant="partial"
          dense
          :description="pageError"
          :action-label="t('schedulePage.retry')"
          action-icon="refresh"
          @action="reload"
        />

        <div class="sch-wk" aria-hidden="true">
          <span v-for="w in 7" :key="w">{{ t(`schedulePage.weekdays.${w - 1}`) }}</span>
        </div>
        <div class="sch-days">
          <button
            v-for="cell in cells"
            :key="cell.key"
            type="button"
            class="sch-day"
            :class="{
              'sch-day--out': !cell.inMonth,
              'sch-day--wkend': cell.weekend,
              'sch-day--today': cell.isToday,
              'sch-day--sel': cell.key === selectedDay,
            }"
            :aria-pressed="cell.key === selectedDay"
            :aria-label="cellAria(cell)"
            @click="selectDay(cell.key)"
          >
            <span class="sch-day__n sch-num"
              ><b>{{ cell.day }}</b></span
            >
            <template v-for="(line, i) in cell.lines" :key="i">
              <span v-if="line.type === 'plan'" class="sch-it sch-it--plan" :class="`sch-st--${line.status}`">
                <span v-if="!line.event.all_day" class="sch-it__tm sch-num">{{ hm(line.event.start_at) }}</span>
                <span class="sch-it__txt">{{ line.event.title }}</span>
              </span>
              <span v-else-if="line.type === 'log'" class="sch-it sch-it--log">
                <q-icon name="check" size="13px" aria-hidden="true" />
                <span class="sch-it__txt">{{
                  line.count === 1
                    ? t('schedulePage.didOne', { title: line.title })
                    : t('schedulePage.didN', { n: line.count })
                }}</span>
              </span>
              <span v-else class="sch-it sch-it--ai">
                <q-icon name="auto_awesome" size="13px" aria-hidden="true" />
                <span class="sch-it__txt">{{ t('schedulePage.aiN', { n: line.count }) }}</span>
              </span>
            </template>
            <span v-if="cell.more" class="sch-day__more sch-num">{{ t('schedulePage.more', { n: cell.more }) }}</span>
          </button>
        </div>
      </section>

      <!-- ============ 当天：计划 / 工作内容 / AI 建议；点一条就地编辑 ============ -->
      <aside class="sch-panel sch-dp" :aria-label="t('schedulePage.dayAria')">
        <ScheduleEventEditor
          v-if="editing"
          :key="editing.event?.id ?? `new-${editing.kind}`"
          :event="editing.event"
          :day="selectedDay"
          :kind="editing.kind"
          @close="closeEditor"
          @saved="onSaved"
        />
        <template v-else>
          <div class="sch-dp__head">
            <h2 class="sch-dp__title sch-num">{{ dayTitle }}</h2>
            <span class="sch-muted">{{ dayWeekday }}</span>
            <span class="sch-sp" />
            <q-btn flat dense no-caps icon="add" :label="t('schedulePage.addPlan')" @click="startNew('plan')" />
            <q-btn flat dense no-caps icon="add" :label="t('schedulePage.addLog')" @click="startNew('log')" />
          </div>

          <div class="sch-dp__body">
            <div class="sch-sect">
              {{ t('schedulePage.sectPlan') }}<span class="sch-sect__n sch-num">{{ dayPlans.length }}</span>
              <span class="sch-sp sch-hint">{{ t('schedulePage.planHint') }}</span>
            </div>
            <p v-if="!dayPlans.length" class="sch-hint">{{ t('schedulePage.emptyPlan') }}</p>
            <div v-for="ev in dayPlans" :key="ev.id" class="sch-ent">
              <button type="button" class="sch-ent__main" @click="startEdit(ev)">
                <span class="sch-ent__t sch-num">{{ timeText(ev) }}</span>
                <span class="sch-ent__title" :class="`sch-st--${statusOf(ev)}`">{{ ev.title }}</span>
                <span v-if="ev.reminder_minutes || ev.task_ref || ev.outcome" class="sch-ent__meta">
                  <span v-if="ev.reminder_minutes" class="sch-chip">
                    <q-icon name="notifications_none" size="13px" aria-hidden="true" />{{
                      reminderText(ev.reminder_minutes)
                    }}
                  </span>
                  <span v-if="ev.task_ref" class="sch-chip">
                    <q-icon name="link" size="13px" aria-hidden="true" />{{ ev.task_title || ev.task_ref }}
                  </span>
                  <span v-if="ev.outcome" class="sch-ent__sub">{{ ev.outcome }}</span>
                </span>
              </button>
              <button
                type="button"
                class="sch-stbtn"
                :class="`sch-st--${statusOf(ev)}`"
                :disabled="statusBusy === ev.id"
                :aria-label="
                  t('schedulePage.statusAria', {
                    s: t(`schedulePage.status.${statusOf(ev)}`),
                    next: t(`schedulePage.status.${nextStatus(ev.status)}`),
                  })
                "
                @click="cycleStatus(ev)"
              >
                {{ t(`schedulePage.status.${statusOf(ev)}`) }}
              </button>
            </div>

            <div class="sch-sect">
              {{ t('schedulePage.sectLog') }}<span class="sch-sect__n sch-num">{{ dayLogs.length }}</span>
            </div>
            <p v-if="!dayLogs.length" class="sch-hint">{{ t('schedulePage.emptyLog') }}</p>
            <div v-for="ev in dayLogs" :key="ev.id" class="sch-ent sch-ent--log">
              <button type="button" class="sch-ent__main" @click="startEdit(ev)">
                <span class="sch-ent__t sch-num">{{ timeText(ev) }}</span>
                <span class="sch-ent__title">{{ ev.title }}</span>
                <span class="sch-ent__meta">
                  <span v-if="ev.outcome" class="sch-ent__sub">{{ ev.outcome }}</span>
                  <span class="sch-chip" :class="{ 'sch-chip--ai': ev.source === 'agent_draft' }">
                    {{ t(`schedulePage.source.${ev.source || 'manual'}`) }}
                  </span>
                </span>
              </button>
            </div>

            <template v-if="dayDrafts.length">
              <div class="sch-sect sch-sect--ai">
                <q-icon name="auto_awesome" size="14px" aria-hidden="true" />{{ t('schedulePage.sectAi')
                }}<span class="sch-sect__n sch-num">{{ dayDrafts.length }}</span>
                <span class="sch-sp sch-hint">{{ t('schedulePage.aiHint') }}</span>
              </div>
              <div v-for="d in dayDrafts" :key="d.key" class="sch-ai">
                <p class="sch-ai__what">
                  <b class="sch-num">{{ hm(d.start_at) }}–{{ hm(d.end_at) }}</b> {{ d.title }}
                </p>
                <p v-if="d.note" class="sch-ai__why">{{ d.note }}</p>
                <p v-if="d.total > 1" class="sch-hint">{{ t('schedulePage.draftBatch', { n: d.total }) }}</p>
                <div class="sch-ai__acts">
                  <q-btn
                    unelevated
                    dense
                    no-caps
                    color="primary"
                    :label="t('schedulePage.accept')"
                    :loading="draftBusy === d.inboxId"
                    :disable="!!draftBusy"
                    @click="decideDraft(d, 'approve')"
                  />
                  <q-btn
                    flat
                    dense
                    no-caps
                    :label="t('schedulePage.reject')"
                    :disable="!!draftBusy"
                    @click="decideDraft(d, 'reject')"
                  />
                </div>
              </div>
            </template>
          </div>
        </template>
      </aside>
    </template>

    <ScheduleICalDialog v-model="icalOpen" />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { AxiosError } from 'axios';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import ScheduleEventEditor from '../features/schedule/components/ScheduleEventEditor.vue';
import ScheduleICalDialog from '../features/schedule/components/ScheduleICalDialog.vue';
import { isForbiddenApiError } from '../utils/kratosError';
import { listEventsRange, setEventStatus } from '../features/office/api';
import type { OfficeEventKind, OfficeEventRow } from '../features/office/types';
import { decideInboxItem, listInbox } from '../features/approval/api';
import {
  bucketByDay,
  dayKey,
  displayStatus,
  gridStart,
  hm,
  monthCells,
  monthPlanCounts,
  monthStats,
  nextStatus,
  parseDayKey,
  parseDrafts,
  sortDay,
  type DayCell,
  type DraftEntry,
} from '../features/schedule/model';

const { t } = useI18n();
const $q = useQuasar();

const FILTERS = ['plan', 'log', 'ai'] as const;

const today = ref(new Date());
const month = ref(new Date(today.value.getFullYear(), today.value.getMonth(), 1));
const selectedDay = ref(dayKey(today.value));
const events = ref<OfficeEventRow[]>([]);
const drafts = ref<DraftEntry[]>([]);
const show = reactive({ plan: true, log: true, ai: true });
const loading = ref(false);
const pageError = ref('');
const pageForbidden = ref(false);
const icalOpen = ref(false);

// ---------- 拉取：每个可见月一次范围请求 + 一次草案列表 ----------

async function loadEvents() {
  loading.value = true;
  pageError.value = '';
  try {
    const start = gridStart(month.value);
    const end = new Date(start.getFullYear(), start.getMonth(), start.getDate() + 42);
    events.value = await listEventsRange(start.toISOString(), end.toISOString());
    today.value = new Date();
  } catch (e) {
    pageError.value = e instanceof Error ? e.message : String(e);
    pageForbidden.value = isForbiddenApiError(e);
  } finally {
    loading.value = false;
  }
}

async function loadDrafts() {
  try {
    drafts.value = parseDrafts((await listInbox({ sources: ['schedule_draft'], pageSize: 100 })).items);
  } catch (e) {
    // 草案只是建议：拉不到不挡月历（比如没有审批权限）。
    console.warn('schedule drafts unavailable', e);
    drafts.value = [];
  }
}

function reload() {
  void loadEvents();
  void loadDrafts();
}

watch(month, () => void loadEvents());
onMounted(reload);

// ---------- 月份 ----------

function goMonth(y: number, m: number) {
  month.value = new Date(y, m, 1);
  const now = today.value;
  // 换月后当天面板跟过去：本月就停在今天，别的月停在 1 号。
  selectedDay.value = y === now.getFullYear() && m === now.getMonth() ? dayKey(now) : dayKey(month.value);
  editing.value = null;
}

function shiftMonth(delta: number) {
  goMonth(month.value.getFullYear(), month.value.getMonth() + delta);
}

function goToday() {
  const now = new Date();
  goMonth(now.getFullYear(), now.getMonth());
}

/** PageUp / PageDown 翻月（输入框里不抢键）。 */
function onKey(e: KeyboardEvent) {
  if (e.key !== 'PageUp' && e.key !== 'PageDown') return;
  const el = e.target as HTMLElement | null;
  if (el && (el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName))) return;
  e.preventDefault();
  shiftMonth(e.key === 'PageUp' ? -1 : 1);
}
onMounted(() => window.addEventListener('keydown', onKey));
onBeforeUnmount(() => window.removeEventListener('keydown', onKey));

// 选月面板：打开时按年拉一次，算 12 个月各有多少计划。
const pickerOpen = ref(false);
const pickerYear = ref(month.value.getFullYear());
const yearCounts = ref<number[]>(Array.from({ length: 12 }, () => 0));

function openPicker() {
  pickerYear.value = month.value.getFullYear();
  void loadYear();
}

async function loadYear() {
  const y = pickerYear.value;
  try {
    const rows = await listEventsRange(new Date(y, 0, 1).toISOString(), new Date(y + 1, 0, 1).toISOString());
    if (pickerYear.value === y) yearCounts.value = monthPlanCounts(rows, y);
  } catch {
    yearCounts.value = Array.from({ length: 12 }, () => 0);
  }
}
watch(pickerYear, () => {
  if (pickerOpen.value) void loadYear();
});

// ---------- 月格与统计 ----------

const cells = computed(() => monthCells(month.value, events.value, drafts.value, show, today.value));
const stats = computed(() => monthStats(events.value, month.value, today.value));

function cellAria(cell: DayCell): string {
  const n = cell.lines.reduce((sum, l) => sum + (l.type === 'plan' ? 1 : l.count), 0) + cell.more;
  return n ? t('schedulePage.cellAria', { date: cell.key, n }) : cell.key;
}

// ---------- 当天面板 ----------

const byDay = computed(() => bucketByDay(events.value, (e) => [e.start_at, e.end_at]));
const dayEvents = computed(() => sortDay(byDay.value.get(selectedDay.value) ?? []));
const dayPlans = computed(() => dayEvents.value.filter((e) => e.kind === 'plan'));
const dayLogs = computed(() => dayEvents.value.filter((e) => e.kind === 'log'));
const dayDrafts = computed(() => bucketByDay(drafts.value, (d) => [d.start_at, d.end_at]).get(selectedDay.value) ?? []);

const dayTitle = computed(() => {
  const d = parseDayKey(selectedDay.value);
  return t('schedulePage.dayTitle', { m: d.getMonth() + 1, d: d.getDate() });
});
const dayWeekday = computed(() => {
  const d = parseDayKey(selectedDay.value);
  const w = t(`schedulePage.weekdays.${(d.getDay() + 6) % 7}`);
  const base = t('schedulePage.weekday', { w });
  return selectedDay.value === dayKey(today.value) ? `${base} · ${t('schedulePage.today')}` : base;
});

function selectDay(key: string) {
  selectedDay.value = key;
  editing.value = null;
}

function statusOf(ev: OfficeEventRow) {
  return displayStatus(ev, today.value);
}

function timeText(ev: OfficeEventRow): string {
  return ev.all_day ? t('schedulePage.allDay') : `${hm(ev.start_at)}–${hm(ev.end_at)}`;
}

function reminderText(n: number): string {
  const v =
    n >= 1440 && n % 1440 === 0
      ? t('schedulePage.days', { n: n / 1440 })
      : n >= 60 && n % 60 === 0
        ? t('schedulePage.hours', { n: n / 60 })
        : t('schedulePage.minutes', { n });
  return t('schedulePage.reminderOpt', { v });
}

const statusBusy = ref('');

async function cycleStatus(ev: OfficeEventRow) {
  statusBusy.value = ev.id;
  try {
    const row = await setEventStatus(ev, nextStatus(ev.status));
    events.value = events.value.map((e) => (e.id === row.id ? row : e));
    // 标完成时问一句「做成什么样」（08 §3.3），点了就地打开编辑写完成情况
    if (row.status === 'done' && !row.outcome.trim()) {
      $q.notify({
        message: t('schedulePage.askOutcome', { title: row.title }),
        actions: [{ label: t('schedulePage.writeOutcome'), color: 'primary', handler: () => startEdit(row) }],
      });
    }
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  } finally {
    statusBusy.value = '';
  }
}

// ---------- 就地编辑 ----------

const editing = ref<{ event: OfficeEventRow | null; kind: OfficeEventKind } | null>(null);

function startNew(kind: OfficeEventKind) {
  editing.value = { event: null, kind };
}

function startEdit(ev: OfficeEventRow) {
  editing.value = { event: ev, kind: ev.kind };
}

function closeEditor() {
  editing.value = null;
}

function onSaved() {
  editing.value = null;
  void loadEvents();
}

// ---------- AI 草案：在日程里就地采纳 / 不要（整份一起） ----------

const draftBusy = ref('');

async function decideDraft(d: DraftEntry, action: 'approve' | 'reject') {
  draftBusy.value = d.inboxId;
  try {
    await decideInboxItem(d.inboxId, action, action === 'reject' ? t('schedulePage.draftRejectNote') : '');
    $q.notify({
      type: 'positive',
      message: action === 'approve' ? t('schedulePage.draftAccepted') : t('schedulePage.draftRejected'),
    });
    reload();
  } catch (e) {
    const status = e instanceof AxiosError ? e.response?.status : undefined;
    // 采纳时 409 = 和已有计划重叠（服务端整份不落）；不要时 409 = 已被别处处理。
    const message =
      status === 409
        ? action === 'approve'
          ? t('schedulePage.draftConflict')
          : t('schedulePage.draftGone')
        : e instanceof Error
          ? e.message
          : String(e);
    $q.notify({ type: 'negative', message });
    if (status === 409 && action === 'reject') void loadDrafts();
  } finally {
    draftBusy.value = '';
  }
}
</script>
