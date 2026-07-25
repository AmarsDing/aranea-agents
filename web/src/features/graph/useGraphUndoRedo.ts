import { ref, computed } from 'vue';
import type { GraphDefinition, NodeDef, EdgeDef, ConditionalEdgeDef, StateFieldDef } from './types';
import { writeGraphNodePosition } from './editor/graphLayout';

interface GraphCommand {
  label: string;
  undo: () => void;
  redo: () => void;
}

const MAX_UNDO_STACK = 50;

export function useGraphUndoRedo(graphDef: GraphDefinition, markDirty: () => void) {
  const undoStack = ref<GraphCommand[]>([]);
  const redoStack = ref<GraphCommand[]>([]);

  const canUndo = computed(() => undoStack.value.length > 0);
  const canRedo = computed(() => redoStack.value.length > 0);

  function execute(command: GraphCommand) {
    command.redo();
    undoStack.value.push(command);
    if (undoStack.value.length > MAX_UNDO_STACK) {
      undoStack.value.shift();
    }
    redoStack.value = [];
    markDirty();
  }

  function undo() {
    const command = undoStack.value.pop();
    if (!command) return;
    command.undo();
    redoStack.value.push(command);
    markDirty();
  }

  function redo() {
    const command = redoStack.value.pop();
    if (!command) return;
    command.redo();
    undoStack.value.push(command);
    markDirty();
  }

  function clear() {
    undoStack.value = [];
    redoStack.value = [];
  }

  function pushAddNode(node: NodeDef, index: number) {
    execute({
      label: `添加节点 ${node.id}`,
      undo: () => {
        const idx = graphDef.nodes.findIndex((n) => n.id === node.id);
        if (idx >= 0) graphDef.nodes.splice(idx, 1);
      },
      redo: () => {
        graphDef.nodes.splice(index, 0, { ...node });
      },
    });
  }

  function pushDeleteNode(node: NodeDef, index: number, edges: EdgeDef[], condEdges: ConditionalEdgeDef[]) {
    execute({
      label: `删除节点 ${node.id}`,
      undo: () => {
        graphDef.nodes.splice(index, 0, { ...node });
        graphDef.edges.push(...edges);
        graphDef.conditionalEdges.push(...condEdges);
      },
      redo: () => {
        const idx = graphDef.nodes.findIndex((n) => n.id === node.id);
        if (idx >= 0) graphDef.nodes.splice(idx, 1);
        graphDef.edges = graphDef.edges.filter((e) => e.from !== node.id && e.to !== node.id);
        graphDef.conditionalEdges = graphDef.conditionalEdges.filter(
          (e) => e.from !== node.id && !Object.values(e.pathMap ?? {}).includes(node.id),
        );
      },
    });
  }

  function pushDeleteNodes(
    deleted: { node: NodeDef; index: number; edges: EdgeDef[]; condEdges: ConditionalEdgeDef[] }[],
  ) {
    execute({
      label: `批量删除 ${deleted.length} 个节点`,
      undo: () => {
        for (const item of deleted) {
          graphDef.nodes.splice(item.index, 0, { ...item.node });
          graphDef.edges.push(...item.edges);
          graphDef.conditionalEdges.push(...item.condEdges);
        }
      },
      redo: () => {
        const ids = new Set(deleted.map((d) => d.node.id));
        graphDef.nodes = graphDef.nodes.filter((n) => !ids.has(n.id));
        graphDef.edges = graphDef.edges.filter((e) => !ids.has(e.from) && !ids.has(e.to));
        graphDef.conditionalEdges = graphDef.conditionalEdges.filter(
          (e) => !ids.has(e.from) && !Object.values(e.pathMap ?? {}).some((v) => ids.has(v)),
        );
      },
    });
  }

  function pushDuplicateNode(originalId: string, newNode: NodeDef, index: number) {
    execute({
      label: `复制节点 ${originalId}`,
      undo: () => {
        const idx = graphDef.nodes.findIndex((n) => n.id === newNode.id);
        if (idx >= 0) graphDef.nodes.splice(idx, 1);
      },
      redo: () => {
        graphDef.nodes.splice(index, 0, { ...newNode });
      },
    });
  }

  function pushAddEdge(edge: EdgeDef) {
    execute({
      label: `添加连线 ${edge.from}→${edge.to}`,
      undo: () => {
        const idx = graphDef.edges.findIndex((e) => e.from === edge.from && e.to === edge.to);
        if (idx >= 0) graphDef.edges.splice(idx, 1);
      },
      redo: () => {
        graphDef.edges.push({ ...edge });
      },
    });
  }

  function pushDeleteEdge(edge: EdgeDef, index: number) {
    execute({
      label: `删除连线 ${edge.from}→${edge.to}`,
      undo: () => {
        graphDef.edges.splice(index, 0, { ...edge });
      },
      redo: () => {
        const idx = graphDef.edges.findIndex((e) => e.from === edge.from && e.to === edge.to);
        if (idx >= 0) graphDef.edges.splice(idx, 1);
      },
    });
  }

  function pushDeleteConditionalEdge(ce: ConditionalEdgeDef, ceIndex: number, label: string) {
    const oldPathMap = { ...ce.pathMap };
    execute({
      label: `删除条件连线 ${ce.from}→${label}`,
      undo: () => {
        const current = graphDef.conditionalEdges[ceIndex];
        if (current === ce) {
          // CE 存活（只删了 label）→ 恢复完整 pathMap
          current.pathMap = { ...oldPathMap };
        } else {
          // CE 被整体删除 → 重新插入，禁止覆盖兄弟 CE
          graphDef.conditionalEdges.splice(ceIndex, 0, { ...ce, pathMap: { ...oldPathMap } });
        }
      },
      redo: () => {
        const newPathMap = { ...graphDef.conditionalEdges[ceIndex]?.pathMap };
        delete newPathMap[label];
        if (Object.keys(newPathMap).length === 0) {
          graphDef.conditionalEdges.splice(ceIndex, 1);
        } else {
          graphDef.conditionalEdges[ceIndex].pathMap = newPathMap;
        }
      },
    });
  }

  function pushSetProperty<K extends keyof NodeDef>(
    nodeId: string,
    field: K,
    oldValue: NodeDef[K],
    newValue: NodeDef[K],
  ) {
    execute({
      label: `修改 ${nodeId}.${String(field)}`,
      undo: () => {
        const node = graphDef.nodes.find((n) => n.id === nodeId);
        if (node) node[field] = oldValue;
      },
      redo: () => {
        const node = graphDef.nodes.find((n) => n.id === nodeId);
        if (node) node[field] = newValue;
      },
    });
  }

  function pushSetGraphProperty<K extends keyof GraphDefinition>(
    field: K,
    oldValue: GraphDefinition[K],
    newValue: GraphDefinition[K],
  ) {
    execute({
      label: `修改 Graph ${String(field)}`,
      undo: () => {
        graphDef[field] = oldValue;
      },
      redo: () => {
        graphDef[field] = newValue;
      },
    });
  }

  function pushDisconnectNode(nodeId: string, edges: EdgeDef[], condEdges: ConditionalEdgeDef[]) {
    execute({
      label: `断开 ${nodeId} 连线`,
      undo: () => {
        graphDef.edges.push(...edges);
        graphDef.conditionalEdges.push(...condEdges);
      },
      redo: () => {
        graphDef.edges = graphDef.edges.filter((e) => e.from !== nodeId && e.to !== nodeId);
        graphDef.conditionalEdges = graphDef.conditionalEdges.filter(
          (e) => e.from !== nodeId && !Object.values(e.pathMap ?? {}).includes(nodeId),
        );
      },
    });
  }

  function pushSetStateProperty<K extends keyof StateFieldDef>(
    idx: number,
    field: K,
    oldValue: StateFieldDef[K],
    newValue: StateFieldDef[K],
  ) {
    execute({
      label: `修改 StateField[${idx}].${String(field)}`,
      undo: () => {
        if (graphDef.stateFields[idx]) {
          graphDef.stateFields[idx][field] = oldValue;
        }
      },
      redo: () => {
        if (graphDef.stateFields[idx]) {
          graphDef.stateFields[idx][field] = newValue;
        }
      },
    });
  }

  function pushAddStateField(field: StateFieldDef, idx: number) {
    execute({
      label: `添加 StateField ${field.name || idx}`,
      undo: () => {
        const i = graphDef.stateFields.findIndex((f) => f === graphDef.stateFields[idx]);
        if (i >= 0) graphDef.stateFields.splice(i, 1);
      },
      redo: () => {
        graphDef.stateFields.splice(idx, 0, { ...field });
      },
    });
  }

  function pushRemoveStateField(field: StateFieldDef, idx: number) {
    execute({
      label: `删除 StateField ${field.name || idx}`,
      undo: () => {
        graphDef.stateFields.splice(idx, 0, { ...field });
      },
      redo: () => {
        if (graphDef.stateFields[idx]) {
          graphDef.stateFields.splice(idx, 1);
        }
      },
    });
  }

  function pushSetConditionalPathMap(
    ceIdx: number,
    oldPathMap: Record<string, string>,
    newPathMap: Record<string, string>,
  ) {
    execute({
      label: `修改条件路由 pathMap[${ceIdx}]`,
      undo: () => {
        if (graphDef.conditionalEdges[ceIdx]) {
          graphDef.conditionalEdges[ceIdx].pathMap = { ...oldPathMap };
        }
      },
      redo: () => {
        if (graphDef.conditionalEdges[ceIdx]) {
          graphDef.conditionalEdges[ceIdx].pathMap = { ...newPathMap };
        }
      },
    });
  }

  function pushSetCondFuncRef(ceIdx: number, oldValue: string, newValue: string) {
    execute({
      label: `修改条件路由 condFuncRef[${ceIdx}]`,
      undo: () => {
        if (graphDef.conditionalEdges[ceIdx]) {
          graphDef.conditionalEdges[ceIdx].condFuncRef = oldValue;
        }
      },
      redo: () => {
        if (graphDef.conditionalEdges[ceIdx]) {
          graphDef.conditionalEdges[ceIdx].condFuncRef = newValue;
        }
      },
    });
  }

  function pushAddConditionalEdge(ce: ConditionalEdgeDef, idx: number) {
    execute({
      label: `添加条件路由 ${ce.from}`,
      undo: () => {
        const i = graphDef.conditionalEdges.findIndex((e) => e === graphDef.conditionalEdges[idx]);
        if (i >= 0) graphDef.conditionalEdges.splice(i, 1);
      },
      redo: () => {
        graphDef.conditionalEdges.splice(idx, 0, { ...ce, pathMap: { ...ce.pathMap } });
      },
    });
  }

  function pushMoveNodes(
    moves: { nodeId: string; oldPos: { x: number; y: number }; newPos: { x: number; y: number } }[],
  ) {
    if (moves.length === 0) return;
    execute({
      label: moves.length === 1 ? `移动节点 ${moves[0].nodeId}` : `移动 ${moves.length} 个节点`,
      undo: () => {
        for (const move of moves) {
          writeGraphNodePosition(graphDef, move.nodeId, move.oldPos);
        }
      },
      redo: () => {
        for (const move of moves) {
          writeGraphNodePosition(graphDef, move.nodeId, move.newPos);
        }
      },
    });
  }

  function pushReconnectEdge(edgeIdx: number, oldFrom: string, oldTo: string, newFrom: string, newTo: string) {
    execute({
      label: `重连边 ${oldFrom}→${oldTo} → ${newFrom}→${newTo}`,
      undo: () => {
        if (graphDef.edges[edgeIdx]) {
          graphDef.edges[edgeIdx].from = oldFrom;
          graphDef.edges[edgeIdx].to = oldTo;
        }
      },
      redo: () => {
        if (graphDef.edges[edgeIdx]) {
          graphDef.edges[edgeIdx].from = newFrom;
          graphDef.edges[edgeIdx].to = newTo;
        }
      },
    });
  }

  return {
    canUndo,
    canRedo,
    undo,
    redo,
    clear,
    pushAddNode,
    pushDeleteNode,
    pushDeleteNodes,
    pushDuplicateNode,
    pushAddEdge,
    pushDeleteEdge,
    pushDeleteConditionalEdge,
    pushSetProperty,
    pushSetGraphProperty,
    pushDisconnectNode,
    pushSetStateProperty,
    pushAddStateField,
    pushRemoveStateField,
    pushSetConditionalPathMap,
    pushSetCondFuncRef,
    pushAddConditionalEdge,
    pushMoveNodes,
    pushReconnectEdge,
  };
}
