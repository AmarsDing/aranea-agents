<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
const props = withDefaults(defineProps<{ text: string; anomaly?: boolean }>(), { anomaly: false });
const { t } = useI18n();
const parts = computed(() => {
  const because = t('ecommerceLoop.loop.because');
  const pattern = new RegExp(`(\\d[\\d,.]*%?|${because})`, 'g');
  return props.text
    .split(pattern)
    .filter(Boolean)
    .map((value, index) => ({
      id: index,
      value,
      reason: value === because,
      numeric: /^\d/.test(value),
      danger: props.anomaly && value === '3.1%',
    }));
});
</script>

<template>
  <span class="ec-copy"
    ><span
      v-for="part in parts"
      :key="part.id"
      :class="{ 'ec-copy-reason': part.reason, 'ec-copy-number': part.numeric, 'ec-copy-danger': part.danger }"
      >{{ part.value }}</span
    ></span
  >
</template>
