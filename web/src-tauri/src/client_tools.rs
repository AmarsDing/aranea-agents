//! Client tool Tauri commands (design 74-voice-companion §6.3).
//!
//! `client_open_app` / `client_open_url` execute on the USER'S machine. The
//! JS side calls them only for `client_tool.invoke` frames that passed the
//! server-side confirmation gate; this module re-enforces the launch
//! whitelist (open_app) and scheme policy (open_url) — JS input is never
//! trusted.
//!
//! Commands never throw across the bridge: every outcome is a structured
//! [`ClientToolResult`] so the JS side can map it 1:1 onto the
//! `client_tool.result` WS uplink.

use crate::whitelist::{is_bare_name, ResolveError, Whitelist};

/// Machine-readable failure codes surfaced to the Agent via the tool result.
pub const CODE_NOT_WHITELISTED: &str = "NOT_WHITELISTED";
pub const CODE_TARGET_NOT_FOUND: &str = "TARGET_NOT_FOUND";
pub const CODE_INVALID_URL: &str = "INVALID_URL";
pub const CODE_UNSUPPORTED: &str = "UNSUPPORTED_CAPABILITY";
pub const CODE_SPAWN_FAILED: &str = "SPAWN_FAILED";

/// Managed state holding the resolved launch whitelist.
pub struct ClientToolState {
    pub whitelist: Whitelist,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ClientToolError {
    pub code: &'static str,
    pub message: String,
}

/// Structured command outcome serialized to JS.
#[derive(Clone, Debug, PartialEq, Eq, serde::Serialize)]
pub struct ClientToolResult {
    pub ok: bool,
    pub output: String,
    pub error: String,
    pub error_code: String,
}

impl ClientToolResult {
    fn success(output: impl Into<String>) -> Self {
        Self {
            ok: true,
            output: output.into(),
            error: String::new(),
            error_code: String::new(),
        }
    }

