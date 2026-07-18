import { defineStore } from 'pinia';
import { reactive } from 'vue';
import type { MediaArtifact } from '../../features/chat/mediaTypes';

/**
 * nodeOutputStore manages per-node media outputs for the observation canvas.
 * Analogous to ComfyUI's NodeOutputStore — maps nodeId to its media artifacts.
 */
export const useNodeOutputStore = defineStore('nodeOutput', () => {
  // nodeId -> MediaArtifact[]
  const outputsByNode = reactive<Map<string, MediaArtifact[]>>(new Map());

  function setNodeOutput(nodeId: string, artifacts: MediaArtifact[]): void {
    outputsByNode.set(nodeId, artifacts);
  }

  function appendNodeOutput(nodeId: string, artifact: MediaArtifact): void {
    const existing = outputsByNode.get(nodeId) || [];
    existing.push(artifact);
    outputsByNode.set(nodeId, existing);
  }

  function getNodeOutput(nodeId: string): MediaArtifact[] {
    return outputsByNode.get(nodeId) || [];
  }

  function clearSession(): void {
    outputsByNode.clear();
  }

  return { outputsByNode, setNodeOutput, appendNodeOutput, getNodeOutput, clearSession };
});
