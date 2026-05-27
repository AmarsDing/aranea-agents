import { computed, reactive, ref, watch, type Ref } from "vue";
import type { AgentPromptFile } from "./types";
import { editPromptFileByAI, estimateAgentTokens } from "./api";
import {
  defaultAgentFiles,
  type AgentFile,
} from "../../components/agents/agentUi";

// PGO-1: only non-optional files are created by default.
const coreAgentFiles = defaultAgentFiles.filter((f) => !f.optional);

type NotifyFn = (opts: { type: string; message: string }) => void;

/** Prompt file editor state for Agent settings. */
export function useAgentPromptFiles(agentId: Ref<string>, notify: NotifyFn) {
  const fileSplitter = ref(28);
  const activeFile = ref("AGENTS_CORE.md");
  const initialFileBodies = ref<Record<string, string>>({});
  const aiEditOpen = ref(false);
  const aiEditing = ref(false);
  const aiInstruction = ref("");
  const fileTokenByName = ref<Record<string, number>>({});

  // PGO-1: start with core (non-optional) files only.
  const files = reactive<AgentFile[]>(coreAgentFiles.map((file) => ({ ...file })));

  const activeFileMeta = computed(() => files.find((file) => file.name === activeFile.value) ?? files[0]);

  const activeFileBody = computed({
    get: () => activeFileMeta.value?.body ?? "",
    set: (value: string) => {
      const row = activeFileMeta.value;
      if (row) row.body = value;
    },
  });

  const fileDirty = computed(
    () => activeFileBody.value !== (initialFileBodies.value[activeFile.value] ?? ""),
  );

  /**
   * PGO-1: optional files defined in defaultAgentFiles that haven't been added yet.
   * Used to populate "Add optional file" picker.
   */
  const availableOptionalFiles = computed(() =>
    defaultAgentFiles
      .filter((f) => f.optional && !files.some((existing) => existing.name === f.name))
  );

  /**
   * addOptionalFile adds an optional file template (e.g. USER_CONTEXT.md) to the
   * active file list. Replaces the old heartbeatFile ref. (PGO-1-WEB-02)
   */
  function addOptionalFile(name: string) {
    const template = defaultAgentFiles.find((f) => f.name === name && f.optional);
    if (!template) return;
    if (files.some((f) => f.name === name)) return;
    files.push({ ...template });
    activeFile.value = name;
  }

  function snapshotFiles() {
    initialFileBodies.value = Object.fromEntries(files.map((file) => [file.name, file.body]));
  }

  function updateFileBody(name: string, body: string) {
    const file = files.find((item) => item.name === name);
    if (file) file.body = body;
  }

  function reloadActiveFile() {
    activeFileBody.value = initialFileBodies.value[activeFile.value] ?? activeFileBody.value;
  }

  function hydrateFiles(savedFiles: AgentPromptFile[]) {
    const byName = new Map(savedFiles.map((file) => [file.name, file]));
    for (const file of files) {
      const saved = byName.get(file.name);
      if (saved) {
        file.body = saved.body;
        file.id = saved.id;
      }
    }
    for (const saved of savedFiles) {
      if (!files.some((file) => file.name === saved.name)) {
        files.push({ id: saved.id, name: saved.name, caption: "自定义 Prompt 文件", body: saved.body });
      }
    }
  }

  async function refreshFileTokenEstimates(formId: string) {
    const id = String(formId ?? "").trim();
    if (!id) {
      fileTokenByName.value = {};
      return;
    }
    try {
      const est = await estimateAgentTokens(id);
      const byName: Record<string, number> = {};
      for (const row of est.file_estimates) {
        const name = String(row.file_name ?? "").trim();
        if (name) byName[name] = row.estimated_tokens;
      }
      fileTokenByName.value = byName;
    } catch {
      fileTokenByName.value = {};
    }
  }

  async function applyAiEdit(formId: string) {
    const instruction = aiInstruction.value.trim();
    const fileId = String(activeFileMeta.value?.id ?? "").trim();
    if (!instruction) {
      notify({ type: "warning", message: "请输入编辑指令" });
      return;
    }
    if (!formId || !fileId) {
      notify({ type: "warning", message: "请先保存 Agent 后再使用 AI 编辑" });
      return;
    }
    aiEditing.value = true;
    try {
      const updated = await editPromptFileByAI(formId, fileId, instruction);
      updateFileBody(activeFile.value, updated.body);
      const row = files.find((f) => f.name === activeFile.value);
      if (row) row.id = updated.id;
      snapshotFiles();
      aiInstruction.value = "";
      aiEditOpen.value = false;
      notify({ type: "positive", message: "AI 修订已应用" });
    } catch (e) {
      notify({ type: "negative", message: e instanceof Error ? e.message : "AI 编辑失败" });
    } finally {
      aiEditing.value = false;
    }
  }

  function filesForSave() {
    return files.map((file, index) => ({
      name: file.name,
      body: file.body,
      sort_order: (index + 1) * 10,
      id: file.id,
    }));
  }

  return {
    fileSplitter,
    activeFile,
    files,
    initialFileBodies,
    aiEditOpen,
    aiEditing,
    aiInstruction,
    fileTokenByName,
    activeFileBody,
    activeFileMeta,
    fileDirty,
    availableOptionalFiles,
    addOptionalFile,
    snapshotFiles,
    updateFileBody,
    reloadActiveFile,
    hydrateFiles,
    refreshFileTokenEstimates,
    applyAiEdit,
    filesForSave,
  };
}

/** Call when files tab is selected to refresh token estimates. */
export function useAgentPromptFilesTabWatcher(
  tab: Ref<string>,
  formId: Ref<string>,
  refresh: (id: string) => void,
) {
  watch(tab, (name) => {
    if (name === "files") void refresh(formId.value);
  });
}
