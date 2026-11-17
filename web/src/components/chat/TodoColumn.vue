<template>
  <div class="todo-column" :class="{ 'todo-column--pulse': pulsing }">
    <div class="todo-column__header">
      <span class="todo-column__dot" :class="`todo-column__dot--${columnKey}`" />
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
import type { TodoColumnKey, TodoItem } from '../../features/chat/agentTreeTypes';
import {
  computeTodoFingerprint,
  readLastFingerprint,
  writeLastFingerprint,
} from '../../features/chat/composables/todoColumnFingerprint';
import TodoCard from './TodoCard.vue';

const props = defineProps<{
  title: string;
  items: TodoItem[];
  columnKey: TodoColumnKey;
}>();

const pulsing = ref(false);
let pulseTimer: ReturnType<typeof setTimeout> | null = null;

function triggerPulse(): void {
  pulsing.value = true;
  if (pulseTimer) clearTimeout(pulseTimer);
  pulseTimer = setTimeout(() => {
    pulsing.value = false;
    pulseTimer = null;
  }, 800);
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
  if (pulseTimer) {
    clearTimeout(pulseTimer);
    pulseTimer = null;
  }
});
</script>

<style scoped lang="sass">
.todo-column
  flex: 1
  min-width: 0
  border-radius: 8px
  background: color-mix(in srgb, var(--glass-surface) 40%, transparent)
  border: 1px solid color-mix(in srgb, var(--glass-border) 60%, transparent)
  overflow: hidden
  transition: box-shadow 0.3s ease, border-color 0.3s ease

.todo-column--pulse
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-accent) 25%, transparent)
  border-color: color-mix(in srgb, var(--color-accent) 20%, transparent)

.todo-column__header
  display: flex
  align-items: center
  gap: 6px
  padding: 8px 10px
  border-bottom: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent)

.todo-column__dot
  width: 8px
  height: 8px
  border-radius: 50%
  flex-shrink: 0

// 列色：pending 弱化、进行中强调、已完成成功色
.todo-column__dot--pending
  background: var(--color-text-secondary)

.todo-column__dot--in_progress
  background: var(--color-accent)

.todo-column__dot--completed
  background: var(--color-success)

.todo-column__title
  font-size: 11px
  font-weight: 600
  color: var(--color-text-secondary)
  flex: 1
  text-transform: uppercase
  letter-spacing: 0.3px

.todo-column__count
  font-size: 10px
  font-weight: 600
  color: var(--color-text-secondary)
  background: color-mix(in srgb, var(--color-text-secondary) 10%, transparent)
  border-radius: 8px
  padding: 1px 6px
  line-height: 16px

.todo-column__body
  padding: 6px 4px

.todo-column__empty
  text-align: center
  color: var(--color-text-secondary)
  font-size: 12px
  padding: var(--nt-space-3) 0
  opacity: 0.5
</style>
