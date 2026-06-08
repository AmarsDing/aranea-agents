<template>
  <q-banner v-if="visible" rounded class="skill-fs-alert q-mb-md" :class="bannerClass">
    <template #avatar>
      <q-icon :name="icon" />
    </template>
    <div class="text-body2">{{ message }}</div>
    <div v-if="health?.resolved_root" class="skill-fs-alert__path text-caption q-mt-xs">
      Skill 根目录：{{ health.resolved_root }}
    </div>
    <template #action>
      <q-btn
        v-if="health && health.pending_filesystem_count > 0"
        flat
        no-caps
        color="primary"
        label="仅看待审核"
        @click="emit('filter-pending')"
      />
      <q-btn
        v-if="health && health.missing_count > 0"
        flat
        no-caps
        color="primary"
        label="仅看磁盘缺失"
        @click="emit('filter-missing')"
      />
    </template>
  </q-banner>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { SkillFilesystemHealth } from '../../features/skills/types';

const props = defineProps<{
  health: SkillFilesystemHealth | null;
}>();

const emit = defineEmits<{
  'filter-pending': [];
  'filter-missing': [];
}>();

const visible = computed(() => {
  const h = props.health;
  if (!h) return false;
  return !h.root_accessible || h.missing_count > 0 || h.pending_filesystem_count > 0;
});

const bannerClass = computed(() => {
  if (!props.health?.root_accessible || (props.health?.missing_count ?? 0) > 0) {
    return 'skill-fs-alert--warning';
  }
  return 'skill-fs-alert--info';
});

const icon = computed(() => {
  if (!props.health?.root_accessible || (props.health?.missing_count ?? 0) > 0) {
    return 'warning';
  }
  return 'folder_open';
});

const message = computed(() => {
  const h = props.health;
  if (!h) return '';
  const parts: string[] = [];
  if (!h.root_accessible) {
    parts.push('Skill 根目录不可访问，请检查系统设置中的工作目录。');
  }
  if (h.missing_count > 0) {
    parts.push(`${h.missing_count} 个 Skill 磁盘目录缺失。`);
  }
  if (h.pending_filesystem_count > 0) {
    parts.push(`${h.pending_filesystem_count} 个磁盘导入 Skill 待发布/启用。`);
  }
  return parts.join(' ');
});
</script>
