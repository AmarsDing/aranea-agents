fn main() {
    // M74 V2-T8 真机修复：声明 app 命令以自动生成 ACL 权限
    // （app:allow-client-open-app / app:allow-client-open-url），
    // 否则桌面端 invoke 被 ACL 拒绝："Command client_open_app not allowed by ACL"。
    let attributes = tauri_build::Attributes::new().app_manifest(
        tauri_build::AppManifest::new().commands(&["client_open_app", "client_open_url"]),
    );
    tauri_build::try_build(attributes).expect("tauri-build failed");
}
