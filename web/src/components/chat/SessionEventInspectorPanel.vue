<template>
  <div class="session-event-inspector column no-wrap">
    <div class="row items-center q-col-gutter-sm q-mb-md">
      <div class="col-auto row q-gutter-xs">
        <q-btn flat dense :icon="paused ? 'play_arrow' : 'pause'" @click="paused = !paused" />
        <q-btn flat dense icon="refresh" @click="reload()" />
        <q-btn flat dense icon="delete_sweep" @click="clearEvents()" />
      </div>
      <div class="col text-right text-caption text-grey">
        历史: {{ activities.length }} · 实时: {{ liveActivities.length }}
      </div>
    </div>

    <div v-if="loading" class="column items-center q-py-lg">
      <q-spinner color="accent" size="28px" />
    </div>
    <div v-else-if="error" class="text-negative q-pa-md">{{ error }}</div>

    <div v-else class="col inspector-body">
      <q-scroll-area class="inspector-body__scroll">
        <!-- 历史 Activity -->
        <div v-if="activities.length" class="q-mb-md">
          <div class="text-caption text-weight-bold q-mb-xs">历史 Activity</div>
          <q-list separator dense>
            <q-item v-for="act in activities" :key="`hist-${act.id}`" class="inspector-event-item">
              <q-item-section>
                <q-item-label class="row items-center q-gutter-xs">
                  <q-chip dense outline size="sm">{{ act.kind }}</q-chip>
                  <q-chip v-if="act.status" dense :color="activityStatusColor(act.status)" text-color="white" size="sm">
                    {{ act.status }}
                  </q-chip>
                  <span v-if="act.agentName" class="text-caption text-grey">{{ act.agentName }}</span>
                  <span class="text-caption text-grey">{{ formatTime(act.timestamp) }}</span>
                </q-item-label>
                <div v-if="act.toolName" class="q-mt-xs text-caption">
                  <q-chip dense size="sm" color="warning" text-color="white">{{ act.toolName }}</q-chip>
                  <span v-if="act.toolDurationMs"> · {{ act.toolDurationMs }}ms</span>
                </div>
                <div v-if="act.content" class="q-mt-xs text-body2 ellipsis-3-lines">{{ act.content }}</div>
                <div v-else-if="act.reasoning" class="q-mt-xs text-body2 ellipsis-3-lines text-grey">
                  {{ act.reasoning }}
                </div>
              </q-item-section>
            </q-item>
          </q-list>
        </div>

        <!-- 实时 Activity -->
        <div v-if="liveActivities.length">
          <div class="text-caption text-weight-bold q-mb-xs">实时 Activity</div>
          <q-list separator dense>
            <q-item v-for="act in liveActivities" :key="`live-${act.id}`" class="inspector-event-item">
              <q-item-section>
                <q-item-label class="row items-center q-gutter-xs">
                  <q-chip dense outline size="sm">{{ act.kind }}</q-chip>
                  <q-chip v-if="act.status" dense :color="activityStatusColor(act.status)" text-color="white" size="sm">
                    {{ act.status }}
                  </q-chip>
                  <span v-if="act.agentName" class="text-caption text-grey">{{ act.agentName }}</span>
                  <span class="text-caption text-grey">{{ formatTime(act.timestamp) }}</span>
                </q-item-label>
                <div v-if="act.toolName" class="q-mt-xs text-caption">
                  <q-chip dense size="sm" color="warning" text-color="white">{{ act.toolName }}</q-chip>
                  <span v-if="act.toolDurationMs"> · {{ act.toolDurationMs }}ms</span>
                </div>
                <div v-if="act.content" class="q-mt-xs text-body2 ellipsis-3-lines">{{ act.content }}</div>
                <div v-else-if="act.reasoning" class="q-mt-xs text-body2 ellipsis-3-lines text-grey">
                  {{ act.reasoning }}
                </div>
              </q-item-section>
            </q-item>
          </q-list>
        </div>

        <div v-if="!activities.length && !liveActivities.length" class="text-center text-grey q-pa-lg">暂无活动</div>
      </q-scroll-area>
    </div>
  </div>
</template>

<script setup lang="ts">
import { toRef } from 'vue';
import {
  useChatEventInspector,
  type ChatEventInspectorStreamDeps,
} from '../../features/chat/composables/useChatEventInspector';
import type { ActivityStatus } from '../../features/chat/activityTypes';

const props = defineProps<{
  sessionId: string;
  active: boolean;
  streamDeps?: ChatEventInspectorStreamDeps;
}>();

const sessionRef = toRef(props, 'sessionId');
const activeRef = toRef(props, 'active');

const { activities, liveActivities, paused, loading, error, clearEvents, reload } = useChatEventInspector(
  sessionRef,
  activeRef,
  props.streamDeps,
);

function activityStatusColor(status: ActivityStatus): string {
  switch (status) {
    case 'completed':
      return 'positive';
    case 'failed':
    case 'partial_failure':
      return 'negative';
    case 'running':
    case 'tool_running':
      return 'info';
    case 'tool_blocked':
    case 'interrupted':
      return 'warning';
    case 'cancelled':
      return 'grey';
    default:
      return 'grey';
  }
}

function formatTime(ts: string): string {
  if (!ts) return '';
  // Show only HH:MM:SS for compactness; full timestamp available on hover via title.
  const parts = ts.split('T');
  if (parts.length < 2) return ts;
  return parts[1].split('.')[0] ?? parts[1];
}

defineExpose({ reload, clearEvents });
</script>

<style scoped>
.inspector-body {
  min-height: 280px;
}

.inspector-body__scroll {
  height: min(520px, 60vh);
}

.ellipsis-3-lines {
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
