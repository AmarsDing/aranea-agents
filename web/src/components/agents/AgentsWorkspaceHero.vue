<template>
  <section class="agents-hero agents-workspace-hero">
    <div>
      <div class="agents-kicker">Agent workspace</div>
      <h1 class="agents-title">Agent</h1>
      <p class="agents-subtitle">管理您的 AI Agent，按模型、业务分类、进化状态快速筛选与维护。</p>
    </div>
    <div class="agents-hero__actions">
      <q-btn outline rounded color="primary" icon="sync_alt" label="Agent 迁移" class="agent-action-btn" @click="$emit('open-migration')" />
      <q-btn outline rounded color="primary" icon="account_tree" label="管理分类" class="agent-action-btn" to="/settings/agent-categories" />
      <q-btn color="primary" unelevated rounded icon="add" label="创建 Agent" class="agent-primary-btn" @click="$emit('create')" />
    </div>
  </section>
</template>

<script setup lang="ts">
defineEmits<{
  create: [];
  "open-migration": [];
}>();
</script>

<style scoped>
.agents-hero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  padding: 4px 2px 0;
}

.agents-kicker {
  display: inline-flex;
  align-items: center;
  height: 28px;
  padding: 0 12px;
  border: 1px solid var(--glass-border);
  border-radius: 999px;
  background: var(--glass-surface);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
  color: var(--color-accent);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  box-shadow: none;
}

.agents-hero__actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.agents-title {
  margin: 12px 0 0;
  font-size: clamp(36px, 5vw, 56px);
  line-height: 1;
  font-weight: 800;
  letter-spacing: -0.055em;
  color: var(--color-text-primary);
}

.agents-subtitle {
  margin: 10px 0 0;
  max-width: 620px;
  color: var(--color-text-secondary);
  font-size: 15px;
  line-height: 1.65;
}

.agent-action-btn,
.agent-primary-btn {
  min-height: 42px;
  padding: 0 18px;
  font-weight: 700;
}

.agent-primary-btn {
  box-shadow: none;
}

@media (max-width: 599px) {
  .agents-hero__actions {
    width: 100%;
  }

  .agent-action-btn,
  .agent-primary-btn {
    flex: 1;
  }
}
</style>
