/**
 * 作战总台 ⌘K 命令注册表（P0 基础集；P1 动作全集）。
 * 结构比照 features/knowledge/commands.ts：静态描述 + 纯函数过滤。
 * 执行闭包由页面装配（组件不各自拉数）。
 */
export type WbCommandId =
  | 'refresh'
  | 'focus-you'
  | 'goto-organization'
  | 'goto-sessions'
  | 'goto-cron'
  | 'goto-overview'
  | 'close-drill';

export type WbCommandDef = {
  id: WbCommandId;
  icon: string;
  shortcut?: string;
  aliases?: readonly string[];
};

export const WB_COMMAND_DEFS: readonly WbCommandDef[] = [
  { id: 'refresh', icon: 'refresh', shortcut: 'R', aliases: ['refresh', 'reload', 'shuaxin'] },
  { id: 'focus-you', icon: 'how_to_reg', shortcut: 'Y', aliases: ['you', 'sign', 'qianshou', 'pidu'] },
  { id: 'goto-organization', icon: 'business', aliases: ['org', 'roster', 'zuzhi', 'huamingce'] },
  { id: 'goto-sessions', icon: 'history', aliases: ['session', 'history', 'huihua'] },
  { id: 'goto-cron', icon: 'schedule', aliases: ['cron', 'schedule', 'dingshi'] },
  { id: 'goto-overview', icon: 'dashboard', aliases: ['overview', 'cost', 'gailan'] },
  { id: 'close-drill', icon: 'close', shortcut: 'Esc', aliases: ['close', 'guanbi'] },
];

export type WbCommandItem = {
  def: WbCommandDef;
  title: string;
  disabled?: boolean;
};

/** 子序列匹配（轻量；不与 knowledge instantFilter 互借防域泄漏） */
export function filterWbCommands(items: readonly WbCommandItem[], query: string, limit = 20): WbCommandItem[] {
  const q = query.trim().toLowerCase();
  if (!q) return [...items];
  const match = (hay: string) => {
    let i = 0;
    for (const ch of hay.toLowerCase()) {
      if (ch === q[i]) i += 1;
      if (i >= q.length) return true;
    }
    return false;
  };
  return items.filter((c) => match(c.title) || (c.def.aliases ?? []).some(match)).slice(0, limit);
}
