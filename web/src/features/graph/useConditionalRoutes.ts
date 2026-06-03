import { computed, toValue } from 'vue';
import type { ComputedRef, Ref } from 'vue';
import type { GraphDefinition, ConditionalEdgeDef } from './types';
import type { useGraphUndoRedo } from './useGraphUndoRedo';

type MaybeRef<T> = Ref<T> | ComputedRef<T> | T;

export function useConditionalRoutes(
  graphDef: MaybeRef<GraphDefinition | null>,
  selectedNodeId: MaybeRef<string | null>,
  undoRedo: MaybeRef<ReturnType<typeof useGraphUndoRedo> | undefined>,
  notifyChange: () => void,
  destinationOptions: MaybeRef<{ label: string; value: string }[]>,
) {
  const routerConditionalEdges = computed<ConditionalEdgeDef[]>(() => {
    const gd = toValue(graphDef);
    const nodeId = toValue(selectedNodeId);
    if (!nodeId || !gd) return [];
    return gd.conditionalEdges.filter((ce) => ce.from === nodeId);
  });

  function findGlobalCeIdx(localIdx: number): number {
    const gd = toValue(graphDef);
    const ce = routerConditionalEdges.value[localIdx];
    if (!ce || !gd) return -1;
    return gd.conditionalEdges.indexOf(ce);
  }

  function updateCondFuncRef(localIdx: number, value: string) {
    const gd = toValue(graphDef);
    const ur = toValue(undoRedo);
    const globalIdx = findGlobalCeIdx(localIdx);
    if (globalIdx < 0 || !gd) return;
    const ce = gd.conditionalEdges[globalIdx];
    const oldValue = ce.condFuncRef;
    ce.condFuncRef = value;
    if (ur) {
      ur.pushSetCondFuncRef(globalIdx, oldValue, value);
    } else {
      notifyChange();
    }
  }

  function updatePathMapLabel(localIdx: number, oldLabel: string, newLabel: string) {
    const gd = toValue(graphDef);
    const ur = toValue(undoRedo);
    const globalIdx = findGlobalCeIdx(localIdx);
    if (globalIdx < 0 || !gd) return;
    const ce = gd.conditionalEdges[globalIdx];
    const oldPathMap = { ...ce.pathMap };
    const target = ce.pathMap[oldLabel];
    const newPathMap = { ...ce.pathMap };
    delete newPathMap[oldLabel];
    newPathMap[newLabel] = target;
    ce.pathMap = newPathMap;
    if (ur) {
      ur.pushSetConditionalPathMap(globalIdx, oldPathMap, newPathMap);
    } else {
      notifyChange();
    }
  }

  function updatePathMapTarget(localIdx: number, label: string, value: string) {
    const gd = toValue(graphDef);
    const ur = toValue(undoRedo);
    const globalIdx = findGlobalCeIdx(localIdx);
    if (globalIdx < 0 || !gd) return;
    const ce = gd.conditionalEdges[globalIdx];
    const oldPathMap = { ...ce.pathMap };
    const newPathMap = { ...ce.pathMap, [label]: value };
    ce.pathMap = newPathMap;
    if (ur) {
      ur.pushSetConditionalPathMap(globalIdx, oldPathMap, newPathMap);
    } else {
      notifyChange();
    }
  }

  function removePathMapEntry(localIdx: number, label: string) {
    const gd = toValue(graphDef);
    const ur = toValue(undoRedo);
    const globalIdx = findGlobalCeIdx(localIdx);
    if (globalIdx < 0 || !gd) return;
    const ce = gd.conditionalEdges[globalIdx];
    const oldPathMap = { ...ce.pathMap };
    const newPathMap = { ...ce.pathMap };
    delete newPathMap[label];
    if (Object.keys(newPathMap).length === 0) {
      gd.conditionalEdges.splice(globalIdx, 1);
      if (ur) {
        ur.pushDeleteConditionalEdge(ce, globalIdx, label);
      } else {
        notifyChange();
      }
    } else {
      ce.pathMap = newPathMap;
      if (ur) {
        ur.pushSetConditionalPathMap(globalIdx, oldPathMap, newPathMap);
      } else {
        notifyChange();
      }
    }
  }

  function addPathMapEntry(localIdx: number) {
    const gd = toValue(graphDef);
    const ur = toValue(undoRedo);
    const globalIdx = findGlobalCeIdx(localIdx);
    if (globalIdx < 0 || !gd) return;
    const ce = gd.conditionalEdges[globalIdx];
    const oldPathMap = { ...ce.pathMap };
    let idx = 1;
    let newLabel = `route_${idx}`;
    while (ce.pathMap[newLabel]) {
      idx++;
      newLabel = `route_${idx}`;
    }
    const firstTarget = toValue(destinationOptions)[0]?.value ?? '';
    const newPathMap = { ...ce.pathMap, [newLabel]: firstTarget };
    ce.pathMap = newPathMap;
    if (ur) {
      ur.pushSetConditionalPathMap(globalIdx, oldPathMap, newPathMap);
    } else {
      notifyChange();
    }
  }

  function addConditionalEdge() {
    const gd = toValue(graphDef);
    const ur = toValue(undoRedo);
    const nodeId = toValue(selectedNodeId);
    if (!nodeId || !gd) return;
    const ce: ConditionalEdgeDef = {
      from: nodeId,
      condFuncRef: '',
      pathMap: { default: toValue(destinationOptions)[0]?.value ?? '' },
    };
    const idx = gd.conditionalEdges.length;
    gd.conditionalEdges.push(ce);
    if (ur) {
      ur.pushAddConditionalEdge(ce, idx);
    } else {
      notifyChange();
    }
  }

  return {
    routerConditionalEdges,
    updateCondFuncRef,
    updatePathMapLabel,
    updatePathMapTarget,
    removePathMapEntry,
    addPathMapEntry,
    addConditionalEdge,
  };
}
