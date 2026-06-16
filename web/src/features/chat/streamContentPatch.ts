import type { Message } from './types';

type StreamExtras = {
  reasoning_markdown?: string;
  reasoning_content?: string;
};

function reasoningFromExtras(extras: StreamExtras): string {
  return (extras.reasoning_markdown ?? extras.reasoning_content ?? '').trim();
}

export function parseMessageExtras(optionsJson: string): StreamExtras {
  try {
    return JSON.parse(optionsJson || '{}') as StreamExtras;
  } catch {
    return {};
  }
}

export function mergeMessageExtras(optionsJson: string, patch: StreamExtras): string {
  return JSON.stringify({ ...parseMessageExtras(optionsJson), ...patch });
}

/** Patch streaming assistant row: accumulate text and/or reasoning separately from final answer. */
export function patchStreamingMessage(
  messages: Message[],
  messageId: string,
  patch: { text?: string; reasoning?: string; replaceText?: string; replaceReasoning?: string; status?: string },
): Message[] {
  return messages.map((m) => {
    if (m.id !== messageId) return m;

    // H-01 fix: lifecycle state guard. Once a message reaches a terminal
    // status (ok, failed), incremental text/reasoning appends are rejected.
    // Only replaceText/replaceReasoning and status updates are allowed on
    // terminal messages. This prevents out-of-order text_delta events from
    // duplicating content after text_done has finalized the message.
    const isTerminal = m.status === 'ok' || m.status === 'failed';
    const allowAppend = !isTerminal;

    let content = m.content_markdown;
    if (patch.replaceText !== undefined) {
      content = patch.replaceText;
    } else if (allowAppend && patch.text) {
      content = `${content}${patch.text}`;
    }
    // reasoning_markdown is the single source of truth (no dual storage in options_json)
    let reasoning = m.reasoning_markdown?.trim() ?? '';
    if (patch.replaceReasoning !== undefined) {
      reasoning = patch.replaceReasoning;
    } else if (allowAppend && patch.reasoning) {
      reasoning = `${reasoning}${patch.reasoning}`;
    }
    return {
      ...m,
      content_markdown: content,
      status: patch.status ?? m.status,
      reasoning_markdown: reasoning.trim() || undefined,
    };
  });
}

export function reasoningMarkdown(message: Message): string {
  return message.reasoning_markdown?.trim() ?? '';
}

/** Map legacy reasoning_content from server rows to reasoning_markdown for UI replay. */
export function normalizeServerMessageOptions(optionsJson: string): string {
  const extras = parseMessageExtras(optionsJson);
  if (!reasoningFromExtras(extras) && extras.reasoning_content?.trim()) {
    return mergeMessageExtras(optionsJson, { reasoning_markdown: extras.reasoning_content.trim() });
  }
  if (extras.reasoning_markdown?.trim() && !extras.reasoning_content?.trim()) {
    return mergeMessageExtras(optionsJson, { reasoning_content: extras.reasoning_markdown.trim() });
  }
  return optionsJson;
}
