<template>
  <div class="session-timeline">
    <div class="session-timeline__toolbar row items-center q-gutter-sm q-mb-md">
      <q-select
        v-model="kindFilter"
        dense
        outlined
        class="session-timeline__filter"
        :options="timelineKindFilterOptions"
        emit-value
        map-options
        label="类型过滤"
        clearable
      />
      <q-select
        v-model="sortOrder"
        dense
        outlined
        class="session-timeline__filter session-timeline__filter--sort"
        :options="timelineSortOptions"
        emit-value
        map-options
        label="排序"
      />
      <q-space />
      <q-btn flat round icon="refresh" class="sessions-icon-btn" aria-label="刷新" @click="loadTimeline" />
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
import { computed, onMounted, ref } from "vue";
import type { SessionTimeline } from "../../features/session/api";
import { useSessionStore } from "../../stores/session/index";
import SessionTimelineEntry from "./SessionTimelineEntry.vue";
import SessionTimelineStats from "./SessionTimelineStats.vue";
import {
  buildTimelineStats,
  filterTimelineItems,
  timelineKindFilterOptions,
  timelineSortOptions
} from "./sessionTimelineUi";

const props = defineProps<{
  sessionId: string;
  sessionTitle?: string;
}>();

const sessionStore = useSessionStore();
const timeline = ref<SessionTimeline | null>(null);
const loading = ref(false);
const error = ref("");
const kindFilter = ref<string | null>(null);
const sortOrder = ref("desc");

const stats = computed(() => buildTimelineStats(timeline.value?.summary));

const filteredItems = computed(() =>
  filterTimelineItems(timeline.value?.items ?? [], kindFilter.value, sortOrder.value)
);

async function loadTimeline() {
  loading.value = true;
  error.value = "";
  try {
    timeline.value = await sessionStore.fetchTimeline(props.sessionId);
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载 Timeline 失败";
    timeline.value = null;
  } finally {
    loading.value = false;
  }
}

onMounted(loadTimeline);
</script>
