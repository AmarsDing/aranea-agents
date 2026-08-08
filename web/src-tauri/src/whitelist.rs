//! Client-tool launch whitelist (design 74-voice-companion §6.3).
//!
//! The Agent-facing `client_open_app` tool sends a free-text `target`; this
//! module is the Rust-side enforcement point: only aliases registered in the
//! whitelist (built-in defaults ∪ user overrides) may be launched, so a
//! hallucinated or malicious path/alias is rejected before any process spawn.
//!
//! User overrides live in `<config_dir>/client-tools-whitelist.json`:
//! `{ "entries": [{ "alias": "wechat", "targets": ["D:\\Apps\\WeChat\\WeChat.exe"] }] }`
//! User entries replace same-name defaults and may add new aliases.
//!
//! Candidate ordering matters: absolute-path candidates are tried first
//! (must exist on disk), a bare executable name is always acceptable as the
//! last fallback (resolved by the OS via App Paths / PATH at spawn time).

use std::collections::HashMap;
use std::path::Path;

/// One whitelist row: launch alias → ordered launch candidates.
#[derive(Clone, Debug, PartialEq, Eq, serde::Deserialize)]
pub struct WhitelistEntry {
    pub alias: String,
    pub targets: Vec<String>,
}

/// Alias → candidates map. Keys are lowercased aliases.
#[derive(Clone, Debug, Default)]
pub struct Whitelist {
    entries: HashMap<String, Vec<String>>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub enum ResolveError {
    /// Alias not present in the whitelist (covers empty/path-injection input).
    UnknownAlias(String),
    /// Alias known, but every candidate is a missing absolute path and no
    /// bare-name fallback exists.
    NoUsableTarget(String),
}

/// User override file name inside the app config dir.
pub const USER_WHITELIST_FILE: &str = "client-tools-whitelist.json";

/// True for a bare executable name (no path separators / drive qualifiers).
pub fn is_bare_name(candidate: &str) -> bool {
    !candidate.contains(['/', '\\', ':'])
}

/// Expand `%VAR%` sequences using process env (Windows-style paths in the
/// default table). Unknown vars are left verbatim so the existence check
/// simply fails and resolution falls through to the next candidate.
#[cfg(windows)]
pub fn expand_windows_env(input: &str) -> String {
    let mut out = String::with_capacity(input.len());
    let mut rest = input;
    while let Some(start) = rest.find('%') {
        out.push_str(&rest[..start]);
        let tail = &rest[start + 1..];
        match tail.find('%') {
            Some(end) if end > 0 => {
                let var = &tail[..end];
                match std::env::var(var) {
                    Ok(val) => out.push_str(&val),
                    Err(_) => {
                        out.push('%');
                        out.push_str(var);
                        out.push('%');
                    }
                }
                rest = &tail[end + 1..];
            }
            _ => {
                out.push('%');
                out.push_str(tail);
                rest = "";
            }
        }
    }
    out.push_str(rest);
    out
}

/// Non-Windows builds never carry `%VAR%` placeholders in candidates.
#[cfg(not(windows))]
pub fn expand_windows_env(input: &str) -> String {
    input.to_string()
}

fn normalize_alias(alias: &str) -> String {
    alias.trim().to_lowercase()
}

fn default_entries() -> Vec<WhitelistEntry> {
    platform_defaults()
}

#[cfg(windows)]
fn platform_defaults() -> Vec<WhitelistEntry> {
    let entries: &[(&str, &[&str])] = &[
        (
            "chrome",
            &[
                r"C:\Program Files\Google\Chrome\Application\chrome.exe",
                r"C:\Program Files (x86)\Google\Chrome\Application\chrome.exe",
                "chrome.exe",
            ],
        ),
        (
            "edge",
            &[
                r"C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe",
                r"C:\Program Files\Microsoft\Edge\Application\msedge.exe",
                "msedge.exe",
            ],
        ),
        (
            "firefox",
            &[r"C:\Program Files\Mozilla Firefox\firefox.exe", "firefox.exe"],
        ),
        (
            "wechat",
            &[
                r"C:\Program Files\Tencent\WeChat\WeChat.exe",
                r"C:\Program Files (x86)\Tencent\WeChat\WeChat.exe",
                "WeChat.exe",
            ],
        ),
        ("qq", &["QQ.exe"]),
        ("dingtalk", &["DingTalk.exe"]),
        (
            "vscode",
            &[r"%LOCALAPPDATA%\Programs\Microsoft VS Code\Code.exe", "Code.exe"],
        ),
        ("notepad", &["notepad.exe"]),
    ];
    entries
        .iter()
        .map(|(alias, targets)| WhitelistEntry {
            alias: (*alias).to_string(),
            targets: targets.iter().map(|t| (*t).to_string()).collect(),
        })
        .collect()
}

#[cfg(target_os = "macos")]
fn platform_defaults() -> Vec<WhitelistEntry> {
    let entries: &[(&str, &[&str])] = &[
        ("chrome", &["Google Chrome"]),
        ("safari", &["Safari"]),
        ("firefox", &["Firefox"]),
        ("wechat", &["WeChat"]),
        ("vscode", &["Visual Studio Code"]),
    ];
    entries
        .iter()
        .map(|(alias, targets)| WhitelistEntry {
            alias: (*alias).to_string(),
            targets: targets.iter().map(|t| (*t).to_string()).collect(),
        })
        .collect()
}

#[cfg(all(unix, not(target_os = "macos"), not(target_os = "android")))]
fn platform_defaults() -> Vec<WhitelistEntry> {
    let entries: &[(&str, &[&str])] = &[
        ("chrome", &["google-chrome", "chrome"]),
        ("firefox", &["firefox"]),
        ("vscode", &["code"]),
    ];
    entries
        .iter()
        .map(|(alias, targets)| WhitelistEntry {
            alias: (*alias).to_string(),
            targets: targets.iter().map(|t| (*t).to_string()).collect(),
        })
        .collect()
}

#[cfg(target_os = "android")]
fn platform_defaults() -> Vec<WhitelistEntry> {
    // Android returns UNSUPPORTED_CAPABILITY before resolution; no defaults.
    Vec::new()
}

impl Whitelist {
    /// Built-in defaults for the current platform.
    pub fn with_defaults() -> Self {
        let mut wl = Whitelist::default();
        for e in default_entries() {
            wl.insert_entry(e);
        }
        wl
    }

