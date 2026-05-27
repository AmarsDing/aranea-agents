<template>
  <q-card flat bordered class="monitor-card">
    <q-card-section class="q-pb-sm">
      <div class="row items-center q-gutter-sm">
        <div class="text-h6 text-weight-bold">实时事件</div>
        <q-badge :color="streamColor">{{ streamText }}</q-badge>
        <q-badge outline color="primary">{{ visibleEvents.length }} 个事件</q-badge>
      </div>
      <div class="text-caption text-grey-7">Team / Agent 运行时事件与持久化监控事件</div>
    </q-card-section>

    <AppPageToolbar class="monitor-events-toolbar">
      <q-select v-model="category" class="app-page-toolbar__field" dense outlined emit-value map-options label="分类" :options="categoryOptions" />
      <template #actions>
        <q-btn flat rounded no-caps :icon="paused ? 'play_arrow' : 'pause'" :label="paused ? '恢复' : '暂停'" @click="toggleStream" />
        <q-btn flat rounded no-caps icon="delete_sweep" label="清除" @click="clearRuntimeEvents" />
      </template>
    </AppPageToolbar>

    <template v-if="visibleEvents.length">
      <div class="app-registry-table-shell">
        <AppRegistryTable
          :shell="false"
          table-class="monitor-events-table"
          :rows="pagedEvents"
          :columns="MONITOR_EVENTS_TABLE_COLUMNS"
          row-key="id"
          hide-pagination
          :pagination="{ rowsPerPage: 0 }"
        >
        <template #body-cell-title="props">
          <q-td :props="props" class="cursor-pointer" @click="openDetail(props.row)">
            <AppRegistryHoverTip :text="props.row.subtitle">
              <div class="app-registry-cell-primary ellipsis">{{ props.row.title }}</div>
            </AppRegistryHoverTip>
          </q-td>
        </template>
        <template #body-cell-tags="props">
          <q-td :props="props">
            <div class="app-registry-chip-wrap">
              <q-chip dense outline>{{ props.row.source }}</q-chip>
              <q-chip dense :color="eventColor(props.row.type)" text-color="white">{{ props.row.type }}</q-chip>
            </div>
          </q-td>
        </template>
        <template #body-cell-time="props">
          <q-td :props="props">
            <span class="app-registry-cell-sub">{{ props.row.time }}</span>
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="app-registry-cell-actions">
              <q-btn
                v-if="props.row.canOpenInRuns && props.row.completionMeta"
                flat
                dense
                round
                icon="timeline"
                color="primary"
                aria-label="在 Runs 中查看"
                @click="openLinkedRun(props.row)"
              >
                <q-tooltip>在 Runs 中查看</q-tooltip>
              </q-btn>
              <q-btn
                v-else-if="props.row.completionSessionId"
                flat
                dense
                round
                icon="chat"
                color="primary"
                aria-label="打开会话"
                @click="openChatSession(props.row.completionSessionId!)"
              >
                <q-tooltip>打开会话</q-tooltip>
              </q-btn>
              <q-btn flat dense round icon="visibility" color="primary" aria-label="查看详情" @click="openDetail(props.row)" />
            </div>
          </q-td>
        </template>
      </AppRegistryTable>

        <AppRegistryPagination
          v-model:page="page"
          v-model:page-size="pageSize"
          :page-max="pageMax"
          :total="visibleEvents.length"
          label="条事件"
          :page-size-options="[12, 24, 48]"
        />
      </div>
    </template>

    <q-card-section v-else class="monitor-empty">
      <q-icon name="sensors" size="36px" color="grey-5" />
      <div>{{ emptyHint }}</div>
    </q-card-section>
  </q-card>

  <q-dialog v-model="detailOpen">
    <q-card class="monitor-detail-card app-dialog-card app-dialog-card--lg">
      <q-card-section class="row items-start justify-between">
        <div>
          <div class="text-h6">事件详情</div>
          <div class="text-caption text-grey-7">{{ selected?.type }}</div>
        </div>
        <q-btn flat round dense icon="close" v-close-popup />
      </q-card-section>
      <q-separator />
      <q-card-section>
        <pre class="monitor-json app-code-block">{{ selectedJSON }}</pre>
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn flat no-caps label="复制 JSON" icon="content_copy" @click="copyJSON" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, toRef, watch } from "vue";
import AppPageToolbar from "../layout/AppPageToolbar.vue";
import AppRegistryTable from "../layout/AppRegistryTable.vue";
import AppRegistryHoverTip from "../layout/AppRegistryHoverTip.vue";
import AppRegistryPagination from "../layout/AppRegistryPagination.vue";

import type { PlatformResource, MonitorTraceEvent } from "../../features/monitor/types";
import { useMonitorRealtimeEvents, type MonitorViewEvent } from "../../features/monitor/useMonitorRealtimeEvents";
import { MONITOR_EVENTS_TABLE_COLUMNS } from "./monitorTableUi";

const props = defineProps<{
  persistedEvents: PlatformResource[];
  traces?: MonitorTraceEvent[];
}>();

const {
  category,
  categoryOptions,
  paused,
  selected,
  detailOpen,
  visibleEvents,
  selectedJSON,
  emptyHint,
  streamText,
  streamColor,
  toggleStream,
  clearRuntimeEvents,
  openDetail,
  openLinkedRun,
  openChatSession,
  copyJSON,
  eventColor
} = useMonitorRealtimeEvents(toRef(() => props.persistedEvents), toRef(() => props.traces));

const page = ref(1);
const pageSize = ref(12);
const pageMax = computed(() => Math.max(1, Math.ceil(visibleEvents.value.length / pageSize.value)));
const pagedEvents = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return visibleEvents.value.slice(start, start + pageSize.value);
});

watch([category, visibleEvents], () => {
  page.value = 1;
});
</script>

<style scoped>
.monitor-events-toolbar {
  padding: 0 16px 8px;
  border-bottom: none;
}
</style>
