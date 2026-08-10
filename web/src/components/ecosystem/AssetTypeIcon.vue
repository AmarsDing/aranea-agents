<template>
  <q-avatar :size="size" rounded class="asset-type-icon" :style="avatarStyle">
    <q-icon :name="meta.icon" :size="iconSize" color="white" />
  </q-avatar>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { ASSET_TYPE_META } from '../../features/ecosystem/marketUi';
import type { MarketAssetType } from '../../features/ecosystem/types';

const props = withDefaults(
  defineProps<{
    type: MarketAssetType;
    size?: string;
  }>(),
  { size: '40px' },
);

const meta = computed(() => ASSET_TYPE_META[props.type]);
const iconSize = computed(() => `calc(${props.size} * 0.55)`);
const avatarStyle = computed(() => ({
  background: `linear-gradient(135deg, ${meta.value.color}, ${meta.value.color}CC)`,
}));
</script>

<style scoped>
.asset-type-icon {
  flex: none;
  box-shadow: 0 2px 8px rgb(0 0 0 / 12%);
}
</style>
