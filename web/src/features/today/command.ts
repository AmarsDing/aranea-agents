/**
 * 指令栏的纯函数：把一句话判成「交办 / 建任务 / 记录」，并认出交办对象。
 * 只做确定性的前缀与名字匹配，不猜语义——判错了用户可以在预览里改。
 */

export type CommandIntent = 'delegate' | 'task' | 'note';

export type Recipient = {
  kind: 'agent' | 'team';
  id: string;
  /** agent_key / team_key，发消息与 @ 匹配时用 */
  key: string;
  name: string;
};

export type ParsedCommand = {
  intent: CommandIntent;
  /** 去掉意图前缀与对象名之后的正文 */
  text: string;
  /** 认出的交办对象（`kind:id`）；没认出为空串 */
  recipientRef: string;
};

export const TITLE_MAX = 60;

export function recipientRef(r: Pick<Recipient, 'kind' | 'id'>): string {
  return `${r.kind}:${r.id}`;
}

// 「记：…」「记 …」需要分隔符（否则「记得买票」会被吞掉）；「记一下…」等不需要。
// 原文：/^(记一下|记一笔|记录一下|备忘)\s*[:：,，]?\s*/
const NOTE_LOOSE =
  /^(\u8bb0\u4e00\u4e0b|\u8bb0\u4e00\u7b14|\u8bb0\u5f55\u4e00\u4e0b|\u5907\u5fd8)\s*[:\uff1a,\uff0c]?\s*/;
// 原文：/^(记录|记|note)\s*[:：\s]\s*/i
const NOTE_STRICT = /^(\u8bb0\u5f55|\u8bb0|note)\s*[:\uff1a\s]\s*/i;
// 原文：/^(让|请|叫|交给|派给|安排|转给)\s*/
const DELEGATE_VERB = /^(\u8ba9|\u8bf7|\u53eb|\u4ea4\u7ed9|\u6d3e\u7ed9|\u5b89\u6392|\u8f6c\u7ed9)\s*/;
// 原文：/^[\s:：,，、]+/
const LEADING_SEPARATOR = /^[\s:\uff1a,\uff0c\u3001]+/;

/** 在 text 开头找名字最长的对象（显示名或 key，不分大小写）。 */
function matchRecipientAtStart(
  text: string,
  recipients: readonly Recipient[],
): { recipient: Recipient; rest: string } | null {
  const lower = text.toLowerCase();
  let best: { recipient: Recipient; length: number } | null = null;
  for (const recipient of recipients) {
    for (const label of [recipient.name, recipient.key]) {
      const candidate = label.trim().toLowerCase();
      if (!candidate || !lower.startsWith(candidate)) continue;
      if (!best || candidate.length > best.length) best = { recipient, length: candidate.length };
    }
  }
  if (!best) return null;
  return { recipient: best.recipient, rest: text.slice(best.length).replace(LEADING_SEPARATOR, '') };
}

export function parseCommand(raw: string, recipients: readonly Recipient[]): ParsedCommand {
  const text = raw.trim();
  if (!text) return { intent: 'task', text: '', recipientRef: '' };

  const note = NOTE_LOOSE.exec(text) ?? NOTE_STRICT.exec(text);
  if (note) return { intent: 'note', text: text.slice(note[0].length).trim(), recipientRef: '' };

  if (text.startsWith('@')) {
    const hit = matchRecipientAtStart(text.slice(1), recipients);
    // @ 是明确的交办意图：没认出对象也按交办处理，由用户在预览里选人。
    if (!hit) return { intent: 'delegate', text: text.slice(1).trim(), recipientRef: '' };
    return { intent: 'delegate', text: hit.rest, recipientRef: recipientRef(hit.recipient) };
  }

  const verb = DELEGATE_VERB.exec(text);
  if (verb) {
    // 「让我想想」「请假条」这类没有对象名的句子不是交办，落回建任务。
    const hit = matchRecipientAtStart(text.slice(verb[0].length), recipients);
    if (hit) return { intent: 'delegate', text: hit.rest, recipientRef: recipientRef(hit.recipient) };
  }

  return { intent: 'task', text, recipientRef: '' };
}

/** 取首行作标题，超长截断。 */
export function commandTitle(text: string, max = TITLE_MAX): string {
  const line = text.trim().split(/\r?\n/)[0]?.trim() ?? '';
  return line.length > max ? `${line.slice(0, max)}…` : line;
}

/** 交办后打开会话的站内链接。 */
export function sessionLink(recipient: Pick<Recipient, 'kind' | 'id'>, sessionId: string): string {
  const owner = recipient.kind === 'team' ? 'team' : 'agent';
  return `/chat?session=${encodeURIComponent(sessionId)}&${owner}=${encodeURIComponent(recipient.id)}`;
}
