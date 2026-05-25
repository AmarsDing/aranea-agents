export type ArtifactMeta = {
  id: string;
  session_id: string;
  name: string;
  mime_type: string;
  size: number;
  sha256: string;
  storage_kind: string;
  storage_uri: string;
  version: number;
  created_at: string;
};

export type ArtifactData = {
  meta: ArtifactMeta;
  /** artifact payload encoded in standard base64 */
  data_base64: string;
};

export type UploadArtifactInput = {
  session_id: string;
  name: string;
  mime_type?: string;
  data_base64: string;
};

export type ListArtifactsParams = {
  session_id?: string;
  limit?: number;
  offset?: number;
  query?: string;
  mime_type_prefix?: string;
};

export type ListArtifactsResult = {
  items: ArtifactMeta[];
  total: number;
};

export type ArtifactPreview = {
  meta: ArtifactMeta;
  preview_kind: string;
  text_content: string;
  data_base64: string;
};
