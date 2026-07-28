mod server;
pub mod config;

use tauri::{Manager, WebviewUrl};

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    // Bind the embedded HTTP server BEFORE creating the window so the port is
    // known up-front and the window can load it immediately (the TCP backlog
    // covers the gap until axum starts accepting).
    let listener = std::net::TcpListener::bind(("127.0.0.1", 0)).expect("bind embedded server");
    listener.set_nonblocking(true).expect("set nonblocking");
    let port = listener.local_addr().expect("local addr").port();
    let url: tauri::Url = format!("http://127.0.0.1:{port}/")
        .parse()
        .expect("parse local url");

    tauri::Builder::default()
        // P2: local notifications (blocking confirm/clarify steps). The JS
        // side gates all calls to the Tauri shell, so registering the plugin
        // on desktop is harmless and keeps one code path.
        .plugin(tauri_plugin_notification::init())
        .setup(move |app| {
            let config_dir = app
                .path()
                .app_config_dir()
                .expect("resolve app config dir");
            tauri::async_runtime::spawn(async move {
                if let Err(err) = server::serve(listener, config_dir).await {
                    eprintln!("embedded server exited: {err}");
                }
            });
            // The webview loads the embedded loopback server on every platform.
            // Desktop-only chrome (title/size) is skipped on Android, where the
            // window is managed by the system Activity.
            let mut builder = tauri::WebviewWindowBuilder::new(app, "main", WebviewUrl::External(url));
            #[cfg(not(target_os = "android"))]
            {
                builder = builder
                    .title("Aranea-Agents")
                    .inner_size(1400.0, 900.0)
                    .min_inner_size(1024.0, 600.0);
            }
            builder.build()?;
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
