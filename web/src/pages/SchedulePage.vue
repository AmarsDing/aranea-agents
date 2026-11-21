<template>
  <q-page class="app-standard-page sch-page" :class="{ 'sch-page--wide': !dayPanelShown }">
    <!-- 操作放在页签行右边，全是图标，悬停看说明 -->
    <Teleport defer to="#app-tab-actions">
      <q-btn
        flat
        dense
        round
        icon="refresh"
        :aria-label="t('schedulePage.btnRefresh')"
        :loading="loading"
        @click="reload"
      >
        <q-tooltip>{{ t('schedulePage.btnRefresh') }}</q-tooltip>
      </q-btn>
      <q-btn flat dense round icon="search" :aria-label="t('schedulePage.search')" @click="openSearch">
        <q-tooltip>{{ t('schedulePage.search') }}</q-tooltip>
      </q-btn>
      <q-btn flat dense round icon="upload_file" :aria-label="t('schedulePage.importIcs')" @click="icsPicker?.click()">
        <q-tooltip>{{ t('schedulePage.importIcs') }}</q-tooltip>
      </q-btn>
      <input ref="icsPicker" type="file" accept=".ics,text/calendar" class="sch-sr" @change="onIcsPicked" />
      <q-btn flat dense round icon="rss_feed" :aria-label="t('schedulePage.btnSubscribe')" @click="icalOpen = true">
        <q-tooltip>{{ t('schedulePage.btnSubscribe') }}</q-tooltip>
      </q-btn>
      <q-btn
        unelevated
        dense
        round
        color="primary"
        icon="add"
        :aria-label="t('schedulePage.btnNewKey')"
        @click="startNew('plan')"
      >
        <q-tooltip>{{ t('schedulePage.btnNewKey') }}</q-tooltip>
      </q-btn>
    </Teleport>

    <AppStatePanel v-if="pageForbidden" class="sch-forbidden" variant="forbidden" />

    <template v-else>
      <!-- ============ 月历：一屏 6 × 7 ============ -->
      <section class="sch-panel sch-cal" :aria-label="t('schedulePage.title')" :aria-busy="loading">
        <div class="sch-bar">
          <q-btn flat dense round icon="chevron_left" :aria-label="prevLabel" @click="shift(-1)">
            <q-tooltip>{{ prevLabel }}</q-tooltip>
          </q-btn>
          <button type="button" class="sch-month" :class="{ 'sch-month--on': pickerOpen }" :aria-expanded="pickerOpen">
            <span class="sch-num">{{ rangeLabel }}</span>
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
          <q-btn flat dense round icon="chevron_right" :aria-label="nextLabel" @click="shift(1)">
            <q-tooltip>{{ nextLabel }}</q-tooltip>
          </q-btn>
          <q-btn flat dense round icon="today" :aria-label="t('schedulePage.todayKey')" @click="goToday">
            <q-tooltip>{{ t('schedulePage.todayKey') }}</q-tooltip>
          </q-btn>
          <q-btn-toggle
            v-model="mode"
            dense
            unelevated
            toggle-color="primary"
            class="sch-modes"
            :options="modeOptions"
            :aria-label="t('schedulePage.viewMode')"
          >
            <template #month
              ><q-tooltip>{{ t('schedulePage.modeMonth') }}</q-tooltip></template
            >
            <template #week
              ><q-tooltip>{{ t('schedulePage.modeWeek') }}</q-tooltip></template
            >
            <template #day
              ><q-tooltip>{{ t('schedulePage.modeDay') }}</q-tooltip></template
            >
            <template #agenda
              ><q-tooltip>{{ t('schedulePage.modeAgenda') }}</q-tooltip></template
            >
          </q-btn-toggle>

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
          <q-btn
            flat
            dense
            round
            icon="view_sidebar"
            :color="panelOpen ? 'primary' : undefined"
            :aria-pressed="panelOpen"
            :aria-label="panelOpen ? t('schedulePage.hidePanel') : t('schedulePage.showPanel')"
            @click="togglePanel"
          >
            <q-tooltip>{{ panelOpen ? t('schedulePage.hidePanel') : t('schedulePage.showPanel') }}</q-tooltip>
          </q-btn>
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

        <ScheduleWeekView
          v-if="mode === 'week' || mode === 'day'"
          :key="mode"
          :start="mode === 'day' ? parseDayKey(selectedDay) : weekStart(parseDayKey(selectedDay))"
          :count="mode === 'day' ? 1 : 7"
          :events="events"
          :drafts="drafts"
          :show="show"
          :today="today"
          :selected-day="selectedDay"
          @select="selectDay"
          @create="startNewAt"
          @edit="startEdit"
          @move="moveEvent"
          @move-day="(ev, day) => moveEvent(ev, sameTimeOn(ev, day))"
          @resize="resizeEvent"
        />
        <!-- 列表：从选中那天起 30 天，按天排 -->
        <div v-else-if="mode === 'agenda'" class="sch-ag" :aria-label="t('schedulePage.modeAgenda')">
          <p v-if="!agendaDays.length" class="sch-hint sch-ag__empty">{{ t('schedulePage.agendaEmpty') }}</p>
          <section v-for="d in agendaDays" :key="d.key" class="sch-ag__day">
            <button
              type="button"
              class="sch-ag__date"
              :class="{ 'sch-ag__date--today': d.isToday }"
              @click="selectDay(d.key)"
            >
              <b class="sch-num">{{ d.date }}</b>
              <span>{{ d.weekday }}</span>
              <small v-if="d.lunar.text" class="sch-lunar" :class="{ 'sch-lunar--fest': d.lunar.festival }">{{
                d.lunar.text
              }}</small>
            </button>
            <div class="sch-ag__list">
              <button
                v-for="ev in d.items"
                :key="ev.id + ev.recurrence_day"
                type="button"
                class="sch-ag__it"
                :class="[`sch-st--${statusOf(ev)}`, ev.color && `sch-c--${ev.color}`, `sch-ag__it--${ev.kind}`]"
                @click="startEdit(ev)"
              >
                <span class="sch-ag__tm sch-num">{{ timeText(ev) }}</span>
                <span class="sch-ag__title">
                  {{ ev.title }}
                  <q-icon v-if="ev.rrule" name="repeat" size="13px" :aria-label="t('schedulePage.recurring')" />
                </span>
                <span v-if="ev.location" class="sch-hint"
                  ><q-icon name="place" size="13px" aria-hidden="true" />{{ ev.location }}</span
                >
              </button>
            </div>
          </section>
        </div>
        <template v-else>
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
                'sch-day--drop': dropDay === cell.key,
              }"
              :aria-pressed="cell.key === selectedDay"
              :aria-label="cellAria(cell)"
              :title="t('schedulePage.cellHint')"
              @click="selectDay(cell.key)"
              @dblclick="startNewAt(cell.key)"
              @dragover.prevent="dropDay = cell.key"
              @dragleave="dropDay = ''"
              @drop.prevent="onDropDay(cell.key)"
            >
              <span class="sch-day__n sch-num"
                ><b>{{ cell.day }}</b
                ><small
                  v-if="lunarOf(cell.key).text"
                  class="sch-lunar"
                  :class="{ 'sch-lunar--fest': lunarOf(cell.key).festival }"
                  >{{ lunarOf(cell.key).text }}</small
                ></span
              >
              <template v-for="(line, i) in cell.lines" :key="i">
                <span
                  v-if="line.type === 'plan'"
                  class="sch-it sch-it--plan"
                  :class="[`sch-st--${line.status}`, line.event.color && `sch-c--${line.event.color}`]"
                  draggable="true"
                  @dragstart.stop="dragEv = line.event"
                  @dragend="dragEv = null"
                >
                  <span v-if="!line.event.all_day" class="sch-it__tm sch-num">{{ hm(line.event.start_at) }}</span>
                  <span class="sch-it__txt">{{ line.event.title }}</span>
                  <q-icon v-if="line.event.rrule" name="repeat" size="11px" class="sch-it__rp" aria-hidden="true" />
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
        </template>
      </section>

      <!-- ============ 当天：计划 / 工作内容 / AI 建议；点一条就地编辑 ============ -->
      <aside v-show="dayPanelShown" class="sch-panel sch-dp" :aria-label="t('schedulePage.dayAria')">
        <ScheduleEventEditor
          v-if="editing"
          :key="editing.event?.id ?? `new-${editing.kind}-${selectedDay}-${editing.time ?? ''}`"
          :event="editing.event"
          :day="selectedDay"
          :kind="editing.kind"
          :time="editing.time"
          @close="closeEditor"
          @saved="onSaved"
        />
        <template v-else-if="searchOpen">
          <div class="sch-dp__head">
            <q-input
              ref="searchInput"
              v-model="searchQ"
              dense
              outlined
              autofocus
              class="sch-search"
              :placeholder="t('schedulePage.searchPlaceholder')"
              @keydown.enter.prevent="runSearch"
              @keydown.esc="searchOpen = false"
            >
              <template #prepend><q-icon name="search" size="18px" /></template>
            </q-input>
            <q-btn flat dense round icon="close" :aria-label="t('schedulePage.close')" @click="searchOpen = false" />
          </div>
          <div class="sch-dp__body" aria-live="polite">
            <q-linear-progress v-if="searching" indeterminate size="2px" color="primary" />
            <p v-else-if="searchDone && !searchHits.length" class="sch-hint">{{ t('schedulePage.searchNone') }}</p>
            <button
              v-for="ev in searchHits"
              :key="ev.id"
              type="button"
              class="sch-ag__it"
              :class="[`sch-st--${statusOf(ev)}`, ev.color && `sch-c--${ev.color}`]"
              @click="openHit(ev)"
            >
              <span class="sch-ag__tm sch-num"
                >{{ dayKey(new Date(ev.start_at)) }} {{ ev.all_day ? '' : hm(ev.start_at) }}</span
              >
              <span class="sch-ag__title">
                {{ ev.title }}
                <q-icon v-if="ev.rrule" name="repeat" size="13px" :aria-label="t('schedulePage.recurring')" />
              </span>
              <span v-if="ev.location" class="sch-hint">{{ ev.location }}</span>
            </button>
          </div>
        </template>
        <template v-else>
          <div class="sch-dp__head">
            <h2 class="sch-dp__title sch-num">{{ dayTitle }}</h2>
            <span class="sch-muted">{{ dayWeekday }}</span>
            <span class="sch-sp" />
            <q-btn flat dense round icon="add_task" :aria-label="t('schedulePage.addPlan')" @click="startNew('plan')">
              <q-tooltip>{{ t('schedulePage.addPlan') }}</q-tooltip>
            </q-btn>
            <q-btn flat dense round icon="edit_note" :aria-label="t('schedulePage.addLog')" @click="startNew('log')">
              <q-tooltip>{{ t('schedulePage.addLog') }}</q-tooltip>
            </q-btn>
          </div>

          <div class="sch-dp__body">
            <div class="sch-sect">
              {{ t('schedulePage.sectPlan') }}<span class="sch-sect__n sch-num">{{ dayPlans.length }}</span>
              <span class="sch-sp sch-hint">{{ t('schedulePage.planHint') }}</span>
            </div>
            <p v-if="!dayPlans.length" class="sch-hint">{{ t('schedulePage.emptyPlan') }}</p>
            <div
              v-for="ev in dayPlans"
              :key="ev.id + ev.recurrence_day"
              class="sch-ent"
              :class="ev.color && `sch-c--${ev.color}`"
            >
              <button type="button" class="sch-ent__main" @click="startEdit(ev)">
                <span class="sch-ent__t sch-num">{{ timeText(ev) }}</span>
                <span class="sch-ent__title" :class="`sch-st--${statusOf(ev)}`"
                  >{{ ev.title
                  }}<q-icon
                    v-if="ev.rrule"
                    name="repeat"
                    size="13px"
                    class="sch-it__rp"
                    :aria-label="t('schedulePage.recurring')"
                /></span>
                <span v-if="ev.location || ev.url" class="sch-ent__meta">
                  <span v-if="ev.location" class="sch-chip"
                    ><q-icon name="place" size="13px" aria-hidden="true" />{{ ev.location }}</span
                  >
                  <a
                    v-if="ev.url"
                    class="sch-chip sch-link"
                    :href="ev.url"
                    target="_blank"
                    rel="noopener noreferrer"
                    @click.stop
                    ><q-icon name="link" size="13px" aria-hidden="true" />{{ t('schedulePage.openUrl') }}</a
                  >
                </span>
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
import ScheduleWeekView from '../features/schedule/components/ScheduleWeekView.vue';
import { isForbiddenApiError } from '../utils/kratosError';
import {
  createEvent,
  getEvent,
  listEventsRange,
  searchEvents,
  setEventStatus,
  updateEvent,
  type EventInput,
} from '../features/office/api';
import { confirm } from '../composables/useAppConfirm';
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
  sameTimeOn,
  movedRange,
  addExdate,
  lunarLabel,
  parseIcs,
  sortDay,
  weekStart,
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
    // 列表视图从选中那天起 30 天；其余按当月 6 周月格（盖住它里面的任何一周、一天）
    const sel = parseDayKey(selectedDay.value);
    const start = mode.value === 'agenda' ? sel : gridStart(month.value);
    const end = new Date(
      start.getFullYear(),
      start.getMonth(),
      start.getDate() + (mode.value === 'agenda' ? AGENDA_DAYS : 42),
    );
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

