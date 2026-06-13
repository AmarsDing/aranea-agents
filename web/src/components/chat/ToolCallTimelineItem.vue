<template>
  <div class="tool-timeline-item row no-wrap" :class="{ 'tool-timeline-item--last': isLast }">
    <!-- 左侧时间线轨道 -->
    <div class="tool-timeline-item__track col-auto">
      <span class="tool-timeline-item__time">{{ node.timestamp }}</span>
      <div
        class="tool-timeline-item__dot"
        :class="{ 'tool-timeline-item__dot--pulse': node.statusPoint.animated }"
        :style="{ backgroundColor: node.statusPoint.color }"
      />
      <div v-if="!isLast" class="tool-timeline-item__line" />
    </div>

    <!-- 右侧内容区 -->
    <div class="tool-timeline-item__content col">
      <!-- 折叠态：单行 -->
      <div
        v-if="collapsed"
        class="tool-timeline-item__header row items-center no-wrap q-gutter-xs cursor-pointer"
        @click="toggle"
      >
        <q-icon :name="node.statusPoint.icon" :style="{ color: node.statusPoint.color }" size="16px" />
        <span class="tool-timeline-item__name text-weight-medium ellipsis">{{ node.summary }}</span>
        <span v-if="node.durationLabel" class="text-caption" style="color: var(--color-text-tertiary)">
          {{ node.durationLabel }}
        </span>
        <q-icon name="expand_more" size="14px" style="color: var(--color-text-tertiary)" />
      </div>

      <!-- 展开态：完整详情 -->
      <template v-else>
        <div
          class="tool-timeline-item__header row items-center no-wrap q-gutter-xs cursor-pointer"
          @click="toggle"
        >
          <q-icon :name="node.statusPoint.icon" :style="{ color: node.statusPoint.color }" size="16px" />
          <span class="tool-timeline-item__name text-weight-medium ellipsis">{{ resolveDisplayLabel(event) }}</span>
          <span v-if="node.durationLabel" class="text-caption" style="color: var(--color-text-tertiary)">
            {{ node.durationLabel }}
          </span>
          <q-icon name="expand_less" size="14px" style="color: var(--color-text-tertiary)" />
        </div>

        <div class="tool-timeline-item__body">
          <!-- Stuck 提示 -->
          <div v-if="node.isStuck" class="tool-timeline-item__stuck row items-center q-gutter-xs">
            <q-icon name="error" class="tool-timeline-item__error-icon" size="14px" />
            <span class="text-caption tool-timeline-item__error-text">{{ t('chat.activity.stuckTool', '工具无返回结果') }}</span>
          </div>

          <!-- todo_write: render as inline task cards -->
          <TodoInlineList v-if="isTodoWriteTool(event.tool_name)" :event="event" />

          <!-- Other tools: original args/result display -->
          <template v-else>
            <!-- 参数 -->
            <div v-if="node.argsPreview" class="tool-timeline-item__detail">
              <div class="text-caption text-weight-medium q-mb-xs" style="color: var(--color-text-tertiary)">
                {{ t('chat.toolArgs', '参数') }}
              </div>
              <pre class="tool-timeline-item__pre tool-timeline-item__pre--compact">{{ node.argsPreview }}</pre>
            </div>

            <!-- 结果 -->
            <div v-if="node.resultPreview" class="tool-timeline-item__detail">
              <div class="text-caption text-weight-medium q-mb-xs" style="color: var(--color-text-tertiary)">
                {{ t('chat.toolResult', '结果') }}
              </div>
              <pre class="tool-timeline-item__pre tool-timeline-item__pre--compact">{{ node.resultPreview }}</pre>
            </div>

            <!-- 错误 -->
            <div v-if="node.errorText" role="alert" class="tool-timeline-item__detail">
              <div class="text-caption text-weight-medium q-mb-xs" style="color: var(--color-text-tertiary)">
                {{ t('chat.activity.failed', '失败') }}
              </div>
              <pre class="tool-timeline-item__pre tool-timeline-item__pre--error">{{ node.i18nKey ? t(node.i18nKey) : node.errorText }}</pre>
            </div>
          </template>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ToolUseEvent } from '../../features/chat/types';
