<template>
  <template v-if="kind === 'Text'">
    <component :is="ctx.textTag" :class="ctx.textClass">{{ ctx.textContent }}</component>
  </template>
  <template v-else-if="kind === 'Divider'">
    <q-separator :vertical="ctx.dividerVertical" class="a2ui-divider" />
  </template>
  <template v-else-if="kind === 'Image'">
    <q-img v-if="ctx.imageUrl" :src="ctx.imageUrl" class="a2ui-image" fit="contain" />
  </template>
  <template v-else-if="kind === 'Icon'">
    <q-icon v-if="ctx.iconName" :name="mapA2UIIcon(ctx.iconName)" size="24px" />
  </template>
  <template v-else-if="kind === 'Video'">
    <video v-if="ctx.imageUrl" class="a2ui-video" controls :src="ctx.imageUrl" />
  </template>
</template>

<script setup lang="ts">
import { mapA2UIIcon, type A2UIComponentCtx } from '../../../../features/chat/a2ui/useA2UIComponent';

defineProps<{
  kind: string;
  ctx: A2UIComponentCtx;
}>();
</script>

<style scoped>
.a2ui-image {
  max-width: 100%;
  max-height: 200px;
  border-radius: 8px;
}

.a2ui-video {
  max-width: 100%;
  max-height: 240px;
  border-radius: 8px;
}

.a2ui-divider {
  margin: var(--space-2) 0;
}
</style>
