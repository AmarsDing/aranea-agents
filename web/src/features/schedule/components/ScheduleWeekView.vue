<template>
  <!-- 周视图：上面全天行，下面 24 小时时间轴；点空白时段新建，拖动改期（2026-11-20 补，对照 Google / Outlook / 飞书） -->
  <div class="sch-wv" :style="{ '--sch-days': String(count ?? 7) }">
    <div class="sch-wv__head">
      <span class="sch-wv__gut" />
      <button
        v-for="d in days"
        :key="d.key"
        type="button"
        class="sch-wv__dh"
        :class="{ 'sch-wv__dh--today': d.isToday, 'sch-wv__dh--sel': d.key === selectedDay }"
        :aria-pressed="d.key === selectedDay"
        @click="emit('select', d.key)"
      >
        <span>{{ t(`schedulePage.weekdays.${d.wd}`) }}</span>
        <b class="sch-num">{{ d.date }}</b>
        <small v-if="d.lunar.text" class="sch-lunar" :class="{ 'sch-lunar--fest': d.lunar.festival }">{{
          d.lunar.text
        }}</small>
      </button>
    </div>

    <!-- 全天：全天的计划；拖到这里只改日期 -->
    <div class="sch-wv__allday">
      <span class="sch-wv__gut sch-hint">{{ t('schedulePage.allDay') }}</span>
      <div
        v-for="d in days"
        :key="d.key"
        class="sch-wv__adc"
        :class="{ 'sch-wv__drop': dropKey === `all|${d.key}` }"
        @dragover.prevent="dropKey = `all|${d.key}`"
        @dragleave="dropKey = ''"
        @drop.prevent="onDropAllDay(d.key)"
      >
        <button
          v-for="ev in d.allDay"
          :key="ev.id"
          type="button"
          class="sch-it sch-it--plan sch-wv__chip"
          :class="[`sch-st--${statusOf(ev)}`, ev.color && `sch-c--${ev.color}`]"
          draggable="true"
          :title="ev.title"
          @dragstart="onDragStart($event, ev)"
          @click="emit('edit', ev)"
        >
          <span class="sch-it__txt">{{ ev.title }}</span>
        </button>
      </div>
    </div>

    <div ref="scroller" class="sch-wv__body">
      <div class="sch-wv__grid" :style="{ height: `${HOUR * 24}px` }">
        <div class="sch-wv__hours" aria-hidden="true">
          <span v-for="h in 23" :key="h" class="sch-num" :style="{ top: `${h * HOUR}px` }"
            >{{ String(h).padStart(2, '0') }}:00</span
          >
        </div>
        <div
          v-for="d in days"
          :key="d.key"
          class="sch-wv__col"
          :class="{ 'sch-wv__col--today': d.isToday, 'sch-wv__drop': dropKey === d.key }"
          role="button"
          tabindex="-1"
          :aria-label="t('schedulePage.slotAria', { date: d.key })"
          @click.self="onSlotClick($event, d.key)"
          @dragover.prevent="dropKey = d.key"
          @dragleave.self="dropKey = ''"
          @drop.prevent="onDropTimed($event, d.key)"
        >
          <button
            v-for="b in d.blocks"
            :key="b.ev.id + b.ev.recurrence_day"
            type="button"
            class="sch-wv__ev"
            :class="[`sch-wv__ev--${b.ev.kind}`, `sch-st--${statusOf(b.ev)}`, b.ev.color && `sch-c--${b.ev.color}`]"
            :style="resizing?.ev === b.ev ? { ...b.style, height: `${resizing.height}px` } : b.style"
            draggable="true"
            :title="`${hm(b.ev.start_at)}–${hm(b.ev.end_at)} ${b.ev.title}`"
            @dragstart="onDragStart($event, b.ev)"
            @click="emit('edit', b.ev)"
          >
            <span class="sch-wv__tm sch-num"
              >{{ hm(b.ev.start_at)
              }}<q-icon v-if="b.ev.rrule" name="repeat" size="11px" :aria-label="t('schedulePage.recurring')"
            /></span>
            <span class="sch-wv__tt">{{ b.ev.title }}</span>
            <span v-if="b.ev.location" class="sch-wv__loc">{{ b.ev.location }}</span>
            <!-- 拖下边缘改结束时间 -->
            <span
              class="sch-wv__rs"
              :title="t('schedulePage.resizeHint')"
              aria-hidden="true"
              @mousedown.stop.prevent="startResize($event, b.ev, d.key)"
              @click.stop
            />
          </button>
          <button
            v-for="b in d.drafts"
            :key="b.dr.key"
            type="button"
            class="sch-wv__ev sch-wv__ev--ai"
            :style="b.style"
            :title="b.dr.title"
            @click="emit('select', d.key)"
          >
            <span class="sch-wv__tm sch-num">{{ hm(b.dr.start_at) }}</span>
            <span class="sch-wv__tt">{{ b.dr.title }}</span>
          </button>
          <div v-if="d.isToday" class="sch-wv__now" :style="{ top: `${nowTop}px` }" aria-hidden="true" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { OfficeEventRow } from '../../office/types';
