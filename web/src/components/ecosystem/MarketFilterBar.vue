<template>
  <div class="market-filter-bar">
    <div class="market-filter-bar__chips row no-wrap items-center q-gutter-xs">
      <q-chip
        v-for="opt in typeOptions"
        :key="opt.value || 'all'"
        :outline="type !== opt.value"
        :class="['market-filter-bar__chip', { 'market-filter-bar__chip--active': type === opt.value }]"
        clickable
        dense
        @click="emit('update:type', opt.value)"
      >
        <q-icon v-if="opt.icon" :name="opt.icon" size="14px" class="q-mr-xs" />
        {{ opt.label }}
      </q-chip>
    </div>

    <div class="row items-center q-gutter-sm q-mt-sm">
      <q-select
        :model-value="priceModel"
        dense
        outlined
        emit-value
        map-options
        options-dense
        class="market-filter-bar__select"
        :label="t('shopPage.filterPrice')"
        :options="priceOptions"
        @update:model-value="emit('update:priceModel', $event)"
      />
      <q-select
        :model-value="sort"
        dense
        outlined
        emit-value
        map-options
        options-dense
        class="market-filter-bar__select"
        :label="t('shopPage.filterSort')"
        :options="sortOptions"
        @update:model-value="emit('update:sort', $event)"
      />
      <q-space />
      <span v-if="total !== undefined" class="text-caption text-grey-7">
        {{ t('shopPage.resultCount', { count: total }) }}
      </span>
      <q-btn flat dense no-caps icon="restart_alt" :label="t('shopPage.filterReset')" @click="emit('reset')" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { ALL_ASSET_TYPES, ASSET_TYPE_META, PRICE_MODELS } from '../../features/ecosystem/marketUi';
import type { BrowseSort, MarketAssetType, PriceModel } from '../../features/ecosystem/types';

const props = defineProps<{
  type: MarketAssetType | '';
  priceModel: PriceModel | '';
  sort: BrowseSort;
  total?: number;
}>();

const emit = defineEmits<{
  'update:type': [value: MarketAssetType | ''];
  'update:priceModel': [value: PriceModel | ''];
  'update:sort': [value: BrowseSort];
  reset: [];
}>();

void props;

const { t } = useI18n();

const typeOptions = computed(() => [
  { value: '' as const, label: t('shopPage.filterAllTypes'), icon: 'apps' },
  ...ALL_ASSET_TYPES.map((type) => ({
    value: type as MarketAssetType,
    label: t(`shopPage.type.${ASSET_TYPE_META[type].labelKey}`),
    icon: ASSET_TYPE_META[type].icon,
  })),
]);

const priceOptions = computed(() => [
  { value: '' as const, label: t('shopPage.priceAll') },
  ...PRICE_MODELS.map((m) => ({ value: m as PriceModel, label: t(`shopPage.priceModel.${m}`) })),
]);

const SORT_KEYS: BrowseSort[] = ['hot', 'new', 'rating', 'installs', 'activity', 'price'];
const sortOptions = computed(() => SORT_KEYS.map((s) => ({ value: s, label: t(`shopPage.sort.${s}`) })));
</script>

<style scoped>
.market-filter-bar__chips {
  overflow-x: auto;
  padding-bottom: 2px;
  scrollbar-width: none;
}
.market-filter-bar__chips::-webkit-scrollbar {
  display: none;
}
.market-filter-bar__chip {
  flex: none;
  border-radius: 999px;
  font-size: 12px;
  color: var(--color-text-secondary);
  border: 1px solid var(--glass-border);
  background: transparent;
  transition:
    background 0.15s ease,
    color 0.15s ease;
}
.market-filter-bar__chip:hover {
  color: var(--color-text-primary);
  background: var(--interaction-surface-hover);
}
body.body--dark .market-filter-bar__chip:hover {
  background: rgba(255, 255, 255, 0.06);
}
.market-filter-bar__chip--active,
.market-filter-bar__chip--active:hover {
  background: var(--color-accent);
  color: var(--color-on-accent);
  border-color: var(--color-accent);
}
.market-filter-bar__select {
  min-width: 130px;
}
</style>
