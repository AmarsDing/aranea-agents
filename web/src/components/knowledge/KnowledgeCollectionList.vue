<template>
  <q-card flat bordered class="knowledge-list-card">
    <q-card-section class="text-subtitle1 text-weight-bold">集合</q-card-section>
    <q-separator />
    <q-list separator>
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
    <q-card-section v-if="!loading && !collections.length" class="text-grey-7 text-center">
      暂无集合，点击「新建集合」开始。
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import type { KnowledgeCollection } from "../../features/knowledge/types";
import { knowledgeStatusColor } from "../../features/knowledge/knowledgeUi";

defineProps<{
  collections: KnowledgeCollection[];
  selectedId: string;
  loading: boolean;
}>();

defineEmits<{ select: [id: string] }>();

const statusColor = knowledgeStatusColor;
</script>
