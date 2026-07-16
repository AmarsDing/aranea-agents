<template>
  <AppPageHero
    kicker="Agent workspace"
    title="Agent"
    subtitle="管理您的 AI Agent，按模型、组织架构、进化状态快速筛选与维护。"
  >
    <template #actions>
      <q-btn outline rounded no-caps color="primary" icon="account_tree" label="管理组织" to="/settings/organization" />
      <q-btn color="primary" unelevated rounded no-caps icon="add" label="创建 Agent" @click="$emit('create')" />
    </template>
  </AppPageHero>
</template>

<script setup lang="ts">
import AppPageHero from '../layout/AppPageHero.vue';

defineEmits<{
  create: [];
}>();
</script>
