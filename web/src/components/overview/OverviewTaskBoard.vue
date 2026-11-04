<template>
  <section class="task-board" data-testid="task-situation-board" :aria-busy="loading">
    <header class="task-board__head">
      <div>
        <h2>{{ title ?? t('overviewPage.v2.taskBoard.title') }}</h2>
        <p>{{ subtitle ?? t('overviewPage.v2.taskBoard.subtitle') }}</p>
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
    <div
      v-else-if="!items.length && !error"
      class="task-board__state task-board__empty"
      data-testid="task-board-empty"
    >
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
      <article v-for="item in items" :key="item.task_id" class="task-board__row" :data-task-id="item.task_id">
        <div class="task-board__task">
          <strong :data-testid="`task-title-${item.task_id}`">{{ item.title ?? t('overviewPage.v2.taskBoard.notCollected') }}</strong>
          <small>{{ item.session_id }}</small>
        </div>
        <div class="task-board__status">
          <span :class="`task-board__badge task-board__badge--${item.status}`">
            {{ statusLabel(item.status) }}
          </span>
          <small v-if="item.intervention_type">{{ item.intervention_type }}</small>
        </div>
        <p class="task-board__progress">{{ item.current_step_summary ?? t('overviewPage.v2.taskBoard.notCollected') }}</p>
        <span class="task-board__executor" :data-testid="`task-executor-${item.task_id}`">{{ item.executor_summary ?? t('overviewPage.v2.taskBoard.notCollected') }}</span>
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
          :aria-label="expandable ? t(expandedTaskIds.includes(item.task_id) ? 'taskWorkspace.collapseTask' : 'taskWorkspace.expandTask') : t('overviewPage.v2.taskBoard.open')"
          :aria-expanded="expandable ? expandedTaskIds.includes(item.task_id) : undefined"
          @click="expandable ? $emit('toggle-expand', item) : $emit('drill', item)"
        >
          <span aria-hidden="true">{{ expandable ? (expandedTaskIds.includes(item.task_id) ? '−' : '+') : '→' }}</span>
          <span class="task-board__open-label">{{ expandable ? t(expandedTaskIds.includes(item.task_id) ? 'taskWorkspace.collapseTask' : 'taskWorkspace.expandTask') : t('overviewPage.v2.taskBoard.open') }}</span>
        </button>
        <dl v-if="expandable && expandedTaskIds.includes(item.task_id)" class="task-board__detail" :data-testid="`task-detail-${item.task_id}`">
          <dt>{{ t('overviewPage.v2.taskBoard.progress') }}</dt>
          <dd>{{ item.current_step_summary ?? t('overviewPage.v2.taskBoard.notCollected') }}</dd>
          <dt>current_turn_id / current_step_id</dt>
          <dd>{{ item.current_turn_id ?? t('overviewPage.v2.taskBoard.notCollected') }} / {{ item.current_step_id ?? t('overviewPage.v2.taskBoard.notCollected') }}</dd>
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

