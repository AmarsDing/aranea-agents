<template>
  <q-expansion-item v-if="sessionId" dense expand-separator icon="inventory_2" label="会话制品" :caption="`${items.length} 项`">
    <ArtifactList :items="items" :loading="loading" @open="$emit('open', $event)" />
  </q-expansion-item>
</template>

<script setup lang="ts">
import ArtifactList from "../../features/artifact/ArtifactList.vue";
import type { ArtifactMeta } from "../../features/artifact/types";

defineProps<{
  sessionId: string;
  items: ArtifactMeta[];
  loading?: boolean;
}>();

defineEmits<{
  open: [id: string];
}>();
</script>
