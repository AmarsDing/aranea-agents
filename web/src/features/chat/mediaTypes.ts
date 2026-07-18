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

/** Progress info for long-running media generation tasks. */
export interface MediaProgress {
  value: number;
  max: number;
  label?: string;
}
