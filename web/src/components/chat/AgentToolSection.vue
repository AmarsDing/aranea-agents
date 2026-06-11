<template>
  <div class="section section--tool">
    <div class="section__label" @click="toggleCollapse">
      <span class="section__label-icon">{{ categoryIcon }}</span>
      <span class="section__label-text" :style="{ color: agentColor }">{{
        section.toolLabel || section.toolName
      }}</span>
      <span
        v-if="section.isLongRunning && section.status === 'running'"
        class="tool-pill tool-pill--long-running"
        :title="t('chat.agentTool.longRunningTitle')"
      >
        {{ t('chat.execution.statusPending') }}
      </span>
      <span v-if="section.status === 'success'" class="tool-status tool-status--success">✓</span>
      <span v-else-if="section.status === 'failed'" class="tool-status tool-status--failed">✗</span>
      <span v-else-if="section.status === 'running'" class="pulse-dot"></span>
      <span v-if="section.durationMs != null" class="section__label-duration">{{ formattedDuration }}</span>
      <span class="section__label-toggle">{{ localCollapsed ? '▶' : '▼' }}</span>
    </div>
    <div class="section__body" :class="{ 'section__body--collapsed': localCollapsed }">
      <div class="section__body-inner">
        <div v-if="section.arguments" class="tool-args">
          <div class="tool-args__label">{{ t('chat.toolArgs') }}</div>
          <pre>{{ section.arguments }}</pre>
        </div>
        <div v-if="section.result" class="tool-result">
          <div class="tool-result__label">{{ t('chat.toolResult') }}</div>
          <pre>{{ section.result }}</pre>
        </div>
        <div v-if="section.error" class="tool-error">
          <div class="tool-result__label">{{ t('chat.agentTool.error') }}</div>
          <pre>{{ section.error }}</pre>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ToolSection } from '../../features/chat/agentTreeTypes';
import { formatDuration } from '../../features/chat/agentTreeUtils';

const { t } = useI18n();

const props = defineProps<{
  section: ToolSection;
  agentColor: string;
}>();

const localCollapsed = ref<boolean>(props.section.collapsed);

function toggleCollapse() {
  localCollapsed.value = !localCollapsed.value;
}

const formattedDuration = computed(() =>
  props.section.durationMs != null ? formatDuration(props.section.durationMs) : '',
);

/**
 * Pick a category glyph for the tool section. Honors the upstream
 * `icon_key` (set by the orchestrator for Skill / MCP / etc.) and falls
 * back to a small prefix-match table by tool name. The default ⚡ is kept
 * for ad-hoc tools that don't declare an icon.
 */
const ICON_BY_KEY: Record<string, string> = {
  file: '📄',
  shell: '🖥️',
  web: '🔍',
  search: '🔍',
  mcp: '🔌',
  skill: '📚',
  agent: '🤖',
  plan: '🗒️',
  spawn: '🧑‍💻',
  write: '✏️',
};
const ICON_BY_NAME: Array<[RegExp, string]> = [
  [/^read_file|^file_read|^fs_read/i, '📄'],
  [/^write_file|^file_write|^fs_write/i, '✏️'],
  [/^shell|^bash|^exec/i, '🖥️'],
  [/^web_|^search|^fetch/i, '🔍'],
  [/^mcp_/i, '🔌'],
  [/^skill_/i, '📚'],
  [/^spawn|^subagent/i, '🧑‍💻'],
  [/^plan|^planner/i, '🗒️'],
];

const categoryIcon = computed(() => {
  const key = props.section.iconKey?.trim();
  if (key && ICON_BY_KEY[key]) return ICON_BY_KEY[key];
  const name = props.section.toolName || '';
  for (const [re, glyph] of ICON_BY_NAME) {
    if (re.test(name)) return glyph;
  }
  return '⚡';
});
</script>

<style scoped lang="sass">
.section
  margin-bottom: 12px

.section__label
  display: flex
  align-items: center
  gap: 5px
  font-size: 12px
  font-weight: 500
  margin-bottom: 4px
  cursor: pointer
  user-select: none
  padding: 2px 0
  border-radius: 4px
  transition: background 0.12s

  &:hover
    background: color-mix(in srgb, var(--color-text-primary) 4%, transparent)

.section__label-icon
  font-size: 14px

.section__label-text
  text-transform: uppercase
  letter-spacing: 0.3px

.section__label-duration
  color: var(--color-text-tertiary)
  font-size: 10px

.section__label-toggle
  margin-left: auto
  color: var(--color-text-tertiary)
  font-size: 10px

.tool-pill
  font-size: 9px
  padding: 1px 6px
  border-radius: 999px
  line-height: 1.4
  font-weight: 500

.tool-pill--long-running
  color: var(--color-text-tertiary)
  background: color-mix(in srgb, var(--color-accent) 14%, transparent)
  border: 1px solid color-mix(in srgb, var(--color-accent) 30%, transparent)

.section__body
  overflow: hidden
  transition: max-height 0.25s ease, opacity 0.15s ease

.section__body--collapsed
  max-height: 0 !important
  opacity: 0

.section__body-inner
  padding: 8px 10px
  background: color-mix(in srgb, var(--color-accent) 4%, transparent)
  border: 1px solid color-mix(in srgb, var(--color-accent) 10%, transparent)
  border-radius: 8px
  font-size: 12px

.tool-status
  font-size: 11px

.tool-status--success
  color: var(--color-success)

.tool-status--failed
  color: var(--color-danger)

.tool-args, .tool-result, .tool-error
  margin-bottom: 6px

.tool-args__label, .tool-result__label
  font-size: 10px
  color: var(--color-text-tertiary)
  text-transform: uppercase
  letter-spacing: 0.3px
  margin-bottom: 3px

.tool-args pre, .tool-result pre, .tool-error pre
  margin: 0
  font-family: 'JetBrains Mono', 'Fira Code', monospace
  font-size: 11px
  line-height: 1.5
  color: var(--color-text-secondary)
  white-space: pre-wrap
  word-break: break-word
  max-height: 120px
  overflow-y: auto

.pulse-dot
  display: inline-block
  width: 6px
  height: 6px
  border-radius: 50%
  background: var(--color-accent)
  animation: pulse 1s infinite
  vertical-align: middle
  margin-left: 4px

@keyframes pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.3
</style>