// ---------- 月 / 周 两种视图（选哪个记在本机） ----------

type ViewMode = 'month' | 'week' | 'day' | 'agenda';
const MODE_KEY = 'aranea.schedule.mode';
const PANEL_KEY = 'aranea.schedule.panel';
function readPref(key: string): string {
  try {
    return localStorage.getItem(key) ?? '';
  } catch {
    return '';
  }
}
function writePref(key: string, v: string) {
  try {
    localStorage.setItem(key, v);
  } catch {
    // 隐私模式等存不了就算了
  }
}

const VIEW_MODES: ViewMode[] = ['month', 'week', 'day', 'agenda'];
const mode = ref<ViewMode>(VIEW_MODES.find((m) => m === readPref(MODE_KEY)) ?? 'month');
watch(mode, (v) => {
  writePref(MODE_KEY, v);
  void loadEvents();
});
const modeOptions = computed(() => [
  { value: 'month', icon: 'calendar_view_month', slot: 'month', attrs: { 'aria-label': t('schedulePage.modeMonth') } },
  { value: 'week', icon: 'calendar_view_week', slot: 'week', attrs: { 'aria-label': t('schedulePage.modeWeek') } },
  { value: 'day', icon: 'calendar_view_day', slot: 'day', attrs: { 'aria-label': t('schedulePage.modeDay') } },
  { value: 'agenda', icon: 'view_agenda', slot: 'agenda', attrs: { 'aria-label': t('schedulePage.modeAgenda') } },
]);

