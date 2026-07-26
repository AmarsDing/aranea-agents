<template>
  <q-card flat class="app-pane-card knowledge-doc-list">
    <div class="app-pane-card__header knowledge-doc-list__header">
      <div class="row items-center no-wrap ellipsis">
        <q-icon name="folder_open" size="18px" class="q-mr-xs" />
        <span class="ellipsis">{{ prefix || t('knowledgePage.vaultRoot') }}</span>
        <span v-if="dirCount" class="text-caption text-grey-6 q-ml-xs">
          {{ t('knowledgePage.docListSubdirs', { count: dirCount }) }}
        </span>
      </div>
      <div class="row items-center no-wrap">
        <q-btn
          flat
          dense
          round
          size="sm"
          icon="note_add"
          :aria-label="t('knowledgePage.pasteText')"
          @click="$emit('ingest')"
        >
          <q-tooltip>{{ t('knowledgePage.pasteText') }}</q-tooltip>
        </q-btn>
        <q-btn flat dense round size="sm" icon="refresh" :aria-label="t('knowledgePage.refreshAria')" @click="$emit('refresh')">
          <q-tooltip>{{ t('knowledgePage.refreshAria') }}</q-tooltip>
        </q-btn>
      </div>
    </div>

    <div class="app-pane-card__body knowledge-doc-list__body">
      <q-list v-if="files.length" separator dense>
        <q-item
          v-for="f in files"
          :key="f.doc_id || f.path"
          clickable
          :active="selectedDocId === f.doc_id"
          active-class="bg-primary text-white"
          class="knowledge-doc-list__item"
          @click="$emit('select', f)"
        >
          <q-item-section avatar>
            <q-icon name="insert_drive_file" />
          </q-item-section>
          <q-item-section>
            <q-item-label lines="1">{{ f.name }}</q-item-label>
            <q-item-label caption :class="selectedDocId === f.doc_id ? 'text-white' : ''">
              <q-chip v-if="f.doc_type" dense size="sm" class="q-mr-xs">{{ f.doc_type }}</q-chip>
              {{ formatKnowledgeDocSize(f.size_bytes) }} · {{ formatKnowledgeTime(f.updated_at) }}
            </q-item-label>
            <q-item-label v-if="f.tags?.length" caption lines="1" :class="selectedDocId === f.doc_id ? 'text-white' : ''">
              <q-chip v-for="tag in f.tags.slice(0, 3)" :key="tag" dense size="sm" outline class="q-mr-xs">{{ tag }}</q-chip>
              <span v-if="f.tags.length > 3">+{{ f.tags.length - 3 }}</span>
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <q-chip dense size="sm" :color="statusColor(f.status)" text-color="white">{{ f.status }}</q-chip>
          </q-item-section>
          <q-tooltip v-if="f.summary" max-width="320px" anchor="center left" self="center right">
            {{ f.summary }}
          </q-tooltip>
        </q-item>
      </q-list>
      <div v-else-if="!loading" class="app-registry-empty app-registry-empty--compact">
        <q-icon name="drafts" size="40px" color="grey-6" />
        <div class="text-body2">{{ t('knowledgePage.docListEmpty') }}</div>
      </div>
      <q-card-section v-else>
        <q-skeleton v-for="i in 3" :key="i" type="rect" height="40px" class="q-mb-sm" />
      </q-card-section>
    </div>
  </q-card>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { VaultTreeNode } from '../../features/knowledge/types';
import { formatKnowledgeDocSize, formatKnowledgeTime, knowledgeStatusColor } from '../../features/knowledge/knowledgeUi';

defineProps<{
  prefix: string;
  files: VaultTreeNode[];
  dirCount: number;
  loading: boolean;
  selectedDocId: string;
}>();

defineEmits<{
  select: [node: VaultTreeNode];
  refresh: [];
  ingest: [];
}>();

const { t } = useI18n();
const statusColor = knowledgeStatusColor;
</script>

<style lang="scss" scoped>
.knowledge-doc-list {
  display: flex;
  flex-direction: column;
  min-height: 0;

  &__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  &__body {
    overflow-y: auto;
    min-height: 200px;
    max-height: 520px;
  }

  &__item {
    position: relative;
  }
}
</style>
