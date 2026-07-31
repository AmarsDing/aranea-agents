<template>
  <span :class="['price-tag', `price-tag--${priceModel}`]">
    <template v-if="priceModel === 'free'">{{ t('shopPage.priceFree') }}</template>
    <template v-else-if="priceModel === 'enterprise'">{{ t('shopPage.priceEnterprise') }}</template>
    <template v-else-if="priceModel === 'subscription'">¥{{ formatted }}{{ t('shopPage.pricePerMonth') }}</template>
    <template v-else>¥{{ formatted }}</template>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { formatCents } from '../../features/ecosystem/marketUi';
import type { PriceModel } from '../../features/ecosystem/types';

const props = defineProps<{
  priceModel: PriceModel;
  priceCents: number;
}>();

const { t } = useI18n();
const formatted = computed(() => formatCents(props.priceCents));
</script>

<style scoped>
.price-tag {
  font-weight: 700;
  font-size: 14px;
  white-space: nowrap;
}
.price-tag--free {
  color: var(--color-success);
}
.price-tag--one_time,
.price-tag--subscription {
  color: var(--color-text-primary);
}
.price-tag--enterprise {
  color: var(--color-accent);
  font-size: 12px;
}
</style>
