<template>
  <div class="section section--thinking">
    <div class="section__label" @click="toggleCollapse">
      <span class="section__label-icon">🧠</span>
      <span class="section__label-text">{{ t('chat.thinking.summary') }}</span>
      <span v-if="section.streaming" class="pulse-dot"></span>
      <span v-if="section.durationMs" class="section__label-duration">{{ formattedDuration }}</span>
      <span class="section__label-toggle">{{ localCollapsed ? '▶' : '▼' }}</span>
    </div>
    <div class="section__body" :class="{ 'section__body--collapsed': localCollapsed }">
      <div class="section__body-inner" :class="{ 'section__body-inner--streaming': section.streaming }">
        <div v-html="renderedContent"></div>
        <span v-if="section.streaming" class="cursor-blink"></span>
      </div>
    </div>
    <div v-if="localCollapsed" class="section__preview">{{ previewText }}</div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ThinkingSection } from '../../features/chat/agentTreeTypes';
import { formatDuration } from '../../features/chat/agentTreeUtils';
import { renderChatMarkdown } from '../../features/chat/chatMessageMarkdown';

const { t } = useI18n();

const props = defineProps<{
  section: ThinkingSection;
}>();

const emit = defineEmits<{
  'streaming-finished': [sectionId: string];
}>();

const localCollapsed = ref<boolean>(props.section.collapsed);

// Auto-collapse when streaming finishes
watch(
  () => props.section.streaming,
  (isStreaming, wasStreaming) => {
    if (wasStreaming && !isStreaming) {
      // Delay auto-collapse to let user see the content briefly
      setTimeout(() => {
        localCollapsed.value = true;
      }, 500);
      emit('streaming-finished', props.section.id);
    }
  },
);

function toggleCollapse() {
  localCollapsed.value = !localCollapsed.value;
}

const formattedDuration = computed(() => formatDuration(props.section.durationMs));

const renderedContent = computed(() => renderChatMarkdown(props.section.content));

const previewText = computed(() => {
  const content = props.section.content || '';
  const firstLine = content.split('\n').find((l) => l.trim() !== '') || '';
  return firstLine.length > 80 ? firstLine.slice(0, 80) + '…' : firstLine;
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
  color: var(--color-text-tertiary)

.section__label-duration
  color: var(--color-text-tertiary)
  font-size: 10px

.section__label-toggle
  margin-left: auto
  color: var(--color-text-tertiary)
  font-size: 10px

.section__body
  overflow: hidden
  transition: max-height 0.25s ease, opacity 0.15s ease

.section__body--collapsed
  max-height: 0 !important
  opacity: 0

.section__body-inner
  padding: 8px 10px
  background: color-mix(in srgb, var(--color-text-primary) 3%, transparent)
  border: 1px solid var(--glass-border)
  border-radius: 8px
  font-size: 12px
  color: var(--color-text-secondary)
  line-height: 1.6
  max-height: 200px
  overflow-y: auto

.section__body-inner--streaming
  border-color: color-mix(in srgb, var(--color-accent) 20%, transparent)

.section__preview
  font-size: 12px
  color: var(--color-text-tertiary)
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
  max-width: 100%

.pulse-dot
  display: inline-block
  width: 6px
  height: 6px
  border-radius: 50%
  background: var(--color-accent)
  animation: pulse 1s infinite
  vertical-align: middle
  margin-left: 4px

.cursor-blink
  display: inline-block
  width: 2px
  height: 14px
  background: var(--color-accent)
  animation: blink 0.8s infinite
  vertical-align: middle
  margin-left: 2px

@keyframes pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.3

@keyframes blink
  0%, 100%
    opacity: 1
  50%
    opacity: 0
</style>
