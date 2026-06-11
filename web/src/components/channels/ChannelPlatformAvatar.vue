<template>
  <q-avatar
    class="app-avatar-cover"
    :size="size"
    :color="fallbackColor"
    :text-color="fallbackTextColor"
    :rounded="rounded"
  >
    <resolved-avatar-img v-if="resolvedIcon" :icon="resolvedIcon" :alt="alt" />
    <span v-else>{{ fallbackLetter }}</span>
  </q-avatar>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import ResolvedAvatarImg from '../avatar/ResolvedAvatarImg.vue';
import { defaultChannelAvatarKey } from '../../domain/channel';
import type { ChannelMetadata } from '../../features/channels/types';

const props = withDefaults(
  defineProps<{
    type: string;
    label: string;
    metadata?: ChannelMetadata;
    size?: string;
    rounded?: boolean;
    fallbackColor?: string;
    fallbackTextColor?: string;
  }>(),
  {
    size: '32px',
    rounded: true,
    fallbackColor: 'primary',
    fallbackTextColor: 'white',
    metadata: () => ({}),
  },
);

const resolvedIcon = computed(() => {
  const custom = props.metadata?.icon_asset_id?.trim();
  if (custom) return custom;
  const legacyUrl = props.metadata?.icon_url?.trim();
  if (legacyUrl && /^(https?:|data:|blob:)/i.test(legacyUrl)) return legacyUrl;
  const platformKey = defaultChannelAvatarKey(props.type);
  return platformKey || '';
});

const fallbackLetter = computed(() => (props.label?.trim() || props.type || '?').slice(0, 1));
const alt = computed(() => props.label || props.type);
</script>