const AGENDA_DAYS = 30;

/** 左右箭头：月视图翻月，周视图翻周。 */
function shift(delta: number) {
  if (mode.value === 'month') {
    shiftMonth(delta);
    return;
  }
  const d = parseDayKey(selectedDay.value);
  const step = mode.value === 'day' ? 1 : mode.value === 'agenda' ? AGENDA_DAYS : 7;
  const next = new Date(d.getFullYear(), d.getMonth(), d.getDate() + step * delta);
  selectedDay.value = dayKey(next);
  editing.value = null;
  if (mode.value === 'agenda') void loadEvents();
  // 拉取范围跟着「选中那天所在的月」走（6 周月格必然盖住它所在的整周）
  if (next.getMonth() !== month.value.getMonth() || next.getFullYear() !== month.value.getFullYear())
    month.value = new Date(next.getFullYear(), next.getMonth(), 1);
}

const prevLabel = computed(() => t(`schedulePage.prev.${mode.value}`));
const nextLabel = computed(() => t(`schedulePage.next.${mode.value}`));
const rangeLabel = computed(() => {
  if (mode.value === 'month')
    return t('schedulePage.monthLabel', { y: month.value.getFullYear(), m: month.value.getMonth() + 1 });
  if (mode.value === 'day') {
    const d = parseDayKey(selectedDay.value);
    return t('schedulePage.dayLabel', { y: d.getFullYear(), m: d.getMonth() + 1, d: d.getDate() });
  }
  if (mode.value === 'agenda') {
    const d = parseDayKey(selectedDay.value);
    return t('schedulePage.agendaLabel', { m: d.getMonth() + 1, d: d.getDate(), n: AGENDA_DAYS });
  }
  const s = weekStart(parseDayKey(selectedDay.value));
  const e = new Date(s.getFullYear(), s.getMonth(), s.getDate() + 6);
  return t('schedulePage.weekLabel', {
    y: s.getFullYear(),
    m1: s.getMonth() + 1,
    d1: s.getDate(),
    m2: e.getMonth() + 1,
    d2: e.getDate(),
  });
});

