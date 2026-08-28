import type {
  Message,
  MessageAgentRef,
  MessageTeamMemberRef,
  MessageSourceMeta,
  MessageAttachmentRef,
} from '../../domain/types';

type RawOptions = {
  schema?: string;
  agent?: { id?: string; agent_key?: string; name?: string; display_name?: string; icon?: string };
  team_member?: { agent_id?: string; name?: string; role?: string; icon?: string; team_id?: string };
  member_agent_key?: string;
  display_name?: string;
  author?: string;
  source?: string;
  platform?: string;
  channel?: string;
  channel_key?: string;
  reasoning_markdown?: string;
  reasoning_content?: string;
  dialog_mode?: string;
  provider?: string;
  model?: string;
  attachments?: unknown[];
  tool_event?: unknown;
  send_meta?: { context_pct?: number };
  feedback?: unknown;
  intent_artifact?: { intent_kind?: string };
};

function parseOptionsJson(raw: string): RawOptions {
  const trimmed = raw?.trim();
  if (!trimmed) return {};
  try {
    return JSON.parse(trimmed) as RawOptions;
  } catch {
    return {};
  }
}

function extractAgentRef(opts: RawOptions): MessageAgentRef | null {
  const a = opts.agent;
  if (!a) return null;
  const key = a.agent_key?.trim() ?? '';
  const name = a.name?.trim() || a.display_name?.trim() || key;
  if (!key && !a.id) return null;
  return {
    id: a.id?.trim() ?? '',
    agent_key: key,
    name,
    icon: a.icon?.trim() ?? '',
  };
}

function extractTeamMember(opts: RawOptions): MessageTeamMemberRef | null {
  const tm = opts.team_member;
  if (tm?.agent_id?.trim()) {
    return {
      agent_id: tm.agent_id.trim(),
      name: tm.name?.trim() || tm.agent_id,
      role: tm.role?.trim() ?? '',
      icon: tm.icon?.trim() || undefined,
      team_id: tm.team_id?.trim() || undefined,
    };
  }
  if (opts.member_agent_key?.trim()) {
    return {
      agent_id: '',
      name: opts.display_name?.trim() || opts.member_agent_key,
      role: '',
    };
  }
  return null;
}

export const VALID_SOURCES = new Set<string>(['web', 'channel', 'cron', 'a2a', 'api']);

function extractSourceMeta(opts: RawOptions): MessageSourceMeta | null {
  const src = (opts.source ?? '').trim().toLowerCase();
  if (!src || !VALID_SOURCES.has(src)) return null;
  return {
    source: src as MessageSourceMeta['source'],
    platform: (opts.platform ?? opts.channel ?? '').trim() || undefined,
    channelKey: (opts.channel_key ?? '').trim() || undefined,
  };
}

function extractReasoning(opts: RawOptions): string | undefined {
  const md = (opts.reasoning_markdown ?? '').trim();
  if (md) return md;
  const rc = (opts.reasoning_content ?? '').trim();
  return rc || undefined;
}

function extractAttachments(opts: RawOptions): MessageAttachmentRef[] | undefined {
  if (!Array.isArray(opts.attachments) || opts.attachments.length === 0) return undefined;
  const out: MessageAttachmentRef[] = [];
  for (const item of opts.attachments) {
    if (!item || typeof item !== 'object') continue;
    const rec = item as Record<string, unknown>;
    const id = typeof rec.id === 'string' ? rec.id.trim() : '';
    if (!id) continue;
    out.push({
      id,
      name: typeof rec.name === 'string' ? rec.name : id,
      mime_type: typeof rec.mime_type === 'string' ? rec.mime_type : 'application/octet-stream',
      size: typeof rec.size === 'number' ? rec.size : undefined,
    });
  }
  return out.length > 0 ? out : undefined;
}

export function parseMessageOptions(
  optionsJson: string,
): Pick<
  Message,
  | 'agent_ref'
  | 'team_member'
  | 'source_meta'
  | 'reasoning_markdown'
  | 'dialog_mode'
  | 'provider'
  | 'model'
  | 'attachments'
  | 'tool_event'
> {
  const opts = parseOptionsJson(optionsJson);
  return {
    agent_ref: extractAgentRef(opts),
    team_member: extractTeamMember(opts),
    source_meta: extractSourceMeta(opts),
    reasoning_markdown: extractReasoning(opts),
    dialog_mode: opts.dialog_mode?.trim() || undefined,
    provider: opts.provider?.trim() || undefined,
    model: opts.model?.trim() || undefined,
    attachments: extractAttachments(opts),
    tool_event: opts.tool_event ?? undefined,
  };
}

export { parseOptionsJson };
export type { RawOptions };
