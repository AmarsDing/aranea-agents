<template>
  <AppPageHero kicker="Usage" title="模型消耗概览" subtitle="Token、费用、调用次数与异常请求运营看板">
    <template v-if="$slots.actions" #actions>
      <slot name="actions" />
    </template>
  </AppPageHero>
</template>

<script setup lang="ts">
import AppPageHero from "../layout/AppPageHero.vue";
</script>
