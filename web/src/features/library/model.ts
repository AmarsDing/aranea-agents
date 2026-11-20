/**
 * 工作台 › 资料（/knowledge）的纯逻辑：空间归属、文件类型、目录面包屑、搜索结果按文件聚合。
 * 设计：docs/aranea-agents/design/2026-11-17-三大模块重设计/10-资料.md §3
 */
import type { KnowledgeChunk, KnowledgeCollection, KnowledgeDocument, VaultTreeNode } from '../knowledge/types';

export type LibrarySpace = 'mine' | 'company';
export const SPACES: LibrarySpace[] = ['mine', 'company'];

/** 资料库所在空间。后端已按「未登记 → team 归公司、其余归我的」给出；旧后端没有这个字段时前端照同一规则推。 */
export function spaceOf(c: Pick<KnowledgeCollection, 'space' | 'vault_backend'>): LibrarySpace {
  if (c.space === 'mine' || c.space === 'company') return c.space;
  return c.vault_backend === 'team' ? 'company' : 'mine';
}

/** 映射了本机文件夹的资料库（能建文件夹、新建 Markdown、上传到目录、移动、用本机程序打开）。 */
export function isLocalLibrary(c: Pick<KnowledgeCollection, 'vault_backend' | 'root_path'>): boolean {
  return c.vault_backend !== 'team' && !!c.root_path;
}

export type FileKind = 'doc' | 'xls' | 'ppt' | 'pdf' | 'md' | 'txt' | 'img' | 'av' | 'zip' | 'file';

export const KIND_LABEL: Record<FileKind, string> = {
  doc: 'DOC',
  xls: 'XLS',
  ppt: 'PPT',
  pdf: 'PDF',
  md: 'MD',
  txt: 'TXT',
  img: 'IMG',
  av: 'AV',
  zip: 'ZIP',
  file: 'FILE',
};

const EXT_KIND: Record<string, FileKind> = {
  doc: 'doc',
  docx: 'doc',
  docm: 'doc',
  rtf: 'doc',
  odt: 'doc',
  wps: 'doc',
  xls: 'xls',
  xlsx: 'xls',
  xlsm: 'xls',
  csv: 'xls',
  tsv: 'xls',
  ods: 'xls',
  et: 'xls',
  ppt: 'ppt',
  pptx: 'ppt',
  pps: 'ppt',
  ppsx: 'ppt',
  odp: 'ppt',
  dps: 'ppt',
  key: 'ppt',
  pdf: 'pdf',
  md: 'md',
  markdown: 'md',
  txt: 'txt',
  text: 'txt',
  log: 'txt',
  json: 'txt',
  yaml: 'txt',
  yml: 'txt',
  xml: 'txt',
  html: 'txt',
  htm: 'txt',
  png: 'img',
  jpg: 'img',
  jpeg: 'img',
  gif: 'img',
  webp: 'img',
  bmp: 'img',
  svg: 'img',
  heic: 'img',
  tif: 'img',
  tiff: 'img',
  mp3: 'av',
  wav: 'av',
  m4a: 'av',
  flac: 'av',
  aac: 'av',
  ogg: 'av',
  mp4: 'av',
  mov: 'av',
  mkv: 'av',
  webm: 'av',
  avi: 'av',
  zip: 'zip',
  rar: 'zip',
  '7z': 'zip',
  gz: 'zip',
  tar: 'zip',
};

export function fileKind(name: string, mime = ''): FileKind {
  const dot = name.lastIndexOf('.');
  const ext = dot >= 0 ? name.slice(dot + 1).toLowerCase() : '';
  if (EXT_KIND[ext]) return EXT_KIND[ext];
  if (mime.startsWith('image/')) return 'img';
  if (mime.startsWith('audio/') || mime.startsWith('video/')) return 'av';
  if (mime === 'application/pdf') return 'pdf';
  if (mime === 'text/markdown') return 'md';
  if (mime.startsWith('text/')) return 'txt';
  return 'file';
}

/** 预览方式：PDF / 图片 / 音视频看原文件；Markdown 渲染；其余（Word、Excel、PPT、文本）看抽出来的文字。 */
export type PreviewMode = 'markdown' | 'pdf' | 'image' | 'media' | 'text';

export function previewMode(kind: FileKind): PreviewMode {
  if (kind === 'md') return 'markdown';
  if (kind === 'pdf') return 'pdf';
  if (kind === 'img') return 'image';
  if (kind === 'av') return 'media';
  return 'text';
}

