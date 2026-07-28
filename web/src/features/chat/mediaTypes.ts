/** A single media artifact produced by a media generation tool. */
export interface MediaArtifact {
  artifact_id: string;
  url: string;
  mime_type: string; // "image/png" / "video/mp4"
  width?: number;
  height?: number;
  duration_ms?: number;
  thumbnail?: string; // video poster URL
}

/**
 * Media generation tool names（后端工具注册名）。
 * 单一来源：媒体产物提取（activityV2Store）与工具分类（classifyTool）
 * 共用此常量，避免两处硬编码漂移。
 */
export const MEDIA_TOOL_NAMES: readonly string[] = ['generate_image', 'generate_video', 'image_to_video'];

/** Progress info for long-running media generation tasks. */
export interface MediaProgress {
  value: number;
  max: number;
  label?: string;
}
