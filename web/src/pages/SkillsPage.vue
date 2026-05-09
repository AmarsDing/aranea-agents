<template>
  <q-page class="app-page-cream skills-page">
    <section class="skills-hero">
      <div>
        <div class="skills-kicker">Skill registry</div>
        <h1 class="skills-title">Skill 管理</h1>
        <p class="skills-subtitle">查看 Skill 使用频率、成功失败统计、最近调用 Agent，并维护启用状态。</p>
      </div>
    </section>

    <skill-upload-placeholder
      class="q-mb-md"
      :upload-skill-zip="uploadSkillZip"
      :get-skill-import-job="getSkillImportJob"
      :refine-skill-conflict-group="refineSkillConflictGroup"
      :apply-skill-import="applySkillImport"
      @completed="loadRows"
    />

    <skill-filter-bar
      v-model:search="search"
      v-model:enabled="enabled"
      v-model:status="status"
      :loading="loading"
      class="q-mb-md"
      @reset="resetFilters"
      @refresh="loadRows"
    />

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <q-card v-if="!loading && rows.length === 0" flat bordered class="skills-empty-card">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="psychology" />
        <div class="text-h6 q-mt-md">{{ search ? "没有匹配的 Skill" : "暂无 Skill" }}</div>
        <div class="text-body2 text-grey-7 q-mt-sm">上传能力将在后续版本启用；当前可先查看已有 Skill 与运行统计。</div>
      </q-card-section>
    </q-card>

    <skill-table
      v-else
      :rows="rows"
      :loading="loading"
      :toggling-id="togglingId"
      :publishing-id="publishingId"
      @toggle-enabled="onToggleEnabled"
      @publish="onPublishSkill"
      @edit="openEditor"
      @delete="confirmDelete"
    />

    <skill-pagination v-model:page="page" v-model:page-size="pageSize" :page-max="pageMax" :total="total" :loading="loading" label="条 Skill" />
    <skill-delete-dialog v-model="deleteOpen" :skill="deleteTarget" :loading="deleting" @confirm="deleteTargetSkill" />
    <skill-editor-dialog
      v-model="editorOpen"
      :skill="editorTarget"
      :list-skill-files="listSkillFiles"
      :read-skill-file="readSkillFile"
      :update-skill-file="updateSkillFile"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useQuasar } from "quasar";
import SkillDeleteDialog from "../components/skills/SkillDeleteDialog.vue";
import SkillEditorDialog from "../components/skills/SkillEditorDialog.vue";
import SkillFilterBar from "../components/skills/SkillFilterBar.vue";
import SkillPagination from "../components/skills/SkillPagination.vue";
import SkillTable from "../components/skills/SkillTable.vue";
import SkillUploadPlaceholder from "../components/skills/SkillUploadPlaceholder.vue";
import {
  applySkillImport,
  deleteSkill,
  getSkillImportJob,
  listSkillFiles,
  listSkills,
  publishSkill,
  readSkillFile,
  refineSkillConflictGroup,
  toggleSkillEnabled,
  updateSkillFile,
  uploadSkillZip
} from "../features/skills/api";
import type { Skill } from "../features/skills/types";

const $q = useQuasar();
const search = ref("");
const enabled = ref<boolean | null>(null);
const status = ref("");
const page = ref(1);
const pageSize = ref(20);
const rows = ref<Skill[]>([]);
const total = ref(0);
const loading = ref(false);
const error = ref("");
const togglingId = ref("");
const publishingId = ref("");
const deleteOpen = ref(false);
const deleteTarget = ref<Skill | null>(null);
const deleting = ref(false);
const editorOpen = ref(false);
const editorTarget = ref<Skill | null>(null);

const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

async function loadRows() {
  loading.value = true;
  error.value = "";
  try {
    const data = await listSkills({
      search: search.value,
      enabled: enabled.value,
      status: status.value,
      page: page.value,
      page_size: pageSize.value
    });
    rows.value = data.items;
    total.value = data.total;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载 Skill 失败";
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  search.value = "";
  enabled.value = null;
  status.value = "";
  page.value = 1;
  void loadRows();
}

async function onPublishSkill(skill: Skill) {
  publishingId.value = skill.id;
  try {
    const updated = await publishSkill(skill.id);
    rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
    $q.notify({ type: "positive", message: "Skill 已发布；请在列表中打开「启用」以便 Agent 运行时挂载" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "发布失败" });
  } finally {
    publishingId.value = "";
  }
}

async function onToggleEnabled(skill: Skill, next: boolean) {
  togglingId.value = skill.id;
  try {
    const updated = await toggleSkillEnabled(skill.id, next);
    rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
    $q.notify({ type: "positive", message: next ? "Skill 已启用" : "Skill 已停用" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "更新启用状态失败" });
  } finally {
    togglingId.value = "";
  }
}

function openEditor(skill: Skill) {
  editorTarget.value = skill;
  editorOpen.value = true;
}

function confirmDelete(skill: Skill) {
  deleteTarget.value = skill;
  deleteOpen.value = true;
}

async function deleteTargetSkill() {
  if (!deleteTarget.value) return;
  deleting.value = true;
  try {
    await deleteSkill(deleteTarget.value.id);
    deleteOpen.value = false;
    $q.notify({ type: "positive", message: "Skill 已删除" });
    await loadRows();
    if (rows.value.length === 0 && page.value > 1) {
      page.value -= 1;
      await loadRows();
    }
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "删除失败" });
  } finally {
    deleting.value = false;
  }
}

watch([search, enabled, status], () => {
  page.value = 1;
  void loadRows();
});
watch([page, pageSize], () => {
  void loadRows();
});

onMounted(loadRows);
</script>

<style scoped lang="sass">
.skills-page
  padding: 24px

.skills-hero
  display: flex
  justify-content: space-between
  gap: 16px
  align-items: flex-start
  margin-bottom: 18px

.skills-kicker
  color: var(--q-primary)
  font-size: 12px
  font-weight: 700
  letter-spacing: .12em
  text-transform: uppercase

.skills-title
  margin: 4px 0
  font-size: 34px
  line-height: 1.15

.skills-subtitle
  margin: 0
  color: var(--q-grey-7)

.skills-empty-card
  border-radius: 22px

@media (max-width: 720px)
  .skills-hero
    flex-direction: column
    align-items: stretch
</style>
