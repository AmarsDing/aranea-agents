// SP1-D/I-4：knowledge.graph.delta WS 增量的纯解析（与 service 层
// knowledge_graph_delta.go 发布的 Meta 形状对齐：snake_case 键）。
import { asRecord, pickBool, pickStr } from '../../shared/wireJson';

/** GraphDeltaEdge 一条块级引用边变更（delta added/removed 元素）。 */
export type GraphDeltaEdge = {
  collection_id: string;
  src_block_id: string;
  src_doc_id: string;
  dst_collection_id: string;
  dst_doc_id: string;
  dst_block_id: string;
  raw_target: string;
  edge_type: string;
  context: string;
  ambiguous: boolean;
};

/** KnowledgeGraphDelta 一次图谱变更增量（version 为变更后内存图版本）。 */
export type KnowledgeGraphDelta = {
  version: number;
  added: GraphDeltaEdge[];
  removed: GraphDeltaEdge[];
};

function mapEdge(raw: unknown): GraphDeltaEdge | null {
  if (raw === null || typeof raw !== 'object') return null;
  const r = asRecord(raw);
  return {
    collection_id: pickStr(r, 'collection_id', 'collectionId'),
    src_block_id: pickStr(r, 'src_block_id', 'srcBlockId'),
    src_doc_id: pickStr(r, 'src_doc_id', 'srcDocId'),
    dst_collection_id: pickStr(r, 'dst_collection_id', 'dstCollectionId'),
    dst_doc_id: pickStr(r, 'dst_doc_id', 'dstDocId'),
    dst_block_id: pickStr(r, 'dst_block_id', 'dstBlockId'),
    raw_target: pickStr(r, 'raw_target', 'rawTarget'),
    edge_type: pickStr(r, 'edge_type', 'edgeType'),
    context: pickStr(r, 'context', 'context'),
    ambiguous: pickBool(r, 'ambiguous', 'ambiguous'),
  };
}

function mapEdges(raw: unknown): GraphDeltaEdge[] {
  if (!Array.isArray(raw)) return [];
  return raw.map(mapEdge).filter((e): e is GraphDeltaEdge => e !== null);
}

/** parseGraphDeltaMeta 从 system.notice Meta 解析图谱增量；非对象 Meta 返回 null。 */
export function parseGraphDeltaMeta(meta: unknown): KnowledgeGraphDelta | null {
  if (meta === null || meta === undefined || typeof meta !== 'object') return null;
  const r = asRecord(meta);
  const version = Number(r.version);
  return {
    version: Number.isFinite(version) ? version : 0,
    added: mapEdges(r.added ?? r.Added),
    removed: mapEdges(r.removed ?? r.Removed),
  };
}

/** graphDeltaAffected 提取受影响面：src/dst 文档（反链/关联/灰显缓存键）与
 *  源/目标集合（悬空链列表缓存键），去重且跳过空 id。 */
export function graphDeltaAffected(delta: KnowledgeGraphDelta): { docIds: string[]; collectionIds: string[] } {
  const docs = new Set<string>();
  const colls = new Set<string>();
  for (const e of [...delta.added, ...delta.removed]) {
    if (e.src_doc_id) docs.add(e.src_doc_id);
    if (e.dst_doc_id) docs.add(e.dst_doc_id);
    if (e.collection_id) colls.add(e.collection_id);
    if (e.dst_collection_id) colls.add(e.dst_collection_id);
  }
  return { docIds: [...docs], collectionIds: [...colls] };
}
