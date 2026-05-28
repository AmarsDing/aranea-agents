<template>
  <AppPageHero
    kicker="Agent workspace"
    title="Agent"
    subtitle="管理您的 AI Agent，按模型、业务分类、进化状态快速筛选与维护。"
  >
    <template #actions>
      <q-btn outline rounded no-caps color="primary" icon="sync_alt" label="Agent 迁移（即将推出）" disable @click="$emit('open-migration')" />
      <q-btn outline rounded no-caps color="primary" icon="account_tree" label="管理分类" to="/settings/agent-categories" />
      <q-btn color="primary" unelevated rounded no-caps icon="add" label="创建 Agent" @click="$emit('create')" />
    </template>
  </AppPageHero>
</template>

<script setup lang="ts">
import AppPageHero from "../layout/AppPageHero.vue";

defineEmits<{
  create: [];
  "open-migration": [];
}>();
</script>
