<template>
  <section class="today-inbox" :aria-busy="!loaded">
    <header class="today-panel__head">
      <h2 class="today-panel__title">
        {{ t('today.inbox.title') }}
        <span class="today-panel__count">{{ total }}</span>
      </h2>
      <router-link class="today-link" to="/approvals">{{ t('today.inbox.viewAll') }}</router-link>
    </header>

    <p v-if="failed" class="today-note today-note--danger" role="alert">{{ t('today.inbox.failed') }}</p>
    <p v-else-if="degradedLabel" class="today-note today-note--warn" role="status">
      {{ t('today.inbox.degraded', { sources: degradedLabel }) }}
    </p>

    <p v-if="!loaded" class="today-empty">{{ t('today.inbox.loading') }}</p>
    <p v-else-if="rows.length === 0" class="today-empty">{{ t('today.inbox.empty') }}</p>

    <template v-else>
      <ul
        ref="listEl"
        class="today-inbox__list"
        role="listbox"
        tabindex="0"
        :aria-label="t('today.inbox.listLabel')"
        :aria-activedescendant="selected ? optionId(selected.id) : undefined"
        @keydown="onKeydown"
      >
        <li
          v-for="row in rows"
          :id="optionId(row.id)"
          :key="row.id"
          class="today-inbox__row"
          :class="{ 'today-inbox__row--selected': row.id === selectedId }"
          role="option"
          :aria-selected="row.id === selectedId"
          @click="emit('select', row.id)"
        >
          <span class="today-risk" :class="`today-risk--${riskOf(row)}`">{{ t(`today.risk.${riskOf(row)}`) }}</span>
          <span class="today-inbox__main">
            <span class="today-inbox__title">
              <span class="today-inbox__kind">{{ kindLabel(row) }}</span>
              {{ row.title || row.summary }}
            </span>
            <span class="today-inbox__sub">
              {{ sourceLabel(row.source) }}
              <template v-if="row.requester"> · {{ t('today.inbox.from', { who: row.requester }) }}</template>
            </span>
          </span>
          <span v-if="dueLabel(row)" class="today-inbox__due" :class="{ 'today-inbox__due--late': isOverdue(row) }">
            {{ dueLabel(row) }}
          </span>
        </li>
      </ul>
      <p v-if="total > rows.length" class="today-note">{{ t('today.inbox.more', { n: total - rows.length }) }}</p>
      <p class="today-inbox__keys">{{ t('today.inbox.shortcuts') }}</p>

      <article v-if="selected" class="today-inbox__detail" :aria-label="selected.title">
        <h3 class="today-inbox__detail-title">{{ selected.title || selected.summary }}</h3>
        <p v-if="selected.summary && selected.summary !== selected.title" class="today-inbox__summary">
          {{ selected.summary }}
        </p>
        <dl class="today-inbox__meta">
          <div>
            <dt>{{ t('today.inbox.meta.source') }}</dt>
            <dd>{{ sourceLabel(selected.source) }}</dd>
          </div>
          <div v-if="selected.requester">
            <dt>{{ t('today.inbox.meta.requester') }}</dt>
            <dd>{{ selected.requester }}</dd>
          </div>
          <div v-if="selected.agent_key">
            <dt>{{ t('today.inbox.meta.agent') }}</dt>
            <dd class="today-mono">{{ selected.agent_key }}</dd>
          </div>
          <div v-if="selected.created_at">
            <dt>{{ t('today.inbox.meta.created') }}</dt>
            <dd class="today-mono">{{ dateTime(selected.created_at) }}</dd>
          </div>
          <div v-if="selected.due_at">
            <dt>{{ t('today.inbox.meta.due') }}</dt>
            <dd class="today-mono">{{ dateTime(selected.due_at) }}</dd>
          </div>
        </dl>

        <form v-if="rejecting" class="today-inbox__reject" @submit.prevent="submitReject">
          <q-input
            ref="noteEl"
            v-model="note"
            type="textarea"
            autogrow
            outlined
            dense
            :label="t('today.inbox.rejectNote')"
            :error="noteMissing"
            :error-message="t('today.inbox.rejectNoteRequired')"
            @keydown.esc.stop="cancelReject()"
          />
          <div class="today-inbox__actions">
            <q-btn
              type="submit"
              unelevated
              no-caps
              color="negative"
              :label="t('today.inbox.confirmReject')"
              :loading="busy"
            />
            <q-btn flat no-caps :label="t('today.inbox.cancel')" :disable="busy" @click="cancelReject()" />
          </div>
        </form>

        <div v-else class="today-inbox__actions">
          <q-btn
            v-if="primary"
            unelevated
            no-caps
            color="primary"
            :label="t(`today.action.${primary}`)"
            :loading="busy"
            @click="act(primary)"
          />
          <q-btn
            v-if="canReject"
            outline
            no-caps
            color="negative"
            :label="t('today.action.reject')"
            :disable="busy"
            @click="startReject"
          />
          <q-btn
            v-if="batchCount > 1"
            flat
            no-caps
            :label="t('today.inbox.batchApprove', { n: batchCount })"
            :disable="busy"
            @click="emit('batch', selected.source)"
          />
          <q-btn v-if="selected.link" flat no-caps :label="t('today.inbox.openDetail')" :to="selected.link" />
          <span v-if="!primary && !canReject" class="today-note">{{ t('today.inbox.onlyLink') }}</span>
        </div>
      </article>
      <p v-else class="today-empty">{{ t('today.inbox.selectHint') }}</p>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import {
  batchableLowRisk,
  clock,
  dayBounds,
  primaryAction,
  type InboxAction,
  type InboxItemRow,
} from '../../features/today/logic';

