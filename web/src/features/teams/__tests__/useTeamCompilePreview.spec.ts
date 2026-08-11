// useTeamCompilePreview — UI-1：无启用成员时跳过编译，空表单落中性空态而非红色后端错误。
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { ref } from 'vue';
import { useTeamCompilePreview } from '../useTeamCompilePreview';

const compileTeamGraph = vi.hoisted(() => vi.fn());
vi.mock('../../orchestration/compileApi', () => ({ compileTeamGraph }));

describe('useTeamCompilePreview', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    compileTeamGraph.mockReset();
    compileTeamGraph.mockResolvedValue({ valid: true, issues: [] });
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  function setup(definitionJSON: string) {
    const json = ref(definitionJSON);
    return useTeamCompilePreview(() => '', json);
  }

  it('空成员数组时跳过编译，保持中性空态', async () => {
    const { compileError, compileResult, compileIssues } = setup('{"members":[]}');
    await vi.advanceTimersByTimeAsync(500);
    expect(compileTeamGraph).not.toHaveBeenCalled();
    expect(compileError.value).toBe('');
    expect(compileResult.value).toBeNull();
    expect(compileIssues.value).toEqual([]);
  });

  it('全部成员禁用时跳过编译', async () => {
    setup('{"members":[{"agent_id":"a1","enabled":false}]}');
    await vi.advanceTimersByTimeAsync(500);
    expect(compileTeamGraph).not.toHaveBeenCalled();
  });

  it('存在启用成员时正常调用编译', async () => {
    setup('{"members":[{"agent_id":"a1","enabled":true}]}');
    await vi.advanceTimersByTimeAsync(500);
    expect(compileTeamGraph).toHaveBeenCalledTimes(1);
  });

  it('enabled 缺省视为启用，正常调用编译', async () => {
    setup('{"members":[{"agent_id":"a1"}]}');
    await vi.advanceTimersByTimeAsync(500);
    expect(compileTeamGraph).toHaveBeenCalledTimes(1);
  });

  it('畸形 JSON 仍交给后端编译并报错', async () => {
    compileTeamGraph.mockRejectedValue(new Error('bad json'));
    const { compileError } = setup('{"members":');
    await vi.advanceTimersByTimeAsync(500);
    expect(compileTeamGraph).toHaveBeenCalledTimes(1);
    expect(compileError.value).toBe('bad json');
  });
});
