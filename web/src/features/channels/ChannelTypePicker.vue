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
          'catalog-card--disabled': !item.bundled,
        },
      ]"
      :disabled="!item.bundled"
      :aria-pressed="item.type === modelValue"
      @click="item.bundled && $emit('update:modelValue', item.type)"
    >
      <channel-platform-avatar :type="item.type" :label="item.label" size="32px" fallback-color="grey-8" />
      <div class="catalog-card__main min-width-0">
        <div class="catalog-card__title">{{ item.label }}</div>
        <div class="catalog-card__group">{{ item.group }}</div>
      </div>
      <div class="catalog-card__modes">{{ item.receive_modes.join(' · ') }}</div>
      <span v-if="!item.bundled" class="catalog-card__badge">{{ t('channelEditor.comingSoon') }}</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import ChannelPlatformAvatar from '../../components/channels/ChannelPlatformAvatar.vue';

const { t } = useI18n();
import type { ChannelTypeItem } from './types';

defineProps<{
  catalog: ChannelTypeItem[];
  modelValue: string;
}>();

defineEmits<{
  'update:modelValue': [value: string];
}>();
</script>
