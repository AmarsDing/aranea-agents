<template>
  <div class="asset-screenshots row no-wrap q-gutter-sm">
    <div v-for="i in count" :key="i" class="asset-screenshots__item">
      <q-img :src="shotUrl(i - 1)" :ratio="16 / 9" fit="cover" class="asset-screenshots__img" spinner-color="primary">
        <template #error>
          <div class="absolute-full flex flex-center asset-screenshots__fallback">
            <q-icon name="image_not_supported" size="24px" />
          </div>
        </template>
      </q-img>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { MarketAsset } from '../../features/ecosystem/types';

const props = defineProps<{
  asset: MarketAsset;
  count: number;
}>();

/** 骨架期截图：按资产语义生成配图，后端就绪后替换为真实截图 URL */
function shotUrl(index: number): string {
  const prompt = encodeURIComponent(
    `clean modern SaaS dashboard UI screenshot for "${props.asset.name}" (${props.asset.type}), panel ${index + 1}, professional software interface, high quality`,
  );
  return `https://trae-api-cn.mchost.guru/api/ide/v1/text_to_image?prompt=${prompt}&image_size=landscape_16_9`;
}
</script>

<style scoped>
.asset-screenshots {
  overflow-x: auto;
}

.asset-screenshots__item {
  flex: none;
  width: min(360px, 72vw);
  border-radius: 12px;
  overflow: hidden;
  border: 1px solid var(--glass-border);
}

.asset-screenshots__img {
  border-radius: 12px;
}

.asset-screenshots__fallback {
  color: var(--color-icon-muted);
  background: var(--glass-surface);
}
</style>
