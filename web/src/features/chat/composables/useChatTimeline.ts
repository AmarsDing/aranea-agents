import { computed, type ComputedRef } from 'vue';
import type { Message, ReactToolLinkIndex } from '../types';
import type { TeamMemberLane } from '../../../components/chat/ChatTeamMemberStrip';
import { useChatMessageRow } from '../useChatMessageRow';
import { groupMessagesByTurn, type TurnBlockGroup } from '../groupMessagesByTurn';
import { useTurnBlockEnabled } from '../useTurnBlock';
import { toolEventFromMessage } from '../envelopeToolCall';
import { resolveDisplayLabel } from '../activityPresentation';
import { resolveAssistantPresentation } from '../messagePlannerPresentation';
import type { TimelineElement } from '../timelineTypes';

export type TimelineItem = { kind: 'block'; block: TurnBlockGroup } | { kind: 'message'; message: Message };

export function useChatTimeline(deps: {
  messages: ComputedRef<Message[]>;
  isTeamSession?: boolean;
  plannerKind?: ComputedRef<string>;
}) {
  const messageRow = useChatMessageRow(deps.messages);

  const teamMemberLanes = computed((): TeamMemberLane[] => {
    if (!deps.isTeamSession) return [];
    const lanes = new Map<string, TeamMemberLane>();
    for (const message of deps.messages.value) {
      if (!messageRow.isTeamMember(message)) continue;
      const key = messageRow.messageIdentityKey(message);
      const meta = messageRow.teamMemberMeta(message);
      const label = meta?.name || meta?.agent_key || messageRow.displayMessageName(message);
      const streaming = message.status === 'streaming' || message.status === 'tool_running';
      const prev = lanes.get(key);
      lanes.set(key, {
        key,
        label,
        streaming: (prev?.streaming ?? false) || streaming,
      });
    }
    return [...lanes.values()];
  });

  // D2: All modes enable TurnBlock (removed !deps.isTeamSession)
  const useTurnBlockMode = computed(() => useTurnBlockEnabled());

  const turnBlocks = computed((): TurnBlockGroup[] =>
    useTurnBlockMode.value ? groupMessagesByTurn(deps.messages.value) : [],
  );

  const timelineItems = computed((): TimelineItem[] => {
    if (useTurnBlockMode.value) {
      return turnBlocks.value.map((block) => ({ kind: 'block' as const, block }));
    }
    return deps.messages.value.map((message) => ({ kind: 'message' as const, message }));
  });

  // D2: TimelineElements — structured timeline with thinking/action/summary/end/error
  const timelineElements = computed((): TimelineElement[] => {
    if (!useTurnBlockMode.value) return [];
    const plannerKind = deps.plannerKind?.value ?? '';
    const elements: TimelineElement[] = [];

    for (const block of turnBlocks.value) {
      const reactLinkedToolIds = new Set<string>();

      // 0. User message
      if (block.user) {
        elements.push({
          kind: 'user',
          id: `${block.key}-user`,
          timestamp: block.user.created_at || '',
          content: block.user.content_markdown || '',
          collapsed: false,
        });
      }

      if (block.assistant) {
        const presentation = resolveAssistantPresentation(plannerKind, block.assistant);
        const steps = presentation.reactSteps?.steps;

        if (steps?.length) {
          for (const step of steps) {
            const isThinking = step.kind === 'planning' || step.kind === 'reasoning' || step.kind === 'replanning';
            const isAction = step.kind === 'action';

            if (isThinking) {
              elements.push({
                kind: 'thinking',
                id: `${block.key}-think-${elements.length}`,
                timestamp: block.assistant.created_at || '',
                reasoning: step.body,
                collapsed: true,
              });
            } else if (isAction) {
              let matchedTool: Message | undefined;
              for (const toolMsg of block.tools) {
                const toolEv = toolEventFromMessage(toolMsg);
                if (toolEv?.id && !reactLinkedToolIds.has(toolEv.id)) {
                  matchedTool = toolMsg;
                  reactLinkedToolIds.add(toolEv.id);
                  break;
                }
              }
              const toolEv = matchedTool ? toolEventFromMessage(matchedTool) : null;
              elements.push({
                kind: 'action',
                id: `${block.key}-action-${elements.length}`,
                timestamp: matchedTool?.created_at || block.assistant.created_at || '',
                reasoning: step.body,
                toolName: toolEv ? resolveDisplayLabel(toolEv) : undefined,
                toolStatus: toolEv?.status,
                toolDuration: toolEv?.duration_ms,
                toolCallId: toolEv?.id,
                toolArguments: toolEv?.arguments ? JSON.stringify(toolEv.arguments, null, 2) : undefined,
                toolResult: toolEv?.result ? (typeof toolEv.result === 'string' ? toolEv.result : JSON.stringify(toolEv.result, null, 2)) : undefined,
                collapsed: true,
              });
            }
          }
        }

        // F-6: Non-ReAct mode — generate thinking from reasoning
        if (!steps?.length && presentation.reasoning?.trim()) {
          elements.push({
            kind: 'thinking',
            id: `${block.key}-think-fallback`,
            timestamp: block.assistant.created_at || '',
            reasoning: presentation.reasoning,
            collapsed: true,
          });
        }

        // Standalone tool messages not covered by ReAct steps
        for (const toolMsg of block.tools) {
          const toolEv = toolEventFromMessage(toolMsg);
          if (toolEv?.id && reactLinkedToolIds.has(toolEv.id)) continue;

          // F-10: Generate error element for failed tools
          if (toolEv && (toolEv.status === 'error' || toolEv.status === 'failed') && toolEv.error) {
            elements.push({
              kind: 'error',
              id: `${block.key}-error-${toolMsg.id}`,
              timestamp: toolMsg.created_at || '',
              errorMessage: toolEv.error,
              toolName: resolveDisplayLabel(toolEv),
              collapsed: true,
            });
            continue;
          }

          elements.push({
            kind: 'action',
            id: `${block.key}-tool-${toolMsg.id}`,
            timestamp: toolMsg.created_at || '',
            toolName: toolEv ? resolveDisplayLabel(toolEv) : undefined,
            toolStatus: toolEv?.status,
            toolDuration: toolEv?.duration_ms,
            toolCallId: toolEv?.id,
            toolArguments: toolEv?.arguments ? JSON.stringify(toolEv.arguments, null, 2) : undefined,
            toolResult: toolEv?.result ? (typeof toolEv.result === 'string' ? toolEv.result : JSON.stringify(toolEv.result, null, 2)) : undefined,
            collapsed: true,
          });
        }

        // Summary: assistant body content
        const body = presentation.bodyMarkdown?.trim();
        if (body) {
          elements.push({
            kind: 'summary',
            id: `${block.key}-summary`,
            timestamp: block.assistant.created_at || '',
            content: body,
            collapsed: false,
          });
        }
      } else {
        // No assistant message — emit standalone tool messages
        for (const toolMsg of block.tools) {
          const toolEv = toolEventFromMessage(toolMsg);

          if (toolEv && (toolEv.status === 'error' || toolEv.status === 'failed') && toolEv.error) {
            elements.push({
              kind: 'error',
              id: `${block.key}-error-${toolMsg.id}`,
              timestamp: toolMsg.created_at || '',
              errorMessage: toolEv.error,
              toolName: toolEv ? resolveDisplayLabel(toolEv) : undefined,
              collapsed: true,
            });
            continue;
          }

          elements.push({
            kind: 'action',
            id: `${block.key}-tool-${toolMsg.id}`,
            timestamp: toolMsg.created_at || '',
            toolName: toolEv ? resolveDisplayLabel(toolEv) : undefined,
            toolStatus: toolEv?.status,
            toolDuration: toolEv?.duration_ms,
            toolCallId: toolEv?.id,
            toolArguments: toolEv?.arguments ? JSON.stringify(toolEv.arguments, null, 2) : undefined,
            toolResult: toolEv?.result ? (typeof toolEv.result === 'string' ? toolEv.result : JSON.stringify(toolEv.result, null, 2)) : undefined,
            collapsed: true,
          });
        }
      }

      // End element when block is completed
      if (block.isCompleted) {
        elements.push({
          kind: 'end',
          id: `${block.key}-end`,
          timestamp: block.assistant?.created_at || block.tools.at(-1)?.created_at || '',
          turnStatus: 'completed',
          collapsed: false,
        });
      }
    }

    return elements;
  });

  return {
    messageRow,
    teamMemberLanes,
    useTurnBlockMode,
    turnBlocks,
    timelineItems,
    timelineElements,
  };
}
