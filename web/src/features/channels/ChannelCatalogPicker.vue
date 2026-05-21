<template>
  <div class="channel-catalog-grid">
    <q-card
      v-for="item in catalog"
      :key="item.type"
      flat
      bordered
      :class="['catalog-card', { 'cursor-pointer': isImplemented(item.type), 'catalog-card--coming-soon': !isImplemented(item.type), selected: item.type === modelValue }]"
      @click="isImplemented(item.type) && $emit('update:modelValue', item.type)"
    >
      <q-card-section class="catalog-card__body">
        <div class="catalog-card__head">
          <q-avatar color="primary" text-color="white" size="30px">{{ item.label.slice(0, 1) }}</q-avatar>
          <div class="catalog-main">
            <div class="text-weight-bold catalog-title">{{ item.label }}</div>
            <div class="text-caption text-grey-7 catalog-group">{{ item.group }}</div>
          </div>
        </div>
        <div class="text-caption text-grey-7 catalog-desc">{{ item.receive_modes.join(", ") }}</div>
        <q-badge v-if="!isImplemented(item.type)" class="catalog-badge" color="grey-6" label="即将支持" />
      </q-card-section>
    </q-card>
  </div>
</template>

<script setup lang="ts">
import type { ChannelCatalogItem } from "./types";

defineProps<{
  catalog: ChannelCatalogItem[];
  modelValue: string;
}>();

defineEmits<{
  "update:modelValue": [value: string];
}>();

// EP-BIZ-05: Only feishu has a backend ingress implementation.
const IMPLEMENTED_CHANNEL_TYPES = new Set(["feishu"]);

function isImplemented(type: string): boolean {
  return IMPLEMENTED_CHANNEL_TYPES.has(type);
}
</script>

<style scoped>
.channel-catalog-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(156px, 1fr));
  gap: 12px;
}

.catalog-card {
  height: 100%;
  min-height: 96px;
  border-radius: 14px;
  transition: border-color 0.16s ease, transform 0.16s ease, box-shadow 0.16s ease;
}

.catalog-card__body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  height: 100%;
}

.catalog-card__head {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  min-width: 0;
}

.catalog-card:hover:not(.catalog-card--coming-soon),
.catalog-card.selected {
  border-color: var(--color-accent);
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--color-accent) 28%, transparent);
  transform: translateY(-1px);
}

.catalog-card--coming-soon {
  opacity: 62%;
  cursor: not-allowed;
}

.catalog-desc,
.catalog-group {
  line-height: 1.35;
}

.catalog-main {
  min-width: 0;
  flex: 1;
}

.catalog-title {
  font-size: 13px;
  line-height: 1.3;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.catalog-badge {
  align-self: flex-start;
}
</style>
