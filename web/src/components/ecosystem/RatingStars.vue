<template>
  <span class="rating-stars row inline items-center no-wrap" :title="`${rating.toFixed(1)} / 5`">
    <q-icon v-for="i in 5" :key="i" :name="starIcon(i)" :size="size" :class="starClass(i)" />
    <span v-if="showValue" class="rating-stars__value">{{ rating.toFixed(1) }}</span>
    <span v-if="count !== undefined" class="rating-stars__count">({{ count }})</span>
  </span>
</template>

<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    rating: number;
    count?: number;
    size?: string;
    showValue?: boolean;
  }>(),
  { count: undefined, size: '14px', showValue: false },
);

function starIcon(i: number): string {
  if (props.rating >= i - 0.25) return 'star';
  if (props.rating >= i - 0.75) return 'star_half';
  return 'star_border';
}

function starClass(i: number): string {
  return props.rating >= i - 0.75 ? 'rating-stars__star--lit' : 'rating-stars__star--dim';
}
</script>

<style scoped>
.rating-stars {
  gap: 1px;
  line-height: 1;
}
.rating-stars__star--lit {
  color: #e9a23b;
}
body.body--dark .rating-stars__star--lit {
  color: #ffcf6e;
}
.rating-stars__star--dim {
  color: var(--color-icon-muted);
  opacity: 0.6;
}
.rating-stars__value {
  margin-left: 4px;
  font-weight: 600;
  font-size: 12px;
  color: var(--color-text-primary);
}
.rating-stars__count {
  margin-left: 2px;
  font-size: 11px;
  color: var(--color-text-secondary);
}
</style>
