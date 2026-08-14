<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', $event)">
    <q-card v-if="invocation" class="app-dialog-card app-glass-dialog tool-run-detail-dialog">
      <q-card-section class="row items-center justify-between">
        <div class="min-width-0">
          <div class="text-h6 ellipsis">{{ invocation.tool_display_name || invocation.tool_key }}</div>
          <div class="text-caption muted-caption ellipsis">{{ invocation.tool_key }}</div>
        </div>
        <div class="row items-center q-gutter-sm">
          <q-badge rounded :color="toolInvocationStatusColor(invocation.status)">
            {{ toolInvocationStatusLabel(invocation.status) }}
          </q-badge>
          <q-btn flat dense round icon="close" class="app-dialog-icon-btn" @click="$emit('update:open', false)" />
        </div>
      </q-card-section>
      <q-separator />

      <q-tabs v-model="tab" dense align="left" active-color="primary" indicator-color="primary">
        <q-tab name="params" :label="t('toolsPage.runDetail.tabParams')" />
        <q-tab name="output" :label="t('toolsPage.runDetail.tabOutput')" />
        <q-tab name="error" :label="t('toolsPage.runDetail.tabError')" />
        <q-tab name="context" :label="t('toolsPage.runDetail.tabContext')" />
      </q-tabs>
      <q-separator />

      <q-tab-panels v-model="tab" animated class="tool-run-detail-panels">
        <q-tab-panel name="params">
          <q-banner dense rounded class="tool-run-detail-tip q-mb-sm">
            {{ t('toolsPage.runDetail.redactionTip') }}
          </q-banner>
          <div v-if="paramsLoading" class="row items-center q-gutter-sm">
            <q-spinner-dots size="24px" color="primary" />
            <span class="muted-caption">{{ t('toolsPage.runDetail.loadingParams') }}</span>
          </div>
          <q-banner v-else-if="paramsError" dense rounded class="app-page-error-banner">
            {{ paramsError }}
          </q-banner>
          <template v-else>
            <div v-if="params?.redaction_applied" class="q-mb-sm">
              <q-badge rounded color="warning">{{ t('toolsPage.runDetail.redacted') }}</q-badge>
            </div>
            <pre class="tool-run-detail-json">{{ prettyJSON(params?.params_json || '{}') }}</pre>
            <div class="text-caption muted-caption q-mt-sm">{{ t('toolsPage.runDetail.inputHash') }}{{ invocation.input_hash || '—' }}</div>
          </template>
        </q-tab-panel>

        <q-tab-panel name="output">
          <pre class="tool-run-detail-json">{{ prettyJSON(invocation.output_preview || '{}') }}</pre>
          <div class="text-caption muted-caption q-mt-sm">{{ t('toolsPage.runDetail.outputHash') }}{{ invocation.output_hash || '—' }}</div>
        </q-tab-panel>

        <q-tab-panel name="error">
          <template v-if="invocation.error_code || invocation.error_message">
            <div class="q-mb-sm">
              <q-badge rounded color="negative">{{ invocation.error_code || 'error' }}</q-badge>
            </div>
            <pre class="tool-run-detail-json">{{ invocation.error_message || '—' }}</pre>
          </template>
          <div v-else class="muted-caption">{{ t('toolsPage.runDetail.noError') }}</div>
          <template v-if="metadataPretty">
            <div class="text-subtitle2 q-mt-md q-mb-xs">metadata</div>
            <pre class="tool-run-detail-json">{{ metadataPretty }}</pre>
          </template>
        </q-tab-panel>

        <q-tab-panel name="context">
          <div class="tool-run-detail-kv">
            <div v-for="item in contextItems" :key="item.label" class="tool-run-detail-kv__row">
              <span class="tool-run-detail-kv__label">{{ item.label }}</span>
              <span class="tool-run-detail-kv__value" :title="item.value">{{ item.value || '—' }}</span>
            </div>
          </div>
        </q-tab-panel>
      </q-tab-panels>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ToolInvocation, ToolInvocationParamDetail } from '../../features/tools/types';
import {
  formatInvocationDuration,
  formatInvocationWhen,
  prettyJSON,
  toolInvocationStatusColor,
  toolInvocationStatusLabel,
} from './toolUi';

const props = defineProps<{
  open: boolean;
  invocation: ToolInvocation | null;
  params: ToolInvocationParamDetail | null;
  paramsLoading: boolean;
  paramsError: string;
}>();

defineEmits<{
  'update:open': [value: boolean];
}>();

const { t } = useI18n();
const tab = ref('params');

const metadataPretty = computed(() => {
  const raw = props.invocation?.metadata_json ?? '';
  return raw.trim() ? prettyJSON(raw) : '';
});

const contextItems = computed(() => {
  const r = props.invocation;
  if (!r) return [];
  return [
    { label: 'request_id', value: r.request_id },
    { label: 'invocation_id', value: r.invocation_id },
    { label: 'session_id', value: r.session_id },
    { label: 'message_id', value: r.message_id },
    { label: 'agent', value: r.agent_display_name || r.agent_key || r.agent_id },
    { label: 'user_id', value: r.user_id },
    { label: 'source', value: r.source },
    { label: t('toolsPage.runDetail.startedAt'), value: formatInvocationWhen(r.started_at) },
    { label: t('toolsPage.runDetail.endedAt'), value: formatInvocationWhen(r.ended_at) },
    { label: t('toolsPage.runDetail.duration'), value: formatInvocationDuration(r.duration_ms) },
    { label: 'streaming', value: r.streaming ? t('toolsPage.runDetail.streamingYes', { count: r.chunk_count ?? 0 }) : t('toolsPage.runDetail.streamingNo') },
  ];
});
</script>

<style scoped>
.tool-run-detail-dialog {
  width: 640px;
  max-width: 92vw;
}

.tool-run-detail-panels {
  min-height: 260px;
}

.tool-run-detail-json {
  margin: 0;
  padding: 8px 12px;
  border-radius: 8px;
  background: var(--color-surface-soft);
  border: 1px solid var(--glass-border);
  color: var(--color-text-heading);
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 320px;
  overflow: auto;
}

.tool-run-detail-tip {
  background: var(--color-info-soft);
}

.tool-run-detail-kv {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.tool-run-detail-kv__row {
  display: flex;
  gap: 12px;
  font-size: 13px;
}

.tool-run-detail-kv__label {
  flex: 0 0 120px;
  opacity: 65%;
}

.tool-run-detail-kv__value {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
