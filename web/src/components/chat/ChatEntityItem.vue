<template>
  <q-item
    clickable
    :active="active"
    active-class="app-sidebar-item--active"
    class="chat-entity-item rounded-borders q-mb-sm"
    :class="{ 'chat-entity-item--active': active }"
    @click="$emit('click')"
  >
    <q-item-section side class="chat-status-icon">
      <q-icon :name="statusIcon" :color="statusColor" size="xs" dense />
    </q-item-section>
    <q-item-section class="chat-entity-main">
      <q-item-label class="chat-entity-name" lines="1">
        {{ name }}
      </q-item-label>
      <q-item-label caption class="chat-entity-meta">
        <span class="chat-status-pill" :class="statusPillClass">
          {{ statusLabel }}
        </span>
      </q-item-label>
    </q-item-section>
    <q-item-section side class="chat-entity-actions">
      <div class="chat-action-stack entity-actions">
        <q-btn
          dense
          round
          flat
          size="sm"
          icon="settings"
          class="chat-action-btn"
          :aria-label="settingsAriaLabel"
          @click.stop="$emit('settings')"
        />
        <q-btn
          dense
          round
          flat
          size="sm"
          color="negative"
          icon="delete"
          class="chat-action-btn chat-danger-btn"
          :aria-label="deleteAriaLabel"
          @click.stop="$emit('delete')"
        />
      </div>
    </q-item-section>
  </q-item>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{
  name: string;
  active: boolean;
  statusIcon: string;
  statusColor: string;
  statusLabel: string;
  settingsAriaLabel?: string;
  deleteAriaLabel?: string;
}>();

defineEmits<{
  click: [];
  settings: [];
  delete: [];
}>();

const statusPillClass = computed(() => {
  if (props.statusIcon === 'bolt') return 'is-working';
  if (props.statusColor === 'grey') return 'is-inactive';
  return 'is-idle';
});
</script>

<style scoped>
.chat-entity-item {
  align-items: center;
  min-height: 56px;
  padding: var(--space-2);
  color: var(--color-text-primary);
  overflow: hidden;
}

.chat-entity-name {
  display: block;
  max-width: 100%;
  overflow: hidden;
  color: inherit;
  font-size: var(--text-base);
  font-weight: 600;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chat-entity-item--active,
:global(.body--dark) .chat-entity-item--active {
  color: var(--chat-text-active, var(--color-on-accent));
}

.chat-status-icon {
  min-width: 22px;
  padding-right: var(--space-1);
}

.chat-entity-main {
  min-width: 0;
  flex: 1 1 auto;
  padding-right: var(--space-1);
}

.chat-entity-actions {
  flex: 0 0 auto;
  min-width: 54px;
  padding-left: 2px;
}

.chat-action-stack {
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

.entity-actions {
  opacity: 0;
  transition: opacity 0.2s;
}

.q-item:hover .entity-actions {
  opacity: 1;
}

.chat-entity-item--active .entity-actions {
  opacity: 1;
}

.chat-action-btn {
  width: 24px;
  height: 24px;
  min-height: 24px;
  border-radius: 10px;
  background: var(--glass-elevated);
}

:global(.body--dark) .chat-action-btn {
  color: var(--color-text-primary);
  background: var(--glass-surface-hover);
}

.chat-entity-item--active .chat-action-btn {
  color: var(--color-on-accent);
  background: var(--glass-surface);
}

.chat-entity-meta {
  margin-top: 3px;
}

.chat-status-pill {
  display: inline-flex;
  align-items: center;
  min-height: 20px;
  padding: 2px var(--space-2);
  border: 1px solid var(--glass-border);
  border-radius: 999px;
  background: var(--glass-elevated);
  color: var(--color-text-secondary);
  font-size: var(--text-xs);
  font-weight: 800;
  line-height: 1.2;
  letter-spacing: 0.02em;
}

.chat-status-pill.is-working {
  border-color: color-mix(in srgb, var(--color-danger) 28%, transparent);
  background: var(--color-danger-soft);
  color: var(--color-danger-text);
}

.chat-status-pill.is-idle {
  border-color: color-mix(in srgb, var(--color-success) 28%, transparent);
  background: color-mix(in srgb, var(--color-success) 10%, var(--glass-surface));
  color: var(--color-accent-green);
}

.chat-status-pill.is-inactive {
  border-color: color-mix(in srgb, var(--color-text-secondary) 24%, transparent);
  background: color-mix(in srgb, var(--glass-surface) 96%, transparent);
  color: var(--color-text-secondary);
}

:global(.body--dark) .chat-status-pill.is-working {
  border-color: color-mix(in srgb, var(--color-danger) 42%, transparent);
  background: color-mix(in srgb, var(--color-danger) 20%, var(--glass-surface));
  color: var(--color-danger-text);
}

:global(.body--dark) .chat-status-pill.is-idle {
  border-color: color-mix(in srgb, var(--color-success) 45%, transparent);
  background: color-mix(in srgb, var(--color-success) 18%, var(--glass-surface));
  color: var(--color-accent-green);
}

:global(.body--dark) .chat-status-pill.is-inactive {
  border-color: var(--glass-border);
  background: color-mix(in srgb, var(--glass-surface) 92%, transparent);
  color: var(--color-text-primary);
}

.chat-entity-item--active .chat-status-pill.is-idle {
  border-color: color-mix(in srgb, var(--color-success) 45%, transparent);
  background: color-mix(in srgb, var(--color-accent-green) 72%, var(--canvas-base));
  color: var(--color-on-accent);
}

.chat-entity-item--active .chat-status-pill.is-working {
  border-color: color-mix(in srgb, var(--color-danger) 45%, transparent);
  background: color-mix(in srgb, var(--color-danger) 55%, var(--canvas-base));
  color: var(--color-on-accent);
}

.chat-entity-item--active .chat-status-pill.is-inactive {
  border-color: color-mix(in srgb, var(--color-text-secondary) 35%, transparent);
  background: color-mix(in srgb, var(--glass-surface-hover) 88%, var(--canvas-base));
  color: var(--color-on-accent);
}

:global(.body--dark) .chat-entity-item--active .chat-status-pill.is-idle {
  border-color: color-mix(in srgb, var(--color-success) 55%, transparent);
  background: color-mix(in srgb, var(--color-success) 32%, var(--canvas-base));
  color: var(--color-text-primary);
}

:global(.body--dark) .chat-entity-item--active .chat-status-pill.is-working {
  border-color: color-mix(in srgb, var(--color-danger) 55%, transparent);
  background: color-mix(in srgb, var(--color-danger) 35%, var(--canvas-base));
  color: var(--color-text-primary);
}

:global(.body--dark) .chat-entity-item--active .chat-status-pill.is-inactive {
  border-color: var(--glass-border-hover);
  background: color-mix(in srgb, var(--glass-surface-hover) 90%, var(--canvas-base));
  color: var(--color-text-primary);
}
</style>