import {
  bucketByDay,
  dayKey,
  displayStatus,
  hm,
  layoutLanes,
  lunarLabel,
  parseDayKey,
  sortDay,
  type DraftEntry,
  type Visible,
} from '../model';

const props = defineProps<{
  /** 这周的周一 */
  start: Date;
  events: OfficeEventRow[];
  drafts: DraftEntry[];
  show: Visible;
  today: Date;
  selectedDay: string;
  /** 几天：周视图 7，日视图 1 */
  count?: number;
}>();

const emit = defineEmits<{
  select: [day: string];
  /** 点空白时段：在这天这个时刻新建计划 */
  create: [day: string, time: string];
  edit: [ev: OfficeEventRow];
  /** 拖动改期：新的开始时间（保持时长） */
  move: [ev: OfficeEventRow, start: Date];
  /** 拖到全天行 / 别的日期：只换天 */
  moveDay: [ev: OfficeEventRow, day: string];
  /** 拖下边缘：新的结束时间 */
  resize: [ev: OfficeEventRow, end: Date];
}>();

const { t } = useI18n();

/** 一小时的高度（px）。 */
const HOUR = 44;
const MIN_BLOCK = 20;

function statusOf(ev: OfficeEventRow) {
  return displayStatus(ev, props.today);
}

const days = computed(() => {
  const byDay = bucketByDay(props.events, (e) => [e.start_at, e.end_at]);
  const draftsByDay = bucketByDay(props.drafts, (d) => [d.start_at, d.end_at]);
  const todayKey = dayKey(props.today);
  const keys = Array.from({ length: props.count ?? 7 }, (_, i) =>
    dayKey(new Date(props.start.getFullYear(), props.start.getMonth(), props.start.getDate() + i)),
  );
  return keys.map((key) => {
    const d0 = parseDayKey(key).getTime();
    const d1 = d0 + 86_400_000;
    const list = sortDay(byDay.get(key) ?? []).filter((e) => (e.kind === 'log' ? props.show.log : props.show.plan));
    const timed = list.filter((e) => !e.all_day);
    // 跨天的按这一天截一段
    const spans = timed.map((e) => ({
      start: Math.max(Date.parse(e.start_at), d0),
      end: Math.min(Math.max(Date.parse(e.end_at), Date.parse(e.start_at) + 15 * 60_000), d1),
    }));
    const lanes = layoutLanes(spans);
    const style = (s: { start: number; end: number }, l: { lane: number; lanes: number }) => ({
      top: `${((s.start - d0) / 3_600_000) * HOUR}px`,
      height: `${Math.max(((s.end - s.start) / 3_600_000) * HOUR - 2, MIN_BLOCK)}px`,
      left: `calc(${(l.lane / l.lanes) * 100}% + 2px)`,
      width: `calc(${100 / l.lanes}% - 4px)`,
    });
    const drafts = props.show.ai ? (draftsByDay.get(key) ?? []) : [];
    const draftSpans = drafts.map((dr) => ({
      start: Math.max(Date.parse(dr.start_at), d0),
      end: Math.min(Date.parse(dr.end_at), d1),
    }));
    const date = parseDayKey(key);
    return {
      key,
      date: date.getDate(),
      wd: (date.getDay() + 6) % 7,
      lunar: lunarLabel(date),
      isToday: key === todayKey,
      allDay: list.filter((e) => e.all_day && e.kind === 'plan'),
      blocks: timed.map((ev, i) => ({ ev, style: style(spans[i]!, lanes[i]!) })),
      drafts: drafts.map((dr, i) => ({ dr, style: style(draftSpans[i]!, { lane: 0, lanes: 1 }) })),
    };
  });
});

