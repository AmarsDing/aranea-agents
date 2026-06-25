<template>
  <div class="session-event-inspector column no-wrap">
    <div class="row items-center q-col-gutter-sm q-mb-md">
      <EventFilterBar :filters="filters" @update:filters="onFiltersUpdate" />
      <div class="col-auto row q-gutter-xs">
        <q-btn flat dense :icon="paused ? 'play_arrow' : 'pause'" @click="paused = !paused" />
        <q-btn flat dense icon="refresh" @click="reload()" />
        <q-btn flat dense icon="delete_sweep" @click="clearEvents()" />
      </div>
    </div>

    <div v-if="loading" class="column items-center q-py-lg">
      <q-spinner color="accent" size="28px" />
    </div>
    <div v-else-if="error" class="text-negative q-pa-md">{{ error }}</div>

    <div v-else class="row col q-col-gutter-md inspector-body">
      <div class="col-12 col-md-4 inspector-branch">
        <div class="text-caption text-weight-bold q-mb-sm">调用树</div>
        <BranchTree :nodes="branchTree" :selected-id="selectedInvocationId" @select="selectedInvocationId = $event" />
      </div>
      <div class="col-12 col-md-8 inspector-list">
        <div class="text-caption text-weight-bold q-mb-sm">
          历史 Activity: {{ activities.length }} · 实时 Envelope: {{ displayEvents.length }}
        </div>
        <q-scroll-area style="height: min(420px, 50vh)">
          <div v-if="activities.length" class="q-mb-md">
            <div class="text-caption text-weight-bold q-mb-xs">历史 Activity</div>
            <q-list separator dense>
              <q-item v-for="act in activities" :key="act.id" class="inspector-event-item">
                <q-item-section>
                  <q-item-label class="row items-center q-gutter-xs">
                    <q-chip dense outline size="sm">{{ act.kind }}</q-chip>
                    <q-chip
                      v-if="act.status"
                      dense
                      :color="activityStatusColor(act.status)"
                      text-color="white"
                      size="sm"
                    >
                      {{ act.status }}
                    </q-chip>
                    <span class="text-caption text-grey">{{ act.timestamp }}</span>
                  </q-item-label>
                  <q-item-label v-if="act.agentName" caption>{{ act.agentName }}</q-item-label>
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
          <div v-if="displayEvents.length">
            <div class="text-caption text-weight-bold q-mb-xs">实时 Envelope</div>
            <q-list separator dense>
              <q-item v-for="ev in displayEvents" :key="ev.id" class="inspector-event-item">
                <q-item-section>
                  <q-item-label class="row items-center q-gutter-xs">
                    <q-chip dense outline size="sm">{{ ev.type }}</q-chip>
                    <span class="text-caption text-grey">{{ ev.timestamp }}</span>
                    <q-chip v-if="ev.tool_call?.is_long_running" dense color="warning" text-color="white" size="sm">
                      long-running
                    </q-chip>
                  </q-item-label>
                  <q-item-label caption>{{ ev.author }} · {{ ev.channel }}</q-item-label>
                  <TransferBadge v-if="ev.transfer" :transfer="ev.transfer" class="q-mt-xs" />
                  <StateDeltaIndicator v-if="ev.state_delta" :delta="ev.state_delta" class="q-mt-xs" />
                  <div v-if="ev.tool_call" class="q-mt-xs text-caption">
                    <q-chip dense size="sm" color="warning" text-color="white">{{ ev.tool_call.name }}</q-chip>
                    {{ ev.tool_call.status }}
                  </div>
                  <div v-if="ev.content?.text" class="q-mt-xs text-body2 ellipsis-3-lines">{{ ev.content.text }}</div>
                </q-item-section>
              </q-item>
            </q-list>
          </div>
          <div v-if="!displayEvents.length && !activities.length" class="text-center text-grey q-pa-lg">暂无事件</div>
        </q-scroll-area>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, toRef } from 'vue';
import {
  useChatEventInspector,
  type ChatEventInspectorStreamDeps,
} from '../../features/chat/composables/useChatEventInspector';
import type { EventFilterState } from '../../features/chat/eventFilter';
import type { ActivityStatus } from '../../features/chat/activityTypes';
import BranchTree from './BranchTree.vue';
import EventFilterBar from './EventFilterBar.vue';
import StateDeltaIndicator from './StateDeltaIndicator.vue';
import TransferBadge from './TransferBadge.vue';

const props = defineProps<{
  sessionId: string;
  active: boolean;
  streamDeps?: ChatEventInspectorStreamDeps;
}>();

const sessionRef = toRef(props, 'sessionId');
const activeRef = toRef(props, 'active');

const {
  activities,
  paused,
  loading,
  error,
  filters,
  filteredEvents,
  branchTree,
  selectedInvocationId,
  clearEvents,
  reload,
} = useChatEventInspector(sessionRef, activeRef, props.streamDeps);

const displayEvents = computed(() => {
  const sel = selectedInvocationId.value;
  if (!sel) return filteredEvents.value;
  return filteredEvents.value.filter((e) => e.invocation_id === sel || e.parent_invocation_id === sel);
});

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

function onFiltersUpdate(next: EventFilterState): void {
  filters.value = { ...next };
}

defineExpose({ reload, clearEvents });
</script>

<style scoped>
.inspector-body {
  min-height: 280px;
}

.inspector-branch {
  border-right: 1px solid var(--glass-border);
}

.ellipsis-3-lines {
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
