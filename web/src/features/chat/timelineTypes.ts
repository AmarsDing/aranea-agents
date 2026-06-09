/** 时间线元素类型 */
export type TimelineElementKind = 'user' | 'thinking' | 'action' | 'summary' | 'end' | 'error';

/** 时间线元素 */
export interface TimelineElement {
  kind: TimelineElementKind;
  id: string;
  timestamp: string;
  /** thinking: reasoning 内容 */
  reasoning?: string;
  /** action: 工具调用信息 */
  toolName?: string;
  toolStatus?: string;
  toolDuration?: number;
  toolCallId?: string;
  toolArguments?: string;
  toolResult?: string;
  /** user/summary: 用户消息或最终回复内容 */
  content?: string;
  /** end: turn 完成状态 */
  turnStatus?: string;
  /** error: 错误信息 */
  errorMessage?: string;
  /** 折叠状态 */
  collapsed: boolean;
}
