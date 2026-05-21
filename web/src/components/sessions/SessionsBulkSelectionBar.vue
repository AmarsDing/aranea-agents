<template>
  <q-card flat class="app-registry-panel app-info-banner q-mb-md">
    <q-card-section class="app-actions-bar app-actions-bar--start">
      <div class="text-body2 text-weight-medium">已选 {{ count }} 项</div>
      <q-space />
      <q-btn flat rounded no-caps icon="inventory_2" label="归档" :loading="archiving" @click="$emit('archive-selected')" />
      <q-btn flat rounded no-caps icon="delete" label="删除" color="negative" :loading="deleting" @click="$emit('delete-selected')" />
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
defineProps<{
  count: number;
  archiving?: boolean;
  deleting?: boolean;
}>();

defineEmits<{
  "archive-selected": [];
  "delete-selected": [];
}>();
</script>
