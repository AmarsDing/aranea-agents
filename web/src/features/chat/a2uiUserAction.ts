/** Client-to-server A2UI userAction envelope (51 消息机制 — WS user_message 正文). */

export type A2UIUserActionPayload = {
  name: string;
  surfaceId: string;
  sourceComponentId: string;
  timestamp: string;
  context: Record<string, unknown>;
};

export function buildUserActionPayload(input: {
  name: string;
  surfaceId: string;
  sourceComponentId: string;
  context: Record<string, unknown>;
}): A2UIUserActionPayload {
  return {
    name: input.name,
    surfaceId: input.surfaceId,
    sourceComponentId: input.sourceComponentId,
    timestamp: new Date().toISOString(),
    context: input.context,
  };
}

/** Single JSONL line for WS user_message.content (A2UI client-to-server). */
export function formatUserActionMessage(payload: A2UIUserActionPayload): string {
  return JSON.stringify({ userAction: payload });
}
