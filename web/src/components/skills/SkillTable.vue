<template>
  <q-table
    flat
    dense
    class="app-registry-table skill-table"
    row-key="id"
    :rows="rows"
    :columns="columns"
    :loading="loading"
    :pagination="tablePagination"
    hide-pagination
  >
    <template #body-cell-name="props">
      <q-td :props="props">
        <div class="app-registry-cell-primary">{{ props.row.name }}</div>
        <div class="app-registry-cell-sub">{{ props.row.slug }}</div>
      </q-td>
    </template>

    <template #body-cell-description="props">
      <q-td :props="props">
        <div class="app-registry-cell-desc skill-table-desc" :title="props.row.description || ''">
          {{ props.row.description || "暂无描述" }}
        </div>
      </q-td>
    </template>

    <template #body-cell-tags="props">
      <q-td :props="props">
        <div class="app-registry-chip-wrap">
          <q-chip v-for="tag in props.row.tags" :key="tag.name" dense :outline="tag.source === 'system'" color="primary" text-color="white">
            {{ tag.name }}
          </q-chip>
          <span v-if="!props.row.tags?.length" class="text-caption text-grey-6">无标签</span>
        </div>
      </q-td>
    </template>

    <template #body-cell-status="props">
      <q-td :props="props">
        <div class="skill-status-cell">
          <q-badge rounded :color="statusColor(props.row.status)">{{ statusLabel(props.row.status) }}</q-badge>
          <span class="skill-status-cell__version">{{ props.row.current_version?.version ?? "无版本" }}</span>
        </div>
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
      <q-td :props="props">
        <div class="app-registry-cell-actions">
        <q-btn
          v-if="props.row.status === 'draft'"
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
        <q-btn flat dense round color="primary" icon="edit" :disable="!props.row.permissions.can_edit" @click="emit('edit', props.row)">
          <q-tooltip>编辑 Skill 文件</q-tooltip>
        </q-btn>
        <q-btn flat dense round color="negative" icon="delete" :disable="!props.row.permissions.can_delete" @click="emit('delete', props.row)">
          <q-tooltip>删除</q-tooltip>
        </q-btn>
        </div>
      </q-td>
    </template>
  </q-table>
</template>

<script setup lang="ts">
import type { QTableColumn } from "quasar";
import SkillStatsStrip from "./SkillStatsStrip.vue";
import type { Skill } from "../../features/skills/types";
import { registryCol } from "../../features/ui/registryTableColumns";

defineProps<{
  rows: Skill[];
  loading: boolean;
  togglingId?: string;
  /** 正在调用发布的 skill id，用于按钮 loading */
  publishingId?: string;
}>();

const emit = defineEmits<{
  "toggle-enabled": [skill: Skill, enabled: boolean];
  publish: [skill: Skill];
  edit: [skill: Skill];
  delete: [skill: Skill];
}>();

const tablePagination = { rowsPerPage: 0 };

const columns: QTableColumn<Skill>[] = [
  { name: "name", label: "名称", field: "name", align: "left", ...registryCol.name },
  { name: "description", label: "描述", field: "description", align: "left", ...registryCol.desc },
  { name: "tags", label: "标签", field: "tags", align: "left", ...registryCol.chips },
  { name: "status", label: "状态 / 版本", field: "status", align: "left", ...registryCol.status },
  { name: "enabled", label: "启用", field: "enabled", align: "center", ...registryCol.toggle },
  { name: "stats", label: "使用统计", field: "invoke_count", align: "left", style: "width: 248px; min-width: 220px" },
  { name: "last", label: "最近调用", field: "last_invoked_at", align: "left", ...registryCol.time },
  { name: "actions", label: "操作", field: "id", align: "right", ...registryCol.actions }
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
  background: transparent

  :deep(thead tr th),
  :deep(tbody tr td)
    vertical-align: middle

  :deep(thead tr th)
    padding-top: 10px
    padding-bottom: 10px
    font-size: 12px

  :deep(tbody tr td)
    padding-top: 12px
    padding-bottom: 12px

.skill-table-desc
  max-width: 100%

.skill-status-cell
  display: flex
  flex-direction: column
  align-items: flex-start
  gap: 4px

.skill-status-cell__version
  font-size: 11px
  line-height: 1.3
  color: var(--color-text-secondary)
</style>