    /// Defaults merged with the user override file (if present and valid).
    /// A missing file means defaults only; an invalid file keeps defaults
    /// (the companion must stay usable even with a corrupt override).
    pub fn load(config_dir: &Path) -> Self {
        let mut wl = Self::with_defaults();
        let path = config_dir.join(USER_WHITELIST_FILE);
        match std::fs::read_to_string(&path) {
            Ok(text) => {
                if let Err(err) = wl.merge_user_json(&text) {
                    eprintln!("client-tools whitelist: invalid override file {path:?}: {err}");
                }
            }
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
            Err(e) => {
                eprintln!("client-tools whitelist: cannot read {path:?}: {e}");
            }
        }
        wl
    }

    /// Merge a user override document. Returns Err on malformed JSON or
    /// schema mismatch; individual entries with empty alias/targets are
    /// skipped (one bad row must not poison the whole file).
    pub fn merge_user_json(&mut self, json: &str) -> Result<usize, String> {
        #[derive(serde::Deserialize)]
        struct OverrideDoc {
            entries: Vec<WhitelistEntry>,
        }
        let doc: OverrideDoc = serde_json::from_str(json).map_err(|e| e.to_string())?;
        let mut merged = 0usize;
        for e in doc.entries {
            if e.alias.trim().is_empty() || e.targets.iter().all(|t| t.trim().is_empty()) {
                continue;
            }
            self.insert_entry(e);
            merged += 1;
        }
        Ok(merged)
    }

