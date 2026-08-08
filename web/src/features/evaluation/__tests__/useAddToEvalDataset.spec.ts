// web/src/features/evaluation/__tests__/useAddToEvalDataset.spec.ts
//
// P3-2：rubric 写入 metadata_json 的契约测试——适配层 ParseCaseMetadata 按
// `rubric` 键解析并映射为框架 EvalCase.Rubrics（llm_as_judge），前端必须保证
// 键名与扁平结构（source/task_id/session_id 与 rubric 同级）稳定。
import { describe, it, expect, vi, beforeEach } from 'vitest';

const notify = vi.fn();
vi.mock('quasar', () => ({
  useQuasar: () => ({ notify }),
}));
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));

const evaluationStoreMock = vi.hoisted(() => ({
  loadDatasets: vi.fn(async () => ({ items: [{ id: 'ds-1', name: 'D', case_count: 1 }], total: 1 })),
  addDataset: vi.fn(async () => ({ id: 'ds-new' })),
  importCases: vi.fn(async () => 1),
}));
vi.mock('../../../stores/evaluation', () => ({ useEvaluationStore: () => evaluationStoreMock }));

import { useAddToEvalDataset } from '../useAddToEvalDataset';

function lastImportedCase(): Record<string, string> {
  const call = evaluationStoreMock.importCases.mock.calls.at(-1);
  expect(call).toBeTruthy();
  const arr = JSON.parse(String(call?.[1])) as Record<string, string>[];
  expect(arr).toHaveLength(1);
  return arr[0]!;
}

describe('useAddToEvalDataset', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('writes rubric into metadata_json alongside chat source meta', async () => {
    const c = useAddToEvalDataset();
    await c.openWith({ input: 'q', expected_output: 'a', source_task_id: 't1', source_session_id: 's1' });
    c.rubric.value = '回答必须包含引用来源';
    expect(await c.submit()).toBe(true);

    const caseObj = lastImportedCase();
    expect(caseObj.input).toBe('q');
    const meta = JSON.parse(caseObj.metadata_json ?? '{}') as Record<string, unknown>;
    expect(meta).toEqual({ source: 'chat', task_id: 't1', session_id: 's1', rubric: '回答必须包含引用来源' });
  });

  it('omits rubric key when rubric is blank', async () => {
    const c = useAddToEvalDataset();
    await c.openWith({ input: 'q', expected_output: 'a', source_task_id: 't1' });
    c.rubric.value = '   ';
    expect(await c.submit()).toBe(true);

    const meta = JSON.parse(lastImportedCase().metadata_json ?? '{}') as Record<string, unknown>;
    expect(meta).toEqual({ source: 'chat', task_id: 't1' });
    expect('rubric' in meta).toBe(false);
  });

  it('writes metadata_json with rubric only when no source meta', async () => {
    const c = useAddToEvalDataset();
    await c.openWith({ input: 'q', expected_output: '' });
    c.rubric.value = 'score by helpfulness';
    expect(await c.submit()).toBe(true);

    const meta = JSON.parse(lastImportedCase().metadata_json ?? '{}') as Record<string, unknown>;
    expect(meta).toEqual({ rubric: 'score by helpfulness' });
  });

  it('resets rubric on each openWith', async () => {
    const c = useAddToEvalDataset();
    await c.openWith({ input: 'q1', expected_output: '' });
    c.rubric.value = 'r1';
    await c.openWith({ input: 'q2', expected_output: '' });
    expect(c.rubric.value).toBe('');
  });
});
