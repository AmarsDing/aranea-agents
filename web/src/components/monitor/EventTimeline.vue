<template>
  <q-card flat bordered class="monitor-card">
    <q-card-section class="row items-center q-col-gutter-md">
      <div class="col-12 col-md">
        <div class="row items-center q-gutter-sm">
          <div class="text-h6 text-weight-bold">事件时间线</div>
          <q-badge outline color="primary">{{ filteredEvents.length }}</q-badge>
        </div>
        <div class="text-caption text-grey-7">Envelope 事件流实时追踪</div>
      </div>
      <q-select v-model="typeFilter" dense outlined emit-value map-options class="col-12 col-md-2" label="类型" :options="typeOptions" />
      <q-input v-model="keyword" dense outlined class="col-12 col-md-2" label="搜索" clearable />
      <q-btn flat rounded :icon="paused ? 'play_arrow' : 'pause'" :label="paused ? '恢复' : '暂停'" @click="paused = !paused" />
      <q-btn flat rounded icon="delete_sweep" label="清除" @click="events = []" />
    </q-card-section>
    <q-separator />
    <q-card-section style="max-height: 480px; overflow-y: auto">
      <q-timeline v-if="filteredEvents.length" color="primary">
        <q-timeline-entry
          v-for="ev in filteredEvents"
          :key="ev.id"
          :title="ev.type"
          :subtitle="ev.timestamp"
          :color="eventColor(ev.type)"
          :icon="eventIcon(ev.type)"
        >
          <div class="row q-gutter-xs">
            <q-chip dense outline>{{ ev.channel }}</q-chip>
            <q-chip dense v-if="ev.author" color="grey" text-color="white">{{ ev.author }}</q-chip>
            <q-chip dense v-if="ev.filter_key" color="cyan" text-color="white">{{ ev.filter_key }}</q-chip>
          </div>
          <div v-if="ev.content?.text" class="q-mt-xs text-body2" style="white-space: pre-wrap; max-height: 120px; overflow: hidden">
            {{ ev.content.text.slice(0, 300) }}
          </div>
          <div v-if="ev.tool_call" class="q-mt-xs">
            <q-chip dense color="orange" text-color="white">{{ ev.tool_call.name }}</q-chip>
            <span class="text-caption q-ml-xs">{{ ev.tool_call.status }}</span>
          </div>
          <div v-if="ev.error" class="q-mt-xs text-negative">{{ ev.error.message }}</div>
        </q-timeline-entry>
      </q-timeline>
      <div v-else class="monitor-empty">
        <q-icon name="timeline" size="36px" color="grey-5" />
        <div>暂无事件</div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from "vue";
import { useEnvelopeStream } from "../../features/chat/useEnvelopeStream";
import type { Envelope, EnvelopeType } from "../../features/chat/envelope";

const props = defineProps<{
  sessionId: string;
  channels?: string[];
  global?: boolean;
}>();

const events = ref<Envelope[]>([]);
const paused = ref(false);
const typeFilter = ref<string>("all");
const keyword = ref("");

const stream = useEnvelopeStream({
  sessionId: props.global ? "*" : props.sessionId,
  channels: props.channels ?? ["chat", "team", "graph", "system", "monitor"],
  logEnabled: false,
  autoConnect: true,
});

stream.onType(
  [
    "text_delta",
    "text_done",
    "tool_call",
    "tool_result",
    "state_delta",
    "transfer",
    "runner_completion",
    "error",
    "log",
    "graph_node_start",
    "graph_node_end",
    "checkpoint",
    "intent_pass",
    "member_message_start",
    "member_delta",
    "member_message_done",
    "team_run_started",
    "team_run_finished",
    "team_run_failed",
    "team_step_started",
    "team_step_finished",
  ],
  (env) => {
    if (paused.value) return;
    events.value = [env, ...events.value].slice(0, 2000);
  }
);

const allTypes: EnvelopeType[] = [
  "text_delta",
  "text_done",
  "tool_call",
  "tool_result",
  "state_delta",
  "transfer",
  "runner_completion",
  "error",
  "log",
  "graph_node_start",
  "graph_node_end",
  "checkpoint",
  "intent_pass",
  "member_message_start",
  "member_delta",
  "member_message_done",
  "team_run_started",
  "team_run_finished",
  "team_run_failed",
  "team_step_started",
  "team_step_finished",
];

const typeOptions = computed(() => [
  { label: "全部", value: "all" },
  ...allTypes.map((t) => ({ label: t, value: t })),
]);

const filteredEvents = computed(() => {
  let list = events.value;
  if (typeFilter.value !== "all") {
    list = list.filter((e) => e.type === typeFilter.value);
  }
  if (keyword.value) {
    const kw = keyword.value.toLowerCase();
    list = list.filter(
      (e) =>
        e.type.includes(kw) ||
        e.author?.toLowerCase().includes(kw) ||
        e.content?.text?.toLowerCase().includes(kw) ||
        e.tool_call?.name?.toLowerCase().includes(kw) ||
        e.filter_key?.toLowerCase().includes(kw)
    );
  }
  return list;
});

function eventColor(type: EnvelopeType): string {
  if (type.includes("error") || type.includes("failed")) return "negative";
  if (type.includes("done") || type.includes("finished") || type.includes("completion")) return "positive";
  if (type.includes("tool") || type.includes("step")) return "orange";
  if (type.includes("log")) return "grey";
  if (type.includes("graph") || type.includes("checkpoint")) return "teal";
  if (type.includes("team") || type.includes("member")) return "purple";
  return "primary";
}

function eventIcon(type: EnvelopeType): string {
  if (type.includes("error") || type.includes("failed")) return "error";
  if (type.includes("tool")) return "build";
  if (type.includes("log")) return "article";
  if (type.includes("graph")) return "account_tree";
  if (type.includes("team") || type.includes("member")) return "groups";
  if (type.includes("transfer")) return "swap_horiz";
  if (type.includes("state")) return "tune";
  return "bolt";
}

onBeforeUnmount(() => {
  stream.disconnect();
});
</script>
