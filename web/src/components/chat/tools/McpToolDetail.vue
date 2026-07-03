<template>
  <div class="tool-detail">
    <div v-if="server" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.mcpServer') }}</div>
      <code class="tool-detail__inline">{{ server }}</code>
    </div>
    <div v-if="method" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.method') }}</div>
      <code class="tool-detail__inline">{{ method }}</code>
    </div>
    <div v-if="step.ToolArgs != null" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.args') }}</div>
      <pre class="tool-detail__code">{{ formatToolData(step.ToolArgs) }}</pre>
    </div>
    <div v-if="step.ToolResult != null" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.result') }}</div>
      <pre class="tool-detail__code">{{ formatToolData(step.ToolResult) }}</pre>
    </div>
    <div v-if="step.ToolErrorCode" class="tool-detail__row tool-detail__row--error">
      <div class="tool-detail__label">{{ t('chat.toolDetail.error') }}</div>
      <pre class="tool-detail__code">{{ step.ToolErrorCode }}</pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Step } from '../../../features/chat/v2Types';
import { formatToolData, asRecord, asString } from './toolDetailShared';

const { t } = useI18n();

const props = defineProps<{ step: Step }>();

const parsedArgs = computed(() => asRecord(props.step.ToolArgs));

const server = computed(
  () =>
    asString(parsedArgs.value?.server) ??
    asString(parsedArgs.value?.server_name) ??
    asString(parsedArgs.value?.mcp_server) ??
    '',
);
const method = computed(() => asString(parsedArgs.value?.method) ?? asString(parsedArgs.value?.tool_name) ?? '');
</script>

<style lang="sass" scoped>
.tool-detail
  &__row
    margin-bottom: 6px
    &--error
      .tool-detail__code
        border-color: var(--color-danger)

  &__label
    font-size: 11px
    color: var(--color-text-secondary)
    margin-bottom: 2px

  &__inline
    font-size: 12px
    background: var(--glass-surface)
    border: 1px solid var(--glass-border)
    border-radius: 4px
    padding: 1px 6px
    word-break: break-all

  &__code
    font-size: 12px
    background: var(--glass-surface)
    border: 1px solid var(--glass-border)
    border-radius: 6px
    padding: 6px 8px
    overflow-x: auto
    max-height: 240px
    overflow-y: auto
    margin: 0
    white-space: pre-wrap
    word-break: break-word
</style>
