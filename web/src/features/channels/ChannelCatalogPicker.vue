<template>
  <div class="row q-col-gutter-xs">
    <div v-for="item in catalog" :key="item.type" class="col-6 col-sm-4 col-md-3 col-lg-2">
      <q-card
        flat
        bordered
        :class="['catalog-card', { 'cursor-pointer': isImplemented(item.type), 'catalog-card--coming-soon': !isImplemented(item.type), selected: item.type === modelValue }]"
        @click="isImplemented(item.type) && $emit('update:modelValue', item.type)"
      >
        <q-card-section class="q-pa-sm">
          <div class="row items-center no-wrap q-gutter-xs">
            <q-avatar color="primary" text-color="white" size="26px">{{ item.label.slice(0, 1) }}</q-avatar>
            <div class="catalog-main">
              <div class="text-weight-bold catalog-title">{{ item.label }}</div>
              <div class="text-caption text-grey-7 ellipsis">{{ item.group }}</div>
            </div>
          </div>
          <div class="text-caption text-grey-7 q-mt-xs catalog-desc">{{ item.receive_modes.join(", ") }}</div>
          <q-badge v-if="!isImplemented(item.type)" class="q-mt-xs" color="grey-6" label="即将支持" />
        </q-card-section>
      </q-card>
    </div>
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
// Other channel types are visible for configuration but not yet active.
const IMPLEMENTED_CHANNEL_TYPES = new Set(["feishu"]);

function isImplemented(type: string): boolean {
  return IMPLEMENTED_CHANNEL_TYPES.has(type);
}
</script>

<style scoped>
.catalog-card {
  height: 100%;
  min-height: 84px;
  border-radius: 12px;
  transition: border-color 0.16s ease, transform 0.16s ease;
}

.catalog-card:hover:not(.catalog-card--coming-soon),
.catalog-card.selected {
  border-color: var(--q-primary);
  transform: translateY(-1px);
}

.catalog-card--coming-soon {
  opacity: 55%;
  cursor: not-allowed;
}

.catalog-desc {
  line-height: 1.25;
}

.catalog-main {
  min-width: 0;
}

.catalog-title {
  font-size: 13px;
  line-height: 1.2;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
