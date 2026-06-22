<template>
  <div class="todo-card" :class="`todo-card--${item.status}`">
    <q-icon :name="statusIconName" size="14px" :color="statusIconColor" class="todo-card__icon" />
    <span class="todo-card__content" :class="`todo-card__content--${item.status}`">{{ item.content }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { TodoItem } from '../../features/chat/agentTreeTypes';

const props = defineProps<{
  item: TodoItem;
}>();

const statusIconName = computed(() => {
  switch (props.item.status) {
    case 'in_progress':
      return 'bolt';
    case 'completed':
      return 'check_circle';
    default:
      return 'radio_button_unchecked';
  }
});

const statusIconColor = computed(() => {
  switch (props.item.status) {
    case 'in_progress':
      return 'accent';
    case 'completed':
      return 'positive';
    default:
      return 'grey';
  }
});
</script>

<style scoped lang="sass">
.todo-card
  display: flex
  align-items: center
  gap: 8px
  padding: 5px 10px
  border-radius: 6px
  background: transparent
  margin-bottom: 2px
  transition: background 0.15s ease

  &:hover
    background: color-mix(in srgb, var(--glass-surface-hover) 50%, transparent)

.todo-card--in_progress
  background: color-mix(in srgb, var(--color-accent) 6%, transparent)

  &:hover
    background: color-mix(in srgb, var(--color-accent) 10%, transparent)

.todo-card--completed
  opacity: 0.6

.todo-card__icon
  flex-shrink: 0

.todo-card__content
  font-size: 13px
  line-height: 1.45
  color: var(--color-text-primary)
  word-break: break-word

.todo-card__content--completed
  color: var(--color-text-secondary)
  text-decoration: line-through
  text-decoration-color: color-mix(in srgb, var(--color-text-secondary) 40%, transparent)
</style>
