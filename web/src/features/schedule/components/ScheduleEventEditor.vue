<template>
  <!-- 当天面板就地编辑（不弹对话框，月历不被挡）：08-日程.md §3.2 -->
  <form class="sch-ed" :aria-label="heading" @submit.prevent="save" @keydown.esc.stop="emit('close')">
    <div class="sch-dp__head">
      <h2 class="sch-dp__title">{{ heading }}</h2>
      <span class="sch-sp" />
      <q-btn flat dense round icon="close" :aria-label="t('schedulePage.close')" @click="emit('close')" />
    </div>

    <div class="sch-dp__body sch-ed__body">
      <q-banner v-if="error" rounded class="app-page-error-banner" role="alert">{{ error }}</q-banner>

      <div class="sch-ed__row">
        <q-btn-toggle
          v-model="form.kind"
          dense
          no-caps
          unelevated
          toggle-color="primary"
          :options="kindOptions"
          :aria-label="t('schedulePage.fieldKind')"
        />
        <span class="sch-hint">{{
          form.kind === 'log' ? t('schedulePage.kindLogHint') : t('schedulePage.kindPlanHint')
        }}</span>
      </div>

      <div v-if="form.kind === 'plan'" class="sch-ed__row">
        <span class="sch-ed__lab">{{ t('schedulePage.fieldStatus') }}</span>
        <q-btn-toggle
          v-model="form.status"
          dense
          no-caps
          unelevated
          toggle-color="primary"
          :options="statusOptions"
          :aria-label="t('schedulePage.fieldStatus')"
        />
      </div>

      <!-- 改的是重复日程里的一次：只改这一次，还是整个系列 -->
      <div v-if="event?.recurrence_day" class="sch-ed__row">
        <span class="sch-ed__lab">{{ t('schedulePage.scope') }}</span>
        <q-btn-toggle
          v-model="scope"
          dense
          no-caps
          unelevated
          toggle-color="primary"
          :options="[
            { value: 'one', label: t('schedulePage.scopeOne') },
            { value: 'all', label: t('schedulePage.scopeAll') },
          ]"
          :aria-label="t('schedulePage.scope')"
        />
      </div>

      <q-input v-model="form.title" outlined dense autofocus :label="t('schedulePage.fieldTitle')" />

      <!-- 分类色 -->
      <div class="sch-ed__row" role="radiogroup" :aria-label="t('schedulePage.fieldColor')">
        <span class="sch-ed__lab">{{ t('schedulePage.fieldColor') }}</span>
        <button
          v-for="c in EVENT_COLORS"
          :key="c || 'none'"
          type="button"
          role="radio"
          class="sch-color"
          :class="[`sch-c--${c || 'none'}`, { 'sch-color--on': form.color === c }]"
          :aria-checked="form.color === c"
          :aria-label="t(`schedulePage.color.${c || 'none'}`)"
          @click="form.color = c"
        >
          <q-tooltip>{{ t(`schedulePage.color.${c || 'none'}`) }}</q-tooltip>
        </button>
      </div>

      <div class="sch-ed__grid">
        <q-input v-model="form.date" outlined dense type="date" :label="t('schedulePage.fieldDate')" />
        <q-toggle v-model="form.allDay" :label="t('schedulePage.allDay')" @update:model-value="onAllDay" />
      </div>
      <div v-if="!form.allDay" class="sch-ed__grid">
        <q-input v-model="form.start" outlined dense type="time" :label="t('schedulePage.fieldStart')" />
        <q-input v-model="form.end" outlined dense type="time" :label="t('schedulePage.fieldEnd')" />
      </div>

      <q-input v-model="form.location" outlined dense :label="t('schedulePage.fieldLocation')">
        <template #prepend><q-icon name="place" size="18px" /></template>
      </q-input>
      <q-input v-model="form.url" outlined dense type="url" :label="t('schedulePage.fieldUrl')">
        <template #prepend><q-icon name="link" size="18px" /></template>
      </q-input>

      <!-- 重复（只给计划；改「这一次」时不显示） -->
      <template v-if="form.kind === 'plan' && !(event?.recurrence_day && scope === 'one')">
        <div class="sch-ed__grid">
          <q-select
            v-model="form.repeat"
            outlined
            dense
            emit-value
            map-options
            :options="repeatOptions"
            :label="t('schedulePage.fieldRepeat')"
          />
          <q-select
            v-if="form.repeat"
            v-model="form.repeatEnd"
            outlined
            dense
            emit-value
            map-options
            :options="repeatEndOptions"
            :label="t('schedulePage.repeatEnd')"
          />
        </div>
        <q-input
          v-if="form.repeat && form.repeatEnd === 'until'"
          v-model="form.until"
          outlined
          dense
          type="date"
          :label="t('schedulePage.repeatUntil')"
        />
        <q-input
          v-if="form.repeat && form.repeatEnd === 'count'"
          v-model.number="form.count"
          outlined
          dense
          type="number"
          min="1"
          max="999"
          :label="t('schedulePage.repeatCount')"
        />
      </template>

      <template v-if="form.kind === 'plan'">
        <q-select
          v-model="form.reminder"
          outlined
          dense
          emit-value
          map-options
          :options="reminderOptions"
          :label="t('schedulePage.fieldReminder')"
        />
        <p class="sch-hint">{{ t('schedulePage.reminderHint') }}</p>
      </template>

      <q-input
        v-model="form.note"
        outlined
        dense
        type="textarea"
        autogrow
        :label="form.kind === 'log' ? t('schedulePage.fieldNoteLog') : t('schedulePage.fieldNotePlan')"
      />
      <q-input
        v-model="form.outcome"
        outlined
        dense
        type="textarea"
        autogrow
        :placeholder="t('schedulePage.outcomePlaceholder')"
        :label="form.kind === 'log' ? t('schedulePage.fieldOutcomeLog') : t('schedulePage.fieldOutcomePlan')"
      />

      <div v-if="event?.task_ref" class="sch-ed__row">
        <span class="sch-ed__lab">{{ t('schedulePage.fieldLink') }}</span>
        <span class="sch-chip"
          ><q-icon name="link" size="14px" aria-hidden="true" />{{ event.task_title || event.task_ref }}</span
        >
      </div>
    </div>

    <div class="sch-dp__foot">
      <q-btn
        unelevated
        no-caps
        color="primary"
        type="submit"
        :label="t('schedulePage.save')"
        :loading="saving"
        :disable="!form.title.trim()"
      />
      <q-btn flat no-caps :label="t('schedulePage.cancel')" @click="emit('close')" />
      <span class="sch-sp" />
      <q-btn
        v-if="event"
        flat
        no-caps
        color="negative"
        icon="delete_outline"
        :label="t('schedulePage.delete')"
        @click="remove"
      />
    </div>
  </form>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { AxiosError } from 'axios';