const props = withDefaults(defineProps<{
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
}>(), { expandable: false, expandedTaskIds: () => [] });

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
  padding: 16px;
  border: 1px solid var(--app-border);
  border-radius: 12px;
  background: var(--app-surface);
  color: var(--app-text);
}
.task-board__head,
.task-board__filters,
.task-board__error {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.task-board__head { justify-content: space-between; margin-bottom: 12px; }
.task-board__head h2 { margin: 0; font-size: 16px; line-height: 24px; }
.task-board__head p { margin: 2px 0 0; color: var(--app-text-secondary); font-size: 13px; line-height: 20px; }
.task-board__head > strong { font-size: 13px; line-height: 20px; }
.task-board__filters { margin-bottom: 12px; }
.task-board__filters label { display: grid; gap: 4px; min-width: 120px; font-size: 12px; line-height: 16px; }
.task-board__filters .task-board__keyword { flex: 1 1 200px; min-width: 160px; }
.task-board__filters input,
.task-board__filters select,
.task-board button {
  min-height: 32px;
  padding: 5px 10px;
  border: 1px solid var(--app-border);
  border-radius: 7px;
  background: var(--app-surface);
  color: var(--app-text);
  font: inherit;
  font-size: 13px;
  line-height: 20px;
}
.task-board button { cursor: pointer; }
.task-board__filters > button { align-self: end; }
.task-board__secondary,
.task-board__open { background: transparent; }
.task-board__error {
  justify-content: space-between;
  margin-bottom: 10px;
  padding: 8px 12px;
  border-radius: 8px;
  color: var(--q-negative);
  background: color-mix(in srgb, var(--q-negative) 9%, transparent);
  font-size: 13px;
  line-height: 20px;
}
.task-board__state { padding: 28px 12px; text-align: center; font-size: 13px; line-height: 20px; }
.task-board__empty p { margin: 5px 0 12px; color: var(--app-text-secondary); }
.task-board__list { min-width: 0; }
.task-board__row {
  display: grid;
  grid-template-columns: minmax(180px, 1.8fr) 88px minmax(160px, 1.5fr) minmax(90px, .8fr) 138px 76px 86px;
  gap: 12px;
  align-items: center;
  min-width: 0;
  padding: 8px 4px;
  border-top: 1px solid var(--app-border);
  font-size: 13px;
  line-height: 20px;
}
.task-board__row > * { min-width: 0; overflow-wrap: anywhere; }
.task-board__detail { grid-column: 1 / -1; display: grid; grid-template-columns: minmax(120px, .3fr) minmax(0, 1fr); gap: 4px 12px; margin: 4px 0 8px; padding: 10px 12px; border-radius: 7px; background: var(--app-surface-muted); }
.task-board__detail dt { color: var(--app-text-secondary); }
.task-board__detail dd { margin: 0; white-space: pre-wrap; }
.task-board__row--head { padding-block: 5px; border-top: 0; color: var(--app-text-secondary); font-size: 12px; line-height: 18px; }
.task-board__task strong,
.task-board__task small { display: block; }
.task-board__task strong { max-height: 40px; overflow: hidden; }
.task-board__task small,
.task-board__status small { color: var(--app-text-secondary); font-size: 12px; line-height: 18px; }
.task-board__progress { display: -webkit-box; margin: 0; overflow: hidden; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
.task-board__executor { display: -webkit-box; overflow: hidden; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
.task-board__badge { display: inline-block; padding: 1px 6px; border-radius: 5px; background: var(--app-surface-muted); }
.task-board__badge--waiting,
.task-board__badge--failed { color: var(--q-warning); }
.task-board__badge--running { color: var(--q-primary); }
.task-board__badge--completed { color: var(--q-positive); }
.task-board__open { white-space: nowrap; }
.task-board :deep(.app-registry-pagination) { flex-wrap: wrap; }
@media (max-width: 1100px) {
  .task-board__row--head { display: none; }
  .task-board__row {
    grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr) auto;
    gap: 6px 12px;
  }
  .task-board__task { grid-column: 1 / 3; }
  .task-board__status { grid-column: 3; grid-row: 1; }
  .task-board__progress { grid-column: 1 / 3; }
  .task-board__open-label { display: none; }
}
@media (max-width: 600px) {
  .task-board { padding: 12px; }
  .task-board__filters label,
  .task-board__filters .task-board__keyword { flex: 1 1 100%; min-width: 0; }
  .task-board__row { grid-template-columns: minmax(0, 1fr) auto; }
  .task-board__task,
  .task-board__progress { grid-column: 1 / -1; }
  .task-board__status { grid-column: 2; grid-row: 1; }
  .task-board__row > time { grid-column: 1 / -1; }
}
@container task-situation (max-width: 1100px) {
  .task-board__row--head { display: none; }
  .task-board__row {
    grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr) auto;
    gap: 6px 12px;
  }
  .task-board__task { grid-column: 1 / 3; }
  .task-board__status { grid-column: 3; grid-row: 1; }
  .task-board__progress { grid-column: 1 / 3; }
  .task-board__open-label { display: none; }
}
@container task-situation (max-width: 600px) {
  .task-board { padding: 12px; }
  .task-board__filters label,
  .task-board__filters .task-board__keyword { flex: 1 1 100%; min-width: 0; }
  .task-board__row { grid-template-columns: minmax(0, 1fr) auto; }
  .task-board__task,
  .task-board__progress { grid-column: 1 / -1; }
  .task-board__status { grid-column: 2; grid-row: 1; }
  .task-board__row > time { grid-column: 1 / -1; }
}
</style>
