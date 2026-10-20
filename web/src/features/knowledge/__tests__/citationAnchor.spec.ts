// G-17：引用锚定回归——chunk 开头前缀直取、空白归一化、递减重试、拒绝中段命中。
import { describe, expect, it } from 'vitest';
import { findCitationOffset } from '../citationAnchor';

describe('findCitationOffset', () => {
  it('locates chunk-start prefix exactly', () => {
    const content = '前置段落。\n\n知识库治理的目标是把个人笔记沉淀为团队资产。';
    const line = '知识库治理的目标是把个人笔记沉淀为团队资产。';
    // raw 锚点 = chunk 首字偏移 7（'。'@4 + '\n\n'@5,6）——空白折叠不出现在 raw 坐标。
    expect(findCitationOffset(content, line)).toBe(content.indexOf('知识库'));
  });

  it('treats line as chunk prefix even when truncated', () => {
    const content = '很长的一段正文，分块算法从这里把它切开成为独立片段，后续继续。';
    const line = '分块算法从这里把它切开成为独立片段';
    expect(findCitationOffset(content, line)).toBe(content.indexOf('分块算法'));
  });

  it('folds whitespace differences between line and content', () => {
    const content = '标题\n\n第一段\n第二行仍在同一段。';
    const line = '第一段  第二行仍在同一段。';
    expect(findCitationOffset(content, line)).toBe(content.indexOf('第一段'));
  });

  it('strips trailing ellipsis before matching', () => {
    const content = '前面内容。核心论点在此展开并延续超过一百二十字符截断点之后还有很多。';
    const line = '核心论点在此展开并延续超过一百二十字符截断点之后还有很多。'.slice(0, 20) + '…';
    expect(findCitationOffset(content, line)).toBe(content.indexOf('核心论点'));
  });

  it('prefers first occurrence when passage repeats', () => {
    const content = '重复的片段。中间隔开。重复的片段。';
    expect(findCitationOffset(content, '重复的片段。')).toBe(0);
  });

  it('shrinks the sample on mismatch instead of anchoring mid-sentence', () => {
    // line 尾部是 tripc 解码差异（正文无此字符），样本递减到可命中长度。
    const content = '系统部署手册第三章说明回滚标准是三分钟可用。';
    const line = '说明回滚标准是三分钟可用。￼';
    expect(findCitationOffset(content, line)).toBe(content.indexOf('说明回滚'));
  });

  it('keeps heading markers as part of the anchor', () => {
    const content = '正文段。\n\n## 部署\n\n部署细节。';
    expect(findCitationOffset(content, '## 部署')).toBe(content.indexOf('## 部署'));
  });

  it('returns -1 when line matches nothing', () => {
    expect(findCitationOffset('abc 段落', '')).toBe(-1);
    // 对抗回归：不命中样本递减到 MIN_SAMPLE=12 时 0.75 收敛曾原地不动死循环
    // （冻结主线程）——此用例必须瞬时返回 -1。
    expect(findCitationOffset('短段落', '另一条充满大河流完全不同内容的句子。')).toBe(-1);
    expect(findCitationOffset('', '内容')).toBe(-1);
  });

  it('terminates when the shrinking sample plateaus at MIN_SAMPLE', () => {
    // 12 字符样本、正文仅有前缀部分重合：命中失败路径逐档递减 18→14→12→停。
    const content = '治理手册的目标是把笔记沉淀为团队资产共冶一炉。';
    const line = '治理手册的目标是把笔记蒸汽机不相关内容。';
    expect(findCitationOffset(content, line)).toBeGreaterThanOrEqual(-1);
  });
});
