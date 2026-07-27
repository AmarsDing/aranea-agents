// Prevents an extra console window on Windows in release.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod server;

use tauri::WebviewUrl;

fn main() {
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
        .setup(move |app| {
            tauri::async_runtime::spawn(async move {
                if let Err(err) = server::serve(listener).await {
                    eprintln!("embedded server exited: {err}");
                }
            });
            tauri::WebviewWindowBuilder::new(app, "main", WebviewUrl::External(url))
                .title("Aranea-Agents")
                .inner_size(1400.0, 900.0)
                .min_inner_size(1024.0, 600.0)
                .build()?;
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
