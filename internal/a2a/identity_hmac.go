package a2a

import (
	"os"
	"strings"
)

// IdentityHMACSecretEnv 是签名身份通道（V-2）的舰队共享密钥环境变量：
// A2A 出站代理（internal/a2a/trpc/agent.go）用它给 X-User-ID 加签，
// A2A 入站服务（internal/service/a2a_extension_compat.go）用它验签。
// 两端同值部署（aranea→aranea）时身份头才会被对端采信；未配置则行为与
// P0 一致（匿名命名空间外的未签名身份头被丢弃）。
const IdentityHMACSecretEnv = "ARANEA_A2A_IDENTITY_HMAC_SECRET"

// IdentityHMACSecretFromEnv returns the fleet shared secret, trimmed.
// 密钥严禁入库/日志——只经环境变量注入（与 KRATOS_AUTH_SECRET 同纪律）。
func IdentityHMACSecretFromEnv() string {
	return strings.TrimSpace(os.Getenv(IdentityHMACSecretEnv))
}
