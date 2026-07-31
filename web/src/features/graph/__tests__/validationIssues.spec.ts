// web/src/features/graph/__tests__/validationIssues.spec.ts
import { describe, it, expect } from 'vitest';
import { buildValidationIssues, pickNodeIssueMap, validationSuggestionKey } from '../validationIssues';
import type { NodeDef, ValidationError, ValidationWarning } from '../types';

function makeNode(id: string, agentName?: string): NodeDef {
  return { id, type: 'agent', agentName } as unknown as NodeDef;
}

describe('buildValidationIssues', () => {
  it('merges errors and warnings with level flags', () => {
    const errors: ValidationError[] = [
      { code: 'unreachable_node', nodeId: 'n1', field: '', message: '节点不可达: n1' },
    ];
    const warnings: ValidationWarning[] = [{ code: 'orphan_node', nodeId: 'n2', field: '', message: '孤立节点: n2' }];
    const issues = buildValidationIssues(errors, warnings, []);
    expect(issues).toHaveLength(2);
    const err = issues.find((i) => i.nodeId === 'n1');
    const warn = issues.find((i) => i.nodeId === 'n2');
    expect(err?.level).toBe('error');
    expect(warn?.level).toBe('warning');
  });

  it('sorts errors before warnings', () => {
    const errors: ValidationError[] = [{ code: 'unreachable_node', nodeId: 'n1', field: '', message: 'e' }];
    const warnings: ValidationWarning[] = [{ code: 'orphan_node', nodeId: 'n2', field: '', message: 'w' }];
    const issues = buildValidationIssues(errors, warnings, []);
    expect(issues[0].level).toBe('error');
    expect(issues[1].level).toBe('warning');
  });

  it('dedups identical issues by level+code+nodeId+field', () => {
    const errors: ValidationError[] = [
      { code: 'unreachable_node', nodeId: 'n1', field: '', message: 'a' },
      { code: 'unreachable_node', nodeId: 'n1', field: '', message: 'a' },
    ];
    const issues = buildValidationIssues(errors, [], []);
    expect(issues).toHaveLength(1);
  });

  it('decorates nodeLabel from nodes: agentName wins, fallback to id', () => {
    const errors: ValidationError[] = [
      { code: 'unreachable_node', nodeId: 'n1', field: '', message: 'e1' },
      { code: 'unreachable_node', nodeId: 'n2', field: '', message: 'e2' },
      { code: 'no_entry_point', nodeId: '', field: 'entryPoint', message: 'e3' },
    ];
    const nodes = [makeNode('n1', '研究助手'), makeNode('n2')];
    const issues = buildValidationIssues(errors, [], nodes);
    expect(issues.find((i) => i.nodeId === 'n1')?.nodeLabel).toBe('研究助手');
    expect(issues.find((i) => i.nodeId === 'n2')?.nodeLabel).toBe('n2');
    expect(issues.find((i) => i.nodeId === '')?.nodeLabel).toBe('');
  });
});

describe('pickNodeIssueMap', () => {
  it('maps nodeId to its issue, error wins over warning for same node', () => {
    const issues = buildValidationIssues(
      [{ code: 'unreachable_node', nodeId: 'n1', field: '', message: 'e' }],
      [{ code: 'orphan_node', nodeId: 'n1', field: '', message: 'w' }],
      [],
    );
    const map = pickNodeIssueMap(issues);
    expect(map.n1.level).toBe('error');
  });

  it('keeps warning when node has no error', () => {
    const issues = buildValidationIssues([], [{ code: 'orphan_node', nodeId: 'n1', field: '', message: 'w' }], []);
    const map = pickNodeIssueMap(issues);
    expect(map.n1.level).toBe('warning');
  });

  it('ignores issues without nodeId', () => {
    const issues = buildValidationIssues(
      [{ code: 'no_entry_point', nodeId: '', field: 'entryPoint', message: 'e' }],
      [],
      [],
    );
    const map = pickNodeIssueMap(issues);
    expect(Object.keys(map)).toHaveLength(0);
  });
});

describe('validationSuggestionKey', () => {
  it('returns i18n key for known codes', () => {
    expect(validationSuggestionKey('no_entry_point')).toBe('graphs.suggestionNoEntryPoint');
    expect(validationSuggestionKey('duplicate_node')).toBe('graphs.suggestionDuplicateNode');
    expect(validationSuggestionKey('edge_source_missing')).toBe('graphs.suggestionEdgeMissingNode');
    expect(validationSuggestionKey('edge_target_missing')).toBe('graphs.suggestionEdgeMissingNode');
    expect(validationSuggestionKey('unreachable_node')).toBe('graphs.suggestionUnreachable');
    expect(validationSuggestionKey('loop_no_exit')).toBe('graphs.suggestionLoopNoExit');
    expect(validationSuggestionKey('conditional_loop')).toBe('graphs.suggestionConditionalLoop');
    expect(validationSuggestionKey('orphan_node')).toBe('graphs.suggestionOrphanNode');
  });

  it('returns empty string for unknown codes', () => {
    expect(validationSuggestionKey('some_server_code')).toBe('');
  });
});
