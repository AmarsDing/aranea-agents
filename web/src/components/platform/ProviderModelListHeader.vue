<template>
  <div :class="['provider-table__head', { 'provider-table__head--dark': isDark }]">
    <span class="provider-table__avatar-spacer" aria-hidden="true" />
    <div class="provider-table__grid">
      <div class="provider-table__cell provider-table__cell--identity">模型</div>
      <div class="provider-table__cell provider-table__cell--size">大小</div>
      <div class="provider-table__cell provider-table__cell--ctx">上下文</div>
      <div class="provider-table__cell provider-table__cell--tps">TPS</div>
      <div class="provider-table__cell provider-table__cell--usage">热度 / 用量</div>
      <div class="provider-table__cell provider-table__cell--secret">密钥</div>
      <div class="provider-table__cell provider-table__cell--actions">操作</div>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  isDark?: boolean;
}>();
</script>