import { confirm } from '../../../composables/useAppConfirm';
import { createEvent, deleteEvent, getEvent, updateEvent } from '../../office/api';
import { prompt } from '../../../composables/useAppPrompt';
import type { OfficeEventKind, OfficeEventRow, OfficeEventStatus } from '../../office/types';
import {
  addExdate,
  buildRRule,
  dayKey,
  endOffsetDays,
  hm,
  parseRepeat,
  parseDayKey,
  rangeFromForm,
  type RepeatEnd,
  type RepeatFreq,
} from '../model';

/** 分类色（'' = 默认）；色值走 --nt-line-* 令牌，见 _schedule.sass .sch-c--* */
const EVENT_COLORS = ['', 'work', 'meeting', 'personal', 'focus', 'travel'] as const;

const props = defineProps<{
  /** null = 新建 */
  event: OfficeEventRow | null;
  /** 新建时落在哪天 */
  day: string;
  kind: OfficeEventKind;
  /** 新建时的开始时间 'HH:MM'（周视图点空白时段带过来），时长一小时 */
  time?: string;
}>();

const emit = defineEmits<{ close: []; saved: [] }>();

const { t } = useI18n();

const REMINDERS = [0, 15, 30, 60, 1440];

/** 'HH:MM' 往后一小时，最晚到 23:59（不跨天）。 */
function plusHour(t: string): string {
  const [h = 0, m = 0] = t.split(':').map(Number);
  return h >= 23 ? '23:59' : `${String(h + 1).padStart(2, '0')}:${String(m).padStart(2, '0')}`;
}

