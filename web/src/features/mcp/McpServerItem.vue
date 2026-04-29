<template>
  <q-card flat bordered class="mcp-server-item">
    <q-card-section class="row q-col-gutter-md items-start">
      <div class="col-12 col-md">
        <div class="row items-center q-gutter-sm no-wrap">
          <div class="text-subtitle1 text-weight-bold ellipsis">{{ displayName }}</div>
          <q-chip dense square :color="server.enabled ? 'primary' : 'grey'" text-color="white">
            {{ server.enabled ? "enabled" : "disabled" }}
          </q-chip>
          <q-chip v-if="metadata.health_status" dense outline :color="healthColor">
            {{ metadata.health_status }}
          </q-chip>
        </div>
        <div class="text-caption text-grey-7 ellipsis">{{ server.key }}</div>
        <div class="row q-col-gutter-sm q-mt-sm text-body2">
          <div class="col-12 col-md-3">
            <span class="mcp-detail-label">传输</span>
            <span>{{ transportLabel }}</span>
          </div>
          <div class="col-12 col-md">
            <span class="mcp-detail-label">地址/命令</span>
            <span class="ellipsis inline-block mcp-endpoint">{{ endpointLabel }}</span>
            <q-tooltip v-if="endpointLabel !== '-'">{{ endpointLabel }}</q-tooltip>
          </div>
        </div>
        <div class="row q-col-gutter-sm q-mt-xs text-caption text-grey-8">
          <div class="col-12 col-sm-4">工具前缀：{{ config.tool_prefix || derivedPrefix }}</div>
          <div class="col-12 col-sm-4">超时：{{ config.timeout_sec || 60 }}s</div>
          <div class="col-12 col-sm-4">用户凭据：{{ config.require_user_credentials ? "需要" : "不需要" }}</div>
        </div>
        <div v-if="metadata.last_error_message" class="text-caption text-negative q-mt-xs ellipsis">
          {{ metadata.last_error_message }}
          <q-tooltip>{{ metadata.last_error_message }}</q-tooltip>
        </div>
      </div>

      <div class="col-12 col-md-auto row justify-end items-center q-gutter-xs">
        <q-btn flat dense rounded icon="science" color="primary" label="测试连接" :loading="testing" @click="$emit('test', server)" />
        <q-btn flat dense rounded icon="edit" color="primary" label="编辑" @click="$emit('edit', server)" />
        <q-btn flat dense rounded icon="delete" color="negative" label="删除" @click="$emit('delete', server)" />
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { McpServerConfig, McpServerMetadata, McpServerRow } from "./types";

const props = defineProps<{
  server: McpServerRow;
  testing?: boolean;
}>();

defineEmits<{
  edit: [server: McpServerRow];
  delete: [server: McpServerRow];
  test: [server: McpServerRow];
}>();

const config = computed(() => parseJSON<McpServerConfig>(props.server.config_json, {}));
const metadata = computed(() => parseJSON<McpServerMetadata>(props.server.metadata_json, {}));
const displayName = computed(() => props.server.name || props.server.key);
const derivedPrefix = computed(() => props.server.key.replaceAll("-", "_"));
const transportLabel = computed(() => {
  if (config.value.transport === "streamable_http") return "Streamable HTTP";
  if (config.value.transport === "sse") return "SSE";
  return "stdio";
});
const endpointLabel = computed(() => {
  if (config.value.transport === "stdio") {
    return [config.value.command, ...(config.value.args ?? [])].filter(Boolean).join(" ") || "-";
  }
  return config.value.url || "-";
});
const healthColor = computed(() => {
  if (metadata.value.health_status === "ok") return "positive";
  if (metadata.value.health_status === "error") return "negative";
  if (metadata.value.health_status === "degraded") return "warning";
  return "grey";
});

function parseJSON<T>(value: string | undefined, fallback: T): T {
  if (!value) return fallback;
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
}
</script>

<style scoped>
.mcp-server-item {
  background: rgba(255, 253, 245, 0.84);
  border-radius: 18px;
}

.mcp-detail-label {
  color: #8d6e63;
  font-weight: 700;
  margin-right: 6px;
}

.mcp-endpoint {
  max-width: min(520px, 100%);
  vertical-align: bottom;
}

body.body--dark .mcp-server-item {
  background: rgba(30, 30, 30, 0.72);
}
</style>
