<template>
  <div class="cross-dept-dag-view">
    <div class="text-subtitle2 q-mb-sm">跨部门协作 DAG</div>
    <p class="text-body2 text-grey-7 q-mb-md">可视化展示跨部门团队的依赖关系与执行流程。</p>

    <div class="cross-dept-dag-view__placeholder">
      <q-icon name="account_tree" size="64px" color="grey-5" />
      <div class="text-body2 text-grey-6 q-mt-md">DAG 可视化将在后续版本实现</div>
    </div>
  </div>
</template>

<script setup lang="ts">
// Placeholder for DAG visualization component
// Will be implemented with a graph library (e.g., dagre, elkjs) in a future iteration
</script>

<style lang="sass" scoped>
.cross-dept-dag-view__placeholder
  display: flex
  flex-direction: column
  align-items: center
  justify-content: center
  min-height: 240px
  border: 1px dashed var(--color-border-soft, rgba(141, 110, 99, 0.12))
  border-radius: 12px
  background: var(--glass-surface, rgba(255, 253, 245, 0.65))
</style>
