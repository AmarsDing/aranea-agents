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

      <q-input v-model="form.title" outlined dense autofocus :label="t('schedulePage.fieldTitle')" />

      <div class="sch-ed__grid">
        <q-input v-model="form.date" outlined dense type="date" :label="t('schedulePage.fieldDate')" />
        <q-toggle v-model="form.allDay" :label="t('schedulePage.allDay')" @update:model-value="onAllDay" />
      </div>
      <div v-if="!form.allDay" class="sch-ed__grid">
        <q-input v-model="form.start" outlined dense type="time" :label="t('schedulePage.fieldStart')" />
        <q-input v-model="form.end" outlined dense type="time" :label="t('schedulePage.fieldEnd')" />
      </div>

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
import { createEvent, deleteEvent, updateEvent } from '../../office/api';
import type { OfficeEventKind, OfficeEventRow, OfficeEventStatus } from '../../office/types';
import { dayKey, endOffsetDays, hm, rangeFromForm } from '../model';

const props = defineProps<{
  /** null = 新建 */
  event: OfficeEventRow | null;
  /** 新建时落在哪天 */
  day: string;
  kind: OfficeEventKind;
}>();

const emit = defineEmits<{ close: []; saved: [] }>();

const { t } = useI18n();

const REMINDERS = [0, 15, 30, 60, 1440];

function initialForm() {
  const ev = props.event;
  if (!ev) {
    return {
      kind: props.kind,
      status: (props.kind === 'log' ? 'done' : 'todo') as OfficeEventStatus,
      title: '',
      date: props.day,
      start: '09:00',
      end: '10:00',
      allDay: false,
      endOffset: 0,
      reminder: props.kind === 'log' ? 0 : 15,
      note: '',
      outcome: '',
    };
  }
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
  };
}

const form = ref(initialForm());
const saving = ref(false);
const error = ref('');

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
  };
  try {
    if (props.event) await updateEvent(props.event.id, input);
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

async function remove() {
  const ev = props.event;
  if (!ev) return;
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
