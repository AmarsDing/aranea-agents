import { computed } from 'vue';
import type { ComputedRef } from 'vue';
import type { Message } from '../../../domain/types';
import type { TodoItem, TodoBoardState } from '../agentTreeTypes';
import { toolEventFromMessage } from '../activityToolCall';

/** Simple hash from string to stable hex id (8 chars). */
function contentHash(s: string): string {
  let h = 0;
  for (let i = 0; i < s.length; i++) {
    h = ((h << 5) - h + s.charCodeAt(i)) | 0;
  }
  return (h >>> 0).toString(16).padStart(8, '0');
}

interface RawTodoItem {
  content: string;
  activeForm: string;
  status: string;
  id?: string;
}

interface TodoWriteOutput {
  todos: RawTodoItem[];
}

export interface TodoBoardEntry {
  agentKey: string;
  agentName: string;
  board: TodoBoardState;
}

function parseTodoItems(raw: unknown): TodoItem[] {
  if (!raw || typeof raw !== 'object') return [];
  const output = raw as TodoWriteOutput;
  if (!Array.isArray(output.todos)) return [];
  return output.todos
    .filter((t) => t && t.content && t.status)
    .map((t) => ({
      id: t.id || contentHash(t.content),
      content: t.content,
      activeForm: t.activeForm ?? '',
      status: t.status as TodoItem['status'],
      updatedAt: new Date().toISOString(),
    }));
}

/**
 * Composable that scans messages for the last successful `todo_write`
 * tool result and derives a TodoBoardState from it.
 */
export function useTodoBoard(messages: ComputedRef<Message[]>) {
  const todoBoardState = computed<TodoBoardState | null>(() => {
    const msgs = messages.value;
    if (!msgs || msgs.length === 0) return null;

    // Walk messages in reverse to find the last successful todo_write
    for (let i = msgs.length - 1; i >= 0; i--) {
      const msg = msgs[i];
      const ev = toolEventFromMessage(msg);
      if (!ev) continue;
      if (ev.tool_name !== 'todo_write') continue;
      if (ev.status !== 'success') continue;
      if (!ev.result) continue;

      const items = parseTodoItems(ev.result);
      if (items.length === 0) continue;

      return {
        todos: items,
        lastUpdated: ev.occurred_at || new Date().toISOString(),
        source: 'tool_result' as const,
      };
    }

    return null;
  });

  return { todoBoardState };
}
