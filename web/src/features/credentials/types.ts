// E 阶段凭据托管（W1a）页面类型。secret 明文永不进入本层：
// 创建/轮换时明文只作为一次性请求体上行，列表/详情/审计均无明文字段。

export type CredentialSecretKind = 'password' | 'token' | 'cookie';
export type CredentialSensitivity = 'normal' | 'high';

export type CredentialRow = {
  id: string;
  workspace_id: string;
  name: string;
  site: string;
  url: string;
  username: string;
  secret_kind: CredentialSecretKind;
  sensitivity: CredentialSensitivity;
  note: string;
  created_at: string;
  updated_at: string;
};

export type CredentialExecutionRow = {
  id: string;
  credential_id: string;
  site: string;
  action: string;
  actor_type: string;
  actor_key: string;
  agent_id: string;
  session_id: string;
  verdict: string;
  reason: string;
  created_at: string;
};

export type CredentialFormModel = {
  name: string;
  site: string;
  url: string;
  username: string;
  secret_kind: CredentialSecretKind;
  sensitivity: CredentialSensitivity | '';
  note: string;
  /** 创建必填；编辑时留空 = 不轮换。 */
  secret: string;
};

export const CREDENTIAL_SECRET_KIND_OPTIONS: CredentialSecretKind[] = ['password', 'token', 'cookie'];

export const CREDENTIAL_VERDICTS = [
  'allowed',
  'denied_policy',
  'denied_sensitivity',
  'denied_not_found',
  'denied_no_credential',
  'failed',
] as const;

export function emptyCredentialForm(): CredentialFormModel {
  return {
    name: '',
    site: '',
    url: '',
    username: '',
    secret_kind: 'password',
    sensitivity: '',
    note: '',
    secret: '',
  };
}
