<template>
  <div class="publish-type-select row q-col-gutter-sm">
    <div v-for="type in ALL_ASSET_TYPES" :key="type" class="col-6 col-sm-4 col-md-3">
      <button
        type="button"
        :class="['publish-type-select__card', { 'publish-type-select__card--active': modelValue === type }]"
        @click="emit('update:modelValue', type)"
      >
        <asset-type-icon :type="type" size="34px" />
        <span class="publish-type-select__label">{{ t(`shopPage.type.${ASSET_TYPE_META[type].labelKey}`) }}</span>
        <span class="publish-type-select__desc text-caption">{{
          t(`shopPage.typeDesc.${ASSET_TYPE_META[type].labelKey}`)
        }}</span>
        <q-icon v-if="modelValue === type" name="check_circle" size="18px" class="publish-type-select__check" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { ALL_ASSET_TYPES, ASSET_TYPE_META } from '../../features/ecosystem/marketUi';
import type { MarketAssetType } from '../../features/ecosystem/types';
import AssetTypeIcon from './AssetTypeIcon.vue';

defineProps<{
  modelValue: MarketAssetType | '';
}>();

const emit = defineEmits<{
  'update:modelValue': [value: MarketAssetType];
}>();

const { t } = useI18n();
</script>

<style scoped>
.publish-type-select__card {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
  width: 100%;
  height: 100%;
  padding: 12px;
  border: 1px solid var(--glass-border);
  border-radius: 12px;
  background: transparent;
  cursor: pointer;
  text-align: left;
  transition:
    border-color 0.15s ease,
    background 0.15s ease,
    transform 0.15s ease;
}
.publish-type-select__card:hover {
  background: var(--interaction-surface-hover);
  transform: translateY(-1px);
}
body.body--dark .publish-type-select__card:hover {
  background: rgba(255, 255, 255, 0.06);
}
.publish-type-select__card--active {
  border-color: var(--color-accent);
  background: var(--interaction-surface-hover);
}
body.body--dark .publish-type-select__card--active {
  background: rgba(0, 229, 255, 0.08);
  box-shadow: 0 0 0 1px var(--color-accent);
}
.publish-type-select__label {
  font-weight: 600;
  font-size: 13px;
  color: var(--color-text-primary);
}
.publish-type-select__desc {
  color: var(--color-text-secondary);
  line-height: 1.4;
}
.publish-type-select__check {
  position: absolute;
  top: 8px;
  right: 8px;
  color: var(--color-accent);
}
</style>
