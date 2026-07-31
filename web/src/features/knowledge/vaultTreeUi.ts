/**
 * vaultTreeUi：G1 资源管理器 V2 树/列表纯函数（无组件依赖，可单测）。
 *
 * - 树节点 key 编解码：库节点 `v|<cid>`，目录节点 `d|<cid>|<prefix>`（prefix 含 '/'）
 * - 节点视觉映射：库=cyan、目录=violet、md=teal、图片=magenta、音视频=orange、
 *   error=red 脉冲；颜色经 CSS 类落到组件作用域变量（双主题，见 KnowledgeVaultTree.vue）
 */

/** 库节点 key。 */
export function vaultNodeKey(collectionId: string): string {
  return `v|${collectionId}`;
}

/** 目录节点 key（prefix 带尾斜杠；根不用此函数，用 vaultNodeKey）。 */
export function dirNodeKey(collectionId: string, prefix: string): string {
  return `d|${collectionId}|${prefix}`;
}

export interface VaultTreeKeyRef {
  collectionId: string;
  /** 目录 prefix（带尾斜杠）；'' = 库根（库节点）。 */
  prefix: string;
}

/** 解析树节点 key；非法/空 key 返回 null。 */
export function parseVaultTreeKey(key: string): VaultTreeKeyRef | null {
  if (!key) return null;
  if (key.startsWith('v|')) {
    const cid = key.slice(2);
    return cid ? { collectionId: cid, prefix: '' } : null;
  }
  if (key.startsWith('d|')) {
    const rest = key.slice(2);
    const sep = rest.indexOf('|');
    if (sep <= 0) return null;
    return { collectionId: rest.slice(0, sep), prefix: rest.slice(sep + 1) };
  }
  return null;
}

// ---------- 节点视觉（科幻图标 + 配色类） ----------

export interface VaultNodeVisual {
  icon: string;
  /** kv-icon--cyan | --violet | --teal | --magenta | --orange | --red | --muted */
  cls: string;
  /** error 状态红色脉冲。 */
  pulse: boolean;
}

const IMAGE_EXTS = new Set(['png', 'jpg', 'jpeg', 'webp', 'gif', 'svg', 'bmp', 'ico']);
const AUDIO_EXTS = new Set(['mp3', 'wav', 'ogg', 'm4a', 'flac', 'aac']);
const VIDEO_EXTS = new Set(['mp4', 'webm', 'mov', 'mkv', 'avi']);
const MARKDOWN_EXTS = new Set(['md', 'markdown']);

function extOf(name: string): string {
  const i = name.lastIndexOf('.');
  if (i <= 0 || i === name.length - 1) return '';
  return name.slice(i + 1).toLowerCase();
}

/** 树/列表节点视觉：error 状态优先红色脉冲（保留类型图标），否则按类别着色。 */
export function vaultNodeVisual(input: { kind: string; name?: string; status?: string }): VaultNodeVisual {
  if (input.kind === 'dir') return { icon: 'folder', cls: 'kv-icon--violet', pulse: false };
  const name = input.name ?? '';
  const ext = extOf(name);
  let icon = 'insert_drive_file';
  let cls = 'kv-icon--muted';
  if (MARKDOWN_EXTS.has(ext)) {
    icon = 'article';
    cls = 'kv-icon--teal';
  } else if (IMAGE_EXTS.has(ext)) {
    icon = 'image';
    cls = 'kv-icon--magenta';
  } else if (AUDIO_EXTS.has(ext)) {
    icon = 'graphic_eq';
    cls = 'kv-icon--orange';
  } else if (VIDEO_EXTS.has(ext)) {
    icon = 'movie';
    cls = 'kv-icon--orange';
  }
  if (input.status === 'error') {
    return { icon, cls: 'kv-icon--red', pulse: true };
  }
  return { icon, cls, pulse: false };
}

/** 库节点视觉（固定 cyan）。 */
export function vaultRootVisual(): VaultNodeVisual {
  return { icon: 'inventory_2', cls: 'kv-icon--cyan', pulse: false };
}

// ---------- G3-F1 拖拽移动（V12.5） ----------

/** 拖拽中的文件（中栏文件行 dragstart 时记录）。 */
export interface DragFileRef {
  docId: string;
  name: string;
  /** 文件所在目录 prefix（带尾斜杠；根 = ''）。 */
  fromPrefix: string;
  /** 所属 vault（中栏列表恒为当前 vault；跨库拖拽禁止）。 */
  vaultId: string;
}

/** drop 目标（树目录节点 / 库节点=根 / 面包屑段）。 */
export interface DropTargetRef {
  vaultId: string;
  /** 目标目录 prefix（带尾斜杠；库根 = ''）。 */
  prefix: string;
}

/** 合法 drop 目标：同 vault（V12.5 跨库禁止）且非原地（目标目录 ≠ 文件当前目录）。 */
export function isValidDropTarget(drag: DragFileRef | null, target: DropTargetRef): boolean {
  if (!drag) return false;
  if (drag.vaultId !== target.vaultId) return false;
  return drag.fromPrefix !== target.prefix;
}
