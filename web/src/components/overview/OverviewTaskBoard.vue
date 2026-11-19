<template>
  <section class="task-board" data-testid="task-situation-board" :aria-busy="loading">
    <header class="task-board__head">
      <div class="task-board__heading">
        <span class="task-board__seat" aria-hidden="true">
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.6"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <rect x="3.5" y="4" width="5" height="16" rx="1.5" />
            <rect x="10.5" y="4" width="5" height="10" rx="1.5" />
            <rect x="17.5" y="4" width="3" height="7" rx="1.5" />
          </svg>
        </span>
        <div>
          <h2>{{ title ?? t('overviewPage.v2.taskBoard.title') }}</h2>
          <p>{{ subtitle ?? t('overviewPage.v2.taskBoard.subtitle') }}</p>
        </div>
      </div>
      <strong data-testid="task-board-total">{{ t('overviewPage.v2.taskBoard.total', { count: total }) }}</strong>
    </header>

    <form class="task-board__filters" @submit.prevent="applyFilters">
      <label>
        <span>{{ t('overviewPage.v2.taskBoard.status') }}</span>
        <select v-model="draft.status" data-testid="task-filter-status">
          <option value="">{{ t('overviewPage.v2.taskBoard.allStatuses') }}</option>
          <option v-for="status in statuses" :key="status" :value="status">
            {{ t(`overviewPage.v2.taskBoard.${status}`) }}
          </option>
        </select>
      </label>
      <label class="task-board__keyword">
        <span>{{ t('overviewPage.v2.taskBoard.keyword') }}</span>
        <input v-model="draft.keyword" type="search" data-testid="task-filter-keyword" />
      </label>
      <label>
        <span>{{ t('overviewPage.v2.taskBoard.sortField') }}</span>
        <select v-model="draft.sort_by">
          <option value="updated_at">{{ t('overviewPage.v2.taskBoard.updatedAt') }}</option>
          <option value="created_at">{{ t('overviewPage.v2.taskBoard.createdAt') }}</option>
        </select>
      </label>
      <label>
        <span>{{ t('overviewPage.v2.taskBoard.sortOrder') }}</span>
        <select v-model="draft.sort_order">
          <option value="desc">{{ t('overviewPage.v2.taskBoard.descending') }}</option>
          <option value="asc">{{ t('overviewPage.v2.taskBoard.ascending') }}</option>
        </select>
      </label>
      <button type="submit">{{ t('overviewPage.v2.taskBoard.apply') }}</button>
      <button type="button" class="task-board__secondary" @click="clearFilters">
        {{ t('overviewPage.v2.taskBoard.clear') }}
      </button>
    </form>

    <div v-if="error" class="task-board__error" role="alert">
      <span>{{ t('overviewPage.v2.taskBoard.loadFailed') }}: {{ error }}</span>
      <button type="button" @click="$emit('refresh')">{{ t('overviewPage.v2.taskBoard.retry') }}</button>
    </div>
    <div v-if="loading && !items.length" class="task-board__state" data-testid="task-board-loading" role="status">
      {{ t('overviewPage.v2.taskBoard.loading') }}
    </div>
    <div v-else-if="!items.length && !error" class="task-board__state task-board__empty" data-testid="task-board-empty">
      <strong>{{ emptyTitle ?? t('overviewPage.v2.taskBoard.emptyTitle') }}</strong>
      <p>{{ emptyHint ?? t('overviewPage.v2.taskBoard.emptyHint') }}</p>
      <button type="button" data-testid="task-board-create" @click="$emit('create')">
        {{ createLabel ?? t('overviewPage.v2.taskBoard.create') }}
      </button>
    </div>

    <div v-if="items.length" class="task-board__list" data-testid="task-board-list">
      <div class="task-board__row task-board__row--head" aria-hidden="true">
        <span>{{ t('overviewPage.v2.taskBoard.task') }}</span>
        <span>{{ t('overviewPage.v2.taskBoard.status') }}</span>
        <span>{{ t('overviewPage.v2.taskBoard.progress') }}</span>
        <span>{{ t('overviewPage.v2.taskBoard.executor') }}</span>
        <span>{{ t('overviewPage.v2.taskBoard.waitingOrUpdated') }}</span>
        <span>{{ t('overviewPage.v2.taskBoard.artifacts') }}</span>
        <span />
      </div>
      <article
        v-for="item in items"
        :key="item.task_id"
        class="task-board__row"
        :class="`task-board__row--${item.status}`"
        :data-task-id="item.task_id"
      >
        <div class="task-board__task">
          <strong :data-testid="`task-title-${item.task_id}`">{{
            item.title ?? t('overviewPage.v2.taskBoard.notCollected')
          }}</strong>
          <small>{{ item.session_id }}</small>
        </div>
        <div class="task-board__status">
          <span :class="`task-board__badge task-board__badge--${item.status}`">
            {{ statusLabel(item.status) }}
          </span>
          <small v-if="item.intervention_type">{{ item.intervention_type }}</small>
        </div>
        <p class="task-board__progress">
          {{ item.current_step_summary ?? t('overviewPage.v2.taskBoard.notCollected') }}
        </p>
        <span class="task-board__executor" :data-testid="`task-executor-${item.task_id}`">{{
          item.executor_summary ?? t('overviewPage.v2.taskBoard.notCollected')
        }}</span>
        <time v-if="item.waiting_since ?? item.updated_at" :datetime="item.waiting_since ?? item.updated_at">
          {{ formatTime(item.waiting_since ?? item.updated_at) }}
        </time>
        <span v-else>{{ t('overviewPage.v2.taskBoard.notCollected') }}</span>
        <span :data-testid="`task-artifacts-${item.task_id}`">
          {{ item.artifact_count ?? t('overviewPage.v2.taskBoard.notCollected') }}
        </span>
        <button
          type="button"
          class="task-board__open"
          :data-testid="expandable ? `task-expand-${item.task_id}` : `task-drill-${item.task_id}`"
          :aria-label="
            expandable
              ? t(expandedTaskIds.includes(item.task_id) ? 'taskWorkspace.collapseTask' : 'taskWorkspace.expandTask')
              : t('overviewPage.v2.taskBoard.open')
          "
          :aria-expanded="expandable ? expandedTaskIds.includes(item.task_id) : undefined"
          @click="expandable ? $emit('toggle-expand', item) : $emit('drill', item)"
        >
          <span aria-hidden="true">{{ expandable ? (expandedTaskIds.includes(item.task_id) ? '−' : '+') : '→' }}</span>
          <span class="task-board__open-label">{{
            expandable
              ? t(expandedTaskIds.includes(item.task_id) ? 'taskWorkspace.collapseTask' : 'taskWorkspace.expandTask')
              : t('overviewPage.v2.taskBoard.open')
          }}</span>
        </button>
        <dl
          v-if="expandable && expandedTaskIds.includes(item.task_id)"
          class="task-board__detail"
          :data-testid="`task-detail-${item.task_id}`"
        >
          <dt>{{ t('overviewPage.v2.taskBoard.progress') }}</dt>
          <dd>{{ item.current_step_summary ?? t('overviewPage.v2.taskBoard.notCollected') }}</dd>
          <dt>current_turn_id / current_step_id</dt>
          <dd>
            {{ item.current_turn_id ?? t('overviewPage.v2.taskBoard.notCollected') }} /
            {{ item.current_step_id ?? t('overviewPage.v2.taskBoard.notCollected') }}
          </dd>
          <dt>failed_step_id</dt>
          <dd>{{ item.failed_step_id ?? t('overviewPage.v2.taskBoard.notCollected') }}</dd>
          <dt>{{ t('overviewPage.v2.taskBoard.executor') }}</dt>
          <dd>{{ item.executor_summary ?? t('overviewPage.v2.taskBoard.notCollected') }}</dd>
        </dl>
      </article>
    </div>

    <AppRegistryPagination
      v-if="total > 0"
      :page="page"
      :page-size="limit"
      :page-max="pageMax"
      :total="total"
      :loading="loading"
      :page-size-options="[20, 50, 100]"
      @update:page="onPage"
      @update:page-size="$emit('page-size', $event)"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppRegistryPagination from '../layout/AppRegistryPagination.vue';
