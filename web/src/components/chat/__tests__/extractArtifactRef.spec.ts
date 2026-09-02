// web/src/components/chat/__tests__/extractArtifactRef.spec.ts
// P1 会话产物点击查看（2026-09-01）：工具结果 artifact 引用提取。
import { describe, it, expect } from 'vitest';
import { extractArtifactRef } from '../tools/toolDetailShared';

describe('extractArtifactRef', () => {
  it('提取 officecli_render 风格的 artifact 引用（artifact_id + file）', () => {
    const result = { ok: true, file: '报告-v1.pdf', artifact_id: 'art-1', artifact_url: 'artifact://art-1' };
    expect(extractArtifactRef(result)).toEqual({ id: 'art-1', name: '报告-v1.pdf' });
  });

  it('无 file 字段时只返回 id', () => {
    expect(extractArtifactRef({ artifact_id: 'art-2' })).toEqual({ id: 'art-2' });
  });

  it('artifact_id 缺失/空白/非字符串时返回 null', () => {
    expect(extractArtifactRef({})).toBeNull();
    expect(extractArtifactRef({ artifact_id: '  ' })).toBeNull();
    expect(extractArtifactRef({ artifact_id: 42 })).toBeNull();
  });

  it('非对象结果返回 null', () => {
    expect(extractArtifactRef(null)).toBeNull();
    expect(extractArtifactRef('artifact://art-3')).toBeNull();
    expect(extractArtifactRef([{ artifact_id: 'art-3' }])).toBeNull();
  });
});
