import type { ToolUseEvent } from '../types'

/**
 * 判断工具调用是否为 stuck（无返回结果）
 * 触发条件：error_code === 'tool_timeout'
 */
export function isStuckTool(event: ToolUseEvent): boolean {
  return event.error_code === 'tool_timeout'
}
