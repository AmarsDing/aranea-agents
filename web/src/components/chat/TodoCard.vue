<template>
  <div class="todo-card" :class="`todo-card--${item.status}`">
    <span class="todo-card__icon" :class="`todo-card__icon--${item.status}`">{{ statusIcon }}</span>
    <div class="todo-card__body">
      <div class="todo-card__content">{{ item.content }}</div>
      <div v-if="item.activeForm" class="todo-card__active-form">{{ item.activeForm }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { TodoItem } from '../../features/chat/agentTreeTypes';

const props = defineProps<{
  item: TodoItem;
}>();

const statusIcon = computed(() => {
  switch (props.item.status) {
    case 'in_progress': return '⚡';
    case 'completed': return '✓';
    default: return '○';
  }
});
</script>

<style scoped lang="sass">
.todo-card
  display: flex
  align-items: flex-start
  gap: 6px
  padding: 6px 8px
  border-radius: 4px
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)
  margin-bottom: 4px
  transition: border-color 0.15s

  &:hover
    border-color: color-mix(in srgb, var(--color-text-primary) 15%, var(--glass-border))

.todo-card--in_progress
  border-color: color-mix(in srgb, var(--color-primary) 30%, var(--glass-border))

.todo-card--completed
  opacity: 0.65

.todo-card__icon
  flex-shrink: 0
  font-size: 12px
  line-height: 18px
  width: 16px
  text-align: center

.todo-card__icon--pending
  color: var(--color-text-tertiary)

.todo-card__icon--in_progress
  color: var(--color-primary)

.todo-card__icon--completed
  color: var(--color-success)

.todo-card__body
  flex: 1
  min-width: 0

.todo-card__content
  font-size: 13px
  line-height: 1.4
  color: var(--color-text-primary)
  word-break: break-word

.todo-card--completed .todo-card__content
  text-decoration: line-through
  color: var(--color-text-tertiary)

.todo-card__active-form
  font-size: 11px
  color: var(--color-text-tertiary)
  margin-top: 2px
  line-height: 1.3
</style>
