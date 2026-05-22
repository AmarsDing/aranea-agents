<template>
  <q-card flat bordered class="monitor-card">
    <q-card-section class="row items-center q-col-gutter-md">
      <div class="col-12 col-md">
        <div class="row items-center q-gutter-sm">
          <div class="text-h6 text-weight-bold">实时事件</div>
          <q-badge :color="streamColor">{{ streamText }}</q-badge>
          <q-badge outline color="primary">{{ visibleEvents.length }} 个事件</q-badge>
        </div>
        <div class="text-caption text-grey-7">Team / Agent 运行时事件与持久化监控事件</div>
      </div>
      <q-select v-model="category" class="app-field-sm" dense outlined emit-value map-options label="分类" :options="categoryOptions" />
      <q-btn flat rounded no-caps :icon="paused ? 'play_arrow' : 'pause'" :label="paused ? '恢复' : '暂停'" @click="toggleStream" />
      <q-btn flat rounded no-caps icon="delete_sweep" label="清除" @click="clearRuntimeEvents" />
    </q-card-section>
    <q-separator />
    <q-card-section>
      <q-list v-if="visibleEvents.length" separator>
        <q-item v-for="event in visibleEvents" :key="event.id" clickable class="monitor-event-item" @click="openDetail(event)">
          <q-item-section avatar>
            <q-avatar :color="eventColor(event.type)" text-color="white" icon="bolt" size="34px" />
          </q-item-section>
          <q-item-section>
            <q-item-label class="text-weight-medium">{{ event.title }}</q-item-label>
            <q-item-label caption lines="2">{{ event.subtitle }}</q-item-label>
            <div class="row q-gutter-xs q-mt-xs">
              <q-chip dense outline>{{ event.source }}</q-chip>
              <q-chip dense :color="eventColor(event.type)" text-color="white">{{ event.type }}</q-chip>
              <q-chip v-if="event.canOpenInRuns" dense outline color="teal">已关联 Runs</q-chip>
            </div>
          </q-item-section>
          <q-item-section side>
            <div class="column items-end q-gutter-xs">
              <span class="text-caption text-grey-7">{{ event.time }}</span>
              <q-btn
                v-if="event.canOpenInRuns && event.completionMeta"
                flat
                dense
                no-caps
                size="sm"
                color="primary"
                label="在 Runs 中查看"
                @click.stop="openLinkedRun(event)"
              />
              <q-btn
                v-else-if="event.completionSessionId"
                flat
                dense
                no-caps
                size="sm"
                color="primary"
                label="打开会话"
                @click.stop="openChatSession(event.completionSessionId!)"
              />
            </div>
          </q-item-section>
        </q-item>
      </q-list>
      <div v-else class="monitor-empty">
        <q-icon name="sensors" size="36px" color="grey-5" />
        <div>{{ emptyHint }}</div>
      </div>
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
import { toRef } from "vue";
import type { PlatformResource, MonitorTraceEvent } from "../../features/monitor/types";
import { useMonitorRealtimeEvents } from "../../features/monitor/useMonitorRealtimeEvents";

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
</script>
