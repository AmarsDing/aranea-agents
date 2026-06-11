<template>
  <div class="plan-card" :class="`plan-card--${plan.status}`">
    <div class="plan-card__header" @click="toggleCollapse">
      <div class="plan-card__header-icon">
        <q-spinner v-if="plan.status === 'planning' || plan.status === 'executing'" size="16px" color="accent" />
        <q-icon v-else-if="plan.status === 'completed'" name="check_circle" size="16px" color="positive" />
        <q-icon v-else name="error" size="16px" color="negative" />
      </div>
      <span class="plan-card__title">{{ t('chat.planCard.title') }}</span>
      <span class="plan-card__count">{{ completedCount }}/{{ plan.entries.length }}</span>
      <span class="plan-card__toggle">{{ localCollapsed ? '▶' : '▼' }}</span>
    </div>

    <div v-if="!localCollapsed" class="plan-card__entries">
      <div v-for="entry in plan.entries" :key="entry.id" class="plan-entry" :class="`plan-entry--${entry.status}`">
        <div class="plan-entry__indicator">
          <q-spinner v-if="entry.status === 'running'" size="12px" color="accent" />
          <q-icon v-else-if="entry.status === 'completed'" name="check_circle" size="12px" color="positive" />
          <q-icon v-else-if="entry.status === 'failed'" name="cancel" size="12px" color="negative" />
          <div v-else class="plan-entry__dot" />
        </div>
        <div class="plan-entry__content">
          <div class="plan-entry__task">{{ entry.task }}</div>
          <div v-if="entry.agentName" class="plan-entry__agent">
            <span class="plan-entry__agent-icon" :style="{ background: entry.agentColor || 'var(--color-accent)' }">
              {{ entry.agentIcon || entry.agentName.charAt(0) }}
            </span>
            <span class="plan-entry__agent-name" :style="entry.agentColor ? { color: entry.agentColor } : undefined">{{
              entry.agentName
            }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { OrchestrationPlan } from '../../features/chat/agentTreeTypes';

const { t } = useI18n();

const props = defineProps<{
  plan: OrchestrationPlan;
}>();

const localCollapsed = ref(props.plan.status === 'completed');

// Auto-collapse when plan completes
watch(
  () => props.plan.status,
  (newStatus, oldStatus) => {
    if (newStatus === 'completed' && oldStatus !== 'completed') {
      setTimeout(() => {
        localCollapsed.value = true;
      }, 600);
    }
  },
);

function toggleCollapse() {
  localCollapsed.value = !localCollapsed.value;
}

const completedCount = computed(() => props.plan.entries.filter((e) => e.status === 'completed').length);
</script>

<style scoped lang="sass">
.plan-card
  border: 1px solid color-mix(in srgb, var(--color-accent) 20%, var(--glass-border))
  border-radius: 10px
  background: color-mix(in srgb, var(--color-accent) 4%, var(--glass-surface))
  margin-bottom: 12px
  overflow: hidden

.plan-card--completed
  border-color: color-mix(in srgb, var(--color-success) 20%, var(--glass-border))
  background: color-mix(in srgb, var(--color-success) 3%, var(--glass-surface))

.plan-card--failed
  border-color: color-mix(in srgb, var(--color-danger) 20%, var(--glass-border))
  background: color-mix(in srgb, var(--color-danger) 3%, var(--glass-surface))

.plan-card__header
  display: flex
  align-items: center
  gap: 6px
  padding: 8px 12px
  cursor: pointer
  user-select: none
  transition: background 0.12s

  &:hover
    background: color-mix(in srgb, var(--color-text-primary) 4%, transparent)

.plan-card__header-icon
  display: flex
  align-items: center
  justify-content: center
  width: 20px
  height: 20px

.plan-card__title
  font-size: 12px
  font-weight: 600
  color: var(--color-text-secondary)
  text-transform: uppercase
  letter-spacing: 0.3px

.plan-card__count
  font-size: 11px
  color: var(--color-text-tertiary)
  margin-left: 4px

.plan-card__toggle
  margin-left: auto
  color: var(--color-text-tertiary)
  font-size: 10px

.plan-card__entries
  padding: 4px 12px 10px

.plan-entry
  display: flex
  align-items: flex-start
  gap: 8px
  padding: 6px 0

  &:not(:last-child)
    border-bottom: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent)

.plan-entry__indicator
  display: flex
  align-items: center
  justify-content: center
  width: 16px
  height: 20px
  flex-shrink: 0
  margin-top: 1px

.plan-entry__dot
  width: 8px
  height: 8px
  border-radius: 50%
  border: 1.5px solid var(--color-text-tertiary)
  background: transparent

.plan-entry--completed .plan-entry__dot
  background: var(--color-success)
  border-color: var(--color-success)

.plan-entry--running .plan-entry__dot
  background: var(--color-accent)
  border-color: var(--color-accent)

.plan-entry--failed .plan-entry__dot
  background: var(--color-danger)
  border-color: var(--color-danger)

.plan-entry__content
  flex: 1
  min-width: 0

.plan-entry__task
  font-size: 13px
  color: var(--color-text-primary)
  line-height: 1.4

.plan-entry--pending .plan-entry__task
  color: var(--color-text-tertiary)

.plan-entry__agent
  display: flex
  align-items: center
  gap: 4px
  margin-top: 3px

.plan-entry__agent-icon
  width: 16px
  height: 16px
  border-radius: 50%
  display: inline-flex
  align-items: center
  justify-content: center
  font-size: 9px
  font-weight: 600
  color: var(--color-surface-solid)
  flex-shrink: 0

.plan-entry__agent-name
  font-size: 11px
  font-weight: 500
</style>
