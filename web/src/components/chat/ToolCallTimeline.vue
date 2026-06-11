<template>
  <div v-if="sortedEvents.length > 0" class="tool-call-timeline">
    <ToolCallTimelineItem
      v-for="(evt, idx) in sortedEvents"
      :key="evt.id"
      :event="evt"
      :is-last="idx === sortedEvents.length - 1"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, provide, ref } from 'vue';
import type { ToolUseEvent } from '../../features/chat/types';
import { useToolCallTimeline } from '../../features/chat/composables/useToolCallTimeline';
import { EXECUTION_COLLAPSE_CONTROL_KEY } from '../../features/chat/executionCardHelpers';
import ToolCallTimelineItem from './ToolCallTimelineItem.vue';

const props = defineProps<{
  events: ToolUseEvent[];
}>();

const eventsRef = computed(() => props.events);
const { sortedEvents } = useToolCallTimeline(eventsRef);

// ── Provide expandAll / collapseAll control ──
const expandAllSignal = ref(0);
const collapseAllSignal = ref(0);

provide(EXECUTION_COLLAPSE_CONTROL_KEY, {
  expandAllSignal,
  collapseAllSignal,
});

function expandAll() {
  expandAllSignal.value++;
}

function collapseAll() {
  collapseAllSignal.value++;
}

defineExpose({ expandAll, collapseAll });
</script>

<style scoped lang="sass">
.tool-call-timeline
  padding: var(--space-1) 0
</style>