import type { ToolCallTimelineNode } from '../../features/chat/timelineTypes';
import { buildTimelineNode } from '../../features/chat/composables/useToolCallTimeline';
import { isTodoWriteTool, resolveDisplayLabel } from '../../features/chat/activityPresentation';
import { EXECUTION_COLLAPSE_CONTROL_KEY } from '../../features/chat/executionCardHelpers';
import TodoInlineList from './TodoInlineList.vue';

const props = defineProps<{
  event: ToolUseEvent;
  isLast: boolean;
}>();

const { t } = useI18n();

const node = computed<ToolCallTimelineNode>(() => buildTimelineNode(props.event));

const isRunning = computed(() => props.event.status === 'running');
const expanded = ref(isRunning.value);

// Running items are always expanded
watch(isRunning, (running) => {
  if (running) expanded.value = true;
});

const collapsed = computed(() => !expanded.value);

function toggle() {
  if (isRunning.value) return;
  expanded.value = !expanded.value;
}

// ── Provide/Inject global collapse control ──
const collapseControl = inject(EXECUTION_COLLAPSE_CONTROL_KEY, null);

if (collapseControl) {
  watch(
    () => collapseControl.expandAllSignal.value,
    () => {
      expanded.value = true;
    },
  );
  watch(
    () => collapseControl.collapseAllSignal.value,
    () => {
      // Running items are immune to collapseAll
      if (!isRunning.value) {
        expanded.value = false;
      }
    },
  );
}
</script>

<style scoped lang="sass">
.tool-timeline-item
  position: relative

.tool-timeline-item__track
  display: flex
  flex-direction: column
  align-items: center
  width: 56px
  flex-shrink: 0
  padding-top: 2px

.tool-timeline-item__time
  font-size: 11px
  color: var(--color-text-tertiary)
  line-height: 1
  margin-bottom: 4px

.tool-timeline-item__dot
  width: 8px
  height: 8px
  border-radius: 50%
  flex-shrink: 0

.tool-timeline-item__dot--pulse
  animation: tl-pulse 1s ease-in-out infinite

.tool-timeline-item__line
  width: 2px
  flex: 1
  background: var(--glass-border)
  margin-top: 4px

.tool-timeline-item__content
  padding-bottom: var(--space-3)
  padding-left: var(--space-2)
  min-width: 0

.tool-timeline-item--last .tool-timeline-item__content
  padding-bottom: 0

.tool-timeline-item__header
  min-height: 24px

.tool-timeline-item__name
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
  max-width: 260px

.tool-timeline-item__body
  margin-top: var(--space-1)
  padding: var(--space-2)
  border-radius: 8px
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent)
  border: 1px solid color-mix(in srgb, var(--glass-border) 40%, transparent)
  animation: tl-expand 0.2s ease

.tool-timeline-item__stuck
  margin-bottom: var(--space-1)

.tool-timeline-item__error-icon
  color: var(--color-danger)

.tool-timeline-item__error-text
  color: var(--color-danger)

.tool-timeline-item__detail
  font-size: var(--text-xs)
  & + &
    margin-top: var(--space-2)

.tool-timeline-item__pre
  margin: 0
  font-size: var(--text-xs)
  line-height: 1.5
  white-space: pre-wrap
  word-break: break-word
  max-height: 280px
  overflow: auto

.tool-timeline-item__pre--compact
  max-height: 160px
  font-size: 11px
  line-height: 1.4

.tool-timeline-item__pre--error
  color: var(--color-danger)

@keyframes tl-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.3

@keyframes tl-expand
  from
    opacity: 0
    max-height: 0
  to
    opacity: 1
    max-height: 500px
</style>
