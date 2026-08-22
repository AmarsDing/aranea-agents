import { computed, ref, watch, type Ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import type { L0AssemblySnapshot, L1Field, L1Task } from './types';
import { buildMemoryAssemblyTableColumns } from './memoryTableUi';
import { useMemoryStore } from '../../stores/memory';

export function useMemoryCenterSessionMemory(opts: { selectedSessionId: Ref<string | null> }) {
  const { t } = useI18n();
  const memoryStore = useMemoryStore();
  const { snapshots } = storeToRefs(memoryStore);

  const tasks = ref<L1Task[]>([]);
  const selectedSnapshot = ref<L0AssemblySnapshot | null>(null);
  const selectedTaskId = ref<string | null>(null);
  const fieldRows = ref<L1Field[]>([]);
  const loadingSnapshots = ref(false);
  const loadingTasks = ref(false);
  const loadingFields = ref(false);
  const snapshotDrawer = ref(false);

  const snapshotColumns = computed(() => buildMemoryAssemblyTableColumns(formatDate, t));

  watch(opts.selectedSessionId, () => {
    selectedTaskId.value = null;
    fieldRows.value = [];
    void loadSessionMemory();
  });

  watch(selectedTaskId, () => {
    void loadTaskFields();
  });

  async function loadSessionMemory() {
    if (!opts.selectedSessionId.value) {
      memoryStore.clearSnapshots();
      tasks.value = [];
      selectedTaskId.value = null;
      fieldRows.value = [];
      return;
    }
    await Promise.all([loadSnapshots(), loadTasks()]);
  }

  async function loadSnapshots() {
    if (!opts.selectedSessionId.value) return;
    loadingSnapshots.value = true;
    try {
      await memoryStore.loadSnapshots(opts.selectedSessionId.value, 20);
    } catch {
      memoryStore.clearSnapshots();
    } finally {
      loadingSnapshots.value = false;
    }
  }

  async function loadTasks() {
    if (!opts.selectedSessionId.value) return;
    loadingTasks.value = true;
    try {
      tasks.value = await memoryStore.loadL1Tasks(opts.selectedSessionId.value, { include_ended: true });
      if (!selectedTaskId.value && tasks.value[0]) {
        selectedTaskId.value = tasks.value[0].id;
      } else if (selectedTaskId.value && !tasks.value.some((row) => row.id === selectedTaskId.value)) {
        selectedTaskId.value = tasks.value[0]?.id ?? null;
      }
      if (selectedTaskId.value) {
        await loadTaskFields();
      }
    } catch {
      tasks.value = [];
      selectedTaskId.value = null;
      fieldRows.value = [];
    } finally {
      loadingTasks.value = false;
    }
  }

  async function loadTaskFields() {
    const sessionID = opts.selectedSessionId.value;
    const taskID = selectedTaskId.value;
    if (!sessionID || !taskID) {
      fieldRows.value = [];
      return;
    }
    loadingFields.value = true;
    try {
      fieldRows.value = await memoryStore.loadL1Fields(sessionID, taskID, true);
    } catch {
      fieldRows.value = [];
    } finally {
      loadingFields.value = false;
    }
  }

  function openSnapshot(row: L0AssemblySnapshot) {
    selectedSnapshot.value = row;
    snapshotDrawer.value = true;
  }

  function formatDate(value?: string) {
    if (!value) return '-';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;
    return date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
  }

  return {
    snapshots,
    tasks,
    selectedSnapshot,
    selectedTaskId,
    fieldRows,
    loadingSnapshots,
    loadingTasks,
    loadingFields,
    snapshotDrawer,
    snapshotColumns,
    loadSessionMemory,
    openSnapshot,
  };
}
