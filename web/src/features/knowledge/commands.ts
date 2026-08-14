/**
 * commands：⌘K 命令面板注册表（SP2 §SP2-7）。
 *
 * 注册表模式：静态描述数组 + 纯函数过滤（子序列匹配，复用 instantFilter）。
 * 命令执行闭包由 KnowledgeWorkbench 装配（数据流纪律：组件不各自拉数）。
 */
import { instantFilter } from './instantMatch';

export type CommandId =
  | 'new-note'
  | 'new-folder'
  | 'save'
  | 'toggle-mode'
  | 'open-graph'
  | 'switch-vault'
  | 'rebuild-index'
  | 'ingest-text'
  | 'promote'
  | 'close-tab'
  | 'apply-autolink'
  | 'backfill-autolink'
  | 'knowledge-health'
  | 'list-experts'
  | 'review-writeback';

export type CommandDef = {
  id: CommandId;
  icon: string;
  /** 快捷键展示文案（右对齐 dim 显示；实际接线在 KnowledgeWorkbench 全局 keydown） */
  shortcut?: string;
  /** P2-6：搜索别名（英文/拼音关键词），标题之外的可检索字段。 */
  aliases?: readonly string[];
};

/** 初版 9 条 + SP2-8 粘贴文本入库（SP2 §SP2-7）；顺序即默认展示顺序。 */
export const COMMAND_DEFS: readonly CommandDef[] = [
  { id: 'new-note', icon: 'note_add', aliases: ['new', 'create', 'xinjian', 'biji'] },
  { id: 'new-folder', icon: 'create_new_folder', aliases: ['folder', 'mkdir', 'wenjianjia'] },
  { id: 'save', icon: 'save', shortcut: 'Ctrl+S', aliases: ['save', 'write', 'baocun'] },
  { id: 'toggle-mode', icon: 'visibility', shortcut: 'Ctrl+E', aliases: ['preview', 'edit', 'yulan', 'moshi'] },
  { id: 'open-graph', icon: 'hub', shortcut: 'Ctrl+G', aliases: ['graph', 'hub', 'tupu'] },
  { id: 'switch-vault', icon: 'inventory_2', aliases: ['vault', 'switch', 'qiehuan'] },
  { id: 'rebuild-index', icon: 'refresh', aliases: ['rebuild', 'index', 'refresh', 'suoyin'] },
  { id: 'ingest-text', icon: 'content_paste', aliases: ['ingest', 'paste', 'ruku', 'zhantie'] },
  { id: 'promote', icon: 'upload', aliases: ['promote', 'upload', 'publish', 'fabu'] },
  { id: 'close-tab', icon: 'close', shortcut: 'Ctrl+W', aliases: ['close', 'guanbi'] },
  { id: 'apply-autolink', icon: 'add_link', aliases: ['autolink', 'wikilink', 'chenglian', 'shuangliian'] },
  { id: 'backfill-autolink', icon: 'link', aliases: ['backfill', 'huichong', 'piliang', 'chengliian'] },
  { id: 'knowledge-health', icon: 'monitor_heart', aliases: ['health', 'jiankang', 'orphan'] },
  { id: 'list-experts', icon: 'groups', aliases: ['expert', 'zhuanjia', 'who'] },
  { id: 'review-writeback', icon: 'rate_review', aliases: ['writeback', 'pending', 'xiehui', 'queren'] },
];

/** 命令项（标题由组件经 i18n 注入）。 */
export type CommandItem = {
  def: CommandDef;
  title: string;
  /** 无可执行上下文时禁用（如无活动 tab 的 save/close-tab） */
  disabled?: boolean;
};

/** MRU 记录（P2-6）：id 置顶、去重、截断到 keep 条。纯函数，持久化由调用方负责。 */
export function pushMru(mru: readonly CommandId[], id: CommandId, keep = 3): CommandId[] {
  return [id, ...mru.filter((x) => x !== id)].slice(0, keep);
}

/**
 * 模糊过滤命令：子序列匹配标题 + 别名（P2-6），打分排序（前缀 > 连续子串 > 散列，见 instantMatch）。
 * 空查询时 MRU 置顶（保持 mru 顺序），其余按注册顺序。
 */
export function filterCommands(
  items: readonly CommandItem[],
  query: string,
  mru: readonly CommandId[] = [],
  limit = 20,
): CommandItem[] {
  if (!query.trim()) {
    if (!mru.length) return [...items];
    const byId = new Map(items.map((c) => [c.def.id, c]));
    const top = mru.map((id) => byId.get(id)).filter((c): c is CommandItem => !!c);
    const rest = items.filter((c) => !mru.includes(c.def.id));
    return [...top, ...rest];
  }
  return instantFilter(items, query, (c) => [c.title, ...(c.def.aliases ?? [])], limit);
}
