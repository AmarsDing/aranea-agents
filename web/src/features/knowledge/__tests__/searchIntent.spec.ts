import { describe, expect, it } from 'vitest';
import { classifySearchIntent } from '../searchIntent';

// 意图分流共享规则（与后端 internal/knowledge/search_intent.go 同一规则表，注释互指）。
describe('classifySearchIntent', () => {
  it('路径分隔符 → 即时区', () => {
    expect(classifySearchIntent('notes/report.md')).toBe('instant');
    expect(classifySearchIntent('财报\\2026')).toBe('instant');
  });

  it('扩展名模式 → 即时区', () => {
    expect(classifySearchIntent('q1 财报.pdf')).toBe('instant');
    expect(classifySearchIntent('readme.md')).toBe('instant');
    expect(classifySearchIntent('数据.xlsx')).toBe('instant');
  });

  it('引号短语 → 即时区', () => {
    expect(classifySearchIntent('"退款政策"')).toBe('instant');
    expect(classifySearchIntent('"exact phrase"')).toBe('instant');
  });

  it('自然语言问句 → 语义区', () => {
    expect(classifySearchIntent('什么是知识库')).toBe('semantic');
    expect(classifySearchIntent('如何配置 embedder？')).toBe('semantic');
    expect(classifySearchIntent('为什么同步失败')).toBe('semantic');
    expect(classifySearchIntent('怎么导入本地文件夹')).toBe('semantic');
    expect(classifySearchIntent('哪些文档提到了退款')).toBe('semantic');
    expect(classifySearchIntent('这个 vault 支持图片吗')).toBe('semantic');
    expect(classifySearchIntent('how does sync work?')).toBe('semantic');
    expect(classifySearchIntent('what is a vault')).toBe('semantic');
  });

  it('路径/扩展名优先于疑问词（强即时信号胜出）', () => {
    // 含扩展名但带疑问语气——用户明显在找文件
    expect(classifySearchIntent('config.yaml 在哪')).toBe('instant');
  });

  it('无强信号 → auto（双区并列展示）', () => {
    expect(classifySearchIntent('退款政策')).toBe('auto');
    expect(classifySearchIntent('embedding')).toBe('auto');
    expect(classifySearchIntent('')).toBe('auto');
    expect(classifySearchIntent('   ')).toBe('auto');
  });
});
