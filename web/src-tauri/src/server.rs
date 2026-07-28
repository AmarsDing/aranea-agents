//! Embedded loopback HTTP server.
//!
//! Mirrors the behaviour of the former Electron main process
//! (`web/src-electron/electron-main.ts`):
//!   * serves the SPA from compile-time embedded assets (`web/dist/spa`)
//!   * reverse-proxies `/healthz`, `/v1/*`, `/api/*`, `/openapi/*` to the
//!     backend on `127.0.0.1:8000`, including WebSocket upgrades
//!   * serves `assets/config/runtime-config.json` from the binary
//!
//! Same-origin + same-site (127.0.0.1) semantics keep SameSite=Lax session
//! cookies working without any CORS / 401 loops.

use axum::{
    body::Body,
    extract::{
        ws::{Message as AxumWs, WebSocket, WebSocketUpgrade},
        FromRequestParts, Request, State,
    },
    http::{header, HeaderMap, StatusCode, Uri},
    response::{IntoResponse, Response},
    routing::any,
    Json, Router,
};
use futures_util::{SinkExt, StreamExt};
use rust_embed::RustEmbed;
use std::path::PathBuf;
use std::sync::{Arc, LazyLock, RwLock};
use tokio_tungstenite::tungstenite::{client::IntoClientRequest, Message as WsMsg};

use crate::config::{self, BackendConfig};

const DEFAULT_DESKTOP_HTTP: &str = "http://127.0.0.1:8000";
const DEFAULT_DESKTOP_WS: &str = "ws://127.0.0.1:8000";

/// True when compiled for Android (mobile entry point).
const IS_ANDROID: bool = cfg!(target_os = "android");

/// Same content the Electron packaging wrote to
/// `assets/config/runtime-config.json`. Desktop only — Android serves `{}`
/// so the SPA talks to the embedded proxy same-origin (see runtime.ts).
const DESKTOP_RUNTIME_CONFIG: &str =
    r#"{"backendUrl":"http://127.0.0.1:8000","wsOrigin":"http://127.0.0.1:8000"}"#;

/// Shared runtime state: the upstream backend can be (re)configured at
/// runtime via the loopback-only `/__local/backend-config` endpoints.
pub struct AppState {
    config_dir: PathBuf,
    upstream: RwLock<Option<BackendConfig>>,
}

impl AppState {
    fn new(config_dir: PathBuf) -> Self {
        let upstream = config::load(&config_dir);
        Self {
            config_dir,
            upstream: RwLock::new(upstream),
        }
    }

    fn current_upstream(&self) -> Option<(String, String)> {
        resolve_upstream(self.upstream.read().unwrap().as_ref(), IS_ANDROID)
    }
}

/// Pick the effective (http, ws) upstream. A configured URL always wins;
/// desktop falls back to the co-located backend; Android without a config
/// is unavailable (the SPA setup page collects the URL first).
fn resolve_upstream(config: Option<&BackendConfig>, is_android: bool) -> Option<(String, String)> {
    if let Some(cfg) = config {
        return Some((cfg.http_base().to_string(), cfg.ws_base()));
    }
    if is_android {
        None
    } else {
        Some((DEFAULT_DESKTOP_HTTP.to_string(), DEFAULT_DESKTOP_WS.to_string()))
    }
}

fn runtime_config_json(is_android: bool) -> &'static str {
    if is_android {
        "{}"
    } else {
        DESKTOP_RUNTIME_CONFIG
    }
}

#[derive(RustEmbed)]
#[folder = "../dist/spa"]
struct SpaAssets;

static HTTP_CLIENT: LazyLock<reqwest::Client> = LazyLock::new(|| {
    reqwest::Client::builder()
        .redirect(reqwest::redirect::Policy::none())
        .build()
        .expect("build reqwest client")
});

/// Serves the embedded SPA + backend proxy on the given (already bound)
/// listener. Runs inside the Tauri async runtime.
pub async fn serve(listener: std::net::TcpListener, config_dir: PathBuf) -> std::io::Result<()> {
    let state = Arc::new(AppState::new(config_dir));
    let app = Router::new()
        .route("/healthz", any(proxy_handler))
        .route("/v1/{*path}", any(proxy_handler))
        .route("/api/{*path}", any(proxy_handler))
        .route("/openapi/{*path}", any(proxy_handler))
        .route(
            "/__local/backend-config",
            axum::routing::get(get_backend_config).put(put_backend_config),
        )
        .fallback(any(spa_handler))
        .with_state(state);
    let listener = tokio::net::TcpListener::from_std(listener)?;
    axum::serve(listener, app).await
}

// ─── Loopback-only backend config ───────────────────────────────────────────

