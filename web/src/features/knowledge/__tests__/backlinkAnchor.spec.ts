// G-18：反链锚点定位回归——单候选直取、context 邻近消歧、容错退路。
import { describe, expect, it } from 'vitest';
import { findBacklinkOffset } from '../backlinkAnchor';

describe('findBacklinkOffset', () => {
  it('locates single [[rawTarget]] occurrence', () => {
    const content = '前言文字[[部署手册]]后续';
    expect(findBacklinkOffset(content, '', '部署手册')).toBe(4);
  });

  it('accepts alias and heading suffixes as same anchor', () => {
    const content = '看 [[ops/runbook#回滚|回滚预案]] 一节';
    expect(findBacklinkOffset(content, '', 'ops/runbook')).toBe(2);
  });

  it('does not mis-pair [[targetX]] for [[target]]', () => {
    const content = '[[部署手册v2]] 与 [[部署手册]]';
    expect(findBacklinkOffset(content, '', '部署手册')).toBe(13); // 第二处才是精确闭合
  });

  it('disambiguates repeated refs via nearest context position', () => {
    const content = '第一章提到[[手册]]是旧版。第三章正式引用[[手册]]作为准绳。';
    const second = content.lastIndexOf('[[手册]]');
    const at = findBacklinkOffset(content, '第三章正式引用[[手册]]作为准绳', '手册');
    expect(at).toBe(second);
  });

  it('falls back to first candidate when context matches nowhere', () => {
    const content = 'A[[指南]]B[[指南]]C';
    expect(findBacklinkOffset(content, '完全不相关的外部片段', '指南')).toBe(1);
  });

  it('falls back to bare rawTarget text when no [[...]] form exists', () => {
    const content = '纯文本出现目标一处。';
    expect(findBacklinkOffset(content, '', '目标')).toBe(5);
  });

  it('normalizes whitespace differences between context and content', () => {
    const content = '段落一。[[目标]]\r\n下一段。';
    expect(findBacklinkOffset(content, '段落一。  [[目标]]', '目标')).toBe(4);
  });

  it('disambiguates whitespace-bearing targets across folding variants', () => {
    // 对抗回归（评审确认缺陷）：rawTarget 含空白时，正文折叠形态 [[Dev  Ops]]
    // 是 raw 坐标系的合法候选；旧实现按"出现序"对齐 norm/raw 两列候选会错配，
    // 把锚跳过头一条。此用例必须落在中间链接（raw 15）。
    const content = '[[Dev Ops]] 甲。 [[Dev  Ops]] 乙处。 [[Dev Ops]] 丙。';
    expect(findBacklinkOffset(content, '[[Dev  Ops]] 乙处。', 'Dev Ops')).toBe(15);
  });

  it('matches folding variants for double-space targets too', () => {
    const content = '[[Dev Ops]] 提到 [[Dev\tOps]]';
    expect(findBacklinkOffset(content, '', 'Dev  Ops')).toBe(0); // 单候选语义扩展到 \s+ 变体后取首个
    expect(findBacklinkOffset(content, '提到 [[Dev\tOps]]', 'Dev  Ops')).toBe(content.indexOf('[[Dev\tOps]]'));
  });

  it('returns -1 when nothing matches', () => {
    expect(findBacklinkOffset('abc', '', 'xyz')).toBe(-1);
    expect(findBacklinkOffset('段落', '', '')).toBe(-1);
  });
});
