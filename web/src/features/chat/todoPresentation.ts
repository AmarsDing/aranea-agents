/**
 * TodoWrite tool presentation helpers.
 *
 * Extracts and transforms todo items from ToolUseEvent payloads
 * for display in TodoInlineList / TodoCard components.
 */

import type { ToolUseEvent } from './types';
import type { TodoItem } from './agentTreeTypes';

// ── Raw types matching the todo_write tool's JSON schema ──

interface RawTodoItem {
  content: string;
  activeForm: string;
  status: string;
  id?: string;
}

// ── Helpers ──

/** Simple hash from string to stable hex id (8 chars). */
function contentHash(s: string): string {
  let h = 0;
  for (let i = 0; i < s.length; i++) {
    h = ((h << 5) - h + s.charCodeAt(i)) | 0;
  }
  return (h >>> 0).toString(16).padStart(8, '0');
}

function parseRawItems(raw: unknown): RawTodoItem[] {
  if (!raw || typeof raw !== 'object') return [];
  const obj = raw as Record<string, unknown>;
  if (!Array.isArray(obj.todos)) return [];
  return obj.todos.filter(
    (item): item is RawTodoItem =>
      item != null && typeof item === 'object' && 'content' in item && 'status' in item,
  );
}

function rawToTodoItems(raw: RawTodoItem[]): TodoItem[] {
  return raw.map((item) => ({
    id: item.id || contentHash(item.content),
    content: item.content,
    activeForm: item.activeForm ?? '',
    status: item.status as TodoItem['status'],
    updatedAt: new Date().toISOString(),
  }));
}

// ── Public API ──

/**
 * Extract todo items from a ToolUseEvent.
 *
 * Priority:
 * 1. success → result.todos (actual state after execution)
 * 2. running/failed → arguments.todos (the declared plan)
 * 3. fallback → result.todos if arguments empty
 */
export function extractTodoItems(event: ToolUseEvent): TodoItem[] {
  const status = event.status;

  // For success, prefer result.todos (contains the actual state after execution)
  if (status === 'success' && event.result) {
    const items = parseRawItems(event.result);
    if (items.length > 0) return rawToTodoItems(items);
  }

  // For running/failed, use arguments.todos (the declared plan)
  if (event.arguments) {
    const items = parseRawItems(event.arguments);
    if (items.length > 0) return rawToTodoItems(items);
  }

  // Fallback: try result even for non-success
  if (event.result) {
    const items = parseRawItems(event.result);
    if (items.length > 0) return rawToTodoItems(items);
  }

  return [];
}
