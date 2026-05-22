/**
 * Default IM preview settings for Feishu long-task channels.
 * Aligned with docs/需求/17 channel.design.md §12.9.5 and execution-plan E-b.
 */
export const FEISHU_IM_PREVIEW_DEFAULTS: Record<string, string | number | boolean> = {
  streaming_enabled: true,
  ack_message: "收到，正在处理…",
  im_render_mode: "transcript",
  im_tool_detail: "label_summary",
  im_show_reasoning: false,
  im_max_preview_chars: 4000,
  im_split_overflow: true,
  im_tool_card_mode: "off",
  turn_timeout_sec: 600,
  first_byte_timeout_sec: 120,
  progress_quiet_sec: 30,
  heartbeat_message: "仍在处理中… {{elapsed}}",
  execution_mode: "sync"
};

export function applyFeishuImPreviewDefaults(
  boolDraft: Record<string, boolean>,
  textDraft: Record<string, string>
) {
  for (const [key, value] of Object.entries(FEISHU_IM_PREVIEW_DEFAULTS)) {
    if (typeof value === "boolean") {
      boolDraft[key] = value;
    } else {
      textDraft[key] = String(value);
    }
  }
}