    fn failure(err: ClientToolError) -> Self {
        Self {
            ok: false,
            output: String::new(),
            error: err.message,
            error_code: err.code.to_string(),
        }
    }
}

/// Map whitelist resolution failures to stable tool error codes.
pub fn resolve_app_target(wl: &Whitelist, target: &str) -> Result<String, ClientToolError> {
    wl.resolve(target).map_err(|e| match e {
        ResolveError::UnknownAlias(t) => ClientToolError {
            code: CODE_NOT_WHITELISTED,
            message: format!("target {t:?} is not in the client whitelist"),
        },
        ResolveError::NoUsableTarget(alias) => ClientToolError {
            code: CODE_TARGET_NOT_FOUND,
            message: format!(
                "whitelisted app {alias:?} was not found on this machine; fix the path in client-tools-whitelist.json"
            ),
        },
    })
}

/// Validate an open_url target: http/https only, bounded length, no
/// whitespace/control characters, non-empty host. Returns the trimmed URL.
pub fn validate_open_url(raw: &str) -> Result<String, ClientToolError> {
    let invalid = |message: String| ClientToolError {
        code: CODE_INVALID_URL,
        message,
    };
    let url = raw.trim();
    if url.is_empty() {
        return Err(invalid("url is empty".to_string()));
    }
    if url.len() > 2048 {
        return Err(invalid(format!("url too long ({} chars, max 2048)", url.len())));
    }
    if url.chars().any(|c| c.is_whitespace() || c.is_control()) {
        return Err(invalid("url contains whitespace/control characters".to_string()));
    }
    let lower = url.to_lowercase();
    let rest = lower
        .strip_prefix("http://")
        .or_else(|| lower.strip_prefix("https://"))
        .ok_or_else(|| invalid("only http/https URLs are allowed".to_string()))?;
    let host = rest.split(['/', '?', '#']).next().unwrap_or("");
    if host.is_empty() {
        return Err(invalid("url has no host".to_string()));
    }
    Ok(url.to_string())
}

fn unsupported() -> ClientToolResult {
    ClientToolResult::failure(ClientToolError {
        code: CODE_UNSUPPORTED,
        message: "client tools are not supported on this platform".to_string(),
    })
}

// ─── OS launch backends (side-effecting; covered by真机 acceptance V2-T8) ───

#[cfg(target_os = "windows")]
fn spawn_app(resolved: &str) -> Result<String, ClientToolError> {
    // Bare names go through `start` so Windows App Paths / PATH resolution
    // applies; absolute paths spawn directly (detached, no shell window).
    let result = if is_bare_name(resolved) {
        std::process::Command::new("cmd.exe")
            .args(["/C", "start", "", resolved])
            .spawn()
    } else {
        std::process::Command::new(resolved).spawn()
    };
    result
        .map(|_| format!("launched {resolved}"))
        .map_err(|e| ClientToolError {
            code: CODE_SPAWN_FAILED,
            message: format!("failed to launch {resolved:?}: {e}"),
        })
}

#[cfg(target_os = "windows")]
fn spawn_url(url: &str) -> Result<String, ClientToolError> {
    // rundll32 FileProtocolHandler avoids cmd re-parsing of `&` in query strings.
    std::process::Command::new("rundll32.exe")
        .args(["url.dll,FileProtocolHandler", url])
        .spawn()
        .map(|_| format!("opened {url}"))
        .map_err(|e| ClientToolError {
            code: CODE_SPAWN_FAILED,
            message: format!("failed to open url: {e}"),
        })
}

#[cfg(target_os = "macos")]
fn spawn_app(resolved: &str) -> Result<String, ClientToolError> {
    // `open -a` resolves app bundle names; absolute paths use `open` directly.
    let mut cmd = std::process::Command::new("open");
    if is_bare_name(resolved) {
        cmd.args(["-a", resolved]);
    } else {
        cmd.arg(resolved);
    }
    cmd.spawn()
        .map(|_| format!("launched {resolved}"))
        .map_err(|e| ClientToolError {
            code: CODE_SPAWN_FAILED,
            message: format!("failed to launch {resolved:?}: {e}"),
        })
}

#[cfg(target_os = "macos")]
fn spawn_url(url: &str) -> Result<String, ClientToolError> {
    std::process::Command::new("open")
        .arg(url)
        .spawn()
        .map(|_| format!("opened {url}"))
        .map_err(|e| ClientToolError {
            code: CODE_SPAWN_FAILED,
            message: format!("failed to open url: {e}"),
        })
}

#[cfg(all(unix, not(target_os = "macos"), not(target_os = "android")))]
fn spawn_app(resolved: &str) -> Result<String, ClientToolError> {
    std::process::Command::new(resolved)
        .spawn()
        .map(|_| format!("launched {resolved}"))
        .map_err(|e| ClientToolError {
            code: CODE_SPAWN_FAILED,
            message: format!("failed to launch {resolved:?}: {e}"),
        })
}

#[cfg(all(unix, not(target_os = "macos"), not(target_os = "android")))]
fn spawn_url(url: &str) -> Result<String, ClientToolError> {
    std::process::Command::new("xdg-open")
        .arg(url)
        .spawn()
        .map(|_| format!("opened {url}"))
        .map_err(|e| ClientToolError {
            code: CODE_SPAWN_FAILED,
            message: format!("failed to open url: {e}"),
        })
}

// ─── Tauri commands ─────────────────────────────────────────────────────────

/// Open an application on the user's desktop. `target` must resolve via the
/// Rust-side whitelist (defaults ∪ user overrides).
#[tauri::command]
pub fn client_open_app(state: tauri::State<ClientToolState>, target: String) -> ClientToolResult {
    #[cfg(target_os = "android")]
    {
        let _ = (state, target);
        return unsupported();
    }
    #[cfg(not(target_os = "android"))]
    match resolve_app_target(&state.whitelist, &target).and_then(|r| spawn_app(&r)) {
        Ok(output) => ClientToolResult::success(output),
        Err(err) => ClientToolResult::failure(err),
    }
}

/// Open an http/https URL in the user's default browser.
#[tauri::command]
pub fn client_open_url(url: String) -> ClientToolResult {
    #[cfg(target_os = "android")]
    {
        let _ = url;
        return unsupported();
    }
    #[cfg(not(target_os = "android"))]
    match validate_open_url(&url).and_then(|u| spawn_url(&u)) {
        Ok(output) => ClientToolResult::success(output),
        Err(err) => ClientToolResult::failure(err),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::whitelist::WhitelistEntry;

    fn wl_with(entries: &[(&str, &[&str])]) -> Whitelist {
        let mut wl = Whitelist::default();
        for (alias, targets) in entries {
            // Reuse the public merge path so tests exercise the same
            // normalization as user overrides.
            let json = serde_json::json!({
                "entries": [{ "alias": alias, "targets": targets }]
            });
            wl.merge_user_json(&json.to_string()).unwrap();
        }
        wl
    }

    // --- resolve_app_target mapping ---

    #[test]
    fn unwhitelisted_target_maps_to_not_whitelisted() {
        let wl = wl_with(&[("wechat", &["WeChat.exe"])]);
        let err = resolve_app_target(&wl, "steam").unwrap_err();
        assert_eq!(err.code, CODE_NOT_WHITELISTED);
        assert!(err.message.contains("steam"));
    }

    #[test]
    fn missing_whitelisted_app_maps_to_target_not_found() {
        let wl = wl_with(&[("gone", &[r"Z:\nonexistent\gone.exe"])]);
        let err = resolve_app_target(&wl, "gone").unwrap_err();
        assert_eq!(err.code, CODE_TARGET_NOT_FOUND);
    }

    #[test]
    fn whitelisted_bare_name_resolves() {
        let wl = wl_with(&[("wechat", &["WeChat.exe"])]);
        assert_eq!(resolve_app_target(&wl, "wechat").unwrap(), "WeChat.exe");
    }

    // --- validate_open_url ---

    #[test]
    fn http_and_https_urls_pass() {
        assert_eq!(
            validate_open_url("https://example.com/a?b=1&c=2").unwrap(),
            "https://example.com/a?b=1&c=2"
        );
        assert!(validate_open_url("http://localhost:8000").is_ok());
        assert!(validate_open_url("HTTPS://EXAMPLE.COM").is_ok());
        // Trimming is applied.
        assert_eq!(
            validate_open_url("  https://example.com  ").unwrap(),
            "https://example.com"
        );
    }

    #[test]
    fn non_http_schemes_are_rejected() {
        for bad in [
            "file:///C:/Windows/System32/drivers/etc/hosts",
            "javascript:alert(1)",
            "ftp://example.com/x",
            "ms-settings:privacy",
            "weixin://dl/chat",
            "C:\\Windows\\System32\\cmd.exe",
        ] {
            let err = validate_open_url(bad).unwrap_err();
            assert_eq!(err.code, CODE_INVALID_URL, "{bad} must be rejected");
        }
    }

    #[test]
    fn malformed_urls_are_rejected() {
        for bad in ["", "   ", "https://", "http://?x=1", "https://exa mple.com"] {
            assert_eq!(
                validate_open_url(bad).unwrap_err().code,
                CODE_INVALID_URL,
                "{bad:?} must be rejected"
            );
        }
    }

    #[test]
    fn oversized_url_is_rejected() {
        let long = format!("https://example.com/{}", "a".repeat(2050));
        assert_eq!(validate_open_url(&long).unwrap_err().code, CODE_INVALID_URL);
    }

    // --- result envelope mapping ---

    #[test]
    fn result_envelope_success_and_failure() {
        let ok = ClientToolResult::success("launched wechat");
        assert!(ok.ok);
        assert_eq!(ok.output, "launched wechat");
        assert!(ok.error.is_empty());
        assert!(ok.error_code.is_empty());

        let fail = ClientToolResult::failure(ClientToolError {
            code: CODE_NOT_WHITELISTED,
            message: "nope".to_string(),
        });
        assert!(!fail.ok);
        assert!(fail.output.is_empty());
        assert_eq!(fail.error, "nope");
        assert_eq!(fail.error_code, CODE_NOT_WHITELISTED);
    }

    #[test]
    fn result_serializes_to_expected_json_shape() {
        // The JS side maps these fields onto the client_tool.result uplink;
        // keep field names stable.
        let v = serde_json::to_value(ClientToolResult::failure(ClientToolError {
            code: CODE_INVALID_URL,
            message: "bad".to_string(),
        }))
        .unwrap();
        assert_eq!(v["ok"], false);
        assert_eq!(v["error_code"], CODE_INVALID_URL);
        assert!(v.get("output").is_some());
        assert!(v.get("error").is_some());
    }
}
