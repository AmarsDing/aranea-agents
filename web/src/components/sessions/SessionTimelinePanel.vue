<template>
  <div>
    <div class="row items-center q-gutter-sm q-mb-md">
      <q-select
        v-model="kindFilter"
        dense outlined
        :options="kindOptions"
        emit-value map-options
        label="类型过滤"
        style="min-width: 140px"
        clearable
      />
      <q-select
        v-model="sortOrder"
        dense outlined
        :options="sortOptions"
        emit-value map-options
        label="排序"
        style="min-width: 120px"
      />
      <q-btn flat round icon="refresh" @click="loadTimeline" />
    </div>

    <div v-if="loading" class="column items-center q-py-lg">
      <q-spinner color="primary" size="32px" />
    </div>
    <div v-else-if="error" class="text-negative q-pa-md">{{ error }}</div>
    <div v-else-if="!timeline?.items.length" class="text-grey-7 q-pa-md">暂无 Timeline 事件</div>
    <template v-else>
      <div class="row q-col-gutter-sm q-mb-md">
        <div v-for="stat in stats" :key="stat.label" class="col-6 col-sm-3">
          <div class="timeline-stat">
            <div class="timeline-stat__value">{{ stat.value }}</div>
            <div class="timeline-stat__label">{{ stat.label }}</div>
          </div>
        </div>
      </div>

      <q-timeline color="primary" layout="comfortable">
        <q-timeline-entry
          v-for="item in timeline.items"
          :key="`${item.kind}:${item.id}`"
          :side="item.side === 'right' ? 'right' : 'left'"
          :icon="entryIcon(item)"
          :color="entryColor(item)"
        >
          <template #title>
            <div class="row items-center no-wrap q-gutter-xs">
              <span class="ellipsis" style="max-width: 260px">{{ item.title }}</span>
              <q-badge v-for="tag in item.tags" :key="tag" rounded :color="tagColor(tag)" style="font-size: 10px">{{ tag }}</q-badge>
            </div>
          </template>
          <template #subtitle>
            <span>{{ formatTime(item.occurred_at) }}</span>
            <span v-if="item.duration_ms" class="q-ml-sm">耗时 {{ formatDuration(item.duration_ms) }}</span>
          </template>
          <q-expansion-item dense switch-toggle-side expand-separator :label="item.preview || item.subtitle || '查看详情'">
            <div v-if="item.actor_name" class="text-caption text-grey-7 q-mb-xs">Actor: {{ item.actor_name }}</div>
            <pre v-if="item.content_markdown" class="timeline-detail">{{ item.content_markdown }}</pre>
            <pre v-else-if="item.detail_json" class="timeline-detail">{{ prettyJSON(item.detail_json) }}</pre>
          </q-expansion-item>
        </q-timeline-entry>
      </q-timeline>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import type { SessionTimeline, SessionTimelineItem } from "../../features/session/api";
import { useSessionStore } from "../../stores/session/index";

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

const kindOptions = [
  { label: "全部", value: "" },
  { label: "消息", value: "message" },
  { label: "工具", value: "tool" },
  { label: "技能", value: "skill" },
  { label: "MCP", value: "mcp" }
];

const sortOptions = [
  { label: "最新优先", value: "desc" },
  { label: "最早优先", value: "asc" }
];

const stats = computed(() => {
  const s = timeline.value?.summary;
  return [
    { label: "Messages", value: s?.message_count ?? 0 },
    { label: "Tools", value: s?.tool_count ?? 0 },
    { label: "Skills", value: s?.skill_count ?? 0 },
    { label: "MCP", value: s?.mcp_count ?? 0 }
  ];
});

watch([kindFilter, sortOrder], loadTimeline);

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

function entryIcon(item: SessionTimelineItem) {
  if (item.kind === "tool") return "build";
  if (item.kind === "skill") return "auto_awesome";
  if (item.kind === "mcp") return "hub";
  if (item.tags.includes("User")) return "person";
  return "smart_toy";
}

function entryColor(item: SessionTimelineItem) {
  if (/fail|error/i.test(item.status)) return "negative";
  if (item.kind === "skill") return "deep-purple";
  if (item.kind === "mcp") return "teal";
  if (item.kind === "tool") return "info";
  if (item.tags.includes("User")) return "grey-7";
  return "primary";
}

function tagColor(tag: string) {
  if (tag === "Skill") return "deep-purple";
  if (tag === "MCP") return "teal";
  if (tag === "Tool") return "info";
  if (tag === "User") return "grey-7";
  return "primary";
}

function formatTime(value: string) {
  if (!value) return "—";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

function formatDuration(ms: number) {
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

function prettyJSON(value: string) {
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return value;
  }
}

onMounted(loadTimeline);
</script>

<style scoped>
.timeline-stat {
  border: 1px solid rgba(100, 116, 139, 0.22);
  border-radius: 12px;
  background: var(--color-surface, #fff);
  padding: 10px 14px;
}

.timeline-stat__value {
  font-size: 20px;
  font-weight: 850;
  color: var(--color-text-primary);
}

.timeline-stat__label {
  font-size: 11px;
  font-weight: 700;
  color: var(--color-text-secondary, #64748b);
  text-transform: uppercase;
}

.timeline-detail {
  max-height: 400px;
  overflow: auto;
  border-radius: 10px;
  border: 1px solid rgba(148, 163, 184, 0.24);
  background: var(--color-surface-soft, #f1f5f9);
  color: var(--color-text-primary);
  padding: 12px;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 13px;
}

:global(.body--dark) .timeline-stat {
  border-color: rgba(148, 163, 184, 0.18);
  background: rgba(15, 23, 42, 0.6);
}

:global(.body--dark) .timeline-detail {
  border-color: rgba(148, 163, 184, 0.18);
  background: var(--color-surface-dark, #0f172a);
  color: var(--color-text-primary-dark, #e2e8f0);
}
</style>