// ---------- 当天面板可收起（收起后月历 / 周视图占满；编辑时自动展开） ----------

const panelOpen = ref(readPref(PANEL_KEY) !== 'off');
function togglePanel() {
  panelOpen.value = !panelOpen.value;
  writePref(PANEL_KEY, panelOpen.value ? 'on' : 'off');
}
const dayPanelShown = computed(() => panelOpen.value || !!editing.value);

/** 快捷键（输入框、弹窗里不抢键）：PageUp / PageDown 翻页，T 今天，N 新建计划，M / W / D / A 月 / 周 / 日 / 列表，/ 搜索。 */
function onKey(e: KeyboardEvent) {
  if (e.ctrlKey || e.metaKey || e.altKey) return;
  const el = e.target as HTMLElement | null;
  if (el && (el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName))) return;
  if (document.querySelector('.q-dialog, .q-menu')) return;
  const act: Record<string, () => void> = {
    PageUp: () => shift(-1),
    PageDown: () => shift(1),
    t: goToday,
    n: () => startNew('plan'),
    m: () => (mode.value = 'month'),
    w: () => (mode.value = 'week'),
    d: () => (mode.value = 'day'),
    a: () => (mode.value = 'agenda'),
    '/': openSearch,
  };
  const fn = act[e.key.length === 1 ? e.key.toLowerCase() : e.key];
  if (!fn) return;
  e.preventDefault();
  fn();
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

