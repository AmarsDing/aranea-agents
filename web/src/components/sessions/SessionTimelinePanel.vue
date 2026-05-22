<template>
  <div class="session-timeline">
    <div class="session-timeline__toolbar row items-center q-gutter-sm q-mb-md">
      <q-select
        :model-value="kindFilter"
        dense
        outlined
        class="session-timeline__filter"
        :options="timelineKindFilterOptions"
        emit-value
        map-options
        label="类型过滤"
        clearable
        @update:model-value="emit('update:kindFilter', $event)"
      />
      <q-select
        :model-value="sortOrder"
        dense
        outlined
        class="session-timeline__filter session-timeline__filter--sort"
        :options="timelineSortOptions"
        emit-value
        map-options
        label="排序"
        @update:model-value="emit('update:sortOrder', $event)"
      />
      <q-space />
      <q-btn flat round icon="refresh" class="sessions-icon-btn" aria-label="刷新" @click="emit('refresh')" />
    </div>

    <div v-if="loading" class="session-timeline__state column items-center q-py-xl">
      <q-spinner color="primary" size="32px" />
      <div class="q-mt-sm sessions-muted">加载 Timeline…</div>
    </div>

    <div v-else-if="error" class="session-timeline__state session-timeline__state--error">
      {{ error }}
    </div>

    <div v-else-if="!timeline?.items.length" class="session-timeline__state">
      <q-icon name="timeline" size="36px" class="sessions-muted" />
      <div class="q-mt-sm sessions-muted">暂无 Timeline 事件</div>
    </div>

    <template v-else>
      <SessionTimelineStats :stats="stats" class="q-mb-lg" />

      <div v-if="!filteredItems.length" class="session-timeline__state">
        <q-icon name="filter_alt_off" size="32px" class="sessions-muted" />
        <div class="q-mt-sm sessions-muted">当前筛选条件下无事件</div>
      </div>

      <div v-else class="session-timeline__rail">
        <SessionTimelineEntry
          v-for="item in filteredItems"
          :key="`${item.kind}:${item.id}`"
          :item="item"
        />
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import type { SessionTimeline, SessionTimelineItem } from "../../features/session/types";
import type { TimelineStat } from "./sessionTimelineUi";
import SessionTimelineEntry from "./SessionTimelineEntry.vue";
import SessionTimelineStats from "./SessionTimelineStats.vue";
import { timelineKindFilterOptions, timelineSortOptions } from "./sessionTimelineUi";

defineProps<{
  sessionId: string;
  sessionTitle?: string;
  focusToolId?: string;
  timeline: SessionTimeline | null;
  loading: boolean;
  error: string;
  kindFilter: string | null;
  sortOrder: string;
  stats: TimelineStat[];
  filteredItems: SessionTimelineItem[];
}>();

defineEmits<{
  refresh: [];
  "update:kindFilter": [value: string | null];
  "update:sortOrder": [value: string];
}>();
</script>
