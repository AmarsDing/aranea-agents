import { computed, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useRoute, useRouter, type LocationQuery, type LocationQueryRaw } from 'vue-router';
import { useTaskSituationStore } from '../../stores/session/taskSituations';
import type { TaskSituationQuery } from './types';

const statuses = new Set(['queued', 'running', 'waiting', 'failed', 'completed', 'cancelled']);

function scalar(value: LocationQuery[string]): string | undefined {
  return typeof value === 'string' ? value : undefined;
}

function nonNegative(value: string | undefined, fallback: number): number {
  if (!value || !/^(0|[1-9]\d*)$/.test(value)) return fallback;
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) ? parsed : fallback;
}

export function taskSituationQueryFromRoute(routeQuery: LocationQuery): TaskSituationQuery {
  const status = scalar(routeQuery.status);
  const sortBy = scalar(routeQuery.sort_by);
  const sortOrder = scalar(routeQuery.sort_order);
  const keyword = scalar(routeQuery.keyword)?.trim();
  return {
    status: status && statuses.has(status) ? status : undefined,
    keyword: keyword || undefined,
    limit: Math.min(100, Math.max(1, nonNegative(scalar(routeQuery.limit), 20))),
    offset: nonNegative(scalar(routeQuery.offset), 0),
    sort_by: sortBy === 'created_at' ? 'created_at' : 'updated_at',
    sort_order: sortOrder === 'asc' ? 'asc' : 'desc',
  };
}

function routeQuery(query: TaskSituationQuery): LocationQueryRaw {
  return {
    status: query.status,
    keyword: query.keyword,
    limit: String(query.limit ?? 20),
    offset: String(query.offset ?? 0),
    sort_by: query.sort_by ?? 'updated_at',
    sort_order: query.sort_order ?? 'desc',
  };
}

export interface TaskSituationOptions {
  sessionId?: () => string | undefined;
  query?: () => TaskSituationQuery;
  enabled?: () => boolean;
  onQueryChange?: (query: TaskSituationQuery) => void;
}

export function useTaskSituations(options: TaskSituationOptions = {}) {
  const route = useRoute();
  const router = useRouter();
  const store = useTaskSituationStore();
  const { items, total, limit, offset, loading, error } = storeToRefs(store);
  const query = computed(() => options.query?.() ?? taskSituationQueryFromRoute(route.query));
  const sessionId = computed(() => options.sessionId?.());
  const enabled = computed(() => options.enabled?.() ?? true);
  let previousSessionId: string | undefined;

  watch(
    [query, sessionId, enabled],
    ([value, sid, active]) => {
      if (!active) return;
      if (options.sessionId && previousSessionId !== sid) store.reset();
      previousSessionId = sid;
      if (options.sessionId && !sid) {
        store.reset();
        return;
      }
      void store.load({ ...value, session_id: sid });
    },
    { immediate: true, deep: true },
  );

  async function setQuery(patch: TaskSituationQuery) {
    const next = { ...query.value, ...patch };
    if (options.onQueryChange) {
      options.onQueryChange(next);
      return;
    }
    await router.replace({ path: '/overview', query: routeQuery(next) });
  }

  function refresh() {
    if (!enabled.value || (options.sessionId && !sessionId.value)) return Promise.resolve();
    return store.load({ ...query.value, session_id: sessionId.value });
  }

  return { items, total, limit, offset, loading, error, query, setQuery, refresh };
}
