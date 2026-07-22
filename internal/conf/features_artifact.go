package conf

// M27 Phase 5: Artifact 本地打开文件夹功能开关。
// 默认关闭；仅本地单机部署时通过环境变量启用，生产/远程部署禁止开启。

// LocalRevealEnabled controls the POST /v1/system/reveal endpoint (M27 Phase 5).
// When enabled, the server can launch the OS file manager at an artifact's
// on-disk location. Enable only for local single-user deployments.
// Env: FEATURES_LOCAL_REVEAL_ENABLED (default: false)
func LocalRevealEnabled() bool {
	return parseBoolFlag("FEATURES_LOCAL_REVEAL_ENABLED")
}
