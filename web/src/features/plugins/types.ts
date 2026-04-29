export type PaginatedResponse<T> = {
  items: T[];
  page: number;
  page_size: number;
  total: number;
};

export type PluginPermissions = {
  can_view: boolean;
  can_toggle: boolean;
  can_edit_config: boolean;
  can_view_logs: boolean;
};

export type Plugin = {
  id: string;
  key: string;
  name: string;
  description: string;
  category: string;
  risk_level: "low" | "medium" | "high" | string;
  enabled: boolean;
  scope: string;
  callback_points: string[];
  sort_order: number;
  config_schema_json: string;
  config_json: string;
  default_config_json: string;
  invoke_count: number;
  block_count: number;
  error_count: number;
  last_invoked_at?: string;
  last_status?: string;
  created_at: string;
  updated_at: string;
  permissions: PluginPermissions;
};

export type PluginListQuery = {
  search?: string;
  category?: string;
  enabled?: boolean | null;
  callback_point?: string;
  page?: number;
  page_size?: number;
};
