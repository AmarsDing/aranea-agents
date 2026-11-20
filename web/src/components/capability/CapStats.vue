<!-- 目录页顶部的一条关键数：一行排开；标了 clickable 的数点了按对应口径筛选，再点取消 -->
<template>
  <div class="cap-stats" role="group" :aria-label="label">
    <component
      :is="stat.clickable ? 'button' : 'div'"
      v-for="stat in stats"
      :key="stat.key"
      class="cap-stats__item"
      :class="[stat.tone && `cap-tone--${stat.tone}`, { 'cap-stats__item--active': stat.key === active }]"
      :type="stat.clickable ? 'button' : undefined"
      :aria-pressed="stat.clickable ? stat.key === active : undefined"
      :title="stat.hint"
      @click="stat.clickable && emit('pick', stat.key)"
    >
      <span class="cap-stats__value">{{ stat.value }}</span>
      <span class="cap-stats__label">{{ stat.label }}</span>
    </component>
  </div>
</template>

<script setup lang="ts">
import type { CapStat } from './capTypes';

defineProps<{
  stats: Array<CapStat & { clickable?: boolean }>;
  label: string;
  /** 当前按哪个数在筛选 */
  active?: string;
}>();

const emit = defineEmits<{ pick: [key: string] }>();
</script>
