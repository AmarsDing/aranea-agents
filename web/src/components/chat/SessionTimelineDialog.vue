<template>
  <q-dialog v-model="dialogOpen" transition-show="scale" transition-hide="scale">
    <q-card class="session-trace">
      <q-card-section class="session-trace__header row items-start justify-between no-wrap">
        <div>
          <div class="session-trace__eyebrow">Session Trace</div>
          <div class="session-trace__title">{{ sessionTitle || "Untitled session" }}</div>
          <div class="session-trace__subtitle">
            {{ timeline?.summary.total ?? 0 }} events · conversation + tools + skills + MCP
          </div>
        </div>
        <q-btn flat round icon="close" aria-label="Close" @click="dialogOpen = false" />
      </q-card-section>

      <q-separator />

      <q-card-section class="session-trace__stats row q-col-gutter-sm">
        <div v-for="stat in stats" :key="stat.label" class="col-6 col-sm-3">
          <div class="session-trace-stat">
            <div class="session-trace-stat__value">{{ stat.value }}</div>
            <div class="session-trace-stat__label">{{ stat.label }}</div>
          </div>
        </div>
      </q-card-section>

      <q-separator />

      <q-card-section class="session-trace__body">
        <div v-if="loading" class="session-trace__empty column items-center justify-center">
          <q-spinner color="primary" size="32px" />
          <div class="q-mt-sm">Loading session trace...</div>
        </div>
        <div v-else-if="error" class="session-trace__empty text-negative">
          {{ error }}
        </div>
        <div v-else-if="!timeline?.items.length" class="session-trace__empty">
          No trace events yet.
        </div>
        <q-timeline v-else color="primary" layout="comfortable" class="session-trace-timeline">
          <q-timeline-entry
            v-for="item in timeline.items"
            :key="`${item.kind}:${item.id}`"
            :side="item.side === 'right' ? 'right' : 'left'"
            :icon="entryIcon(item)"
            :color="entryColor(item)"
          >
            <template #title>
              <div class="row items-center no-wrap q-gutter-xs">
                <span class="session-trace-entry__title ellipsis">{{ item.title }}</span>
                <q-badge
                  v-for="tag in item.tags"
                  :key="tag"
                  rounded
                  :color="tagColor(tag)"
                  class="session-trace-entry__tag"
                >
                  {{ tag }}
                </q-badge>
              </div>
            </template>
            <template #subtitle>
              <span>{{ formatTime(item.occurred_at) }}</span>
              <span v-if="item.duration_ms" class="q-ml-sm">耗时 {{ formatDuration(item.duration_ms) }}</span>
              <span v-if="item.status" class="q-ml-sm">状态 {{ item.status }}</span>
            </template>

            <q-expansion-item
              dense
              switch-toggle-side
              expand-separator
              class="session-trace-entry"
              :label="item.preview || item.subtitle || '查看详情'"
            >
              <div class="session-trace-entry__meta row q-col-gutter-sm">
                <div v-if="item.actor_name" class="col-12 col-sm-6">
                  <span>Actor</span>
                  <strong>{{ item.actor_name }}</strong>
                </div>
                <div v-if="item.subtitle" class="col-12 col-sm-6">
                  <span>Source</span>
                  <strong>{{ item.subtitle }}</strong>
                </div>
              </div>
              <pre v-if="item.content_markdown" class="session-trace-entry__detail">{{ item.content_markdown }}</pre>
              <pre v-else-if="item.detail_json" class="session-trace-entry__detail">{{ prettyJSON(item.detail_json) }}</pre>
              <div v-else class="session-trace-entry__detail session-trace-entry__detail--empty">
                No detail payload.
              </div>
            </q-expansion-item>
          </q-timeline-entry>
        </q-timeline>
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { SessionTimeline, SessionTimelineItem } from "../../features/session/api";
import { useSessionStore } from "../../stores/session/index";

const props = defineProps<{
  modelValue: boolean;
  sessionId?: string | null;
  sessionTitle?: string;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

const sessionStore = useSessionStore();
const timeline = ref<SessionTimeline | null>(null);
const loading = ref(false);
const error = ref("");

const dialogOpen = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit("update:modelValue", value)
});

const stats = computed(() => {
  const summary = timeline.value?.summary;
  return [
    { label: "Messages", value: summary?.message_count ?? 0 },
    { label: "Tools", value: summary?.tool_count ?? 0 },
    { label: "Skills", value: summary?.skill_count ?? 0 },
    { label: "MCP", value: summary?.mcp_count ?? 0 }
  ];
});

watch(
  () => [props.modelValue, props.sessionId] as const,
  async ([open, sessionId]) => {
    if (!open || !sessionId) return;
    loading.value = true;
    error.value = "";
    try {
      timeline.value = await sessionStore.fetchTimeline(sessionId);
    } catch (err) {
      error.value = err instanceof Error ? err.message : "Failed to load session trace.";
      timeline.value = null;
    } finally {
      loading.value = false;
    }
  },
  { immediate: true }
);

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
  if (tag === "Team") return "orange";
  return "primary";
}

function formatTime(value: string) {
  if (!value) return "unknown time";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat([], {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit"
  }).format(date);
}

function formatDuration(value: number) {
  if (value < 1000) return `${value}ms`;
  return `${(value / 1000).toFixed(2)}s`;
}