import type { TaskSituationItem, TaskSituationQuery } from '../../features/session/types';

const props = withDefaults(
  defineProps<{
    items: TaskSituationItem[];
    total: number;
    limit: number;
    offset: number;
    loading: boolean;
    error: string;
    query: TaskSituationQuery;
    title?: string;
    subtitle?: string;
    emptyTitle?: string;
    emptyHint?: string;
    createLabel?: string;
    expandable?: boolean;
    expandedTaskIds?: string[];
  }>(),
  { expandable: false, expandedTaskIds: () => [] },
);

const emit = defineEmits<{
  refresh: [];
  create: [];
  drill: [item: TaskSituationItem];
  filter: [query: TaskSituationQuery];
  page: [offset: number];
  'page-size': [limit: number];
  'toggle-expand': [item: TaskSituationItem];
}>();

const { t } = useI18n();
const statuses = ['queued', 'running', 'waiting', 'failed', 'completed', 'cancelled'];
const draft = reactive<TaskSituationQuery>({ ...props.query });
watch(
  () => props.query,
  (query) => Object.assign(draft, query),
  { deep: true },
);

const page = computed(() => Math.floor(props.offset / Math.max(props.limit, 1)) + 1);
const pageMax = computed(() => Math.max(1, Math.ceil(props.total / Math.max(props.limit, 1))));

