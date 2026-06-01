import { ref } from "vue";
import { useQuasar } from "quasar";
import { useRouter } from "vue-router";
import type { GraphDefinition, GraphVersionInfo } from "./types";
import { useGraphStore } from "../../stores/graph";

export function useGraphEditorAssets(graphDef: GraphDefinition, isNew: () => boolean) {
  const $q = useQuasar();
  const router = useRouter();
  const graphStore = useGraphStore();

  const versionDialogOpen = ref(false);
  const versions = ref<GraphVersionInfo[]>([]);
  const versionsLoading = ref(false);
  const rollingBackVersion = ref<number | null>(null);
  const templateDialogOpen = ref(false);
  const templateName = ref("");
  const templateCategory = ref("custom");
  const templateSaving = ref(false);
  const importInputRef = ref<HTMLInputElement | null>(null);

  async function openVersionDialog() {
    if (!graphDef.id) {
      $q.notify({ type: "warning", message: "请先保存 Graph" });
      return;
    }
    versionDialogOpen.value = true;
    versionsLoading.value = true;
    try {
      versions.value = await graphStore.fetchGraphVersions(graphDef.id);
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "加载版本失败" });
    } finally {
      versionsLoading.value = false;
    }
  }

  async function rollbackVersion(version: number, onRestored?: () => Promise<void>) {
    if (!graphDef.id) return;
    rollingBackVersion.value = version;
    try {
      const restored = await graphStore.rollbackGraph(graphDef.id, version);
      Object.assign(graphDef, restored);
      versionDialogOpen.value = false;
      $q.notify({ type: "positive", message: `已回滚到 v${version}` });
      await onRestored?.();
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "回滚失败" });
    } finally {
      rollingBackVersion.value = null;
    }
  }

  async function exportCurrentGraph() {
    if (!graphDef.id) {
      $q.notify({ type: "warning", message: "请先保存 Graph" });
      return;
    }
    try {
      const result = await graphStore.exportGraphDefinition(graphDef.id);
      const blob = new Blob([result.json], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = `${graphDef.name || graphDef.id}.graph.json`;
      anchor.click();
      URL.revokeObjectURL(url);
      $q.notify({ type: "positive", message: "Graph 已导出" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "导出失败" });
    }
  }

  function triggerImport() {
    importInputRef.value?.click();
  }

  async function onImportFile(event: Event) {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    input.value = "";
    if (!file) return;
    try {
      const json = await file.text();
      const created = await graphStore.importGraphDefinition(json, graphDef.name || file.name.replace(/\.json$/i, ""), graphDef.description);
      Object.assign(graphDef, created);
      $q.notify({ type: "positive", message: "Graph 已导入" });
      router.replace({ name: "graph-editor", params: { id: created.id } });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "导入失败" });
    }
  }

  function openTemplateDialog() {
    if (!graphDef.id) {
      $q.notify({ type: "warning", message: "请先保存 Graph" });
      return;
    }
    templateName.value = graphDef.name ? `${graphDef.name} 模板` : "";
    templateCategory.value = "custom";
    templateDialogOpen.value = true;
  }

  async function saveTemplate() {
    if (!graphDef.id || !templateName.value.trim()) return;
    templateSaving.value = true;
    try {
      await graphStore.saveAsTemplate(graphDef.id, templateName.value.trim(), templateCategory.value, graphDef.description);
      templateDialogOpen.value = false;
      $q.notify({ type: "positive", message: "已保存为用户模板" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存模板失败" });
    } finally {
      templateSaving.value = false;
    }
  }

  return {
    versionDialogOpen,
    versions,
    versionsLoading,
    rollingBackVersion,
    templateDialogOpen,
    templateName,
    templateCategory,
    templateSaving,
    importInputRef,
    openVersionDialog,
    rollbackVersion,
    exportCurrentGraph,
    triggerImport,
    onImportFile,
    openTemplateDialog,
    saveTemplate,
  };
}
