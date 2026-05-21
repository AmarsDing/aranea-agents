<template>
  <q-card flat class="sessions-bulk-toolbar q-mb-md">
    <q-card-section class="row items-center q-gutter-sm">
      <q-btn
        flat
        rounded
        :icon="selectionMode ? 'close' : 'checklist'"
        :label="selectionMode ? '取消选择' : '批量选择'"
        :class="selectionMode ? 'sessions-btn-accent-outline' : 'sessions-btn-ghost'"
        @click="$emit('toggle-selection')"
      />
      <q-separator vertical inset class="q-mx-xs" />
      <q-btn flat rounded icon="inventory_2" label="按天数归档" class="sessions-btn-ghost" @click="$emit('retention-archive')" />
      <q-btn flat rounded icon="delete_sweep" label="按天数删除" class="sessions-btn-ghost" @click="$emit('retention-delete')" />
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
defineProps<{
  selectionMode: boolean;
}>();

defineEmits<{
  "toggle-selection": [];
  "retention-archive": [];
  "retention-delete": [];
}>();
</script>
