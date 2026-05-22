/**
 * Long-task config presets aligned with docs/需求/17 channel.md §8.6 and IM Preview §12.9.
 */
import { FEISHU_IM_PREVIEW_DEFAULTS } from "./channelImPreviewDefaults";

export type ChannelLongTaskPresetId =
  | ""
  | "feishu_im_preview"
  | "feishu_ops_reasoning"
  | "agent_heavy_tools"
  | "team_pipeline"
  | "async_background";

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
    id: "feishu_im_preview",
    label: "飞书 · IM Preview（推荐）",
    description: "流式 transcript + 工具/MCP 有序展示；ACK 合并单条 preview（§12.9）",
    receiveMode: "websocket",
    config: { ...FEISHU_IM_PREVIEW_DEFAULTS }
  },
  {
    id: "feishu_ops_reasoning",
    label: "飞书 · 运维 / 思考链可见",
    description: "Transcript + 思考链；工具卡片 append；超长自动分页",
    receiveMode: "websocket",
    config: {
      ...FEISHU_IM_PREVIEW_DEFAULTS,
      im_render_mode: "transcript_with_reasoning",
      im_show_reasoning: true,
      im_reasoning_max_chars: 800,
      im_tool_card_mode: "feishu_append",
      im_split_overflow: true,
      im_max_preview_chars: 8000,
      progress_quiet_sec: 45,
      heartbeat_message: "仍在处理中… {{elapsed}}"
    }
  },
  {
    id: "agent_heavy_tools",
    label: "单 Agent · 重工具 / 长生成",
    description: "同「飞书 IM Preview」；Turn 10 分钟、首字节 120 秒",
    receiveMode: "websocket",
    config: { ...FEISHU_IM_PREVIEW_DEFAULTS }
  },
  {
    id: "team_pipeline",
    label: "Team 流水线 · 群 @",
    description: "Team 成员 inline 摘要；Turn 15 分钟",
    config: {
      require_mention: true,
      streaming_enabled: true,
      ack_message: "收到，Team 正在协作处理…",
      im_render_mode: "transcript",
      im_tool_detail: "label_summary",
      im_team_mode: "steps",
      im_show_reasoning: false,
      turn_timeout_sec: 900,
      first_byte_timeout_sec: 120,
      progress_quiet_sec: 20,
      heartbeat_message: "仍在处理中… {{elapsed}}",
      execution_mode: "sync"
    }
  },
  {
    id: "async_background",
    label: "超长任务 · Graph/Cron 异步",
    description: "全部入站走 async；需填写 async_team_id、async_graph_id 或 async_cron_task_id",
    config: {
      ack_message: "收到，已提交后台任务…",
      execution_mode: "async",
      im_render_mode: "reply_only",
      streaming_enabled: false
    }
  }
];

export function findLongTaskPreset(id: string): ChannelLongTaskPreset | undefined {
  return CHANNEL_LONG_TASK_PRESETS.find((p) => p.id === id);
}
