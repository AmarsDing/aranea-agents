/**
 * Long-task config presets aligned with docs/需求/17 channel.md §8.6 and IM Preview §12.9.
 */
import { FEISHU_IM_PREVIEW_DEFAULTS } from './channelImPreviewDefaults';

export type ChannelLongTaskPresetId =
  | ''
  | 'feishu_im_preview'
  | 'feishu_long_analysis'
  | 'feishu_ops_reasoning'
  | 'agent_heavy_tools'
  | 'team_pipeline'
  | 'async_background';

export type ChannelLongTaskPreset = {
  id: ChannelLongTaskPresetId;
  label: string;
  description: string;
  /** Partial config.config fields */
  config: Record<string, string | number | boolean>;
  /** Optional receive_mode override */
  receiveMode?: string;
};

export const CHANNEL_LONG_TASK_PRESETS: ChannelLongTaskPreset[] = [
  {
    id: 'feishu_im_preview',
    label: 'channelEditor.longTaskPresets.feishuImPreview',
    description: 'channelEditor.longTaskPresets.feishuImPreviewDesc',
    receiveMode: 'websocket',
    config: { ...FEISHU_IM_PREVIEW_DEFAULTS },
  },
  {
    id: 'feishu_long_analysis',
    label: 'channelEditor.longTaskPresets.feishuLongAnalysis',
    description: 'channelEditor.longTaskPresets.feishuLongAnalysisDesc',
    receiveMode: 'websocket',
    config: {
      ...FEISHU_IM_PREVIEW_DEFAULTS,
      turn_timeout_sec: 900,
      first_byte_timeout_sec: 45,
      progress_mode: 'steps',
      execution_mode: 'auto',
      im_render_mode: 'transcript',
      streaming_enabled: true,
    },
  },
  {
    id: 'feishu_ops_reasoning',
    label: 'channelEditor.longTaskPresets.feishuOpsReasoning',
    description: 'channelEditor.longTaskPresets.feishuOpsReasoningDesc',
    receiveMode: 'websocket',
    config: {
      ...FEISHU_IM_PREVIEW_DEFAULTS,
      im_render_mode: 'transcript_with_reasoning',
      im_show_reasoning: true,
      im_reasoning_max_chars: 800,
      im_tool_card_mode: 'feishu_append',
      im_split_overflow: true,
      im_max_preview_chars: 8000,
      progress_quiet_sec: 45,
      heartbeat_message: '仍在处理中… {{elapsed}}',
    },
  },
  {
    id: 'agent_heavy_tools',
    label: 'channelEditor.longTaskPresets.agentHeavyTools',
    description: 'channelEditor.longTaskPresets.agentHeavyToolsDesc',
    receiveMode: 'websocket',
    config: { ...FEISHU_IM_PREVIEW_DEFAULTS },
  },
  {
    id: 'team_pipeline',
    label: 'channelEditor.longTaskPresets.teamPipeline',
    description: 'channelEditor.longTaskPresets.teamPipelineDesc',
    config: {
      require_mention: true,
      streaming_enabled: true,
      ack_message: '收到，Team 正在协作处理…',
      im_render_mode: 'transcript',
      im_tool_detail: 'label_summary',
      im_team_mode: 'steps',
      im_show_reasoning: false,
      turn_timeout_sec: 900,
      first_byte_timeout_sec: 120,
      progress_quiet_sec: 20,
      heartbeat_message: '仍在处理中… {{elapsed}}',
      execution_mode: 'sync',
    },
  },
  {
    id: 'async_background',
    label: 'channelEditor.longTaskPresets.asyncBackground',
    description: 'channelEditor.longTaskPresets.asyncBackgroundDesc',
    config: {
      ack_message: '收到，已提交后台任务…',
      execution_mode: 'async',
      im_render_mode: 'reply_only',
      streaming_enabled: false,
    },
  },
];

export function findLongTaskPreset(id: string): ChannelLongTaskPreset | undefined {
  return CHANNEL_LONG_TASK_PRESETS.find((p) => p.id === id);
}

function configValueMatches(actual: unknown, expected: string | number | boolean): boolean {
  if (typeof expected === 'boolean') return Boolean(actual) === expected;
  if (typeof expected === 'number') {
    const num = Number(actual);
    return Number.isFinite(num) && num === expected;
  }
  return String(actual ?? '').trim() === expected;
}

/** Match saved channel config to a preset (for restoring the dropdown after reload). */
export function inferLongTaskPresetId(receiveMode: string, config: Record<string, unknown>): ChannelLongTaskPresetId {
  for (const preset of CHANNEL_LONG_TASK_PRESETS) {
    if (preset.receiveMode && preset.receiveMode !== receiveMode) continue;
    const matches = Object.entries(preset.config).every(([key, value]) => configValueMatches(config[key], value));
    if (matches) return preset.id;
  }
  return '';
}
