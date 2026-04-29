<template>
  <q-card flat bordered class="skill-filter-card">
    <q-card-section class="row q-col-gutter-sm items-center">
      <div class="col-12 col-md-5">
        <q-input :model-value="search" dense outlined clearable debounce="350" placeholder="搜索 Skill 名称、Slug、描述..." @update:model-value="emit('update:search', String($event ?? ''))">
          <template #prepend><q-icon name="search" /></template>
        </q-input>
      </div>
      <div class="col-12 col-sm-6 col-md-2">
        <q-select :model-value="enabled" dense outlined clearable emit-value map-options label="启用状态" :options="enabledOptions" @update:model-value="emit('update:enabled', $event as boolean | null)" />
      </div>
      <div class="col-12 col-sm-6 col-md-2">
        <q-select :model-value="status" dense outlined clearable emit-value map-options label="状态" :options="statusOptions" @update:model-value="emit('update:status', String($event ?? ''))" />
      </div>
      <div class="col-12 col-md-3 row justify-end q-gutter-sm">
        <q-btn flat rounded icon="restart_alt" label="重置" @click="emit('reset')" />
        <q-btn flat rounded icon="refresh" label="刷新" :loading="loading" @click="emit('refresh')" />
        <q-btn outline rounded color="primary" icon="history" label="运行记录" to="/skills/runs" />
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
defineProps<{
  search: string;
  enabled: boolean | null;
  status: string;
  loading?: boolean;
}>();

const emit = defineEmits<{
  "update:search": [value: string];
  "update:enabled": [value: boolean | null];
  "update:status": [value: string];
  reset: [];
  refresh: [];
}>();

const enabledOptions = [
  { label: "仅启用", value: true },
  { label: "仅停用", value: false }
];

const statusOptions = [
  { label: "草稿", value: "draft" },
  { label: "已发布", value: "published" },
  { label: "已归档", value: "archived" }
];
</script>

<style scoped lang="sass">
.skill-filter-card
  border-radius: 22px
</style>
