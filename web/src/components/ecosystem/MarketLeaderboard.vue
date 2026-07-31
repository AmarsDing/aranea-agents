<template>
  <q-card flat class="app-glass-panel market-leaderboard">
    <q-card-section class="q-pb-xs">
      <div class="row items-center q-gutter-xs">
        <q-icon :name="icon" size="16px" color="primary" />
        <span class="text-weight-bold">{{ title }}</span>
      </div>
    </q-card-section>
    <q-list dense class="q-pb-sm">
      <q-item
        v-for="(asset, i) in items"
        :key="asset.id"
        v-ripple
        clickable
        class="market-leaderboard__item"
        @click="emit('open', asset)"
      >
        <q-item-section side class="market-leaderboard__rank" :class="`market-leaderboard__rank--${i + 1}`">
          {{ i + 1 }}
        </q-item-section>
        <q-item-section class="ellipsis">
          <q-item-label class="ellipsis">{{ asset.name }}</q-item-label>
          <q-item-label caption class="ellipsis">{{ asset.creator.name }}</q-item-label>
        </q-item-section>
        <q-item-section side class="market-leaderboard__metric text-caption">
          {{ metric(asset) }}
        </q-item-section>
      </q-item>
    </q-list>
  </q-card>
</template>

<script setup lang="ts">
import type { MarketAsset } from '../../features/ecosystem/types';

defineProps<{
  title: string;
  icon: string;
  items: MarketAsset[];
  /** 右侧指标展示（如 8.9k / 4.8★） */
  metric: (asset: MarketAsset) => string;
}>();

const emit = defineEmits<{
  open: [asset: MarketAsset];
}>();
</script>

<style scoped>
.market-leaderboard {
  border-radius: 14px;
}
.market-leaderboard__item {
  border-radius: 8px;
  min-height: 44px;
}
.market-leaderboard__rank {
  width: 20px;
  font-weight: 800;
  font-size: 13px;
  color: var(--color-icon-muted);
  justify-content: center;
  font-style: italic;
}
.market-leaderboard__rank--1 {
  color: #e9a23b;
}
.market-leaderboard__rank--2 {
  color: #b8a590;
}
.market-leaderboard__rank--3 {
  color: #cd7f32;
}
.market-leaderboard__metric {
  color: var(--color-text-secondary);
  font-weight: 600;
  min-width: 40px;
  text-align: right;
}
</style>
