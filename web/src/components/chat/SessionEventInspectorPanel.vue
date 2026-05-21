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
      <q-spinner color="primary" size="28px" />
    </div>
    <div v-else-if="error" class="text-negative q-pa-md">{{ error }}</div>

    <div v-else class="row col q-col-gutter-md inspector-body">
      <div class="col-12 col-md-4 inspector-branch">
        <div class="text-caption text-weight-bold q-mb-sm">调用树</div>
        <BranchTree
          :nodes="branchTree"
          :selected-id="selectedInvocationId"
          @select="selectedInvocationId = $event"
        />
      </div>
      <div class="col-12 col-md-8 inspector-list">
        <div class="text-caption text-weight-bold q-mb-sm">{{ displayEvents.length }} 个事件</div>
        <q-scroll-area style="height: min(420px, 50vh)">
          <q-list separator dense>
            <q-item v-for="ev in displayEvents" :key="ev.id" class="inspector-event-item">
              <q-item-section>
                <q-item-label class="row items-center q-gutter-xs">
                  <q-chip dense outline size="sm">{{ ev.type }}</q-chip>
                  <span class="text-caption text-grey-7">{{ ev.timestamp }}</span>
                  <q-chip v-if="ev.tool_call?.is_long_running" dense color="orange" text-color="white" size="sm">
                    long-running
                  </q-chip>
                </q-item-label>
                <q-item-label caption>{{ ev.author }} · {{ ev.channel }}</q-item-label>
                <TransferBadge v-if="ev.transfer" :transfer="ev.transfer" class="q-mt-xs" />
                <StateDeltaIndicator v-if="ev.state_delta" :delta="ev.state_delta" class="q-mt-xs" />
                <div v-if="ev.tool_call" class="q-mt-xs text-caption">
                  <q-chip dense size="sm" color="orange" text-color="white">{{ ev.tool_call.name }}</q-chip>
                  {{ ev.tool_call.status }}
                </div>
                <div v-if="ev.content?.text" class="q-mt-xs text-body2 ellipsis-3-lines">{{ ev.content.text }}</div>
              </q-item-section>
            </q-item>
          </q-list>
          <div v-if="!displayEvents.length" class="text-center text-grey-7 q-pa-lg">暂无事件</div>
        </q-scroll-area>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, toRef } from "vue";
import {
  useChatEventInspector,
  type ChatEventInspectorStreamDeps,
} from "../../features/chat/composables/useChatEventInspector";
import type { EventFilterState } from "../../features/chat/eventFilter";
import BranchTree from "./BranchTree.vue";
import EventFilterBar from "./EventFilterBar.vue";
import StateDeltaIndicator from "./StateDeltaIndicator.vue";
import TransferBadge from "./TransferBadge.vue";

const props = defineProps<{
  sessionId: string;
  active: boolean;
  streamDeps?: ChatEventInspectorStreamDeps;
}>();

const sessionRef = toRef(props, "sessionId");
const activeRef = toRef(props, "active");

const {
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
  border-right: 1px solid var(--glass-border, rgb(0 0 0 / 8%));
}

.ellipsis-3-lines {
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
