# Provider 模块优化（2026-05-21）

## 摘要

按 `9-provider-development.md` 完成 Inspect 扩展、llminspect 专属探测、AES-256-GCM 凭据加密、运行时限流、健康检查定时任务、HuggingFace/Bedrock 注册，以及前端四步表单与列表 Chips。

## 后端

- **Proto**：`InspectProviderModelRequest/Response` 增加 variant、secret_id/secret_key、aws_region、enable_token_tailoring、supports_cache/supports_thinking。
- **Biz**：`InspectMerge` 扩展；`needInspectMerge` 支持 Hunyuan/Bedrock 凭据；`mergeInspectConfigJSON` 合并 7 字段；`credential_crypto.go` AES-256-GCM（`ARANEA_CREDENTIAL_KEY`）；List/Get 脱敏，运行时 `GetByProviderAndModel` 解密。
- **llminspect**：Gemini / Ollama / Hunyuan / Bedrock 专属路由与单测。
- **Provider**：`register_extra.go` 注册 huggingface/bedrock；`rate_limit_transport.go`；`RunHealthChecks` + `ProviderHealthScanner`（5min，`PROVIDER_HEALTH_DISABLED=1` 可关）。

## 前端

- `ProviderModelRow`：Variant / HA Chip；密钥状态含 secret/aws。
- `ProviderHAConfig.vue`；`ResourceManagerPage` 四步 QStepper + 类型筛选 + Inspect 新字段。

## 运维

- 生产需设置 32 字节 `ARANEA_CREDENTIAL_KEY`（hex 或 base64）后新建/更新 Provider 才会加密存库。
- 已有明文 `api_key` 在下次 Update 带新密钥或运维脚本迁移时会被加密。

## 验证

```bash
make api && make wire && make build && make test && make runtime-boundary
cd web && pnpm build
```
