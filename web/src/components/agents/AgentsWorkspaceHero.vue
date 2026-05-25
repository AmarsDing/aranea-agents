<template>
  <section class="app-entity-workspace-hero">
    <div>
      <div class="app-entity-workspace-hero__kicker">Agent workspace</div>
      <h1 class="app-entity-workspace-hero__title">Agent</h1>
      <p class="app-entity-workspace-hero__subtitle">管理您的 AI Agent，按模型、业务分类、进化状态快速筛选与维护。</p>
    </div>
    <div class="app-entity-workspace-hero__actions">
      <q-btn outline rounded color="primary" icon="sync_alt" label="Agent 迁移" class="app-entity-workspace-hero__btn" @click="$emit('open-migration')" />
      <q-btn outline rounded color="primary" icon="account_tree" label="管理分类" class="app-entity-workspace-hero__btn" to="/settings/agent-categories" />
      <q-btn color="primary" unelevated rounded icon="add" label="创建 Agent" class="app-entity-workspace-hero__btn" @click="$emit('create')" />
    </div>
  </section>
</template>

<script setup lang="ts">
defineEmits<{
  create: [];
  "open-migration": [];
}>();
</script>
