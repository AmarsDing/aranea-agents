package conf

import (
	"os"
	"strings"
)

// M27 Phase 5: Artifact 本地打开文件夹功能开关。
// 默认关闭；仅本地单机部署时通过环境变量启用，生产/远程部署禁止开启。

// LocalRevealEnabled controls the POST /v1/system/reveal endpoint (M27 Phase 5).
// When enabled, the server can launch the OS file manager at an artifact's
// on-disk location. Enable only for local single-user deployments.
// Env: FEATURES_LOCAL_REVEAL_ENABLED (default: false)
func LocalRevealEnabled() bool {
	return parseBoolFlag("FEATURES_LOCAL_REVEAL_ENABLED")
}

// ArtifactPublicBase returns the user-reachable base path of the artifact
// storage root (e.g. a UNC share `\\192.168.0.108\deploy102\...\artifacts`
// on remote server deployments). When set, artifact API responses map
// StorageUri to this base so users can locate produced files in their file
// manager; when empty the stored relative URI is returned (no leak).
// Env: ARANEA_ARTIFACT_PUBLIC_BASE (default: empty)
// 注意：108 IP 漂移时须随 CORS/前端配置同步更新该值（见项目记忆硬约束）。
func ArtifactPublicBase() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("ARANEA_ARTIFACT_PUBLIC_BASE")), "/\\")
}
