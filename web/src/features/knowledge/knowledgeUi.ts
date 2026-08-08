import type { KnowledgeCollection, KnowledgeDocument } from './types';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../ui/registryTableColumns';

export const KNOWLEDGE_DOC_TABLE_COLUMNS = [
  registryCol<KnowledgeDocument>('source', '来源', 'source', 'left', REGISTRY_COL_W.stats),
  registryCol<KnowledgeDocument>('mime_type', '类型', 'mime_type', 'left', REGISTRY_COL_W.name),
  registryCol<KnowledgeDocument>('status', '状态', 'status', 'left', REGISTRY_COL_W.status),
  registryCol<KnowledgeDocument>('chunk_count', '分块', 'chunk_count', 'right', REGISTRY_COL_W.metric),
  registryCol<KnowledgeDocument>('size_bytes', '大小', 'size_bytes', 'right', REGISTRY_COL_W.metric),
  registryCol<KnowledgeDocument>('created_at', '入库时间', 'created_at', 'left', REGISTRY_COL_W.timeWide),
  registryCol<KnowledgeDocument>('updated_at', '更新时间', 'updated_at', 'left', REGISTRY_COL_W.timeWide),
  registryColActions<KnowledgeDocument>(REGISTRY_COL_W.actions, ''),
];

/** @deprecated 使用 KNOWLEDGE_DOC_TABLE_COLUMNS */
export const knowledgeDocColumns = KNOWLEDGE_DOC_TABLE_COLUMNS;

export function knowledgeStatusColor(status: string): string {
  if (status === 'active' || status === 'indexed') return 'positive';
  if (status === 'error') return 'negative';
  if (status === 'indexing' || status === 'pending') return 'warning';
  return 'grey';
}

/** 文档状态本地化 i18n key；未知状态返回 ''（调用方回退展示原始 status）。 */
export function knowledgeStatusLabelKey(status: string): string {
  if (status === 'active' || status === 'indexed') return 'knowledgePage.statusIndexed';
  if (status === 'indexing') return 'knowledgePage.statusIndexing';
  if (status === 'pending') return 'knowledgePage.statusPending';
  if (status === 'error') return 'knowledgePage.statusError';
  return '';
}

export const INDEXING_DOC_STATUSES = new Set(['indexing', 'pending']);

export function hasIndexingDocuments(documents: KnowledgeDocument[]): boolean {
  return documents.some((d) => INDEXING_DOC_STATUSES.has(d.status));
}

export function formatKnowledgeDocSize(bytes?: number): string {
  const n = Number(bytes) || 0;
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

export function formatKnowledgeTime(iso?: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

export const KNOWLEDGE_HYBRID_MODE_OPTIONS = [
  { label: '自动', value: 'auto' },
  { label: '向量检索', value: 'dense' },
  { label: '全文检索', value: 'sparse' },
  { label: '混合 (RRF)', value: 'rrf' },
];

// ── G2-F：详情面板媒体分类（V12.4） ──────────────────────────────────────────

/** 详情面板媒体区类别：image/audio/video 走 B6 原始流内联渲染；word 显示解析后
 *  md + 原文下载；markdown/text 可编辑（B5）；other 只读预览。 */
export type KnowledgeMediaKind = 'image' | 'audio' | 'video' | 'word' | 'markdown' | 'text' | 'other';

const KNOWLEDGE_MEDIA_EXTS: Record<string, readonly string[]> = {
  image: ['.png', '.jpg', '.jpeg', '.gif', '.webp', '.svg', '.bmp', '.avif'],
  audio: ['.mp3', '.wav', '.ogg', '.m4a', '.flac', '.aac', '.opus'],
  video: ['.mp4', '.webm', '.mov', '.mkv', '.avi', '.m4v'],
  word: ['.doc', '.docx'],
  markdown: ['.md', '.markdown'],
  text: ['.txt'],
};

/** 按文件名扩展名分类（VaultTreeNode 不带 mime_type，扩展名对媒体渲染判定足够可靠）。 */
export function knowledgeMediaKind(name: string): KnowledgeMediaKind {
  const i = name.lastIndexOf('.');
  if (i < 0) return 'other';
  const ext = name.slice(i).toLowerCase();
  for (const [kind, exts] of Object.entries(KNOWLEDGE_MEDIA_EXTS)) {
    if (exts.includes(ext)) return kind as KnowledgeMediaKind;
  }
  return 'other';
}

/** 媒体区是否需要 B6 原始流（内联播放器/图片）。 */
export function knowledgeMediaNeedsAsset(kind: KnowledgeMediaKind): boolean {
  return kind === 'image' || kind === 'audio' || kind === 'video';
}

/** md/txt 可编辑（V12.4）；vault 文档还需 base_hash 非空（由调用方组合判定）。 */
export function knowledgeMediaEditable(kind: KnowledgeMediaKind): boolean {
  return kind === 'markdown' || kind === 'text';
}

export const KNOWLEDGE_REWRITE_STRATEGY_OPTIONS = [
  { label: '无', value: '' },
  { label: 'HyDE', value: 'hyde' },
  { label: '查询分解', value: 'decomposition' },
  { label: '多查询', value: 'multi_query' },
];

export const KNOWLEDGE_CHUNK_STRATEGY_OPTIONS = [
  { label: '字符 (默认)', value: '' },
  { label: 'Token', value: 'token' },
  { label: 'Markdown', value: 'markdown' },
  { label: 'JSON', value: 'json' },
  { label: '递归', value: 'recursive' },
];

// ── SP1-I（I-2）：浏览视图 dangling 灰显 ─────────────────────────────────────

/** 预览分段：dangling=true 的 wikilink 原文（含 ![[embed]] 形式）在浏览视图灰显。 */
export type PreviewSegment = { text: string; dangling: boolean };

const WIKILINK_RE = /!?\[\[[^\]]+\]\]/g;

/** splitDanglingPreview 把正文切为纯文本/dangling wikilink 分段。
 *  无 dangling 命中返回 null（调用方走纯文本渲染，避免大文档无谓切分）。
 *  target 口径与 blockparse 一致：`[[target|alias]]` 取 `|` 前 trim；`#heading`/`#^anchor` 保留。 */
export function splitDanglingPreview(content: string, danglingTargets: ReadonlySet<string>): PreviewSegment[] | null {
  if (!content || danglingTargets.size === 0 || !content.includes('[[')) return null;
  const segs: PreviewSegment[] = [];
  let last = 0;
  let found = false;
  WIKILINK_RE.lastIndex = 0;
  let m: RegExpExecArray | null;
  while ((m = WIKILINK_RE.exec(content))) {
    const raw = m[0];
    const inner = raw.slice(raw.startsWith('!') ? 3 : 2, -2);
    const target = inner.split('|')[0].trim();
    if (!danglingTargets.has(target)) continue;
    found = true;
    if (m.index > last) segs.push({ text: content.slice(last, m.index), dangling: false });
    segs.push({ text: raw, dangling: true });
    last = m.index + raw.length;
  }
  if (!found) return null;
  if (last < content.length) segs.push({ text: content.slice(last), dangling: false });
  return segs;
}

// ── SP1-I（I-3）：晋升到团队库 ──────────────────────────────────────────────

/** 晋升目标库选项：仅 team 后端库，排除源库（同库晋升无意义，后端亦拒绝）。 */
export function promoteTargetOptions(
  collections: KnowledgeCollection[],
  sourceCollectionId: string,
): Array<{ label: string; value: string }> {
  return collections
    .filter((c) => c.vault_backend === 'team' && c.id !== sourceCollectionId)
    .map((c) => ({ label: c.name || c.id, value: c.id }));
}
