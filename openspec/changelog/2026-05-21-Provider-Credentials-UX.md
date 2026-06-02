# Provider 凭据入库与前端掩码查看（2026-05-21）

## 变更摘要

- **入库**：`config_json` 中 `api_key` / `secret_key` / HA 候选密钥经 AES-256-GCM 加密为 `*_enc`；未配置 `ARANEA_CREDENTIAL_KEY` 时禁止提交明文凭据。
- **更新合并**：`mergeConfigJSONForUpdate` 在 PATCH 未带新明文时保留库内 `api_key_enc` 等字段，修复编辑保存清空密钥问题。
- **Reveal API**：`GET /v1/llm-provider-models/{id}/credentials` 供管理端按需解密查看（List/Get 仍脱敏）。
- **前端**：编辑表单与列表行默认 `••••••••` 掩码，点击眼睛图标拉取真实密钥；保存时仅在新输入非空时提交 `api_key`。

## 验证

```bash
make api && make build
go test ./internal/biz/... -run 'Credential|Merge'
cd web && pnpm build
```

## 凭据密钥来源（2026-05-21 更新）

- 默认：`system_settings.credential_encryption_key` 在 **服务启动** 时自动生成（32 字节 hex，条件 UPDATE 防并发覆盖）。
- 可选：环境变量 `ARANEA_CREDENTIAL_KEY`（hex/base64）覆盖 DB；**配置错误将显式报错**，不再静默回退。
- `GET /v1/system-settings` 为只读，不再触发写库。
- 无加密密钥时 **禁止** `api_key` 等明文写入 `config_json`。
- `GET /v1/llm-provider-models/{id}/credentials`：仅管理员控制台用途，FlowLog 审计（成功/失败均记录，不记录密钥值）。
- `config_json` 非法 JSON 统一 `400`；Create/Update 透传 `processConfigJSONForStorage` 业务错误（不再一律 `500`）。
