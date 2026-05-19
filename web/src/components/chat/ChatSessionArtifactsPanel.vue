<template>
  <q-expansion-item v-if="sessionId" dense expand-separator icon="inventory_2" label="会话制品" :caption="`${items.length} 项`">
    <q-list dense separator>
      <q-item v-for="item in items" :key="item.id" clickable @click="$emit('open', item.id)">
        <q-item-section>
          <q-item-label>{{ item.name }}</q-item-label>
          <q-item-label caption>{{ item.mime_type }} · {{ formatBytes(item.size) }}</q-item-label>
        </q-item-section>
      </q-item>
      <q-item v-if="!loading && !items.length">
        <q-item-section class="text-grey-7">暂无制品</q-item-section>
      </q-item>
    </q-list>
  </q-expansion-item>
</template>

<script setup lang="ts">
import type { ArtifactMeta } from "../../features/artifact/types";

defineProps<{
  sessionId: string;
  items: ArtifactMeta[];
  loading?: boolean;
}>();

defineEmits<{
  open: [id: string];
}>();

function formatBytes(n: number) {
  if (!n) return "0 B";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}
</script>