function prettyJSON(value: string) {
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return value;
  }
}
</script>

<style scoped>
.session-trace {
  width: 1200px;
  max-height: 88vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-radius: 24px;
  background: var(--color-surface-soft);
  color: var(--color-surface-solid);
  box-shadow: 0 32px 90px rgb(15 23 42 / 22%);
}

.session-trace__header {
  padding: 22px 24px 16px;
}

.session-trace__eyebrow {
  color: var(--color-text-tertiary);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.session-trace__title {
  margin-top: 4px;
  color: var(--color-surface-solid);
  font-size: 22px;
  font-weight: 850;
  line-height: 1.35;
}

.session-trace__subtitle {
  margin-top: 4px;
  color: var(--color-text-tertiary);
  font-size: 13px;
}

.session-trace__stats {
  flex: 0 0 auto;
  padding: 14px 20px;
}

.session-trace-stat {
  border: 1px solid rgb(100 116 139 / 22%);
  border-radius: 16px;
  background: var(--color-on-accent);
  padding: 14px;
  box-shadow: 0 10px 28px rgb(15 23 42 / 6%);
}

.session-trace-stat__value {
  color: var(--color-surface-solid);
  font-size: 22px;
  font-weight: 850;
}

.session-trace-stat__label {
  color: var(--color-text-tertiary);
  font-size: 12px;
  font-weight: 700;
}

.session-trace__body {
  flex: 1 1 auto;
  min-height: 360px;
  overflow: auto;
  padding: 24px 22px 28px;
}

.session-trace__empty {
  min-height: 280px;
  color: var(--color-text-tertiary);
  text-align: center;
}

.session-trace-timeline {
  max-width: 100%;
  margin: 0 auto;
}

.session-trace-entry {
  border: 1px solid rgb(100 116 139 / 20%);
  border-radius: 16px;
  background: var(--color-on-accent);
  box-shadow: 0 12px 30px rgb(15 23 42 / 8%);
  overflow: hidden;
}

.session-trace-entry__title {
  max-width: 220px;
  color: var(--color-surface-soft);
  font-weight: 850;
}

.session-trace-entry__tag {
  font-size: 10px;
  font-weight: 800;
}

.session-trace-entry :deep(.q-item__label) {
  color: var(--color-text-slate-700);
  font-weight: 650;
  line-height: 1.55;
}

.session-trace-entry__meta {
  padding: 12px 16px 0;
  color: var(--color-text-tertiary);
  font-size: 12px;
}

.session-trace-entry__meta span {
  display: block;
  font-weight: 800;
  text-transform: uppercase;
}

.session-trace-entry__meta strong {
  color: var(--color-surface-solid);
}

.session-trace-entry__detail {
  max-height: 460px;
  margin: 12px 16px 16px;
  overflow: auto;
  border-radius: 12px;
  border: 1px solid rgb(148 163 184 / 24%);
  background: var(--color-page-tint-cool);
  color: var(--color-surface-solid);
  padding: 14px;
  white-space: pre-wrap;
  overflow-wrap: break-word;
}

.session-trace-entry__detail--empty {
  background: rgb(148 163 184 / 12%);
  color: var(--color-text-tertiary);
}

.session-trace-timeline :deep(.q-timeline__title) {
  color: var(--color-surface-soft);
}

.session-trace-timeline :deep(.q-timeline__subtitle) {
  color: var(--color-text-slate-600);
  font-weight: 650;
  opacity: 100%;
}

.session-trace-timeline :deep(.q-timeline__content) {
  color: var(--color-text-slate-700);
}

:global(.body--dark) .session-trace {
  background: var(--canvas-base);
  color: var(--color-text-dark);
  box-shadow: 0 32px 90px rgb(0 0 0 / 52%);
}

:global(.body--dark) .session-trace__title,
:global(.body--dark) .session-trace-stat__value,
:global(.body--dark) .session-trace-entry__title {
  color: var(--color-surface-soft);
}

:global(.body--dark) .session-trace__subtitle,
:global(.body--dark) .session-trace__eyebrow,
:global(.body--dark) .session-trace-stat__label,
:global(.body--dark) .session-trace__empty,
:global(.body--dark) .session-trace-entry__meta {
  color: rgb(203 213 225 / 76%);
}

:global(.body--dark) .session-trace-stat,
:global(.body--dark) .session-trace-entry {
  border-color: rgb(148 163 184 / 18%);
  background: rgb(15 23 42 / 88%);
}

:global(.body--dark) .session-trace-entry :deep(.q-item__label) {
  color: rgb(226 232 240 / 92%);
}

:global(.body--dark) .session-trace-entry__detail {
  border-color: rgb(148 163 184 / 18%);
  background: var(--color-surface-solid);
  color: var(--color-text-dark);
}

:global(.body--dark) .session-trace-entry__meta strong {
  color: var(--color-surface-soft);
}

:global(.body--dark) .session-trace-timeline :deep(.q-timeline__title) {
  color: var(--color-surface-soft);
}

:global(.body--dark) .session-trace-timeline :deep(.q-timeline__subtitle) {
  color: rgb(203 213 225 / 82%);
  opacity: 100%;
}

:global(.body--dark) .session-trace-timeline :deep(.q-timeline__content) {
  color: rgb(226 232 240 / 90%);
}
</style>