function applyFilters() {
  emit('filter', { ...draft, keyword: draft.keyword?.trim() || undefined, offset: 0 });
}
function clearFilters() {
  Object.assign(draft, { status: undefined, keyword: undefined, sort_by: 'updated_at', sort_order: 'desc' });
  applyFilters();
}
function onPage(value: number) {
  emit('page', (value - 1) * props.limit);
}
function statusLabel(status: string) {
  return statuses.includes(status) ? t(`overviewPage.v2.taskBoard.${status}`) : status;
}
function formatTime(value: string | undefined) {
  if (!value) return t('overviewPage.v2.taskBoard.notCollected');
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
</script>

<style scoped>
.task-board {
  min-width: 0;
  padding: 14px 16px 16px;
  border: 1px solid var(--ov-shell-border, var(--nt-border-1));
  border-radius: var(--nt-radius-l);
  background: var(--ov-shell, var(--nt-bg-1));
  box-shadow: var(--nt-shadow-depth);
  color: var(--nt-text-1);
}

.task-board__head,
.task-board__filters,
.task-board__error {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.task-board__head {
  justify-content: space-between;
  margin-bottom: var(--nt-space-3);
}

.task-board__heading {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.task-board__seat {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  flex-shrink: 0;
  border-radius: var(--nt-radius-m);
  border: 1px solid color-mix(in srgb, var(--nt-accent) 34%, transparent);
  background: color-mix(in srgb, var(--nt-accent) 10%, transparent);
  color: var(--nt-accent);
}

.task-board__seat svg {
  width: 20px;
  height: 20px;
}

.task-board__head h2 {
  margin: 0;
  font-size: var(--nt-fs-title);
  font-weight: 700;
  line-height: 1.3;
  color: var(--nt-text-1);
}

.task-board__head p {
  margin: 2px 0 0;
  color: var(--nt-text-3);
  font-size: var(--nt-fs-caption);
  line-height: 1.5;
}

.task-board__head > strong {
  padding: var(--nt-space-1) var(--nt-space-3);
  border: 1px solid var(--ov-hairline, var(--nt-border-1));
  border-radius: var(--nt-radius-pill);
  background: var(--ov-inset, var(--nt-data-inset));
  font-family: var(--nt-font-mono);
  font-variant-numeric: tabular-nums;
  font-size: var(--nt-fs-caption);
  font-weight: 700;
  color: var(--nt-text-2);
}

.task-board__filters {
  margin-bottom: var(--nt-space-3);
}

.task-board__filters label {
  display: grid;
  gap: var(--nt-space-1);
  min-width: 120px;
  font-size: var(--nt-fs-caption);
  line-height: 1.4;
  color: var(--nt-text-2);
}

.task-board__filters label:focus-within {
  outline: 2px solid var(--nt-accent);
  outline-offset: 2px;
  border-radius: var(--nt-radius-m);
}

.task-board__filters .task-board__keyword {
  flex: 1 1 200px;
  min-width: 160px;
}

.task-board__filters input,
.task-board__filters select {
  min-height: 36px;
  padding: 6px 10px;
  border: 1px solid var(--ov-hairline, var(--nt-border-1));
  border-radius: var(--nt-radius-m);
  background: var(--ov-inset, var(--nt-data-inset));
  color: var(--nt-text-1);
  font: inherit;
  font-size: var(--nt-fs-body);
  line-height: 1.4;
}

.task-board__filters input:focus,
.task-board__filters select:focus {
  outline: none;
}

.task-board button {
  min-height: 36px;
  padding: 6px 14px;
  border: 1px solid transparent;
  border-radius: var(--nt-radius-m);
  background: var(--nt-accent);
  color: var(--nt-on-accent);
  font: inherit;
  font-size: var(--nt-fs-body);
  font-weight: 600;
  line-height: 1.4;
  cursor: pointer;
  transition:
    box-shadow var(--nt-transition),
    background var(--nt-transition),
    color var(--nt-transition),
    border-color var(--nt-transition);
}

.task-board__filters > button {
  align-self: end;
}

.task-board__secondary,
.task-board__open,
.task-board__error button {
  background: transparent;
  border-color: var(--nt-accent-border);
  color: var(--nt-accent);
}

.task-board__open {
  white-space: nowrap;
  padding: 6px 10px;
  color: var(--nt-text-3);
  border-color: transparent;
}

.task-board button:hover {
  box-shadow: var(--nt-glow-accent);
}

.task-board button:focus-visible {
  outline: 2px solid var(--nt-accent);
  outline-offset: 4px;
}

.task-board__secondary:hover,
.task-board__error button:hover {
  background: var(--nt-accent-soft);
  box-shadow: none;
}

.task-board__open:hover {
  background: var(--nt-accent-soft);
  box-shadow: none;
  color: var(--nt-accent);
  border-color: var(--nt-accent-border);
}

.task-board__error {
  justify-content: space-between;
  margin-bottom: 10px;
  padding: var(--nt-space-2) var(--nt-space-3);
  border: 1px solid color-mix(in srgb, var(--nt-danger) 50%, transparent);
  border-radius: var(--nt-radius-m);
  color: var(--nt-text-1);
  background: color-mix(in srgb, var(--nt-danger) 10%, var(--ov-inset, var(--nt-data-inset)));
  font-size: var(--nt-fs-body);
  line-height: 1.5;
}

.task-board__state {
  padding: 28px 12px;
  text-align: center;
  font-size: var(--nt-fs-body);
  line-height: 1.6;
  color: var(--nt-text-2);
}

.task-board__empty strong {
  display: block;
  font-size: var(--nt-fs-panel);
  color: var(--nt-text-1);
}

.task-board__empty p {
  margin: 5px 0 12px;
  color: var(--nt-text-3);
}

.task-board__list {
  min-width: 0;
}

.task-board__row {
  display: grid;
  grid-template-columns: minmax(180px, 1.8fr) 96px minmax(160px, 1.5fr) minmax(90px, 0.8fr) 138px 64px 92px;
  gap: var(--nt-space-3);
  align-items: center;
  min-width: 0;
  padding: 8px 6px;
  border-top: 1px solid var(--ov-hairline, var(--nt-border-1));
  box-shadow: inset 3px 0 0 var(--nt-text-3);
  font-size: var(--nt-fs-body);
  line-height: 1.5;
  transition: background var(--nt-transition);
}

.task-board__row:hover {
  background: var(--ov-raised, var(--nt-data-raised));
}

.task-board__row--queued {
  box-shadow: inset 3px 0 0 var(--nt-info);
}

.task-board__row--running {
  box-shadow: inset 3px 0 0 var(--nt-accent);
}

.task-board__row--waiting {
  box-shadow: inset 3px 0 0 var(--nt-warn);
}

.task-board__row--failed {
  box-shadow: inset 3px 0 0 var(--nt-danger);
}

.task-board__row--completed {
  box-shadow: inset 3px 0 0 var(--nt-success);
}

.task-board__row > * {
  min-width: 0;
  overflow-wrap: anywhere;
}

.task-board__detail {
  grid-column: 1 / -1;
  display: grid;
  grid-template-columns: minmax(120px, 0.3fr) minmax(0, 1fr);
  gap: var(--nt-space-1) var(--nt-space-3);
  margin: var(--nt-space-1) 0 var(--nt-space-2);
  padding: 10px 12px;
  border-radius: var(--nt-radius-m);
  background: var(--ov-inset, var(--nt-data-inset));
}

.task-board__detail dt {
  color: var(--nt-text-3);
  font-size: var(--nt-fs-caption);
}

.task-board__detail dd {
  margin: 0;
  white-space: pre-wrap;
  font-family: var(--nt-font-mono);
  font-size: var(--nt-fs-caption);
  color: var(--nt-text-2);
}

.task-board__row--head {
  padding-block: 5px;
  border-top: 0;
  box-shadow: none;
  color: var(--nt-text-3);
  font-size: var(--nt-fs-micro);
  letter-spacing: 0.04em;
}

.task-board__row--head:hover {
  background: transparent;
}

.task-board__task strong,
.task-board__task small {
  display: block;
}

.task-board__task strong {
  max-height: 42px;
  overflow: hidden;
  font-weight: 600;
  color: var(--nt-text-1);
}

.task-board__task small {
  font-family: var(--nt-font-mono);
  font-size: var(--nt-fs-micro);
  color: var(--nt-text-3);
}

.task-board__status small {
  display: block;
  color: var(--nt-text-3);
  font-size: var(--nt-fs-micro);
}

.task-board__progress {
  display: -webkit-box;
  margin: 0;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  color: var(--nt-text-2);
  font-size: var(--nt-fs-caption);
}

.task-board__executor {
  display: -webkit-box;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  color: var(--nt-text-2);
  font-size: var(--nt-fs-caption);
}

.task-board__row > time {
  font-family: var(--nt-font-mono);
  font-variant-numeric: tabular-nums;
  font-size: var(--nt-fs-micro);
  color: var(--nt-text-3);
}

.task-board__badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 10px;
  border-radius: var(--nt-radius-pill);
  border: 1px solid var(--ov-hairline, var(--nt-border-1));
  background: var(--ov-inset, var(--nt-data-inset));
  font-size: var(--nt-fs-micro);
  font-weight: 700;
  color: var(--nt-text-2);
}

.task-board__badge::before {
  content: '';
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: currentcolor;
}

.task-board__badge--queued {
  color: var(--nt-info);
  border-color: color-mix(in srgb, var(--nt-info) 45%, transparent);
  background: color-mix(in srgb, var(--nt-info) 10%, transparent);
}

.task-board__badge--running {
  color: var(--nt-accent);
  border-color: color-mix(in srgb, var(--nt-accent) 45%, transparent);
  background: color-mix(in srgb, var(--nt-accent) 10%, transparent);
}

.task-board__badge--waiting {
  color: var(--nt-warn);
  border-color: color-mix(in srgb, var(--nt-warn) 45%, transparent);
  background: color-mix(in srgb, var(--nt-warn) 10%, transparent);
}

.task-board__badge--failed {
  color: var(--nt-danger);
  border-color: color-mix(in srgb, var(--nt-danger) 45%, transparent);
  background: color-mix(in srgb, var(--nt-danger) 10%, transparent);
}

.task-board__badge--completed {
  color: var(--nt-success);
  border-color: color-mix(in srgb, var(--nt-success) 45%, transparent);
  background: color-mix(in srgb, var(--nt-success) 10%, transparent);
}

.task-board__open-label {
  font-size: var(--nt-fs-caption);
}

.task-board :deep(.app-registry-pagination) {
  flex-wrap: wrap;
  margin-top: 10px;
}

@media (width <= 1100px) {
  .task-board__row--head {
    display: none;
  }

  .task-board__row {
    grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr) auto;
    gap: 6px 12px;
  }

  .task-board__task {
    grid-column: 1 / 3;
  }

  .task-board__status {
    grid-column: 3;
    grid-row: 1;
  }

  .task-board__progress {
    grid-column: 1 / 3;
  }

  .task-board__open-label {
    display: none;
  }
}

@media (width <= 600px) {
  .task-board {
    padding: var(--nt-space-3);
  }

  .task-board__filters label,
  .task-board__filters .task-board__keyword {
    flex: 1 1 100%;
    min-width: 0;
  }

  .task-board__row {
    grid-template-columns: minmax(0, 1fr) auto;
  }

  .task-board__task,
  .task-board__progress {
    grid-column: 1 / -1;
  }

  .task-board__status {
    grid-column: 2;
    grid-row: 1;
  }

  .task-board__row > time {
    grid-column: 1 / -1;
  }
}

@container task-situation (max-width: 1100px) {
  .task-board__row--head {
    display: none;
  }

  .task-board__row {
    grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr) auto;
    gap: 6px 12px;
  }

  .task-board__task {
    grid-column: 1 / 3;
  }

  .task-board__status {
    grid-column: 3;
    grid-row: 1;
  }

  .task-board__progress {
    grid-column: 1 / 3;
  }

  .task-board__open-label {
    display: none;
  }
}

@container task-situation (max-width: 600px) {
  .task-board {
    padding: var(--nt-space-3);
  }

  .task-board__filters label,
  .task-board__filters .task-board__keyword {
    flex: 1 1 100%;
    min-width: 0;
  }

  .task-board__row {
    grid-template-columns: minmax(0, 1fr) auto;
  }

  .task-board__task,
  .task-board__progress {
    grid-column: 1 / -1;
  }

  .task-board__status {
    grid-column: 2;
    grid-row: 1;
  }

  .task-board__row > time {
    grid-column: 1 / -1;
  }
}

@media (pointer: coarse) {
  .task-board button,
  .task-board__filters input,
  .task-board__filters select {
    min-height: 44px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .task-board button,
  .task-board__row {
    transition: none;
  }
}
</style>
