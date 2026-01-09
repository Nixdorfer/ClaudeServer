use crate::{database, websocket::WebSocketManager};
use serde::{Deserialize, Serialize};
use tauri::{AppHandle, State};

const UPDATE_URL: &str = "https://raw.githubusercontent.com/Nixdorfer/ClaudeServer/main/info.json";

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UsageStatus {
    pub five_hour: f64,
    pub five_hour_reset: String,
    pub seven_day: f64,
    pub seven_day_reset: String,
    pub seven_day_sonnet: f64,
    pub seven_day_sonnet_reset: String,
    #[serde(default)]
    pub is_blocked: bool,
    #[serde(default)]
    pub block_reason: String,
    #[serde(default)]
    pub block_reset_time: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct UsageResponse {
    five_hour_utilization: f64,
    five_hour_resets_at: Option<String>,
    seven_day_utilization: f64,
    seven_day_resets_at: Option<String>,
    seven_day_opus_utilization: f64,
    seven_day_opus_resets_at: Option<String>,
    #[serde(default)]
    is_blocked: bool,
    #[serde(default)]
    block_reason: String,
    #[serde(default)]
    block_reset_time: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UpdateCheckResult {
    pub has_update: bool,
    pub current_version: String,
    pub latest_version: String,
    pub notes: Vec<String>,
    pub download_url: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct VersionInfo {
    version: String,
    note: Vec<String>,
    url: String,
}

#[tauri::command]
pub async fn connect(
    app: AppHandle,
    ws: State<'_, WebSocketManager>,
) -> Result<(), String> {
    ws.connect(app).await
}

#[tauri::command]
pub async fn disconnect(ws: State<'_, WebSocketManager>) -> Result<(), String> {
    ws.disconnect().await;
    Ok(())
}

#[tauri::command]
pub async fn is_connected(ws: State<'_, WebSocketManager>) -> Result<bool, String> {
    Ok(ws.is_connected().await)
}

#[tauri::command]
pub async fn send_message(
    conversation_id: String,
    message: String,
    ws: State<'_, WebSocketManager>,
) -> Result<(), String> {
    ws.send_message(&conversation_id, &message).await
}

#[tauri::command]
pub async fn get_local_conversations() -> Result<Vec<database::LocalConversation>, String> {
    tokio::task::spawn_blocking(|| database::get_conversations().map_err(|e| e.to_string()))
        .await
        .map_err(|e| e.to_string())?
}

#[tauri::command]
pub async fn get_local_messages(conversation_id: String) -> Result<Vec<database::LocalMessage>, String> {
    tokio::task::spawn_blocking(move || database::get_messages(&conversation_id).map_err(|e| e.to_string()))
        .await
        .map_err(|e| e.to_string())?
}

#[tauri::command]
pub async fn save_local_message(
    conversation_id: String,
    role: String,
    content: String,
) -> Result<(), String> {
    tokio::task::spawn_blocking(move || {
        if !database::conversation_exists(&conversation_id) {
            database::create_conversation(&conversation_id, "").map_err(|e| e.to_string())?;
        }
        database::add_message(&conversation_id, &role, &content).map_err(|e| e.to_string())
    })
    .await
    .map_err(|e| e.to_string())?
}

#[tauri::command]
pub async fn create_local_conversation(id: String, first_message: String) -> Result<(), String> {
    tokio::task::spawn_blocking(move || database::create_conversation(&id, &first_message).map_err(|e| e.to_string()))
        .await
        .map_err(|e| e.to_string())?
}

#[tauri::command]
pub async fn rename_conversation(id: String, name: String) -> Result<(), String> {
    tokio::task::spawn_blocking(move || database::update_conversation_name(&id, &name).map_err(|e| e.to_string()))
        .await
        .map_err(|e| e.to_string())?
}

#[tauri::command]
pub async fn delete_local_conversation(id: String) -> Result<(), String> {
    tokio::task::spawn_blocking(move || database::delete_conversation(&id).map_err(|e| e.to_string()))
        .await
        .map_err(|e| e.to_string())?
}

#[tauri::command]
pub fn generate_conversation_id() -> String {
    format!("local_{}", chrono::Utc::now().timestamp_nanos_opt().unwrap_or(0))
}

#[tauri::command]
pub async fn get_usage_status() -> Result<UsageStatus, String> {
    let client = reqwest::Client::builder()
        .danger_accept_invalid_certs(true)
        .build()
        .map_err(|e| e.to_string())?;

    let resp = client
        .get("https://claude.nixdorfer.com/api/usage")
        .header("Origin", "https://claude.nixdorfer.com")
        .header("User-Agent", "Mozilla/5.0")
        .send()
        .await
        .map_err(|e| format!("获取用量失败: {}", e))?;

    let usage: UsageResponse = resp.json().await.map_err(|e| e.to_string())?;

    Ok(UsageStatus {
        five_hour: usage.five_hour_utilization,
        five_hour_reset: usage.five_hour_resets_at.unwrap_or_default(),
        seven_day: usage.seven_day_utilization,
        seven_day_reset: usage.seven_day_resets_at.unwrap_or_default(),
        seven_day_sonnet: usage.seven_day_opus_utilization,
        seven_day_sonnet_reset: usage.seven_day_opus_resets_at.unwrap_or_default(),
        is_blocked: usage.is_blocked,
        block_reason: usage.block_reason,
        block_reset_time: usage.block_reset_time,
    })
}

#[tauri::command]
pub fn format_reset_time(iso_time: String) -> String {
    if let Ok(dt) = chrono::DateTime::parse_from_rfc3339(&iso_time) {
        dt.with_timezone(&chrono::Local).format("%m-%d %H:%M").to_string()
    } else {
        iso_time
    }
}

#[tauri::command]
pub async fn check_for_update(app: AppHandle) -> Result<UpdateCheckResult, String> {
    let client = reqwest::Client::new();

    let resp = client
        .get(UPDATE_URL)
        .send()
        .await
        .map_err(|e| e.to_string())?;

    let versions: Vec<VersionInfo> = resp.json().await.map_err(|e| e.to_string())?;

    let latest = versions.last().ok_or("No version info")?;

    let current_version = app.package_info().version.to_string();
    let has_update = compare_versions(&current_version, &latest.version);

    Ok(UpdateCheckResult {
        has_update,
        current_version,
        latest_version: latest.version.clone(),
        notes: latest.note.clone(),
        download_url: latest.url.clone(),
    })
}

#[tauri::command]
pub fn get_current_version(app: AppHandle) -> String {
    app.package_info().version.to_string()
}

#[tauri::command]
pub async fn get_notice() -> Result<String, String> {
    tokio::task::spawn_blocking(|| {
        let exe_dir = std::env::current_exe()
            .map(|p| p.parent().unwrap_or(&p).to_path_buf())
            .map_err(|e| e.to_string())?;
        let notice_path = exe_dir.join("notice.md");
        if notice_path.exists() {
            std::fs::read_to_string(&notice_path).map_err(|e| e.to_string())
        } else {
            Ok(String::new())
        }
    })
    .await
    .map_err(|e| e.to_string())?
}

fn compare_versions(current: &str, latest: &str) -> bool {
    let parse = |v: &str| -> Vec<u32> {
        v.split('.')
            .filter_map(|s| s.parse().ok())
            .collect()
    };

    let curr = parse(current);
    let lat = parse(latest);

    for i in 0..3 {
        let c = curr.get(i).copied().unwrap_or(0);
        let l = lat.get(i).copied().unwrap_or(0);
        if l > c {
            return true;
        }
        if c > l {
            return false;
        }
    }
    false
}
