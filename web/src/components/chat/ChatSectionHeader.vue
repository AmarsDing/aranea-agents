<template>
  <div class="chat-section-header" @click="$emit('update:collapsed', !collapsed)">
    <q-icon :name="icon" size="14px" />
    <span class="chat-section-header__label">{{ label }}</span>
    <q-badge v-if="count > 0" rounded color="primary" :label="count" />
    <q-icon
      name="expand_more"
      size="16px"
      class="chat-section-header__arrow"
      :class="{ 'chat-section-header__arrow--collapsed': collapsed }"
    />
  </div>
</template>

<script setup lang="ts">
defineProps<{
  icon: string;
  label: string;
  count: number;
  collapsed: boolean;
}>();

defineEmits<{
  'update:collapsed': [value: boolean];
}>();
</script>

<style scoped>
.chat-section-header {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-height: 28px;
  padding: 0 var(--space-2);
  color: var(--color-text-secondary);
  font-size: var(--text-xs);
  font-weight: 800;
  letter-spacing: 0.06em;
  cursor: pointer;
  user-select: none;
}

.chat-section-header:hover {
  color: var(--color-text-primary);
}

.chat-section-header__label {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chat-section-header__arrow {
  transition: transform 0.2s ease;
}

.chat-section-header__arrow--collapsed {
  transform: rotate(-90deg);
}
</style>
