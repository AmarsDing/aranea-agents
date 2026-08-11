// TECH-DEBT(FL5): composable 直接调用 orchestration/compileApi 而非通过 Store，
// 因为编译预览是即时操作且不需要全局状态。如需持久化编译结果应迁入 Store。
import { ref, watch, type MaybeRefOrGetter, toValue } from 'vue';
import { compileTeamGraph, type CompileTeamGraphResult } from '../orchestration/compileApi';

export type CompileIssue = { message?: string; code?: string; warning?: boolean };

/** 无启用成员时跳过编译：空表单落预览面板中性空态，避免「no enabled members」红错。 */
function hasEnabledMember(json: string): boolean {
  try {
    const parsed = JSON.parse(json) as { members?: Array<{ enabled?: boolean }> };
    return Array.isArray(parsed.members) && parsed.members.some((m) => m?.enabled !== false);
  } catch {
    return true; // 解析失败交给后端编译报错
  }
}

export function useTeamCompilePreview(editingId: MaybeRefOrGetter<string>, definitionJSON: MaybeRefOrGetter<string>) {
  const compileResult = ref<CompileTeamGraphResult | null>(null);
  const compileLoading = ref(false);
  const compileError = ref('');
  const compileIssues = ref<CompileIssue[]>([]);

  let compileDebounceTimer: ReturnType<typeof setTimeout> | null = null;

  async function refreshCompile() {
    const json = toValue(definitionJSON)?.trim();
    if (!json || json === '{}' || !json.includes('members') || !hasEnabledMember(json)) {
      compileResult.value = null;
      compileError.value = '';
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
