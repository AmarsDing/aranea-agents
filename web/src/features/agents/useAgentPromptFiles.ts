import { computed, reactive, ref, watch, type Ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { AgentPromptFile } from './types';
import { useAgentDetailStore } from '../../stores/agents/detail';
import { defaultAgentFiles, isRemovableAgentFile, type AgentFile } from '../../components/agents/agentUi';

// PGO-1: only non-optional files are created by default.
const coreAgentFiles = defaultAgentFiles.filter((f) => !f.optional);

type NotifyFn = (opts: { type: string; message: string }) => void;

/** Prompt file editor state for Agent settings. */
export function useAgentPromptFiles(agentId: Ref<string>, notify: NotifyFn) {
  const { t } = useI18n();
  const detailStore = useAgentDetailStore();
  const fileSplitter = ref(28);
  const activeFile = ref('AGENTS_CORE.md');
  const initialFileBodies = ref<Record<string, string>>({});
  const fileTokenByName = ref<Record<string, number>>({});

  // PGO-1: start with core (non-optional) files only.
  const files = reactive<AgentFile[]>(coreAgentFiles.map((file) => ({ ...file })));

  const activeFileMeta = computed(() => files.find((file) => file.name === activeFile.value) ?? files[0]);

  const activeFileBody = computed({
    get: () => activeFileMeta.value?.body ?? '',
    set: (value: string) => {
      const row = activeFileMeta.value;
      if (row) row.body = value;
    },
  });

  /**
   * Aggregate dirty check across ALL files: body changes plus removed files
   * (a removal only takes effect on save, so it must mark the form dirty).
   */
  const fileDirty = computed(() => {
    const initial = initialFileBodies.value;
    const currentNames = new Set(files.map((file) => file.name));
    if (Object.keys(initial).some((name) => !currentNames.has(name))) return true;
    return files.some((file) => file.body !== (initial[file.name] ?? ''));
  });

  /**
   * PGO-1: optional files defined in defaultAgentFiles that haven't been added yet.
   * Used to populate "Add optional file" picker.
   */
  const availableOptionalFiles = computed(() =>
    defaultAgentFiles.filter((f) => f.optional && !files.some((existing) => existing.name === f.name)),
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

  /**
   * removeFile drops an optional/custom file from the working set. Core files
   * cannot be removed. The removal only takes effect after save (the backend
   * ReplaceAgentPromptFiles rewrites the full set).
   */
  function removeFile(name: string) {
    if (!isRemovableAgentFile(name)) return;
    const index = files.findIndex((f) => f.name === name);
    if (index < 0) return;
    files.splice(index, 1);
    if (activeFile.value === name) {
      activeFile.value = files[0]?.name ?? '';
    }
  }

  function snapshotFiles() {
    initialFileBodies.value = Object.fromEntries(files.map((file) => [file.name, file.body]));
  }

  function updateFileBody(name: string, body: string) {
    const file = files.find((item) => item.name === name);
    if (file) file.body = body;
  }

  /**
   * Reload the active file from the server: pull the latest persisted body and
   * overwrite both the editor and the local snapshot. Falls back to the local
   * snapshot when the agent has no id yet (unsaved new agent).
   */
  async function reloadActiveFile() {
    const id = String(agentId.value ?? '').trim();
    const name = activeFile.value;
    if (!id) {
      activeFileBody.value = initialFileBodies.value[name] ?? activeFileBody.value;
      return;
    }
    try {
      const agent = await detailStore.fetchById(id);
      const latest = (agent.files ?? []).find((file) => file.name === name);
      if (!latest) {
        notify({ type: 'warning', message: t('agentSettings.files.reloadNotFound') });
        return;
      }
      updateFileBody(name, latest.body);
      initialFileBodies.value[name] = latest.body;
    } catch (e) {
      notify({ type: 'negative', message: e instanceof Error ? e.message : t('agentSettings.files.reloadFailed') });
    }
  }

  function hydrateFiles(savedFiles: AgentPromptFile[]) {
    // Reset to the core template set first so entries from a previously loaded
    // agent (route switch / reloadAgent) do not leak into the current one.
    files.splice(0, files.length, ...coreAgentFiles.map((file) => ({ ...file })));
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
        files.push({ id: saved.id, name: saved.name, caption: '自定义 Prompt 文件', body: saved.body });
      }
    }
    if (!files.some((file) => file.name === activeFile.value)) {
      activeFile.value = files[0]?.name ?? '';
    }
  }

  async function refreshFileTokenEstimates(formId: string) {
    const id = String(formId ?? '').trim();
    if (!id) {
      fileTokenByName.value = {};
      return;
    }
    try {
      const est = await detailStore.estimateTokens(id);
      const byName: Record<string, number> = {};
      for (const row of est.file_estimates) {
        const name = String(row.file_name ?? '').trim();
        if (name) byName[name] = row.estimated_tokens;
      }
      fileTokenByName.value = byName;
    } catch {
      fileTokenByName.value = {};
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
    fileTokenByName,
    activeFileBody,
    activeFileMeta,
    fileDirty,
    availableOptionalFiles,
    addOptionalFile,
    removeFile,
    snapshotFiles,
    updateFileBody,
    reloadActiveFile,
    hydrateFiles,
    refreshFileTokenEstimates,
    filesForSave,
  };
}

/** Call when files tab is selected to refresh token estimates. */
export function useAgentPromptFilesTabWatcher(tab: Ref<string>, formId: Ref<string>, refresh: (id: string) => void) {
  watch(tab, (name) => {
    if (name === 'files') void refresh(formId.value);
  });
}