async fn get_backend_config(State(state): State<Arc<AppState>>) -> impl IntoResponse {
    let url = state
        .upstream
        .read()
        .unwrap()
        .as_ref()
        .map(|c| c.http_base().to_string());
    Json(serde_json::json!({
        "url": url,
        "platform": if IS_ANDROID { "android" } else { "desktop" },
        "requiresSetup": IS_ANDROID && url.is_none(),
    }))
}

async fn put_backend_config(
    State(state): State<Arc<AppState>>,
    Json(body): Json<serde_json::Value>,
) -> Response {
    let raw = body.get("url").and_then(|v| v.as_str()).unwrap_or("");
    let cfg = match BackendConfig::new(raw) {
        Ok(c) => c,
        Err(msg) => {
            return (
                StatusCode::BAD_REQUEST,
                Json(serde_json::json!({ "error": msg })),
            )
                .into_response();
        }
    };
    if let Err(err) = config::save(&state.config_dir, &cfg) {
        return (
            StatusCode::INTERNAL_SERVER_ERROR,
            Json(serde_json::json!({ "error": format!("persist failed: {err}") })),
        )
            .into_response();
    }
    *state.upstream.write().unwrap() = Some(cfg);
    StatusCode::OK.into_response()
}

// ─── Reverse proxy ──────────────────────────────────────────────────────────

async fn proxy_handler(State(state): State<Arc<AppState>>, req: Request) -> Response {
    let Some((http_base, ws_base)) = state.current_upstream() else {
        return (
            StatusCode::SERVICE_UNAVAILABLE,
            "Backend not configured. Open the app setup page to enter the server address.",
        )
            .into_response();
    };

    let path_q = req
        .uri()
        .path_and_query()
        .map(|pq| pq.as_str())
        .unwrap_or("/")
        .to_string();

    if !is_websocket_upgrade(req.headers()) {
        return http_proxy(req, &path_q, &http_base).await;
    }

    let (mut parts, _body) = req.into_parts();
    match WebSocketUpgrade::from_request_parts(&mut parts, &()).await {
        Ok(ws) => {
            let headers = std::mem::take(&mut parts.headers);
            ws.on_upgrade(move |socket| ws_tunnel(socket, path_q, headers, ws_base))
        }
        Err(rejection) => rejection.into_response(),
    }
}

fn is_websocket_upgrade(headers: &HeaderMap) -> bool {
    headers
        .get(header::UPGRADE)
        .and_then(|v| v.to_str().ok())
        .is_some_and(|v| v.eq_ignore_ascii_case("websocket"))
}

async fn http_proxy(req: Request, path_q: &str, http_base: &str) -> Response {
    let (parts, body) = req.into_parts();
    let url = format!("{http_base}{path_q}");

    let mut headers = parts.headers;
    headers.remove(header::HOST);
    headers.remove(header::CONNECTION);
    headers.remove("keep-alive");

    let result = HTTP_CLIENT
        .request(parts.method, &url)
        .headers(headers)
        .body(reqwest::Body::wrap_stream(body.into_data_stream()))
        .send()
        .await;

    match result {
        Ok(upstream) => {
            let status = upstream.status();
            let mut resp_headers = upstream.headers().clone();
            resp_headers.remove(header::CONNECTION);
            resp_headers.remove(header::TRANSFER_ENCODING);
            let stream = upstream.bytes_stream();
            let mut resp = Response::new(Body::from_stream(stream));
            *resp.status_mut() = status;
            *resp.headers_mut() = resp_headers;
            resp
        }
        Err(_) => (
            StatusCode::BAD_GATEWAY,
            "Backend unavailable. Start Aranea via desktop shortcut / AraneaLauncher.exe",
        )
            .into_response(),
    }
}

/// Bidirectional WebSocket tunnel: browser <-> upstream backend.
/// Auth headers (Cookie / Authorization) are forwarded so the backend session
/// check keeps working; handshake headers are owned by tungstenite.
async fn ws_tunnel(client: WebSocket, path_q: String, headers: HeaderMap, ws_base: String) {
    let url = format!("{ws_base}{path_q}");
    let mut request = match url.into_client_request() {
        Ok(r) => r,
        Err(_) => return,
    };
    for (name, value) in headers.iter() {
        let n = name.as_str();
        if n.eq_ignore_ascii_case("host")
            || n.eq_ignore_ascii_case("connection")
            || n.eq_ignore_ascii_case("upgrade")
            || n.starts_with("sec-websocket-")
        {
            continue;
        }
        request.headers_mut().insert(name.clone(), value.clone());
    }

    let (upstream, _) = match tokio_tungstenite::connect_async(request).await {
        Ok(v) => v,
        Err(_) => return, // dropping `client` closes the browser-side socket
    };

    let (mut client_tx, mut client_rx) = client.split();
    let (mut up_tx, mut up_rx) = upstream.split();

    let client_to_up = async move {
        while let Some(Ok(msg)) = client_rx.next().await {
            let Some(msg) = axum_to_tungstenite(msg) else {
                continue;
            };
            if up_tx.send(msg).await.is_err() {
                break;
            }
        }
        let _ = up_tx.close().await;
    };

    let up_to_client = async move {
        while let Some(Ok(msg)) = up_rx.next().await {
            let Some(msg) = tungstenite_to_axum(msg) else {
                continue;
            };
            if client_tx.send(msg).await.is_err() {
                break;
            }
        }
        let _ = client_tx.close().await;
    };

    tokio::join!(client_to_up, up_to_client);
}

