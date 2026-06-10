/**
 * Default IM preview settings for Feishu long-task channels.
 * Aligned with docs/需求/17 channel.design.md §12.9.5 and execution-plan E-b.
 */
export const FEISHU_IM_PREVIEW_DEFAULTS: Record<string, string | number | boolean> = {
  streaming_enabled: true,
  // TECH-DEBT(#channel-locale-defaults): 默认文案硬编码为中文。修复路径：从 i18n 获取默认值，
  // 或在后端 ChannelType catalog 中提供 per-locale 默认文案，前端仅做 fallback。
  ack_message: '收到，正在处理…',
  im_render_mode: 'transcript',
  im_tool_detail: 'label_summary',
  im_show_reasoning: true,
  im_reasoning_max_chars: 800,
  im_max_preview_chars: 4000,
  im_split_overflow: true,
  im_tool_card_mode: 'off',
  turn_timeout_sec: 600,
  first_byte_timeout_sec: 120,
  progress_quiet_sec: 30,
  // TECH-DEBT(#channel-locale-defaults): 默认文案硬编码为中文。修复路径：从 i18n 获取默认值，
  // 或在后端 ChannelType catalog 中提供 per-locale 默认文案，前端仅做 fallback。
  heartbeat_message: '仍在处理中… {{elapsed}}',
  execution_mode: 'sync',
  progress_mode: 'text',
};

export const IM_PREVIEW_TEXT_KEYS = [
  'im_render_mode',
  'im_tool_detail',
  'im_reasoning_max_chars',
  'im_max_preview_chars',
  'im_tool_card_mode',
  'im_team_mode',
] as const;

export const IM_PREVIEW_BOOL_KEYS = new Set<string>(['im_show_reasoning', 'im_split_overflow']);

export function isImPreviewFormKey(key: string): boolean {
  return IM_PREVIEW_BOOL_KEYS.has(key) || (IM_PREVIEW_TEXT_KEYS as readonly string[]).includes(key);
}

export function applyFeishuImPreviewDefaults(boolDraft: Record<string, boolean>, textDraft: Record<string, string>) {
  for (const [key, value] of Object.entries(FEISHU_IM_PREVIEW_DEFAULTS)) {
    if (typeof value === 'boolean') {
      boolDraft[key] = value;
    } else {
      textDraft[key] = String(value);
    }
  }
}
