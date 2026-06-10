import { describe, expect, it } from 'vitest';
import { buildMessagePresentation } from '../messagePlannerPresentation';
import { buildReactToolLinkIndex, emptyReactToolLinkIndex } from '../reactToolLinkIndex';
import type { Message, ReactToolLinkIndex } from '../types';

function reactActionAndToolMessages(): Message[] {
  return [
    {
      id: 'm1',
      session_id: 's1',
      parent_message_id: '',
      turn_id: '',
      turn_number: 1,
      seq_in_turn: 0,
      role: 'assistant',
      content_markdown: '/*ACTION*/\nfunctions.search',
      model_name: '',
      token_in: 0,
      token_out: 0,
      latency_ms: 0,
      status: 'ok',
      attachments_count: 0,
      options_json: '{}',
      error_message: '',
      created_at: '2026-05-21T10:00:00Z',
    },
    {
      id: 'm2',
      session_id: 's1',
      parent_message_id: '',
      turn_id: '',
      turn_number: 1,
      seq_in_turn: 0,
      role: 'assistant',
      content_markdown: '',
      model_name: '',
      token_in: 0,
      token_out: 0,
      latency_ms: 0,
      status: 'tool_ok',
      attachments_count: 0,
      options_json: JSON.stringify({
        schema: 'chat.activity/v1',
        tool_event: {
          id: 'tc-1',
          phase: 'after',
          status: 'success',
          agent_id: 'a1',
          agent_key: 'agent',
          agent_name: 'Agent',
          tool_name: 'search',
          tool_label: 'search',
          occurred_at: '2026-05-21T10:00:01Z',
        },
      }),
      error_message: '',
      created_at: '2026-05-21T10:00:01Z',
    },
  ];
}

describe('buildMessagePresentation', () => {
  it('suppresses tool row when reactLinkIndex links tc-1 to prior ACTION', () => {
    const messages = reactActionAndToolMessages();
    const index = buildReactToolLinkIndex(messages);
    const bundle = buildMessagePresentation('react', messages[1], 1, index);
    expect(bundle.suppressToolRow).toBe(true);
  });

  it('does not suppress tool row with empty session index', () => {
    const messages = reactActionAndToolMessages();
    const bundle = buildMessagePresentation('react', messages[1], 1, emptyReactToolLinkIndex());
    expect(bundle.suppressToolRow).toBe(false);
  });

  it('shows ReAct steps without linked tools when index has no entry', () => {
    const messages = reactActionAndToolMessages();
    const bundle = buildMessagePresentation('react', messages[0], 0, emptyReactToolLinkIndex());
    expect(bundle.reactStepsWithTools).toHaveLength(1);
    expect(bundle.reactStepsWithTools[0].linkedTools).toEqual([]);
  });

  it('trusts empty cached steps array from index (no fallback remap)', () => {
    const messages = reactActionAndToolMessages();
    const index: ReactToolLinkIndex = {
      linkedToolIds: new Set(),
      stepsByAssistantIndex: new Map([[0, []]]),
    };
    const bundle = buildMessagePresentation('react', messages[0], 0, index);
    expect(bundle.reactStepsWithTools).toEqual([]);
  });

  it('hides reasoning details block in react planner mode', () => {
    const messages = reactActionAndToolMessages();
    messages[0].options_json = JSON.stringify({ reasoning_markdown: 'think step' });
    const bundle = buildMessagePresentation('react', messages[0], 0, emptyReactToolLinkIndex());
    expect(bundle.presentation.mode).toBe('react');
    expect(bundle.presentation.reasoning).toBe('');
  });
});