function initialForm() {
  const ev = props.event;
  if (!ev) {
    return {
      kind: props.kind,
      status: (props.kind === 'log' ? 'done' : 'todo') as OfficeEventStatus,
      title: '',
      date: props.day,
      start: props.time || '09:00',
      end: props.time ? plusHour(props.time) : '10:00',
      allDay: false,
      endOffset: 0,
      reminder: props.kind === 'log' ? 0 : 15,
      note: '',
      outcome: '',
      location: '',
      url: '',
      color: '',
      repeat: '' as RepeatFreq,
      repeatEnd: 'never' as RepeatEnd['kind'],
      until: '',
      count: 10,
    };
  }
  const rp = parseRepeat(ev.rrule);
  return {
    kind: ev.kind,
    status: ev.status,
    title: ev.title,
    date: dayKey(new Date(ev.start_at)),
    start: hm(ev.start_at),
    end: hm(ev.end_at),
    allDay: ev.all_day,
    endOffset: endOffsetDays(ev.start_at, ev.end_at),
    reminder: ev.reminder_minutes,
    note: ev.note,
    outcome: ev.outcome,
    location: ev.location,
    url: ev.url,
    color: ev.color,
    repeat: rp.freq,
    repeatEnd: rp.end.kind,
    until: rp.end.kind === 'until' ? rp.end.day : '',
    count: rp.end.kind === 'count' ? rp.end.n : 10,
  };
}

const form = ref(initialForm());
const saving = ref(false);
const error = ref('');
/** 重复日程的一次：只改这一次（one）还是整个系列（all） */
const scope = ref<'one' | 'all'>('one');

const repeatOptions = computed(() =>
  (['', 'DAILY', 'WEEKDAYS', 'WEEKLY', 'MONTHLY', 'YEARLY'] as const).map((f) => ({
    value: f,
    label: t(`schedulePage.repeat.${f || 'none'}`),
  })),
);
const repeatEndOptions = computed(() =>
  (['never', 'until', 'count'] as const).map((k) => ({ value: k, label: t(`schedulePage.repeatEndKind.${k}`) })),
);

function rruleOf(f: typeof form.value): string {
  if (f.kind !== 'plan') return '';
  const end: RepeatEnd =
    f.repeatEnd === 'until'
      ? { kind: 'until', day: f.until }
      : f.repeatEnd === 'count'
        ? { kind: 'count', n: Number(f.count) || 1 }
        : { kind: 'never' };
  return buildRRule(f.repeat, end, parseDayKey(f.date).getDay());
}

const heading = computed(() => {
  if (props.event) return form.value.kind === 'log' ? t('schedulePage.kindLog') : t('schedulePage.kindPlan');
  return form.value.kind === 'log' ? t('schedulePage.newLog') : t('schedulePage.newPlan');
});

const kindOptions = computed(() => [
  { value: 'plan', label: t('schedulePage.kindPlan') },
  { value: 'log', label: t('schedulePage.kindLog') },
]);

const statusOptions = computed(() =>
  (['todo', 'doing', 'done', 'cancelled'] as const).map((s) => ({ value: s, label: t(`schedulePage.status.${s}`) })),
);

function reminderText(n: number): string {
  if (n >= 1440 && n % 1440 === 0) return t('schedulePage.days', { n: n / 1440 });
  if (n >= 60 && n % 60 === 0) return t('schedulePage.hours', { n: n / 60 });
  return t('schedulePage.minutes', { n });
}

const reminderOptions = computed(() => {
  const values = REMINDERS.includes(form.value.reminder)
    ? REMINDERS
    : [...REMINDERS, form.value.reminder].sort((a, b) => a - b);
  return values.map((n) => ({
    value: n,
    label: n ? t('schedulePage.reminderOpt', { v: reminderText(n) }) : t('schedulePage.reminderNone'),
  }));
});