    fn insert_entry(&mut self, e: WhitelistEntry) {
        let key = normalize_alias(&e.alias);
        if key.is_empty() {
            return;
        }
        let targets: Vec<String> = e
            .targets
            .into_iter()
            .map(|t| t.trim().to_string())
            .filter(|t| !t.is_empty())
            .collect();
        if targets.is_empty() {
            return;
        }
        self.entries.insert(key, targets);
    }

    /// Number of registered aliases (observability for tests).
    pub fn alias_count(&self) -> usize {
        self.entries.len()
    }

    /// Resolve a user/Agent-supplied target to a launchable candidate.
    /// Enforcement: anything not registered as an alias is rejected —
    /// including raw absolute paths (path-injection guard).
    pub fn resolve(&self, target: &str) -> Result<String, ResolveError> {
        let key = normalize_alias(target);
        let candidates = self
            .entries
            .get(&key)
            .ok_or_else(|| ResolveError::UnknownAlias(target.trim().to_string()))?;
        for c in candidates {
            if is_bare_name(c) {
                return Ok(c.clone());
            }
            let expanded = expand_windows_env(c);
            if Path::new(&expanded).exists() {
                return Ok(expanded);
            }
        }
        Err(ResolveError::NoUsableTarget(key))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn wl_with(entries: &[(&str, &[&str])]) -> Whitelist {
        let mut wl = Whitelist::default();
        for (alias, targets) in entries {
            wl.insert_entry(WhitelistEntry {
                alias: (*alias).to_string(),
                targets: targets.iter().map(|t| (*t).to_string()).collect(),
            });
        }
        wl
    }

    #[test]
    fn defaults_cover_common_apps_on_desktop() {
        let wl = Whitelist::with_defaults();
        #[cfg(windows)]
        {
            for alias in ["chrome", "wechat", "edge", "vscode", "notepad"] {
                assert!(wl.resolve(alias).is_ok(), "default alias {alias} must resolve");
            }
        }
        #[cfg(target_os = "macos")]
        {
            for alias in ["chrome", "safari", "wechat"] {
                assert!(wl.resolve(alias).is_ok(), "default alias {alias} must resolve");
            }
        }
        #[cfg(all(unix, not(target_os = "macos"), not(target_os = "android")))]
        {
            for alias in ["chrome", "firefox"] {
                assert!(wl.resolve(alias).is_ok(), "default alias {alias} must resolve");
            }
        }
        #[cfg(target_os = "android")]
        assert_eq!(wl.alias_count(), 0);
    }

    #[test]
    fn resolve_is_case_insensitive_and_trims() {
        let wl = wl_with(&[("wechat", &["WeChat.exe"])]);
        assert_eq!(wl.resolve("WeChat").unwrap(), "WeChat.exe");
        assert_eq!(wl.resolve("  WECHAT ").unwrap(), "WeChat.exe");
    }

    #[test]
    fn unknown_alias_is_rejected() {
        let wl = wl_with(&[("wechat", &["WeChat.exe"])]);
        assert!(matches!(
            wl.resolve("evilapp"),
            Err(ResolveError::UnknownAlias(_))
        ));
    }

    #[test]
    fn raw_paths_are_never_accepted() {
        // Path injection must not bypass the whitelist even when the file exists.
        let wl = wl_with(&[("wechat", &["WeChat.exe"])]);
        for injected in [r"C:\Windows\System32\cmd.exe", "../foo", "./run.sh", r"\\share\x.exe"] {
            assert!(
                matches!(wl.resolve(injected), Err(ResolveError::UnknownAlias(_))),
                "injected path {injected} must be rejected"
            );
        }
    }

    #[test]
    fn empty_target_is_rejected() {
        let wl = wl_with(&[("wechat", &["WeChat.exe"])]);
        assert!(matches!(wl.resolve(""), Err(ResolveError::UnknownAlias(_))));
        assert!(matches!(wl.resolve("   "), Err(ResolveError::UnknownAlias(_))));
    }

    #[test]
    fn bare_name_fallback_used_when_absolute_candidates_missing() {
        let wl = wl_with(&[("app", &[r"Z:\nonexistent\app.exe", "app.exe"])]);
        assert_eq!(wl.resolve("app").unwrap(), "app.exe");
    }

    #[test]
    fn existing_absolute_candidate_wins_over_bare_name() {
        let dir = std::env::temp_dir().join(format!("aranea-wl-test-{}", std::process::id()));
        std::fs::create_dir_all(&dir).unwrap();
        let exe = dir.join("myapp.exe");
        std::fs::write(&exe, b"stub").unwrap();
        let exe_str = exe.to_string_lossy().to_string();
        let wl = wl_with(&[("myapp", &[exe_str.as_str(), "myapp.exe"])]);
        assert_eq!(wl.resolve("myapp").unwrap(), exe_str);
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn alias_with_only_missing_absolute_targets_fails() {
        let wl = wl_with(&[("gone", &[r"Z:\nonexistent\gone.exe"])]);
        assert!(matches!(
            wl.resolve("gone"),
            Err(ResolveError::NoUsableTarget(_))
        ));
    }

    #[test]
    fn user_override_replaces_default_and_adds_alias() {
        let mut wl = Whitelist::with_defaults();
        let merged = wl
            .merge_user_json(r#"{"entries":[{"alias":"WeChat","targets":["D:\\Apps\\WeChat\\WeChat.exe"]},{"alias":"mytool","targets":["mytool.exe"]}]}"#)
            .unwrap();
        assert_eq!(merged, 2);
        // Replaced: single candidate now (missing abs → NoUsableTarget, proving replacement).
        assert!(matches!(
            wl.resolve("wechat"),
            Err(ResolveError::NoUsableTarget(_))
        ));
        assert_eq!(wl.resolve("MYTOOL").unwrap(), "mytool.exe");
    }

    #[test]
    fn invalid_override_json_is_an_error_and_keeps_defaults() {
        let mut wl = Whitelist::with_defaults();
        assert!(wl.merge_user_json("{not json").is_err());
        assert!(wl.merge_user_json(r#"{"wrong_key":[]}"#).is_err());
        // Defaults still intact after failed merges.
        #[cfg(windows)]
        assert!(wl.resolve("notepad").is_ok());
    }

    #[test]
    fn malformed_entries_are_skipped_not_fatal() {
        let mut wl = Whitelist::default();
        let merged = wl
            .merge_user_json(r#"{"entries":[{"alias":"","targets":["x.exe"]},{"alias":"ok","targets":[]},{"alias":"real","targets":["real.exe"]}]}"#)
            .unwrap();
        assert_eq!(merged, 1);
        assert_eq!(wl.alias_count(), 1);
        assert_eq!(wl.resolve("real").unwrap(), "real.exe");
    }

    #[cfg(windows)]
    #[test]
    fn windows_env_vars_expand_and_unknown_vars_stay_verbatim() {
        let sysroot = std::env::var("SystemRoot").unwrap();
        assert_eq!(expand_windows_env(r"%SystemRoot%\x.exe"), format!(r"{sysroot}\x.exe"));
        assert_eq!(
            expand_windows_env(r"%ARANEA_DEFINITELY_MISSING_VAR%\x.exe"),
            r"%ARANEA_DEFINITELY_MISSING_VAR%\x.exe"
        );
    }

    #[test]
    fn bare_name_detection() {
        assert!(is_bare_name("chrome.exe"));
        assert!(is_bare_name("Google Chrome"));
        assert!(!is_bare_name(r"C:\x\chrome.exe"));
        assert!(!is_bare_name("/usr/bin/chrome"));
        assert!(!is_bare_name("./chrome"));
    }
}
