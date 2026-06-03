<template>
  <q-card flat class="app-pane-card knowledge-list-card">
    <div class="app-pane-card__header">集合</div>
    <q-list v-if="collections.length" separator class="app-pane-card__body">
      <q-item
        v-for="col in collections"
        :key="col.id"
        clickable
        :active="selectedId === col.id"
        active-class="bg-primary text-white"
        @click="$emit('select', col.id)"
      >
        <q-item-section>
          <q-item-label>{{ col.name }}</q-item-label>
          <q-item-label caption :class="selectedId === col.id ? 'text-white' : ''">
            {{ col.embedding_model }} · {{ col.document_count }} 文档
          </q-item-label>
        </q-item-section>
        <q-item-section side>
          <q-chip dense :color="statusColor(col.status)" text-color="white" size="sm">{{ col.status }}</q-chip>
        </q-item-section>
      </q-item>
    </q-list>
    <div v-else-if="!loading" class="app-registry-empty app-registry-empty--compact app-pane-card__body">
      <q-icon name="folder_open" size="40px" color="grey-6" />
      <div class="text-subtitle1">暂无集合</div>
      <div class="text-body2">点击右上角「新建集合」开始。</div>
    </div>
    <q-card-section v-else class="app-pane-card__body">
      <q-skeleton type="rect" height="48px" class="q-mb-sm" />
      <q-skeleton type="rect" height="48px" />
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import type { KnowledgeCollection } from '../../features/knowledge/types';
import { knowledgeStatusColor } from '../../features/knowledge/knowledgeUi';

defineProps<{
  collections: KnowledgeCollection[];
  selectedId: string;
  loading: boolean;
}>();

defineEmits<{ select: [id: string] }>();

const statusColor = knowledgeStatusColor;
</script>
