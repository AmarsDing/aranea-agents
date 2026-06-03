export type AvatarAsset = {
  id: string;
  key: string;
  name: string;
  description: string;
  mime_type: string;
  workspace_id: string;
  owner_user_id: string;
  source: 'system' | 'upload';
  is_system: boolean;
  category: 'agent' | 'channel';
  file_size_bytes: number;
  width_px: number;
  height_px: number;
  sort_order: number;
  created_at: string;
};