fn axum_to_tungstenite(msg: AxumWs) -> Option<WsMsg> {
    match msg {
        AxumWs::Text(t) => Some(WsMsg::Text(t.as_str().to_owned().into())),
        AxumWs::Binary(b) => Some(WsMsg::Binary(b)),
        AxumWs::Ping(p) => Some(WsMsg::Ping(p)),
        AxumWs::Pong(p) => Some(WsMsg::Pong(p)),
        AxumWs::Close(c) => Some(WsMsg::Close(c.map(|f| {
            tokio_tungstenite::tungstenite::protocol::CloseFrame {
                code: f.code.into(),
                reason: f.reason.as_str().to_owned().into(),
            }
        }))),
    }
}

fn tungstenite_to_axum(msg: WsMsg) -> Option<AxumWs> {
    match msg {
        WsMsg::Text(t) => Some(AxumWs::Text(t.as_str().to_owned().into())),
        WsMsg::Binary(b) => Some(AxumWs::Binary(b)),
        WsMsg::Ping(p) => Some(AxumWs::Ping(p)),
        WsMsg::Pong(p) => Some(AxumWs::Pong(p)),
        WsMsg::Close(c) => Some(AxumWs::Close(c.map(|f| axum::extract::ws::CloseFrame {
            code: f.code.into(),
            reason: f.reason.as_str().to_owned().into(),
        }))),
        _ => None, // raw frames are not surfaced to the browser
    }
}

// ─── Static SPA ─────────────────────────────────────────────────────────────

async fn spa_handler(uri: Uri) -> Response {
    let raw = uri.path().trim_start_matches('/');
    let path = if raw.is_empty() { "index.html" } else { raw };

    // Runtime config is baked in (was a build-time generated file in Electron).
    // Desktop points the SPA at the co-located backend; Android returns `{}`
    // so the SPA uses the embedded proxy same-origin.
    if path == "assets/config/runtime-config.json" {
        return (
            [(header::CONTENT_TYPE, "application/json; charset=utf-8")],
            runtime_config_json(IS_ANDROID),
        )
            .into_response();
    }

    if let Some(file) = SpaAssets::get(path) {
        return asset_response(path, &file.data);
    }
    // SPA fallback: extension-less paths route to index.html (history mode).
    let has_ext = path.rsplit('/').next().unwrap_or(path).contains('.');
    if !has_ext {
        if let Some(file) = SpaAssets::get("index.html") {
            return asset_response("index.html", &file.data);
        }
    }
    (StatusCode::NOT_FOUND, "Not found").into_response()
}

fn asset_response(path: &str, data: &[u8]) -> Response {
    let mut mime = mime_guess::from_path(path)
        .first_or_octet_stream()
        .to_string();
    if mime.starts_with("text/") || mime == "application/json" {
        mime.push_str("; charset=utf-8");
    }
    ([(header::CONTENT_TYPE, mime)], data.to_vec()).into_response()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::BackendConfig;

    #[test]
    fn resolve_upstream_prefers_configured_url() {
        let cfg = BackendConfig::new("https://aranea.example.com").unwrap();
        let (http, ws) = resolve_upstream(Some(&cfg), false).expect("resolved");
        assert_eq!(http, "https://aranea.example.com");
        assert_eq!(ws, "wss://aranea.example.com");
    }

    #[test]
    fn resolve_upstream_desktop_falls_back_to_local_backend() {
        let (http, ws) = resolve_upstream(None, false).expect("desktop fallback");
        assert_eq!(http, "http://127.0.0.1:8000");
        assert_eq!(ws, "ws://127.0.0.1:8000");
    }

    #[test]
    fn resolve_upstream_android_without_config_is_unavailable() {
        assert!(resolve_upstream(None, true).is_none());
    }

    #[test]
    fn runtime_config_json_desktop_points_to_local_backend() {
        let json = runtime_config_json(false);
        assert!(json.contains("127.0.0.1:8000"));
    }

    #[test]
    fn runtime_config_json_android_uses_same_origin() {
        assert_eq!(runtime_config_json(true), "{}");
    }
}
