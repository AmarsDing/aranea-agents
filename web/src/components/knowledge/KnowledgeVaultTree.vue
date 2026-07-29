<template>
  <q-card flat class="app-pane-card knowledge-vault-tree">
    <div class="app-pane-card__header knowledge-vault-tree__header">
      <q-btn-dropdown flat no-caps dense class="knowledge-vault-tree__switcher">
        <template #label>
          <div class="row items-center no-wrap">
            <q-icon name="inventory_2" size="18px" class="q-mr-xs" />
            <span class="ellipsis">{{ currentVault?.name || t('knowledgePage.vaultSwitcherEmpty') }}</span>
            <q-chip
              v-if="currentVault"
              dense
              size="sm"
              class="q-ml-xs"
              :color="syncColor(currentVault.sync_state)"
              text-color="white"
            >
              {{ syncLabel(currentVault.sync_state) }}
            </q-chip>
          </div>
        </template>
        <q-list dense>
          <q-item
            v-for="col in collections"
            :key="col.id"
            v-close-popup
            clickable
            :active="selectedVaultId === col.id"
            active-class="text-primary"
            @click="$emit('select-vault', col.id)"
          >
            <q-item-section>
              <q-item-label>{{ col.name }}</q-item-label>
              <q-item-label caption>
                {{ t('knowledgePage.collectionCounts', { docs: col.document_count, chunks: col.chunk_count }) }}
              </q-item-label>
            </q-item-section>
            <q-item-section side>
              <q-chip dense size="sm" :color="syncColor(col.sync_state)" text-color="white">
                {{ syncLabel(col.sync_state) }}
              </q-chip>
            </q-item-section>
          </q-item>
        </q-list>
      </q-btn-dropdown>
      <q-btn
        v-if="currentVault"
        flat
        dense
        round
        size="sm"
        icon="delete_outline"
        color="negative"
        :aria-label="t('knowledgePage.vaultDeleteAria')"
        @click="$emit('delete-vault')"
      />
    </div>

    <div v-if="currentVault?.root_path" class="knowledge-vault-tree__root-path ellipsis" :title="currentVault.root_path">
      {{ currentVault.root_path }}
    </div>

    <q-banner v-if="error" dense rounded class="app-banner-warning q-ma-sm">
      {{ t('knowledgePage.vaultTreeError') }}
    </q-banner>

    <div class="app-pane-card__body knowledge-vault-tree__body">
      <q-list dense>
        <q-item
          clickable
          :active="selectedPrefix === ''"
          active-class="bg-primary text-white"
          @click="$emit('select-prefix', '')"
        >
          <q-item-section avatar><q-icon name="home" /></q-item-section>
          <q-item-section>{{ t('knowledgePage.vaultRoot') }}</q-item-section>
        </q-item>
      </q-list>
      <q-tree
        v-if="nodes.length"
        :nodes="nodes"
        node-key="key"
        label-key="label"
        dense
        :selected="selectedPrefix || null"
        @update:selected="onTreeSelect"
        @lazy-load="onLazy"
      />
      <div v-else-if="!loading && !error" class="text-caption text-grey-6 q-pa-sm">
        {{ t('knowledgePage.vaultTreeEmpty') }}
      </div>
      <q-linear-progress v-if="loading" indeterminate color="primary" />
    </div>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { KnowledgeCollection } from '../../features/knowledge/types';
import type { VaultLazyLoadPayload, VaultQTreeNode } from '../../features/knowledge/useVaultExplorer';

const props = defineProps<{
  collections: KnowledgeCollection[];
  selectedVaultId: string;
  nodes: VaultQTreeNode[];
  selectedPrefix: string;
  loading: boolean;
  error: string;
}>();

const emit = defineEmits<{
  'select-vault': [id: string];
  'select-prefix': [prefix: string];
  'lazy-load': [payload: VaultLazyLoadPayload];
  'delete-vault': [];
}>();

const { t } = useI18n();

const currentVault = computed(() => props.collections.find((c) => c.id === props.selectedVaultId));

function onTreeSelect(key: string | null) {
  if (key) emit('select-prefix', key);
}

function onLazy(payload: VaultLazyLoadPayload) {
  emit('lazy-load', payload);
}

function syncColor(state: string): string {
  if (state === 'active') return 'positive';
  if (state === 'error') return 'negative';
  if (state === 'migrating') return 'warning';
  return 'grey';
}

function syncLabel(state: string): string {
  if (state === 'active') return t('knowledgePage.vaultSyncActive');
  if (state === 'paused') return t('knowledgePage.vaultSyncPaused');
  if (state === 'error') return t('knowledgePage.vaultSyncError');
  if (state === 'migrating') return t('knowledgePage.vaultSyncMigrating');
  return state || t('knowledgePage.vaultSyncActive');
}
</script>

<style lang="scss" scoped>
.knowledge-vault-tree {
  display: flex;
  flex-direction: column;
  min-height: 0;

  &__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 4px;
  }

  &__switcher {
    min-width: 0;
    flex: 1;

    :deep(.q-btn__content) {
      min-width: 0;
    }
  }

  &__root-path {
    padding: 0 12px 6px;
    font-size: 11px;
    color: var(--q-grey-6, #757575);
  }

  &__body {
    overflow-y: auto;
    min-height: 120px;
  }
}
</style>
