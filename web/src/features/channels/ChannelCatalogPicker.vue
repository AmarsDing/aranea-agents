// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <div class="channel-catalog-grid">
    <button
      v-for="item in catalog"
      :key="item.type"
      type="button"
      :class="[
        'catalog-card',
        {
          'catalog-card--selected': item.type === modelValue,
          'catalog-card--disabled': !item.bundled
        }
      ]"
      :disabled="!item.bundled"
      :aria-pressed="item.type === modelValue"
      @click="item.bundled && $emit('update:modelValue', item.type)"
    >
      <channel-platform-avatar
        :type="item.type"
        :label="item.label"
        size="32px"
        fallback-color="grey-8"
      />
      <div class="catalog-card__main min-width-0">
        <div class="catalog-card__title">{{ item.label }}</div>
        <div class="catalog-card__group">{{ item.group }}</div>
      </div>
      <div class="catalog-card__modes">{{ item.receive_modes.join(" · ") }}</div>
      <span v-if="!item.bundled" class="catalog-card__badge">{{ t("channelEditor.comingSoon") }}</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";
import ChannelPlatformAvatar from "../../components/channels/ChannelPlatformAvatar.vue";

const { t } = useI18n();
import type { ChannelCatalogItem } from "./types";

defineProps<{
  catalog: ChannelCatalogItem[];
  modelValue: string;
}>();

defineEmits<{
  "update:modelValue": [value: string];
}>();
</script>

<style scoped lang="sass">
.channel-catalog-grid
  display: grid
  grid-template-columns: repeat(auto-fill, minmax(168px, 1fr))
  gap: 10px

.catalog-card
  display: flex
  flex-direction: column
  align-items: flex-start
  gap: 8px
  min-height: 108px
  padding: 12px
  border: 1px solid var(--glass-border)
  border-radius: 14px
  background: var(--glass-surface)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))
  text-align: left
  cursor: pointer
  transition: border-color 0.16s ease, background 0.16s ease, transform 0.16s ease

  &:hover:not(:disabled)
    background: var(--glass-surface-hover)
    border-color: color-mix(in srgb, var(--color-accent) 22%, var(--glass-border))

  &.catalog-card--selected
    border-color: color-mix(in srgb, var(--color-accent) 38%, var(--glass-border))
    background: color-mix(in srgb, var(--color-accent) 10%, var(--glass-surface))

  &.catalog-card--disabled
    opacity: 0.55
    cursor: not-allowed

.catalog-card__title
  font-size: 13px
  font-weight: 600
  line-height: 1.3
  color: var(--color-text-heading)
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

.catalog-card__group,
.catalog-card__modes
  font-size: 11px
  line-height: 1.35
  color: var(--color-text-secondary)

.catalog-card__modes
  width: 100%

.catalog-card__badge
  align-self: flex-start
  padding: 2px 8px
  border-radius: 999px
  border: 1px solid var(--glass-border)
  font-size: 10px
  font-weight: 600
  letter-spacing: 0.04em
  text-transform: uppercase
  color: var(--color-text-tertiary)

.min-width-0
  min-width: 0
  width: 100%
</style>
