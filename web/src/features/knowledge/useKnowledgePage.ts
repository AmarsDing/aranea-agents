import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useQuasar } from "quasar";
import { hasIndexingDocuments } from "./knowledgeUi";
import type { KnowledgeChunk, KnowledgeDocument, EmbedderConfig, UpdateEmbedderConfigInput } from "./types";
import { useKnowledgeStore } from "../../stores/knowledge";
import { useKnowledgeIngestWs } from "./useKnowledgeIngestWs";

export function useKnowledgePage() {
  const $q = useQuasar();
  const knowledgeStore = useKnowledgeStore();
  const selectedId = ref("");
  const docsLoading = ref(false);
  const error = ref("");
  const unavailable = ref("");
  const tab = ref("documents");
  const createOpen = ref(false);
  const createLoading = ref(false);
  const ingestOpen = ref(false);
  const ingestLoading = ref(false);
  const ingestFile = ref<File | null>(null);
  const searchQuery = ref("");
  const searchTopK = ref(5);
  const searchResults = ref<KnowledgeChunk[]>([]);
  const searchLoading = ref(false);
  const searchRan = ref(false);
  const embedderSaving = ref(false);
  const createForm = ref({ name: "", description: "", embedding_model: "text-embedding-3-small" });
  const ingestForm = ref({ source: "", mime_type: "text/plain", text: "" });
  const embedderConfig = computed<EmbedderConfig | null>(() => knowledgeStore.embedderConfig);

  useKnowledgeIngestWs(
    () => (hasIndexingDocuments(documents.value) ? selectedId.value : ""),
    () => void loadDocuments()
  );

  const collections = computed(() => knowledgeStore.collections);
  const loading = computed(() => knowledgeStore.loading);
  const documents = computed(() => knowledgeStore.documentsByCollection[selectedId.value] ?? []);
  const selectedCollection = computed(() => collections.value.find((c) => c.id === selectedId.value));

  function friendlyError(err: unknown): string {
    const msg = err instanceof Error ? err.message : String(err);
    if (/unavailable|pgvector|postgres|not configured/i.test(msg)) {
      unavailable.value = msg;
      return "";
    }
    return msg;
  }

  async function loadCollections() {
    error.value = "";
    unavailable.value = "";
    try {
      const res = await knowledgeStore.loadCollections({ limit: 100 });
      if (!selectedId.value && res.items.length) {
        selectedId.value = res.items[0].id;
      }
    } catch (e) {
      error.value = friendlyError(e) || "加载集合失败";
    }
  }

  async function loadDocuments() {
    if (!selectedId.value) return;
    docsLoading.value = true;
    try {
      await knowledgeStore.loadDocuments(selectedId.value, { limit: 100 });
    } catch (e) {
      $q.notify({ type: "negative", message: friendlyError(e) || "加载文档失败" });
    } finally {
      docsLoading.value = false;
    }
  }

  function selectCollection(id: string) {
    selectedId.value = id;
    tab.value = "documents";
    void loadDocuments();
  }

  function openCreateCollection() {
    createForm.value = { name: "", description: "", embedding_model: "text-embedding-3-small" };
    createOpen.value = true;
  }

  async function submitCreateCollection() {
    if (!createForm.value.name.trim()) {
      $q.notify({ type: "warning", message: "请填写名称" });
      return;
    }
    createLoading.value = true;
    try {
      const col = await knowledgeStore.addCollection({
        name: createForm.value.name.trim(),
        description: createForm.value.description.trim(),
        embedding_model: createForm.value.embedding_model.trim()
      });
      createOpen.value = false;
      await loadCollections();
      selectedId.value = col.id;
      await loadDocuments();
      $q.notify({ type: "positive", message: "集合已创建" });
    } catch (e) {
      $q.notify({ type: "negative", message: friendlyError(e) || "创建失败" });
    } finally {
      createLoading.value = false;
    }
  }

  function confirmDeleteCollection() {
    const col = selectedCollection.value;
    if (!col) return;
    $q.dialog({
      title: "删除集合",
      message: `确定删除「${col.name}」及其全部文档？`,
      cancel: true,
      persistent: true
    }).onOk(() => void deleteCollectionConfirmed(col.id));
  }

  async function deleteCollectionConfirmed(id: string) {
    try {
      await knowledgeStore.removeCollection(id);
      if (selectedId.value === id) {
        selectedId.value = "";
      }
      await loadCollections();
      $q.notify({ type: "positive", message: "已删除" });
    } catch (e) {
      $q.notify({ type: "negative", message: friendlyError(e) || "删除失败" });
    }
  }

  function onIngestFile(file: File | File[] | null) {
    const picked = Array.isArray(file) ? file[0] : file;
    if (!picked) return;
    ingestForm.value.source = ingestForm.value.source || picked.name;
    const reader = new FileReader();
    reader.onload = () => {
      ingestForm.value.text = String(reader.result ?? "");
    };
    reader.readAsText(picked);
  }

  async function submitIngest() {
    if (!selectedId.value) return;
    const text = ingestForm.value.text.trim();
    if (!text) {
      $q.notify({ type: "warning", message: "请提供文件或文本内容" });
      return;
    }
    ingestLoading.value = true;
    try {
      const b64 = btoa(unescape(encodeURIComponent(text)));
      await knowledgeStore.ingest({
        collection_id: selectedId.value,
        source: ingestForm.value.source || "upload",
        mime_type: ingestForm.value.mime_type || "text/plain",
        content_base64: b64
      });
      ingestOpen.value = false;
      ingestForm.value = { source: "", mime_type: "text/plain", text: "" };
      ingestFile.value = null;
      await loadDocuments();
      await loadCollections();
      $q.notify({ type: "positive", message: "文档已提交入库" });
    } catch (e) {
      $q.notify({ type: "negative", message: friendlyError(e) || "入库失败" });
    } finally {
      ingestLoading.value = false;
    }
  }

  function confirmDeleteDocument(doc: KnowledgeDocument) {
    $q.dialog({
      title: "删除文档",
      message: `删除「${doc.source}」？`,
      cancel: true
    }).onOk(async () => {
      try {
        await knowledgeStore.removeDocument(doc.id, selectedId.value);
        await loadDocuments();
        $q.notify({ type: "positive", message: "已删除" });
      } catch (e) {
        $q.notify({ type: "negative", message: friendlyError(e) || "删除失败" });
      }
    });
  }

  async function runSearch() {
    if (!selectedId.value || !searchQuery.value.trim()) return;
    searchLoading.value = true;
    searchRan.value = true;
    try {
      searchResults.value = await knowledgeStore.search({
        collection_id: selectedId.value,
        query: searchQuery.value.trim(),
        top_k: searchTopK.value
      });
    } catch (e) {
      $q.notify({ type: "negative", message: friendlyError(e) || "检索失败" });
    } finally {
      searchLoading.value = false;
    }
  }

  let docPollTimer: ReturnType<typeof setInterval> | null = null;

  function stopDocPoll() {
    if (docPollTimer) {
      clearInterval(docPollTimer);
      docPollTimer = null;
    }
  }

  function syncDocPoll() {
    stopDocPoll();
    if (!selectedId.value || tab.value !== "documents" || !hasIndexingDocuments(documents.value)) return;
    docPollTimer = setInterval(() => {
      if (!selectedId.value || !hasIndexingDocuments(documents.value)) {
        stopDocPoll();
        return;
      }
      void loadDocuments();
    }, 3000);
  }

  watch(selectedId, (id) => {
    if (id) void loadDocuments();
  });

  watch(
    () => [documents.value.map((d) => d.status).join(","), tab.value, selectedId.value] as const,
    () => syncDocPoll()
  );

  onUnmounted(stopDocPoll);

  async function saveEmbedderConfig(input: UpdateEmbedderConfigInput) {
    embedderSaving.value = true;
    try {
      await knowledgeStore.saveEmbedderConfig(input);
      $q.notify({ type: "positive", message: "Embedder 配置已保存" });
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "保存失败" });
    } finally {
      embedderSaving.value = false;
    }
  }

  return {
    collections,
    selectedId,
    selectedCollection,
    documents,
    loading,
    docsLoading,
    error,
    unavailable,
    tab,
    createOpen,
    createLoading,
    ingestOpen,
    ingestLoading,
    ingestFile,
    searchQuery,
    searchTopK,
    searchResults,
    searchLoading,
    searchRan,
    createForm,
    ingestForm,
    embedderConfig,
    embedderSaving,
    saveEmbedderConfig,
    loadCollections,
    loadDocuments,
    selectCollection,
    openCreateCollection,
    submitCreateCollection,
    confirmDeleteCollection,
    onIngestFile,
    submitIngest,
    confirmDeleteDocument,
    runSearch
  };
}
