<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { SceneId } from '../../features/ecommerce-loop/types';
defineProps<{ scene: SceneId }>();
const { t } = useI18n();
</script>

<template>
  <header class="ec-stage-header">
    <div class="ec-stage-progress" :style="{ width: `${scene * 20}%` }" />
    <div class="ec-brand">
      <span class="ec-brand-dots"><i /><i /><i /></span>AI NATIVE
      <span>/ {{ t(`ecommerceLoop.scenes.${scene}`) }}</span>
    </div>
    <div v-if="scene === 2 || scene === 4" class="ec-legend">
      <span v-for="tone in ['data', 'ai', 'ceo', 'owner', 'working']" :key="tone" :class="`ec-tone-${tone}`"
        ><i />{{ t(`ecommerceLoop.legend.${tone}`) }}</span
      >
    </div>
    <div class="ec-page-counter">
      <b>0{{ scene }}</b> / 05
    </div>
  </header>
</template>