const nowTop = computed(() => (props.today.getHours() + props.today.getMinutes() / 60) * HOUR);

// 打开时滚到早上 7 点半附近（今天这周滚到现在前一小时）
const scroller = ref<HTMLElement | null>(null);
onMounted(() => {
  const isThisWeek = days.value.some((d) => d.isToday);
  const hour = isThisWeek ? Math.max(props.today.getHours() - 1, 0) : 7.5;
  if (scroller.value) scroller.value.scrollTop = hour * HOUR;
});

/** 点空白：按半小时取整。 */
function onSlotClick(e: MouseEvent, day: string) {
  const half = Math.min(Math.floor((e.offsetY / HOUR) * 2), 47);
  emit('create', day, `${String(Math.floor(half / 2)).padStart(2, '0')}:${half % 2 ? '30' : '00'}`);
}

// ——— 拖动：记下抓的是哪条、抓在条内什么位置 ———
const dropKey = ref('');
let dragging: { ev: OfficeEventRow; grabY: number } | null = null;

function onDragStart(e: DragEvent, ev: OfficeEventRow) {
  dragging = { ev, grabY: e.offsetY };
  e.dataTransfer?.setData('text/plain', ev.title);
  if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
}

/** 放到时间轴：按 15 分钟取整。 */
function onDropTimed(e: DragEvent, day: string) {
  dropKey.value = '';
  const d = dragging;
  dragging = null;
  if (!d) return;
  if (d.ev.all_day) {
    emit('moveDay', d.ev, day);
    return;
  }
  const col = (e.currentTarget as HTMLElement).getBoundingClientRect();
  const minutes = Math.round((((e.clientY - col.top - d.grabY) / HOUR) * 60) / 15) * 15;
  const base = parseDayKey(day);
  emit(
    'move',
    d.ev,
    new Date(base.getFullYear(), base.getMonth(), base.getDate(), 0, Math.min(Math.max(minutes, 0), 1425)),
  );
}

// ——— 拖下边缘改时长：跟着鼠标实时变高，松手按 15 分钟取整 ———
const resizing = ref<{ ev: OfficeEventRow; height: number } | null>(null);

function startResize(e: MouseEvent, ev: OfficeEventRow, day: string) {
  const col = (e.target as HTMLElement).closest('.sch-wv__col')?.getBoundingClientRect();
  if (!col) return;
  const d0 = parseDayKey(day).getTime();
  const start = Math.max(Date.parse(ev.start_at), d0);
  const topPx = ((start - d0) / 3_600_000) * HOUR;
  const endOf = (y: number) => {
    const minutes = Math.round((((y - col.top) / HOUR) * 60) / 15) * 15;
    return Math.min(Math.max(d0 + minutes * 60_000, start + 15 * 60_000), d0 + 24 * 3_600_000);
  };
  const move = (ev2: MouseEvent) => {
    resizing.value = { ev, height: Math.max(((endOf(ev2.clientY) - d0) / 3_600_000) * HOUR - topPx - 2, MIN_BLOCK) };
  };
  const up = (ev2: MouseEvent) => {
    window.removeEventListener('mousemove', move);
    window.removeEventListener('mouseup', up);
    resizing.value = null;
    const end = endOf(ev2.clientY);
    if (end !== Date.parse(ev.end_at)) emit('resize', ev, new Date(end));
  };
  window.addEventListener('mousemove', move);
  window.addEventListener('mouseup', up);
}

function onDropAllDay(day: string) {
  dropKey.value = '';
  const d = dragging;
  dragging = null;
  if (d) emit('moveDay', d.ev, day);
}
</script>
