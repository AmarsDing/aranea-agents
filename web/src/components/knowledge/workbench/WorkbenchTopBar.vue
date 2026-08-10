<template>
  <div class="kb-glass kb-topbar">
    <!-- Vault 切换 -->
    <q-btn-dropdown
      flat
      no-caps
      dense
      class="kb-topbar__vault"
      :label="currentVaultName"
      icon="inventory_2"
      :title="t('knowledgePage.workbench.vaultSwitcher')"
    >
      <q-list dense class="kb-topbar__vault-list">
        <q-item
          v-for="c in collections"
          :key="c.id"
          v-close-popup
          clickable
          :active="c.id === currentVaultId"
          @click="$emit('switch-vault', c.id)"
        >
          <q-item-section avatar>
            <q-icon :name="c.vault_backend === 'team' ? 'groups' : 'inventory_2'" size="18px" />
          </q-item-section>
          <q-item-section>{{ c.name || c.id }}</q-item-section>
          <q-item-section side>
            <span class="kb-topbar__vault-count">{{ c.document_count }}</span>
          </q-item-section>
        </q-item>
      </q-list>
    </q-btn-dropdown>

    <div class="kb-topbar__spacer" />

    <!-- 快速动作 -->
    <q-btn
      flat
      dense
      no-caps
      icon="bolt"
      :label="t('knowledgePage.workbench.quickSwitcher')"
      class="kb-topbar__action"
      @click="$emit('open-quick-switcher')"
    >
      <q-tooltip>Ctrl+O</q-tooltip>
    </q-btn>
    <q-btn
      flat
      dense
      no-caps
      icon="terminal"
      :label="t('knowledgePage.workbench.commandPalette')"
      class="kb-topbar__action"
      @click="$emit('open-command-palette')"
    >
      <q-tooltip>Ctrl+K</q-tooltip>
    </q-btn>
    <q-btn
      flat
      dense
      round
      icon="hub"
      :title="t('knowledgePage.workbench.openGraph')"
      class="kb-topbar__icon"
      @click="$emit('open-graph')"
    />
    <q-btn
      flat
      dense
      round
      icon="tune"
      :title="t('knowledgePage.workbench.openSettings')"
      class="kb-topbar__icon"
      @click="$emit('open-settings')"
    />
  </div>
</template>

<script setup lang="ts">
// SP2 §SP2-1 顶栏：Vault 切换 / ⌘O / ⌘K / 图谱 / 设置浮层入口（纯受控组件）。
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { KnowledgeCollection } from '../../../features/knowledge/types';

const props = defineProps<{
  collections: KnowledgeCollection[];
  currentVaultId: string;
}>();

defineEmits<{
  'switch-vault': [id: string];
  'open-quick-switcher': [];
  'open-command-palette': [];
  'open-graph': [];
  'open-settings': [];
}>();

const { t } = useI18n();

const currentVaultName = computed(() => props.collections.find((c) => c.id === props.currentVaultId)?.name ?? '—');
</script>

<style lang="sass" scoped>
.kb-topbar
  display: flex
  align-items: center
  gap: 6px
  padding: 6px 12px
  flex: none

  &__vault
    color: var(--kb-text-primary)
    font-weight: 600

  &__vault-list
    min-width: 220px
    background: var(--kb-bg-deep)
    border: 1px solid var(--kb-glass-border)

  &__vault-count
    font-size: 11px
    color: var(--kb-text-dim)
    font-variant-numeric: tabular-nums

  &__spacer
    flex: 1

  &__action
    color: var(--kb-text-dim)
    font-size: 12px

    &:hover
      color: var(--kb-accent-cyan)

  &__icon
    color: var(--kb-text-dim)

    &:hover
      color: var(--kb-accent-cyan)
</style>
