<template>
  <q-card flat class="app-registry-panel app-glass-panel--dense">
    <q-card-section class="app-actions-bar app-actions-bar--start q-py-sm">
      <q-btn
        flat
        rounded
        no-caps
        :icon="selectionMode ? 'close' : 'checklist'"
        :label="selectionMode ? '取消选择' : '批量选择'"
        :color="selectionMode ? 'primary' : undefined"
        @click="$emit('toggle-selection')"
      />
      <q-separator vertical inset class="q-mx-xs" />
      <q-btn flat rounded no-caps icon="inventory_2" label="按天数归档" @click="$emit('retention-archive')" />
      <q-btn flat rounded no-caps icon="delete_sweep" label="按天数删除" @click="$emit('retention-delete')" />
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
defineProps<{
  selectionMode: boolean;
}>();

defineEmits<{
  'toggle-selection': [];
  'retention-archive': [];
  'retention-delete': [];
}>();
</script>
