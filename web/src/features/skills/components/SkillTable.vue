<template>
  <q-table
    flat
    bordered
    class="skill-table"
    row-key="id"
    :rows="rows"
    :columns="columns"
    :loading="loading"
    :pagination="tablePagination"
    hide-pagination
  >
    <template #body-cell-name="props">
      <q-td :props="props">
        <div class="text-weight-medium">{{ props.row.name }}</div>
        <div class="text-caption text-grey-7">{{ props.row.slug }}</div>
      </q-td>
    </template>

    <template #body-cell-tags="props">
      <q-td :props="props">
        <div class="row q-gutter-xs">
          <q-chip v-for="tag in props.row.tags" :key="tag.name" dense :outline="tag.source === 'system'" color="primary" text-color="white">
            {{ tag.name }}
          </q-chip>
          <span v-if="!props.row.tags?.length" class="text-caption text-grey-6">无标签</span>
        </div>
      </q-td>
    </template>

    <template #body-cell-status="props">
      <q-td :props="props">
        <q-badge rounded :color="statusColor(props.row.status)">{{ statusLabel(props.row.status) }}</q-badge>
        <div class="text-caption text-grey-7 q-mt-xs">{{ props.row.current_version?.version ?? "无版本" }}</div>
      </q-td>
    </template>

    <template #body-cell-enabled="props">
      <q-td :props="props">
        <q-toggle
          dense
          color="primary"
          :model-value="props.row.enabled"
          :disable="!props.row.permissions.can_toggle_enabled || togglingId === props.row.id"
          @update:model-value="emit('toggle-enabled', props.row, Boolean($event))"
        />
      </q-td>
    </template>

    <template #body-cell-stats="props">
      <q-td :props="props">
        <skill-stats-strip :skill="props.row" />
      </q-td>
    </template>

    <template #body-cell-last="props">
      <q-td :props="props">
        <div>{{ props.row.last_agent_display_name || "未调用" }}</div>
        <div class="text-caption text-grey-7">{{ formatDate(props.row.last_invoked_at) }}</div>
      </q-td>
    </template>

    <template #body-cell-actions="props">
      <q-td :props="props" class="q-gutter-xs">
        <q-btn flat dense round color="primary" icon="edit" :disable="!props.row.permissions.can_edit" @click="emit('edit', props.row)">
          <q-tooltip>编辑 Skill 文件</q-tooltip>
        </q-btn>
        <q-btn flat dense round color="negative" icon="delete" :disable="!props.row.permissions.can_delete" @click="emit('delete', props.row)">
          <q-tooltip>删除</q-tooltip>
        </q-btn>
      </q-td>
    </template>
  </q-table>
</template>

<script setup lang="ts">
import type { QTableColumn } from "quasar";
import SkillStatsStrip from "./SkillStatsStrip.vue";
import type { Skill } from "../types";

defineProps<{
  rows: Skill[];
  loading: boolean;
  togglingId?: string;
}>();

const emit = defineEmits<{
  "toggle-enabled": [skill: Skill, enabled: boolean];
  edit: [skill: Skill];
  delete: [skill: Skill];
}>();

const tablePagination = { rowsPerPage: 0 };

const columns: QTableColumn<Skill>[] = [
  { name: "name", label: "名称", field: "name", align: "left" },
  { name: "description", label: "描述", field: "description", align: "left", style: "max-width: 260px; white-space: normal;" },
  { name: "tags", label: "标签", field: "tags", align: "left" },
  { name: "status", label: "状态 / 版本", field: "status", align: "left" },
  { name: "enabled", label: "启用", field: "enabled", align: "center" },
  { name: "stats", label: "使用统计", field: "invoke_count", align: "left" },
  { name: "last", label: "最近调用", field: "last_invoked_at", align: "left" },
  { name: "actions", label: "操作", field: "id", align: "right" }
];

function statusLabel(status: string) {
  return ({ draft: "草稿", published: "已发布", archived: "已归档" } as Record<string, string>)[status] ?? status;
}

function statusColor(status: string) {
  return status === "published" ? "positive" : status === "draft" ? "warning" : "grey";
}

function formatDate(value?: string) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}
</script>

<style scoped lang="sass">
.skill-table
  border-radius: 22px
  overflow: hidden
</style>
