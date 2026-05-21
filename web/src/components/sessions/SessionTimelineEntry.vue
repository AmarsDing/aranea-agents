<template>
  <article
    class="session-timeline-entry"
    :class="[
      `session-timeline-entry--${accent}`,
      { 'session-timeline-entry--message': isMessage }
    ]"
  >
    <div class="session-timeline-entry__axis" aria-hidden="true">
      <div class="session-timeline-entry__dot">
        <q-icon :name="icon" size="14px" />
      </div>
      <div class="session-timeline-entry__line" />
    </div>

    <div class="session-timeline-entry__card">
      <header class="session-timeline-entry__header">
        <div class="session-timeline-entry__title-row">
          <h3 class="session-timeline-entry__title">{{ item.title }}</h3>
          <span
            v-for="tag in item.tags"
            :key="tag"
            class="session-timeline-entry__tag"
            :class="`session-timeline-entry__tag--${tagKind(tag)}`"
          >
            {{ tag }}
          </span>
        </div>
        <div class="session-timeline-entry__meta">
          <time>{{ formatTimelineTime(item.occurred_at) }}</time>
          <span v-if="item.duration_ms">· {{ formatTimelineDuration(item.duration_ms) }}</span>
          <span v-if="item.status && item.status !== 'ok'" class="session-timeline-entry__status">
            · {{ item.status }}
          </span>
        </div>
      </header>

      <p v-if="inlinePreview" class="session-timeline-entry__preview">{{ inlinePreview }}</p>

      <q-expansion-item
        v-if="showExpansion"
        dense
        switch-toggle-side
        expand-separator
        class="session-timeline-entry__expansion"
        :label="expansionLabel"
        header-class="session-timeline-entry__expansion-header"
      >
        <div v-if="item.actor_name" class="session-timeline-entry__meta-row">
          <span class="session-timeline-entry__meta-key">Actor</span>
          <span class="session-timeline-entry__meta-val">{{ item.actor_name }}</span>
        </div>
        <div v-if="item.subtitle && !isMessage" class="session-timeline-entry__meta-row">
          <span class="session-timeline-entry__meta-key">Source</span>
          <span class="session-timeline-entry__meta-val">{{ item.subtitle }}</span>
        </div>
        <pre v-if="item.content_markdown" class="session-timeline-entry__detail">{{ item.content_markdown }}</pre>
        <pre v-else-if="item.detail_json" class="session-timeline-entry__detail">{{ prettyTimelineJSON(item.detail_json) }}</pre>
        <div v-else class="session-timeline-entry__detail session-timeline-entry__detail--empty">暂无详情</div>
      </q-expansion-item>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { SessionTimelineItem } from "../../features/session/api";
import {
  formatTimelineDuration,
  formatTimelineTime,
  isTimelineMessage,
  prettyTimelineJSON,
  timelineEntryAccent,
  timelineEntryIcon,
  timelineHasDetail
} from "./sessionTimelineUi";

const props = defineProps<{
  item: SessionTimelineItem;
}>();

const accent = computed(() => timelineEntryAccent(props.item));
const icon = computed(() => timelineEntryIcon(props.item));
const isMessage = computed(() => isTimelineMessage(props.item));

const inlinePreview = computed(() => {
  const text = (props.item.preview || props.item.subtitle || "").trim();
  if (!text || !isMessage.value) return "";
  return text.length > 480 ? `${text.slice(0, 480)}…` : text;
});

const showExpansion = computed(() => {
  if (!timelineHasDetail(props.item)) return false;
  if (isMessage.value && props.item.content_markdown) return true;
  if (isMessage.value && props.item.detail_json) return true;
  return !isMessage.value;
});

const expansionLabel = computed(() => {
  if (isMessage.value) return "查看完整内容";
  return props.item.preview || props.item.subtitle || "查看详情";
});

function tagKind(tag: string): string {
  if (tag === "User") return "user";
  if (tag === "Tool") return "tool";
  if (tag === "Skill") return "skill";
  if (tag === "MCP") return "mcp";
  return "default";
}
</script>
