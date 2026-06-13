<template>
  <div class="todo-column" :class="{ 'todo-column--pulse': pulsing }">
    <div class="todo-column__header">
      <span v-if="color" class="todo-column__dot" :style="{ background: color }" />
      <span class="todo-column__title">{{ title }}</span>
      <span class="todo-column__count">{{ items.length }}</span>
    </div>
    <div class="todo-column__body">
      <TodoCard v-for="item in items" :key="item.id" :item="item" />
      <div v-if="items.length === 0" class="todo-column__empty">—</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import type { TodoItem } from '../../features/chat/agentTreeTypes';
import { computeTodoFingerprint, readLastFingerprint, writeLastFingerprint } from '../../features/chat/composables/todoColumnFingerprint';
import TodoCard from './TodoCard.vue';

const props = withDefaults(defineProps<{
  title: string;
  items: TodoItem[];
  color?: string;
  columnKey: string;
}>(), {
  color: undefined,
});

const pulsing = ref(false);
let pulseTimer: ReturnType<typeof setTimeout> | null = null;

function triggerPulse(): void {
  pulsing.value = true;
  if (pulseTimer) clearTimeout(pulseTimer);
  pulseTimer = setTimeout(() => { pulsing.value = false; pulseTimer = null; }, 800);
}

watch(
  () => computeTodoFingerprint(props.items),
  (fp) => {
    if (props.columnKey) writeLastFingerprint(props.columnKey, fp);
    triggerPulse();
  },
);

onMounted(() => {
  if (!props.columnKey) return;
  const last = readLastFingerprint(props.columnKey);
  const current = computeTodoFingerprint(props.items);
  writeLastFingerprint(props.columnKey, current);
  if (last !== undefined && last !== current) triggerPulse();
});

onBeforeUnmount(() => {
  if (pulseTimer) { clearTimeout(pulseTimer); pulseTimer = null; }
});
</script>

<style scoped lang="sass">
.todo-column
  flex: 1
  min-width: 0
  border-radius: 6px
  background: color-mix(in srgb, var(--glass-surface) 60%, transparent)
  border: 1px solid var(--glass-border)
  overflow: hidden
  transition: box-shadow 0.3s ease

.todo-column--pulse
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-primary) 25%, transparent)

.todo-column__header
  display: flex
  align-items: center
  gap: 6px
  padding: 6px 10px
  border-bottom: 1px solid var(--glass-border)
  background: color-mix(in srgb, var(--glass-surface) 80%, transparent)

.todo-column__dot
  width: 8px
  height: 8px
  border-radius: 50%
  flex-shrink: 0

.todo-column__title
  font-size: 12px
  font-weight: 600
  color: var(--color-text-secondary)
  flex: 1

.todo-column__count
  font-size: 11px
  color: var(--color-text-tertiary)
  background: color-mix(in srgb, var(--color-text-primary) 8%, transparent)
  border-radius: 8px
  padding: 0 6px
  line-height: 18px

.todo-column__body
  padding: 6px

.todo-column__empty
  text-align: center
  color: var(--color-text-tertiary)
  font-size: 12px
  padding: 8px 0
</style>