/** 切全天时重置跨度：全天至少一天，按时间的默认当天结束。 */
function onAllDay(on: boolean) {
  form.value.endOffset = on ? Math.max(1, form.value.endOffset) : 0;
  // 全天转按时间：原来的 00:00–00:00 没法存，给个默认时段。
  if (!on && form.value.start === form.value.end) Object.assign(form.value, { start: '09:00', end: '10:00' });
}

async function save() {
  const f = form.value;
  const range = rangeFromForm(f);
  if (!range) {
    error.value = t('schedulePage.invalidTime');
    return;
  }
  saving.value = true;
  error.value = '';
  const input = {
    title: f.title.trim(),
    note: f.note,
    start_at: range[0],
    end_at: range[1],
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Shanghai',
    reminder_minutes: f.kind === 'log' ? 0 : f.reminder,
    kind: f.kind,
    status: f.status,
    all_day: f.allDay,
    outcome: f.outcome,
    location: f.location.trim(),
    url: f.url.trim(),
    color: f.color,
    rrule: rruleOf(f),
  };
  try {
    const ev = props.event;
    if (ev?.recurrence_day && scope.value === 'one') {
      // 只改这一次：系列跳过这天，另建一条单次的
      const series = await getEvent(ev.id);
      await updateEvent(ev.id, { ...seriesKeep(series), exdates: addExdate(series.exdates, ev.recurrence_day) });
      await createEvent({ ...input, rrule: '' });
    } else if (ev?.recurrence_day) {
      // 改整个系列：时间按这一次的改动量平移系列的开始
      const series = await getEvent(ev.id);
      const delta = Date.parse(range[0]) - Date.parse(ev.start_at);
      const start = new Date(Date.parse(series.start_at) + delta);
      const end = new Date(start.getTime() + (Date.parse(range[1]) - Date.parse(range[0])));
      await updateEvent(ev.id, { ...input, start_at: start.toISOString(), end_at: end.toISOString() });
    } else if (ev) await updateEvent(ev.id, input);
    else await createEvent(input);
    emit('saved');
  } catch (e) {
    // 409 = 时间重叠（只有按时间的计划会撞）；面板保持打开，错误就地给出。
    error.value =
      e instanceof AxiosError && e.response?.status === 409
        ? t('schedulePage.conflictError')
        : e instanceof Error
          ? e.message
          : String(e);
  } finally {
    saving.value = false;
  }
}

/** 只动 exdates 时把系列其余字段原样带上（UpdateEvent 的标题 / 时间空值表示不改）。 */
function seriesKeep(sr: OfficeEventRow) {
  return {
    title: sr.title,
    note: sr.note,
    start_at: sr.start_at,
    end_at: sr.end_at,
    timezone: sr.timezone,
    reminder_minutes: sr.reminder_minutes,
  };
}

async function remove() {
  const ev = props.event;
  if (!ev) return;
  if (ev.recurrence_day) {
    const which = await prompt({
      title: t('schedulePage.deleteConfirmTitle'),
      message: t('schedulePage.deleteRecurBody', { title: ev.title }),
      type: 'radio',
      items: [
        { label: t('schedulePage.scopeOne'), value: 'one' },
        { label: t('schedulePage.scopeAll'), value: 'all' },
      ],
      tone: 'danger',
      confirmLabel: t('schedulePage.delete'),
    });
    if (!which) return;
    try {
      if (which === 'one') {
        const series = await getEvent(ev.id);
        await updateEvent(ev.id, { ...seriesKeep(series), exdates: addExdate(series.exdates, ev.recurrence_day) });
      } else await deleteEvent(ev.id);
      emit('saved');
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    }
    return;
  }
  const ok = await confirm({
    title: t('schedulePage.deleteConfirmTitle'),
    message: t('schedulePage.deleteConfirmBody', { title: ev.title }),
    tone: 'danger',
  });
  if (!ok) return;
  try {
    await deleteEvent(ev.id);
    emit('saved');
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  }
}
</script>
