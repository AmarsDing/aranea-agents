export type SessionBatchScope = {
  owner_type?: string;
  agent_id?: string;
  team_id?: string;
  status?: string;
  context_status?: string;
  keyword?: string;
};

export type BatchPreviewResult = {
  matched: number;
  skipped_running: number;
  skipped_not_found: number;
  truncated: boolean;
  sample_ids: string[];
};

export type BatchOperationResult = {
  matched: number;
  processed: number;
  skipped_running: number;
  skipped_not_found: number;
  truncated: boolean;
  failed_ids: string[];
};

export type BulkProgress = {
  active: boolean;
  label: string;
  indeterminate?: boolean;
};

export type RetentionDialogMode = "archive" | "delete";
