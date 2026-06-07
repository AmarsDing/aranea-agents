import { ref, watch, type MaybeRefOrGetter, toValue } from 'vue';
import { compileTeamGraph, type CompileTeamGraphResult } from '../orchestration/compileApi';

export type CompileIssue = { message?: string; code?: string; warning?: boolean };

export function useTeamCompilePreview(editingId: MaybeRefOrGetter<string>, definitionJSON: MaybeRefOrGetter<string>) {
  const compileResult = ref<CompileTeamGraphResult | null>(null);
  const compileLoading = ref(false);
  const compileError = ref('');
  const compileIssues = ref<CompileIssue[]>([]);

  let compileDebounceTimer: ReturnType<typeof setTimeout> | null = null;

  async function refreshCompile() {
    const json = toValue(definitionJSON)?.trim();
    if (!json || json === '{}' || !json.includes('members')) {
      compileResult.value = null;
      compileIssues.value = [];
      return;
    }
    compileLoading.value = true;
    compileError.value = '';
    try {
      const teamId = toValue(editingId)?.trim() || 'draft-preview';
      compileResult.value = await compileTeamGraph(teamId, json);
      compileIssues.value = (compileResult.value.issues ?? []).map((i) => ({
        message: i.message,
        code: i.code,
        warning: Boolean(i.warning),
      }));
    } catch (e) {
      compileError.value = e instanceof Error ? e.message : String(e);
      compileResult.value = null;
      compileIssues.value = [];
    } finally {
      compileLoading.value = false;
    }
  }

  function scheduleCompileRefresh() {
    if (compileDebounceTimer) clearTimeout(compileDebounceTimer);
    compileDebounceTimer = setTimeout(refreshCompile, 400);
  }

  watch(() => [toValue(editingId), toValue(definitionJSON)], scheduleCompileRefresh, { immediate: true });

  return {
    compileResult,
    compileLoading,
    compileError,
    compileIssues,
    refreshCompile,
  };
}
