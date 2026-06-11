<template>
  <div
    class="spirit-entry"
    :class="{ 'spirit-entry--active': active }"
    role="button"
    tabindex="0"
    @click="$emit('click')"
    @keydown.enter="$emit('click')"
    @keydown.space.prevent="$emit('click')"
  >
    <div class="spirit-entry__icon">
      <q-icon name="auto_awesome" size="20px" />
    </div>
    <div class="spirit-entry__info col min-width-0">
      <div class="spirit-entry__name ellipsis">{{ t('spirit.spiritAssistant') }}</div>
      <div class="spirit-entry__status ellipsis">{{ t('spirit.spiritDescription') }}</div>
    </div>
    <div v-if="active" class="spirit-entry__indicator" />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';

const { t } = useI18n();

defineProps<{
  active: boolean;
}>();

defineEmits<{
  click: [];
}>();
</script>

<style scoped lang="sass">
.spirit-entry
  display: flex
  align-items: center
  gap: var(--space-3)
  padding: var(--space-3) var(--space-3)
  border-radius: 12px
  cursor: pointer
  transition: background 0.15s ease, border-color 0.15s ease
  border: 1px solid transparent
  background: color-mix(in srgb, var(--glass-surface) 40%, transparent)

.spirit-entry:hover
  background: color-mix(in srgb, var(--glass-surface) 65%, transparent)
  border-color: var(--glass-border)

.spirit-entry--active
  background: color-mix(in srgb, var(--color-accent) 8%, var(--glass-surface))
  border-color: color-mix(in srgb, var(--color-accent) 30%, var(--glass-border))

.spirit-entry__icon
  display: flex
  align-items: center
  justify-content: center
  width: 36px
  height: 36px
  border-radius: 10px
  background: color-mix(in srgb, var(--color-accent) 12%, var(--glass-surface))
  color: var(--color-accent)
  flex-shrink: 0

.spirit-entry__name
  font-size: var(--text-sm)
  font-weight: 600
  color: var(--color-text-primary)
  line-height: 1.3

.spirit-entry__status
  font-size: var(--text-xs)
  color: var(--color-text-tertiary)
  line-height: 1.3

.spirit-entry__indicator
  width: 6px
  height: 6px
  border-radius: 50%
  background: var(--color-accent)
  flex-shrink: 0
</style>
