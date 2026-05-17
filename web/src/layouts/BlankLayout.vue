<template>
  <div class="blank-layout-root fit">
    <router-view />
  </div>
</template>

<style scoped>
.blank-layout-root {
  min-height: 100vh;
  background: linear-gradient(160deg, var(--color-surface-soft) 0%, var(--color-info-soft) 45%, var(--color-page-tint-cool) 100%);
}

.body--dark .blank-layout-root {
  background: linear-gradient(160deg, var(--color-surface-solid) 0%, var(--color-surface-soft) 50%, var(--canvas-base) 100%);
}
</style>