/** 重复日程的一次单独改：系列跳过这天，另建一条单次的（带上改动）。返回新建的那条。 */
async function detachOccurrence(ev: OfficeEventRow, patch: Partial<EventInput>): Promise<OfficeEventRow> {
  const series = await getEvent(ev.id);
  await updateEvent(ev.id, {
    title: series.title,
    note: series.note,
    start_at: series.start_at,
    end_at: series.end_at,
    timezone: series.timezone,
    reminder_minutes: series.reminder_minutes,
    exdates: addExdate(series.exdates, ev.recurrence_day),
  });
  return createEvent({
    title: ev.title,
    note: ev.note,
    start_at: ev.start_at,
    end_at: ev.end_at,
    timezone: ev.timezone,
    reminder_minutes: ev.reminder_minutes,
    kind: ev.kind,
    status: ev.status,
    all_day: ev.all_day,
    outcome: ev.outcome,
    location: ev.location,
    url: ev.url,
    color: ev.color,
    ...patch,
  });
}

async function cycleStatus(ev: OfficeEventRow) {
  statusBusy.value = ev.id;
  try {
    if (ev.recurrence_day) {
      // 重复日程：只把这一次标成下一个状态
      const row = await detachOccurrence(ev, { status: nextStatus(ev.status) });
      await loadEvents();
      if (row.status === 'done') $q.notify({ message: t('schedulePage.detachedNote') });
      return;
    }
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

const editing = ref<{ event: OfficeEventRow | null; kind: OfficeEventKind; time?: string } | null>(null);

function startNew(kind: OfficeEventKind) {
  editing.value = { event: null, kind };
}

/** 月视图双击某天 / 周视图点空白时段：在那天（那个时刻）新建计划。 */
function startNewAt(day: string, time?: string) {
  selectedDay.value = day;
  editing.value = { event: null, kind: 'plan', time };
}

// ---------- 拖动改期 ----------

const dragEv = ref<OfficeEventRow | null>(null);
const dropDay = ref('');

function onDropDay(day: string) {
  dropDay.value = '';
  const ev = dragEv.value;
  dragEv.value = null;
  if (ev) void moveEvent(ev, sameTimeOn(ev, day));
}

/** 改期：保持时长，开始挪到 start；和别的计划撞了服务端回 409。 */
async function moveEvent(ev: OfficeEventRow, start: Date) {
  if (start.getTime() === Date.parse(ev.start_at)) return;
  const [start_at, end_at] = movedRange(ev, start);
  try {
    if (ev.recurrence_day) {
      // 拖重复日程的一次：只挪这一次
      await detachOccurrence(ev, { start_at, end_at });
      await loadEvents();
      selectedDay.value = dayKey(start);
      $q.notify({ message: t('schedulePage.movedOne', { title: ev.title, when: `${dayKey(start)} ${hm(start_at)}` }) });
      return;
    }
    const row = await updateEvent(ev.id, {
      title: ev.title,
      note: ev.note,
      start_at,
      end_at,
      timezone: ev.timezone,
      reminder_minutes: ev.reminder_minutes,
    });
    events.value = events.value.map((e) => (e.id === row.id ? row : e));
    selectedDay.value = dayKey(start);
    $q.notify({ message: t('schedulePage.moved', { title: row.title, when: `${dayKey(start)} ${hm(start_at)}` }) });
  } catch (e) {
    const status = e instanceof AxiosError ? e.response?.status : undefined;
    $q.notify({
      type: 'negative',
      message: status === 409 ? t('schedulePage.moveConflict') : e instanceof Error ? e.message : String(e),
    });
  }
}

function startEdit(ev: OfficeEventRow) {
  editing.value = { event: ev, kind: ev.kind };
}

/** 周 / 日视图拖下边缘：只改结束时间。 */
async function resizeEvent(ev: OfficeEventRow, end: Date) {
  try {
    if (ev.recurrence_day) await detachOccurrence(ev, { end_at: end.toISOString() });
    else
      await updateEvent(ev.id, {
        title: ev.title,
        note: ev.note,
        start_at: ev.start_at,
        end_at: end.toISOString(),
        timezone: ev.timezone,
        reminder_minutes: ev.reminder_minutes,
      });
    await loadEvents();
    $q.notify({ message: t('schedulePage.resized', { title: ev.title, end: hm(end.toISOString()) }) });
  } catch (e) {
    const status = e instanceof AxiosError ? e.response?.status : undefined;
    $q.notify({
      type: 'negative',
      message: status === 409 ? t('schedulePage.moveConflict') : e instanceof Error ? e.message : String(e),
    });
  }
}

// ---------- 农历 ----------
const lunarCache = new Map<string, ReturnType<typeof lunarLabel>>();
function lunarOf(key: string) {
  let v = lunarCache.get(key);
  if (!v) {
    v = lunarLabel(parseDayKey(key));
    lunarCache.set(key, v);
  }
  return v;
}

// ---------- 列表视图 ----------
const agendaDays = computed(() => {
  if (mode.value !== 'agenda') return [];
  const byDay = bucketByDay(events.value, (e) => [e.start_at, e.end_at]);
  const start = parseDayKey(selectedDay.value);
  const todayKey = dayKey(today.value);
  const out = [];
  for (let i = 0; i < AGENDA_DAYS; i++) {
    const d = new Date(start.getFullYear(), start.getMonth(), start.getDate() + i);
    const key = dayKey(d);
    const items = sortDay(byDay.get(key) ?? []).filter((e) => (e.kind === 'log' ? show.log : show.plan));
    if (!items.length) continue;
    out.push({
      key,
      date: t('schedulePage.dayTitle', { m: d.getMonth() + 1, d: d.getDate() }),
      weekday: t(`schedulePage.weekdays.${(d.getDay() + 6) % 7}`),
      isToday: key === todayKey,
      lunar: lunarOf(key),
      items,
    });
  }
  return out;
});

// ---------- 搜索 ----------
const searchOpen = ref(false);
const searchQ = ref('');
const searching = ref(false);
const searchDone = ref(false);
const searchHits = ref<OfficeEventRow[]>([]);

function openSearch() {
  editing.value = null;
  searchOpen.value = true;
  if (!panelOpen.value) togglePanel();
}

async function runSearch() {
  const q = searchQ.value.trim();
  if (!q) return;
  searching.value = true;
  try {
    searchHits.value = await searchEvents(q);
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  } finally {
    searching.value = false;
    searchDone.value = true;
  }
}

function openHit(ev: OfficeEventRow) {
  const d = new Date(ev.start_at);
  goMonth(d.getFullYear(), d.getMonth());
  selectedDay.value = dayKey(d);
}

// ---------- 导入 .ics ----------
const icsPicker = ref<HTMLInputElement | null>(null);

async function onIcsPicked(e: Event) {
  const input = e.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = '';
  if (!file) return;
  const items = parseIcs(await file.text());
  if (!items.length) {
    $q.notify({ type: 'warning', message: t('schedulePage.importNone') });
    return;
  }
  const ok = await confirm({
    title: t('schedulePage.importIcs'),
    message: t('schedulePage.importConfirm', { n: items.length, name: file.name }),
    confirmLabel: t('schedulePage.importIcs'),
  });
  if (!ok) return;
  const tz = Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Shanghai';
  let done = 0;
  let failed = 0;
  for (const it of items) {
    const base: EventInput = {
      title: it.title,
      note: it.note,
      start_at: it.start.toISOString(),
      end_at: it.end.toISOString(),
      timezone: tz,
      reminder_minutes: 0,
      all_day: it.allDay,
      location: it.location,
      url: it.url,
      rrule: it.rrule,
    };
    try {
      await createEvent(base);
      done += 1;
    } catch {
      // 重复规则不认 / 时间撞了：去掉重复、改成不占时间的全天外再试一次不合适，直接记失败
      try {
        await createEvent({ ...base, rrule: '' });
        done += 1;
      } catch {
        failed += 1;
      }
    }
  }
  $q.notify({ type: failed ? 'warning' : 'positive', message: t('schedulePage.imported', { n: done, failed }) });
  await loadEvents();
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
