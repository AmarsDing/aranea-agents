<template>
  <q-card flat bordered class="sessions-bulk-selection q-mb-md">
    <q-card-section class="row items-center q-gutter-md">
      <div class="text-body2 text-weight-medium">已选 {{ count }} 项</div>
      <q-space />
      <q-btn flat rounded icon="inventory_2" label="归档" class="sessions-btn-ghost" :loading="archiving" @click="$emit('archive-selected')" />
      <q-btn flat rounded icon="delete" label="删除" color="negative" :loading="deleting" @click="$emit('delete-selected')" />
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
