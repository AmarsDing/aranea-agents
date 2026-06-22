import { computed, ref } from 'vue';
import type { Ref } from 'vue';
import type { GraphDefinition, ValidationError, ValidationWarning } from './types';

export function useGraphLocalValidation(graphDef: Ref<GraphDefinition>) {
  const cycleWarnings = ref<ValidationWarning[]>([]);

  const localErrors = computed<ValidationError[]>(() => {
    const errors: ValidationError[] = [];
    const def = graphDef.value;
    const nodeIds = new Set(def.nodes.map((n) => n.id));

    cycleWarnings.value = [];

    if (def.nodes.length === 0) return errors;

    if (!def.entryPoint) {
      errors.push({ code: 'no_entry_point', nodeId: '', field: 'entryPoint', message: '缺少入口节点' });
    } else if (!nodeIds.has(def.entryPoint)) {
      errors.push({ code: 'no_entry_point', nodeId: def.entryPoint, field: 'entryPoint', message: '入口节点不存在' });
    }

    const seen = new Map<string, number>();
    for (const n of def.nodes) {
      const prev = seen.get(n.id);
      if (prev !== undefined) {
        errors.push({ code: 'duplicate_node', nodeId: n.id, field: 'id', message: `节点 ID 重复: ${n.id}` });
      } else {
        seen.set(n.id, 1);
      }
    }

    for (const e of def.edges) {
      if (!nodeIds.has(e.from)) {
        errors.push({ code: 'edge_source_missing', nodeId: '', field: 'from', message: `边源节点不存在: ${e.from}` });
      }
      if (!nodeIds.has(e.to)) {
        errors.push({ code: 'edge_target_missing', nodeId: e.from, field: 'to', message: `边目标节点不存在: ${e.to}` });
      }
    }
    for (const ce of def.conditionalEdges) {
      if (!nodeIds.has(ce.from)) {
        errors.push({
          code: 'edge_source_missing',
          nodeId: '',
          field: 'from',
          message: `条件边源节点不存在: ${ce.from}`,
        });
        continue;
      }
      const targets = Object.values(ce.pathMap ?? {});
      for (const t of targets) {
        if (!nodeIds.has(t)) {
          errors.push({
            code: 'edge_target_missing',
            nodeId: ce.from,
            field: 'pathMap',
            message: `条件边目标节点不存在: ${t}`,
          });
        }
      }
    }

    if (def.entryPoint && nodeIds.has(def.entryPoint)) {
      const reachable = new Set<string>();
      const queue = [def.entryPoint];
      while (queue.length > 0) {
        const cur = queue.pop()!;
        if (reachable.has(cur)) continue;
        reachable.add(cur);
        for (const e of def.edges) {
          if (e.from === cur && !reachable.has(e.to)) queue.push(e.to);
        }
        for (const ce of def.conditionalEdges) {
          if (ce.from === cur) {
            for (const t of Object.values(ce.pathMap ?? {})) {
              if (!reachable.has(t)) queue.push(t);
            }
          }
        }
      }
      for (const n of def.nodes) {
        if (!reachable.has(n.id)) {
          errors.push({ code: 'unreachable_node', nodeId: n.id, field: '', message: `节点不可达: ${n.id}` });
        }
      }
    }

    const unconditionalAdj = new Map<string, string[]>();
    const fullAdj = new Map<string, string[]>();
    for (const e of def.edges) {
      const ul = unconditionalAdj.get(e.from) ?? [];
      ul.push(e.to);
      unconditionalAdj.set(e.from, ul);
      const fl = fullAdj.get(e.from) ?? [];
      fl.push(e.to);
      fullAdj.set(e.from, fl);
    }
    for (const ce of def.conditionalEdges) {
      const fl = fullAdj.get(ce.from) ?? [];
      for (const t of Object.values(ce.pathMap ?? {})) fl.push(t);
      fullAdj.set(ce.from, fl);
    }

    const cycleVisited = new Set<string>();
    const cycleRecStack = new Set<string>();
    function hasCycleIn(adj: Map<string, string[]>, nodeId: string): boolean {
      cycleVisited.add(nodeId);
      cycleRecStack.add(nodeId);
      for (const neighbor of adj.get(nodeId) ?? []) {
        if (!cycleVisited.has(neighbor)) {
          if (hasCycleIn(adj, neighbor)) return true;
        } else if (cycleRecStack.has(neighbor)) {
          return true;
        }
      }
      cycleRecStack.delete(nodeId);
      return false;
    }

    let hasUnconditionalCycle = false;
    const uv = new Set<string>();
    for (const n of def.nodes) {
      if (!uv.has(n.id)) {
        cycleVisited.clear();
        cycleRecStack.clear();
        if (hasCycleIn(unconditionalAdj, n.id)) {
          hasUnconditionalCycle = true;
          break;
        }
        for (const v of cycleVisited) uv.add(v);
      }
    }
    if (hasUnconditionalCycle) {
      errors.push({ code: 'loop_no_exit', nodeId: '', field: '', message: '图中存在无条件循环（死循环）' });
    } else {
      let hasConditionalCycle = false;
      const cv = new Set<string>();
      for (const n of def.nodes) {
        if (!cv.has(n.id)) {
          cycleVisited.clear();
          cycleRecStack.clear();
          if (hasCycleIn(fullAdj, n.id)) {
            hasConditionalCycle = true;
            break;
          }
          for (const v of cycleVisited) cv.add(v);
        }
      }
      if (hasConditionalCycle) {
        cycleWarnings.value.push({
          code: 'conditional_loop',
          nodeId: '',
          field: '',
          message: '图中存在条件循环（运行时可能回退）',
        });
      }
    }

    return errors;
  });

  const localWarnings = computed<ValidationWarning[]>(() => {
    const warnings: ValidationWarning[] = [];
    const def = graphDef.value;

    const connectedNodes = new Set<string>();
    for (const e of def.edges) {
      connectedNodes.add(e.from);
      connectedNodes.add(e.to);
    }
    for (const ce of def.conditionalEdges) {
      connectedNodes.add(ce.from);
      for (const t of Object.values(ce.pathMap ?? {})) connectedNodes.add(t);
    }
    if (def.entryPoint) connectedNodes.add(def.entryPoint);
    if (def.finishPoint) connectedNodes.add(def.finishPoint);

    for (const n of def.nodes) {
      if (!connectedNodes.has(n.id)) {
        warnings.push({ code: 'orphan_node', nodeId: n.id, field: '', message: `孤立节点（无连接）: ${n.id}` });
      }
    }

    warnings.push(...cycleWarnings.value);

    return warnings;
  });

  const localValid = computed(() => localErrors.value.length === 0);

  return { localErrors, localWarnings, localValid };
}
