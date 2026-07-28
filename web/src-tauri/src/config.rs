//! Backend upstream configuration, persisted as JSON under the app config dir.
//!
//! Desktop keeps working without any file (fallback `http://127.0.0.1:8000`).
//! Android requires an explicit remote URL (e.g. `https://aranea.example.com`)
//! entered on the in-app setup page; the value is applied at runtime without
//! restarting the app.

use serde::{Deserialize, Serialize};
use std::path::Path;

const FILE_NAME: &str = "backend-config.json";

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct BackendConfig {
    /// Normalized origin only, e.g. `https://aranea.example.com` (no path/query).
    url: String,
}

impl BackendConfig {
    /// Validate and normalize a user-supplied URL.
    pub fn new(raw_url: &str) -> Result<Self, String> {
        Ok(Self { url: normalize_url(raw_url)? })
    }

    pub fn http_base(&self) -> &str {
        &self.url
    }

    pub fn ws_base(&self) -> String {
        self.url.replacen("http", "ws", 1)
    }
}

fn normalize_url(raw: &str) -> Result<String, String> {
    let trimmed = raw.trim().trim_end_matches('/');
    if trimmed.is_empty() {
        return Err("URL is empty".into());
    }
    let parsed = tauri::Url::parse(trimmed).map_err(|e| format!("invalid URL: {e}"))?;
    // ws(s) is stored in its http(s) form; ws_base() derives the WS scheme.
    let http_form = match parsed.scheme() {
        "https" | "http" => trimmed.to_string(),
        "wss" => trimmed.replacen("wss://", "https://", 1),
        "ws" => trimmed.replacen("ws://", "http://", 1),
        s => return Err(format!("unsupported scheme: {s}")),
    };
    if parsed.host_str().is_none() {
        return Err("missing host".into());
    }
    if parsed.path() != "" && parsed.path() != "/" {
        return Err("must be an origin without a path".into());
    }
    if parsed.query().is_some() || parsed.fragment().is_some() {
        return Err("must not contain query or fragment".into());
    }
    Ok(http_form)
}

pub fn load(dir: &Path) -> Option<BackendConfig> {
    let data = std::fs::read(dir.join(FILE_NAME)).ok()?;
    serde_json::from_slice(&data).ok()
}

pub fn save(dir: &Path, cfg: &BackendConfig) -> std::io::Result<()> {
    std::fs::create_dir_all(dir)?;
    let data = serde_json::to_vec_pretty(cfg).expect("serialize BackendConfig");
    std::fs::write(dir.join(FILE_NAME), data)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn accepts_https_url_and_strips_trailing_slash() {
        let cfg = BackendConfig::new("https://aranea.example.com/").expect("valid");
        assert_eq!(cfg.http_base(), "https://aranea.example.com");
        assert_eq!(cfg.ws_base(), "wss://aranea.example.com");
    }

    #[test]
    fn accepts_http_for_lan_and_maps_ws_scheme() {
        let cfg = BackendConfig::new("http://192.168.1.10:8000").expect("valid");
        assert_eq!(cfg.http_base(), "http://192.168.1.10:8000");
        assert_eq!(cfg.ws_base(), "ws://192.168.1.10:8000");
    }

    #[test]
    fn normalizes_ws_scheme_to_http_form() {
        let cfg = BackendConfig::new("wss://aranea.example.com").expect("valid");
        assert_eq!(cfg.http_base(), "https://aranea.example.com");
        assert_eq!(cfg.ws_base(), "wss://aranea.example.com");
    }

    #[test]
    fn rejects_url_with_path_query_fragment_or_missing_host() {
        assert!(BackendConfig::new("https://example.com/api").is_err());
        assert!(BackendConfig::new("https://example.com?x=1").is_err());
        assert!(BackendConfig::new("https://example.com#frag").is_err());
        assert!(BackendConfig::new("not-a-url").is_err());
        assert!(BackendConfig::new("ftp://example.com").is_err());
        assert!(BackendConfig::new("").is_err());
    }

    #[test]
    fn roundtrip_save_and_load() {
        let dir = std::env::temp_dir().join(format!("aranea-cfg-test-{}", std::process::id()));
        let cfg = BackendConfig::new("https://aranea.example.com").expect("valid");
        save(&dir, &cfg).expect("save");
        let loaded = load(&dir).expect("load");
        assert_eq!(loaded, cfg);
        std::fs::remove_dir_all(&dir).ok();
    }

    #[test]
    fn load_returns_none_when_missing_or_corrupt() {
        let dir = std::env::temp_dir().join(format!("aranea-cfg-test-missing-{}", std::process::id()));
        assert!(load(&dir).is_none());
        std::fs::create_dir_all(&dir).unwrap();
        std::fs::write(dir.join("backend-config.json"), b"{ not json").unwrap();
        assert!(load(&dir).is_none());
        std::fs::remove_dir_all(&dir).ok();
    }
}