const props = defineProps<{
  rows: InboxItemRow[];
  total: number;
  loaded: boolean;
  failed: boolean;
  degraded: string[];
  busyIds: Set<string>;
  selectedId: string;
}>();

const emit = defineEmits<{
  select: [id: string];
  decide: [row: InboxItemRow, action: InboxAction, note: string];
  batch: [source: string];
}>();

const { t, te } = useI18n();
const router = useRouter();

const listEl = ref<HTMLElement | null>(null);
const noteEl = ref<{ focus: () => void } | null>(null);
const rejecting = ref(false);
const note = ref('');
const noteMissing = ref(false);

const selected = computed(() => props.rows.find((row) => row.id === props.selectedId) ?? null);
const primary = computed(() => (selected.value ? primaryAction(selected.value) : ''));
const canReject = computed(() => Boolean(selected.value?.actions.includes('reject')));
const busy = computed(() => Boolean(selected.value && props.busyIds.has(selected.value.id)));
const batchCount = computed(() =>
  selected.value && selected.value.risk_level === 'low'
    ? batchableLowRisk(props.rows, selected.value.source).length
    : 0,
);
const degradedLabel = computed(() => props.degraded.map(sourceLabel).join('、'));

// 换了选中条目就收起驳回表单，避免把理由写到另一条上。
watch(
  () => props.selectedId,
  () => cancelReject(false),
);

function optionId(id: string): string {
  return `today-inbox-${id.replace(/[^a-zA-Z0-9_-]/g, '_')}`;
}

function riskOf(row: InboxItemRow): 'high' | 'medium' | 'low' {
  return row.risk_level || 'medium';
}

function sourceLabel(source: string): string {
  const key = `today.source.${source}`;
  return te(key) ? t(key) : source;
}

function kindLabel(row: InboxItemRow): string {
  const key = `today.kind.${row.kind}`;
  return te(key) ? t(key) : '';
}

function isOverdue(row: InboxItemRow): boolean {
  return Boolean(row.due_at) && new Date(row.due_at).getTime() < Date.now();
}

function dueLabel(row: InboxItemRow): string {
  if (!row.due_at) return '';
  if (isOverdue(row)) return t('today.inbox.overdue');
  const due = new Date(row.due_at);
  const today = due.getTime() < dayBounds(new Date()).end.getTime();
  return t('today.inbox.due', { time: today ? clock(row.due_at) : dateTime(row.due_at) });
}

function dateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${clock(iso)}`;
}

function act(action: InboxAction | '') {
  if (!selected.value || !action || busy.value) return;
  emit('decide', selected.value, action, '');
}

function startReject() {
  if (!canReject.value || busy.value) return;
  rejecting.value = true;
  noteMissing.value = false;
  void nextTick(() => noteEl.value?.focus());
}

function cancelReject(refocus = true) {
  if (!rejecting.value) return;
  rejecting.value = false;
  note.value = '';
  noteMissing.value = false;
  if (refocus) listEl.value?.focus();
}

function submitReject() {
  if (!selected.value || busy.value) return;
  const reason = note.value.trim();
  if (!reason && selected.value.reject_requires_note) {
    noteMissing.value = true;
    return;
  }
  emit('decide', selected.value, 'reject', reason);
}

function move(delta: number) {
  if (props.rows.length === 0) return;
  const index = props.rows.findIndex((row) => row.id === props.selectedId);
  const next = Math.min(props.rows.length - 1, Math.max(0, (index < 0 ? -1 : index) + delta));
  const id = props.rows[next]?.id;
  if (!id) return;
  emit('select', id);
  void nextTick(() => document.getElementById(optionId(id))?.scrollIntoView({ block: 'nearest' }));
}

function onKeydown(event: KeyboardEvent) {
  if (event.ctrlKey || event.metaKey || event.altKey) return;
  const key = event.key.length === 1 ? event.key.toLowerCase() : event.key;
  switch (key) {
    case 'ArrowDown':
    case 'j':
      move(1);
      break;
    case 'ArrowUp':
    case 'k':
      move(-1);
      break;
    case 'Home':
      move(-props.rows.length);
      break;
    case 'End':
      move(props.rows.length);
      break;
    case 'Enter':
      if (primary.value) act(primary.value);
      else if (selected.value?.link) void router.push(selected.value.link);
      break;
    case 'r':
      startReject();
      break;
    case 'o':
      if (selected.value?.link) void router.push(selected.value.link);
      break;
    default:
      return;
  }
  event.preventDefault();
}

defineExpose({ focusList: () => listEl.value?.focus() });
</script>
