mod database;
mod websocket;
mod commands;
mod hwid;

use tauri::Manager;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    #[cfg(debug_assertions)]
    tracing_subscriber::fmt::init();

    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_dialog::init())
        .setup(|app| {
            let exe_dir = std::env::current_exe()
                .map(|p| p.parent().unwrap_or(&p).to_path_buf())
                .expect("Failed to get exe dir");

            database::init_db(&exe_dir).expect("Failed to initialize database");

            let ws_manager = websocket::WebSocketManager::new();
            app.manage(ws_manager);

            #[cfg(debug_assertions)]
            {
                if let Some(window) = app.get_webview_window("main") {
                    window.open_devtools();
                }
            }

            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            commands::connect,
            commands::disconnect,
            commands::is_connected,
            commands::send_message,
            commands::get_local_conversations,
            commands::get_local_messages,
            commands::save_local_message,
            commands::create_local_conversation,
            commands::rename_conversation,
            commands::delete_local_conversation,
            commands::generate_conversation_id,
            commands::get_usage_status,
            commands::format_reset_time,
            commands::check_for_update,
            commands::get_current_version,
            commands::get_notice,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
