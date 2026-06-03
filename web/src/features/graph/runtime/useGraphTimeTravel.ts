import { ref, type Ref } from 'vue';
import type { CheckpointInfo } from '../types';
import { useGraphStore } from '../../../stores/graph';

export function useGraphTimeTravel(executionId: Ref<string>) {
  const graphStore = useGraphStore();
  const checkpoints = ref<CheckpointInfo[]>([]);
  const checkpointsLoading = ref(false);
  const selectedCheckpoint = ref<CheckpointInfo | null>(null);
  const stateSnapshot = ref<Record<string, unknown> | null>(null);
  const statePatchJson = ref('');
  const snapshotLoading = ref(false);
  const editLoading = ref(false);
  const timeTravelLoading = ref(false);
  const stepIndexInput = ref(0);

  async function loadCheckpoints() {
    if (!executionId.value) return;
    checkpointsLoading.value = true;
    try {
      checkpoints.value = await graphStore.fetchCheckpoints(executionId.value);
    } finally {
      checkpointsLoading.value = false;
    }
  }

  async function selectCheckpoint(checkpoint: CheckpointInfo) {
    if (!executionId.value) return;
    selectedCheckpoint.value = checkpoint;
    snapshotLoading.value = true;
    try {
      stateSnapshot.value = await graphStore.fetchStateSnapshot(
        executionId.value,
        checkpoint.checkpointId,
        checkpoint.namespace,
      );
      statePatchJson.value = JSON.stringify(stateSnapshot.value, null, 2);
    } finally {
      snapshotLoading.value = false;
    }
  }

  async function applyEditState(): Promise<{ newCheckpointId: string; lineageId: string } | null> {
    if (!executionId.value || !selectedCheckpoint.value) return null;
    editLoading.value = true;
    try {
      let patch: Record<string, unknown>;
      try {
        patch = JSON.parse(statePatchJson.value) as Record<string, unknown>;
      } catch {
        throw new Error('状态 JSON 格式无效，请检查编辑内容');
      }
      return await graphStore.editStateSnapshot(
        executionId.value,
        selectedCheckpoint.value.checkpointId,
        selectedCheckpoint.value.namespace,
        patch,
      );
    } finally {
      editLoading.value = false;
    }
  }

  async function travelToStep(stepIndex: number) {
    if (!executionId.value) return null;
    timeTravelLoading.value = true;
    try {
      const result = await graphStore.timeTravelExecution(executionId.value, stepIndex);
      stateSnapshot.value = result.stateSnapshot;
      statePatchJson.value = JSON.stringify(result.stateSnapshot, null, 2);
      return result;
    } finally {
      timeTravelLoading.value = false;
    }
  }

  return {
    checkpoints,
    checkpointsLoading,
    selectedCheckpoint,
    stateSnapshot,
    statePatchJson,
    snapshotLoading,
    editLoading,
    timeTravelLoading,
    stepIndexInput,
    loadCheckpoints,
    selectCheckpoint,
    applyEditState,
    travelToStep,
  };
}
