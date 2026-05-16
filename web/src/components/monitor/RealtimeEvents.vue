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
      <q-select v-model="category" dense outlined emit-value map-options class="col-12 col-md-2" label="分类" :options="categoryOptions" />
      <q-btn flat rounded :icon="paused ? 'play_arrow' : 'pause'" :label="paused ? '恢复' : '暂停'" @click="toggleStream" />
      <q-btn flat rounded icon="delete_sweep" label="清除" @click="runtimeEvents = []" />
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
            </div>
          </q-item-section>
          <q-item-section side class="text-caption text-grey-7">
            {{ event.time }}
          </q-item-section>
        </q-item>
      </q-list>
      <div v-else class="monitor-empty">
        <q-icon name="sensors" size="36px" color="grey-5" />
        <div>暂无实时事件</div>
      </div>
    </q-card-section>
  </q-card>

  <q-dialog v-model="detailOpen">
    <q-card class="monitor-detail-card">
      <q-card-section class="row items-start justify-between">
        <div>
          <div class="text-h6">事件详情</div>
          <div class="text-caption text-grey-7">{{ selected?.type }}</div>
        </div>
        <q-btn flat round dense icon="close" v-close-popup />
      </q-card-section>
      <q-separator />
      <q-card-section>
        <pre class="monitor-json">{{ selectedJSON }}</pre>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat label="复制 JSON" icon="content_copy" @click="copyJSON" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { copyToClipboard, Notify } from "quasar";
import { subscribeMonitorRuntimeEventsWs } from "../../features/monitor/api";
import type { PlatformResource, StreamState, TeamRunEvent } from "../../features/monitor/types";
import { compactJSON, formatDate, parseJSON } from "../../features/monitor/utils";

type ViewEvent = {
  id: string;
  type: string;
  title: string;
  subtitle: string;
  category: string;
  source: string;
  time: string;
  raw: unknown;
};

const props = defineProps<{
  persistedEvents: PlatformResource[];
}>();

const category = ref("all");
const state = ref<StreamState>("connecting");
const paused = ref(false);
const runtimeEvents = ref<ViewEvent[]>([]);
const selected = ref<ViewEvent | null>(null);
const detailOpen = ref(false);
let wsSub: ReturnType<typeof subscribeMonitorRuntimeEventsWs> | null = null;

const categoryOptions = [
  { label: "全部", value: "all" },
  { label: "任务", value: "task" },
  { label: "消息", value: "message" },
  { label: "Agent", value: "agent" },
  { label: "工具", value: "tool" },
  { label: "系统", value: "system" }
];

const persistedViewEvents = computed<ViewEvent[]>(() =>
  props.persistedEvents.map((row) => {
    const cfg = parseJSON(row.config_json);
    const type = String(cfg.type || row.key || "monitor.event");
    return {
      id: row.id,
      type,
      title: row.name || type,
      subtitle: row.description || JSON.stringify(cfg),
      category: categoryFor(type),
      source: "persisted",
      time: formatDate(row.created_at),
      raw: { ...row, config: cfg, metadata: parseJSON(row.metadata_json) }
    };
  })
);

const visibleEvents = computed(() => {
  const items = [...runtimeEvents.value, ...persistedViewEvents.value];
  if (category.value === "all") return items;
  return items.filter((event) => event.category === category.value);
});

const selectedJSON = computed(() => compactJSON(selected.value?.raw ?? {}));
const streamColor = computed(() => state.value === "live" ? "positive" : state.value === "error" ? "negative" : state.value === "paused" ? "grey" : "orange");
const streamText = computed(() => ({ connecting: "连接中", live: "实时", paused: "已暂停", error: "连接异常" }[state.value]));

onMounted(startStream);
onBeforeUnmount(stopStream);

watch(paused, (isPaused) => {
  if (isPaused) {
    stopStream();
    state.value = "paused";
  } else {
    startStream();
  }
});

function startStream() {
  stopStream();
  state.value = "connecting";
  wsSub = subscribeMonitorRuntimeEventsWs(
    "monitor",
    (event) => {
      state.value = "live";
      runtimeEvents.value = [teamRunEventToView(event), ...runtimeEvents.value].slice(0, 1000);
    },
    () => {
      if (!paused.value) state.value = "error";
    }
  );
}

function stopStream() {
  wsSub?.close();
  wsSub = null;
}

function toggleStream() {
  paused.value = !paused.value;
}

function teamRunEventToView(event: TeamRunEvent): ViewEvent {
  const type = event.type || "runtime.event";
  const step = event.step;
  const run = event.run;
  const payload = event.payload || {};
  let title = step?.agent_name || run?.mode || type;
  let subtitle = step?.error_message || run?.error_message || step?.output_preview || run?.output_preview || "runtime event";
  if (type === "intent_pass") {
    title = "意图 Pass";
    const outcome = String(payload.outcome ?? "");
    const ms = payload.duration_ms;
    subtitle =
      outcome +
      (typeof ms === "number" ? ` · ${ms} ms` : "") +
      (payload.intent_kind ? ` · ${String(payload.intent_kind)}` : "");
  }
  return {
    id: `${type}-${run?.id || step?.id || Date.now()}-${runtimeEvents.value.length}`,
    type,
    title,
    subtitle,
    category: categoryFor(type),
    source: "ws",
    time: formatDate(step?.created_at || run?.updated_at || run?.created_at || new Date().toISOString()),
    raw: event
  };
}

function categoryFor(type: string) {
  if (type === "intent_pass") return "agent";
  if (type.startsWith("run") || type.includes("team_run")) return "task";
  if (type.startsWith("message") || type.startsWith("chat")) return "message";
  if (type.startsWith("agent")) return "agent";
  if (type.startsWith("tool") || type.includes("step")) return "tool";
  return "system";
}

function eventColor(type: string) {
  if (type.includes("failed") || type.includes("error")) return "negative";
  if (type.includes("finished") || type.includes("completed")) return "positive";
  if (type === "intent_pass") return "cyan";
  if (type.includes("tool") || type.includes("step")) return "orange";
  if (type.includes("agent")) return "cyan";
  return "primary";
}

function openDetail(event: ViewEvent) {
  selected.value = event;
  detailOpen.value = true;
}

async function copyJSON() {
  await copyToClipboard(selectedJSON.value);
  Notify.create({ message: "已复制", color: "positive", position: "top" });
}
</script>