/** 节点 / 文档的显示名：路径最后一段。 */
export function baseName(path: string): string {
  const clean = path.replace(/\\/g, '/').replace(/\/+$/, '');
  return clean.slice(clean.lastIndexOf('/') + 1) || clean;
}

export function docName(d: Pick<KnowledgeDocument, 'rel_path' | 'source'>): string {
  return baseName(d.rel_path || d.source);
}

/** 文件夹在前，再按名字（中文按拼音、数字按大小）。 */
export function sortNodes(nodes: VaultTreeNode[]): VaultTreeNode[] {
  return [...nodes].sort((a, b) => {
    if (a.kind !== b.kind) return a.kind === 'dir' ? -1 : 1;
    return a.name.localeCompare(b.name, 'zh-CN', { numeric: true });
  });
}

/** 目录前缀 'a/b/' → 面包屑 [{a, 'a/'}, {b, 'a/b/'}]；根层为空数组。 */
export function crumbsOf(prefix: string): Array<{ name: string; path: string }> {
  const parts = prefix.split('/').filter(Boolean);
  return parts.map((name, i) => ({ name, path: `${parts.slice(0, i + 1).join('/')}/` }));
}

/** 文件所在目录：'a/b/c.md' → 'a/b/'；根层文件 → ''。 */
export function dirOf(relPath: string): string {
  const i = relPath.lastIndexOf('/');
  return i >= 0 ? relPath.slice(0, i + 1) : '';
}

/** 本机完整路径（展示用）：按根目录的分隔符拼接。 */
export function localPathOf(root: string, rel: string): string {
  if (!root) return '';
  const sep = root.includes('\\') || /^[A-Za-z]:/.test(root) ? '\\' : '/';
  const tail = rel.split('/').filter(Boolean).join(sep);
  if (!tail) return root;
  return root.endsWith(sep) ? root + tail : root + sep + tail;
}

/** 新文件名的合法性：不能为空、不能带路径分隔符或 Windows 保留字符。 */
export function validFileName(name: string): boolean {
  const n = name.trim();
  return !!n && n !== '.' && n !== '..' && !/[<>:"/\\|?*]/.test(n);
}

export type SearchHit = {
  docId: string;
  collectionId: string;
  name: string;
  relPath: string;
  /** 命中片段（文件名命中且没有内容命中时为空） */
  snippet: string;
  nameMatch: boolean;
  score: number;
};

const SNIPPET_MAX = 160;

function snippetOf(text: string): string {
  const flat = text.replace(/\s+/g, ' ').trim();
  return flat.length > SNIPPET_MAX ? `${flat.slice(0, SNIPPET_MAX)}…` : flat;
}

/** 文件名是否包含关键字（不分大小写）。 */
export function nameMatches(q: string, d: Pick<KnowledgeDocument, 'rel_path' | 'source'>): boolean {
  const needle = q.trim().toLowerCase();
  return !!needle && docName(d).toLowerCase().includes(needle);
}

/**
 * 搜索结果按文件聚合：文件名命中的排前面，再按内容命中的最高分；
 * 同一文件多个片段只留最高分那段。docs 用来把 chunk 的 doc_id 换成文件名和路径，查不到的丢掉（没权限或已删）。
 */
export function mergeSearchHits(
  nameHits: KnowledgeDocument[],
  chunks: KnowledgeChunk[],
  docs: ReadonlyMap<string, KnowledgeDocument>,
): SearchHit[] {
  const out = new Map<string, SearchHit>();
  for (const d of nameHits) {
    out.set(d.id, {
      docId: d.id,
      collectionId: d.collection_id,
      name: docName(d),
      relPath: d.rel_path,
      snippet: '',
      nameMatch: true,
      score: 0,
    });
  }
  for (const c of chunks) {
    const d = docs.get(c.doc_id);
    if (!d) continue;
    const prev = out.get(d.id);
    if (prev && prev.snippet && prev.score >= c.score) continue;
    out.set(d.id, {
      docId: d.id,
      collectionId: d.collection_id,
      name: docName(d),
      relPath: d.rel_path,
      snippet: snippetOf(c.content),
      nameMatch: prev?.nameMatch ?? false,
      score: Math.max(prev?.score ?? 0, c.score),
    });
  }
  return [...out.values()].sort((a, b) => Number(b.nameMatch) - Number(a.nameMatch) || b.score - a.score);
}
