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
        FromRequestParts, Request,
    },
    http::{header, HeaderMap, StatusCode, Uri},
    response::{IntoResponse, Response},
    routing::any,
    Router,
};
use futures_util::{SinkExt, StreamExt};
use rust_embed::RustEmbed;
use std::sync::LazyLock;
use tokio_tungstenite::tungstenite::{client::IntoClientRequest, Message as WsMsg};

const BACKEND_HTTP: &str = "http://127.0.0.1:8000";
const BACKEND_WS: &str = "ws://127.0.0.1:8000";

/// Same content the Electron packaging wrote to
/// `assets/config/runtime-config.json`.
const RUNTIME_CONFIG: &str =
    r#"{"backendUrl":"http://127.0.0.1:8000","wsOrigin":"http://127.0.0.1:8000"}"#;

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
pub async fn serve(listener: std::net::TcpListener) -> std::io::Result<()> {
    let app = Router::new()
        .route("/healthz", any(proxy_handler))
        .route("/v1/{*path}", any(proxy_handler))
        .route("/api/{*path}", any(proxy_handler))
        .route("/openapi/{*path}", any(proxy_handler))
        .fallback(any(spa_handler));
    let listener = tokio::net::TcpListener::from_std(listener)?;
    axum::serve(listener, app).await
}

// ─── Reverse proxy ──────────────────────────────────────────────────────────

async fn proxy_handler(req: Request) -> Response {
    let path_q = req
        .uri()
        .path_and_query()
        .map(|pq| pq.as_str())
        .unwrap_or("/")
        .to_string();

    if !is_websocket_upgrade(req.headers()) {
        return http_proxy(req, &path_q).await;
    }

    let (mut parts, _body) = req.into_parts();
    match WebSocketUpgrade::from_request_parts(&mut parts, &()).await {
        Ok(ws) => {
            let headers = std::mem::take(&mut parts.headers);
            ws.on_upgrade(move |socket| ws_tunnel(socket, path_q, headers))
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

async fn http_proxy(req: Request, path_q: &str) -> Response {
    let (parts, body) = req.into_parts();
    let url = format!("{BACKEND_HTTP}{path_q}");

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

/// Bidirectional WebSocket tunnel: browser <-> backend :8000.
/// Auth headers (Cookie / Authorization) are forwarded so the backend session
/// check keeps working; handshake headers are owned by tungstenite.
async fn ws_tunnel(client: WebSocket, path_q: String, headers: HeaderMap) {
    let url = format!("{BACKEND_WS}{path_q}");
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
    if path == "assets/config/runtime-config.json" {
        return (
            [(header::CONTENT_TYPE, "application/json; charset=utf-8")],
            RUNTIME_CONFIG,
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
