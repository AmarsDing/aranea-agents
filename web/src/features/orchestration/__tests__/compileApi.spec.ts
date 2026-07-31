import { describe, expect, it } from 'vitest';
import { compiledGraphToGraphDef, type CompileTeamGraphResult } from '../compileApi';

function mkCompiled(graphJson: string): CompileTeamGraphResult {
  return {
    template_id: 'sequential',
    mode: 'sequential',
    entry_point: 'start',
    finish_point: 'end',
    nodes: [],
    edges: [],
    conditional_edges: [],
    graph_json: graphJson,
    issues: [],
    valid: true,
    definition_graph_json: '',
  };
}

describe('compileApi.compiledGraphToGraphDef enableCheckpoint (M53 Phase 11)', () => {
  it('reads enable_checkpoint from compiled graph_json (backend GraphBuildConfig)', () => {
    const on = compiledGraphToGraphDef(mkCompiled(JSON.stringify({ enable_checkpoint: true })));
    expect(on.enableCheckpoint).toBe(true);

    const off = compiledGraphToGraphDef(mkCompiled(JSON.stringify({ enable_checkpoint: false })));
    expect(off.enableCheckpoint).toBe(false);
  });

  it('falls back to false when graph_json is empty or invalid', () => {
    expect(compiledGraphToGraphDef(mkCompiled('')).enableCheckpoint).toBe(false);
    expect(compiledGraphToGraphDef(mkCompiled('not-json')).enableCheckpoint).toBe(false);
  });
});
