<template>
  <q-dialog :model-value="modelValue" maximized persistent @update:model-value="onDialogUpdate">
    <q-card class="app-dialog-card app-glass-dialog app-maximized-dialog">
      <q-card-section class="row items-center justify-between q-pb-sm">
        <div>
          <div class="text-h6">编辑 Skill 文件</div>
          <div class="text-caption text-grey-7">{{ skill?.name }}</div>
        </div>
        <q-btn flat round dense icon="close" @click="tryClose" />
      </q-card-section>
      <q-separator />
      <q-card-section class="app-dual-column-editor">
        <q-card flat bordered class="app-dual-column-editor__sidebar app-glass-panel">
          <q-list separator>
            <q-item v-if="filesLoading">
              <q-item-section>正在加载文件...</q-item-section>
            </q-item>
            <q-item v-for="file in files" :key="file.path" clickable :active="file.path === selectedFile?.path" @click="selectFile(file.path)">
              <q-item-section avatar>
                <q-icon :name="fileIcon(file.language)" />
              </q-item-section>
              <q-item-section>
                <q-item-label>{{ file.name }}</q-item-label>
                <q-item-label caption>{{ file.path }}</q-item-label>
              </q-item-section>
            </q-item>
            <q-item v-if="!filesLoading && files.length === 0">
              <q-item-section>该 Skill 暂无可编辑文件</q-item-section>
            </q-item>
          </q-list>
        </q-card>
        <q-card flat bordered class="app-dual-column-editor__main app-glass-panel">
          <q-card-section class="app-dual-column-editor__toolbar row items-center justify-between">
            <div>
              <div class="text-subtitle1">{{ selectedFile?.path || "请选择文件" }}</div>
              <div class="text-caption text-grey-7">
                {{ selectedFile?.language || "text" }}
                <span v-if="hasChanges"> · 有未保存修改</span>
              </div>
            </div>
            <div class="row q-gutter-sm">
              <q-btn flat rounded icon="undo" label="取消" :disable="!hasChanges || savingFile" @click="cancelEdit" />
              <q-btn color="primary" rounded unelevated icon="save" label="保存" :disable="!selectedFile || !hasChanges || readingFile" :loading="savingFile" @click="saveFile" />
            </div>
          </q-card-section>
          <q-separator />
          <q-card-section class="q-pa-none">
            <q-input
              v-model="content"
              type="textarea"
              borderless
              autogrow
              class="app-dual-column-editor__textarea"
              :disable="!selectedFile || readingFile"
              :placeholder="readingFile ? '正在读取文件...' : '选择左侧文件后编辑内容'"
            />
          </q-card-section>
        </q-card>
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useQuasar } from "quasar";
import type { Skill, SkillFile, SkillFileContent } from "../../features/skills/types";

const props = defineProps<{
  modelValue: boolean;
  skill: Skill | null;
  /** 由 Page 绑定 `features/skills/api`，展示层不直连 API */
  listSkillFiles: (id: string) => Promise<SkillFile[]>;
  readSkillFile: (id: string, path: string) => Promise<SkillFileContent>;
  updateSkillFile: (id: string, path: string, content: string) => Promise<SkillFileContent>;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

const $q = useQuasar();
const files = ref<SkillFile[]>([]);
const selectedFile = ref<SkillFile | null>(null);
const content = ref("");
const originalContent = ref("");
const filesLoading = ref(false);
const readingFile = ref(false);
const savingFile = ref(false);

const hasChanges = computed(() => selectedFile.value !== null && content.value !== originalContent.value);

watch(
  () => [props.modelValue, props.skill?.id] as const,
  ([open]) => {
    if (open && props.skill) {
      void loadFiles();
    }
    if (!open) {
      resetEditor();
    }
  }
);

async function loadFiles() {
  if (!props.skill) return;
  resetEditor();
  filesLoading.value = true;
  try {
    files.value = await props.listSkillFiles(props.skill.id);
    const preferred = files.value.find((file) => file.path.toLowerCase() === "skill.md") ?? files.value[0];
    if (preferred) {
      await selectFile(preferred.path);
    }
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "加载 Skill 文件失败" });
  } finally {
    filesLoading.value = false;
  }
}

async function selectFile(path: string) {
  if (!props.skill) return;
  const file = files.value.find((item) => item.path === path);
  if (!file) return;
  selectedFile.value = file;
  readingFile.value = true;
  try {
    const data = await props.readSkillFile(props.skill.id, path);
    content.value = data.content;
    originalContent.value = data.content;
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "读取文件失败" });
  } finally {
    readingFile.value = false;
  }
}

async function saveFile() {
  if (!props.skill || !selectedFile.value || !hasChanges.value) return;
  savingFile.value = true;
  try {
    const data = await props.updateSkillFile(props.skill.id, selectedFile.value.path, content.value);
    content.value = data.content;
    originalContent.value = data.content;
    $q.notify({ type: "positive", message: "文件已保存" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存文件失败" });
  } finally {
    savingFile.value = false;
  }
}

function cancelEdit() {
  content.value = originalContent.value;
}

function resetEditor() {
  files.value = [];
  selectedFile.value = null;
  content.value = "";
  originalContent.value = "";
  filesLoading.value = false;
  readingFile.value = false;
  savingFile.value = false;
}

function fileIcon(language: string) {
  return language === "markdown" ? "description" : language === "python" ? "data_object" : language === "javascript" || language === "typescript" ? "code" : "insert_drive_file";
}

function tryClose() {
  if (hasChanges.value) {
    $q.dialog({
      title: "未保存的更改",
      message: "当前文件有未保存的修改，确定要关闭吗？",
      cancel: { label: "继续编辑", flat: true, noCaps: true },
      ok: { label: "放弃更改", noCaps: true, color: "negative" },
      persistent: true,
    }).onOk(() => {
      emit("update:modelValue", false);
    });
  } else {
    emit("update:modelValue", false);
  }
}

function onDialogUpdate(val: boolean) {
  if (!val) {
    tryClose();
  }
}
</script>
