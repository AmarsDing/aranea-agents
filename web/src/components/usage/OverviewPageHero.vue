<template>
  <section class="app-page-hero">
    <div>
      <div class="app-page-kicker">Usage</div>
      <h1 class="app-page-title">模型消耗概览</h1>
      <p class="app-page-subtitle">Token、费用、调用次数与异常请求运营看板</p>
    </div>
    <div v-if="$slots.actions" class="app-actions-bar row q-gutter-sm items-center">
      <slot name="actions" />
    </div>
  </section>
</template>

<script setup lang="ts"></script>
