<template>
  <div class="blank-layout-root fit">
    <router-view />
  </div>
</template>

<style scoped>
.blank-layout-root {
  min-height: 100vh;
  background: linear-gradient(160deg, #f8fafc 0%, #eef6ff 45%, #f1f5f9 100%);
}
.body--dark .blank-layout-root {
  background: linear-gradient(160deg, #0f172a 0%, #1e293b 50%, #0b1224 100%);
}
</style>
