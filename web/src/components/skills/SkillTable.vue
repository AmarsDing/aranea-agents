<template>
  <AppRegistryTable
    table-class="skill-table"
    row-key="id"
    :rows="rows"
    :columns="SKILL_TABLE_COLUMNS"
    :loading="loading"
    :pagination="tablePagination"
    hide-pagination
  >
    <template #body-cell-name="props">
      <q-td :props="props">
        <AppRegistryHoverTip :text="props.row.description" empty-label="暂无描述">
          <div class="min-width-0">
            <div class="app-registry-cell-primary">{{ props.row.name }}</div>
            <div class="app-registry-cell-sub">{{ props.row.slug }}</div>
          </div>
        </AppRegistryHoverTip>
      </q-td>
    </template>

    <template #body-cell-tags="props">
      <q-td :props="props">
        <div class="app-registry-chip-wrap">
          <q-chip
            v-for="tag in props.row.tags"
            :key="tag.name"
            dense
            :outline="tag.source === 'system'"
            color="primary"
            text-color="white"
          >
            {{ tag.name }}
          </q-chip>
          <span v-if="!props.row.tags?.length" class="text-caption text-grey-6">无标签</span>
        </div>
      </q-td>
    </template>

    <template #body-cell-origin="props">
      <q-td :props="props">
        <q-chip
          v-if="props.row.sync_origin"
          dense
          size="sm"
          :outline="props.row.sync_origin !== 'filesystem'"
          color="primary"
          text-color="white"
        >
          {{ originLabel(props.row.sync_origin) }}
        </q-chip>
        <span v-else class="text-caption text-grey-6">—</span>
      </q-td>
    </template>

    <template #body-cell-disk="props">
      <q-td :props="props">
        <q-badge v-if="props.row.filesystem_missing" rounded color="negative">缺失</q-badge>
        <q-badge v-else rounded color="positive" outline>正常</q-badge>
      </q-td>
    </template>

    <template #body-cell-status="props">
      <q-td :props="props">
        <div class="skill-status-cell">
          <q-badge rounded :color="statusColor(props.row.status)">{{ statusLabel(props.row.status) }}</q-badge>
          <span class="skill-status-cell__version">{{ props.row.current_version?.version ?? '无版本' }}</span>
        </div>
      </q-td>
    </template>

    <template #body-cell-enabled="props">
      <q-td :props="props">
        <q-toggle
          dense
          color="primary"
          :model-value="props.row.enabled"
          :disable="!props.row.permissions.can_toggle_enabled || props.row.status !== 'published' || togglingId === props.row.id"
          @update:model-value="emit('toggle-enabled', props.row, Boolean($event))"
        >
          <q-tooltip v-if="props.row.status !== 'published'">仅已发布的 Skill 可启用</q-tooltip>
        </q-toggle>
      </q-td>
    </template>

    <template #body-cell-stats="props">
      <q-td :props="props">
        <skill-stats-strip :skill="props.row" />
      </q-td>
    </template>

    <template #body-cell-last="props">
      <q-td :props="props">
        <div>{{ props.row.last_agent_display_name || '未调用' }}</div>
        <div class="text-caption text-grey-7">{{ formatDate(props.row.last_invoked_at) }}</div>
      </q-td>
    </template>

    <template #body-cell-actions="props">
      <q-td :props="props">
        <div class="app-registry-cell-actions">
          <q-btn
            v-if="props.row.status !== 'published'"
            flat
            dense
            round
            color="positive"
            icon="publish"
            :disable="!props.row.permissions.can_edit || publishingId === props.row.id"
            :loading="publishingId === props.row.id"
            @click="emit('publish', props.row)"
          >
            <q-tooltip>发布（发布后才能在运行时挂载并启用）</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            color="primary"
            icon="edit"
            :disable="!props.row.permissions.can_edit"
            @click="emit('edit-meta', props.row)"
          >
            <q-tooltip>编辑元数据</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            color="primary"
            icon="folder_open"
            :disable="!props.row.permissions.can_edit"
            @click="emit('edit-files', props.row)"
          >
            <q-tooltip>编辑文件</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            color="negative"
            icon="delete"
            :disable="!props.row.permissions.can_delete"
            @click="emit('delete', props.row)"
          >
            <q-tooltip>删除</q-tooltip>
          </q-btn>
        </div>
      </q-td>
    </template>
  </AppRegistryTable>
</template>

<script setup lang="ts">
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import SkillStatsStrip from './SkillStatsStrip.vue';
import type { Skill } from '../../features/skills/types';
import {
  SKILL_TABLE_COLUMNS,
  skillStatusLabel as statusLabel,
  skillStatusColor as statusColor,
  skillOriginLabel as originLabel,
} from './skillTableUi';

defineProps<{
  rows: Skill[];
  loading: boolean;
  togglingId?: string;
  /** 正在调用发布的 skill id，用于按钮 loading */
  publishingId?: string;
}>();

const emit = defineEmits<{
  'toggle-enabled': [skill: Skill, enabled: boolean];
  publish: [skill: Skill];
  'edit-meta': [skill: Skill];
  'edit-files': [skill: Skill];
  delete: [skill: Skill];
}>();

const tablePagination = { rowsPerPage: 0 };

function formatDate(value?: string) {
  if (!value) return '-';
  return new Date(value).toLocaleString();
}
</script>
