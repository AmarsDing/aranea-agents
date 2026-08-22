import { describe, it, expect } from 'vitest';
import { generateSummaryFallback } from '../executionCardHelpers';
import type { ToolUseEvent } from '../types';

function event(partial: Partial<ToolUseEvent>): ToolUseEvent {
  return {
    id: 'e1',
    phase: 'before',
    status: 'running',
    agent_id: 'a',
    agent_key: '__spirit__',
    agent_name: 'Spirit',
    tool_name: '',
    tool_label: '',
    occurred_at: '2026-08-22T00:00:00Z',
    ...partial,
  };
}

describe('generateSummaryFallback', () => {
  it('plan_and_execute shows planning copy while the tool is running', () => {
    expect(generateSummaryFallback(event({ tool_name: 'plan_and_execute' }))).toBe('正在规划并执行…');
  });

  it('file_read uses the filename', () => {
    expect(
      generateSummaryFallback(event({ tool_name: 'file_read', arguments: { file_name: 'a.go' } })),
    ).toBe('读取 a.go');
  });

  // 调用契约 7.4：前端 summary 必须命中运行时真实工具名（trpc file 工具集
  // diff_edit/search_content + hostexec exec_command），而非仅历史别名。
  it('diff_edit uses the file_name arg', () => {
    expect(
      generateSummaryFallback(event({ tool_name: 'diff_edit', arguments: { file_name: 'src/main.go' } })),
    ).toBe('修改 main.go');
  });

  it('search_content uses content_pattern', () => {
    expect(
      generateSummaryFallback(
        event({ tool_name: 'search_content', arguments: { path: 'src', content_pattern: 'func main' } }),
      ),
    ).toBe('搜索 "func main"');
  });

  it('exec_command uses the command arg', () => {
    expect(
      generateSummaryFallback(event({ tool_name: 'exec_command', arguments: { command: 'go build ./...' } })),
    ).toBe('> go build ./...');
  });
});
