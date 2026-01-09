use crate::hwid;
use futures_util::{SinkExt, StreamExt};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use tauri::{AppHandle, Emitter};
use tokio::sync::{mpsc, Mutex, RwLock};
use tokio_tungstenite::{connect_async_tls_with_config, tungstenite::Message};
use tokio_tungstenite::tungstenite::client::IntoClientRequest;

const SERVER_URL: &str = "wss://claude.nixdorfer.com/data/websocket/create";

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WSMessage {
    #[serde(rename = "type")]
    pub msg_type: String,
    pub data: serde_json::Value,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DialogueRequest {
    pub request: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub conversation_id: String,
    pub device_id: String,
}

pub struct WebSocketManager {
    sender: Arc<Mutex<Option<mpsc::Sender<String>>>>,
    connected: Arc<RwLock<bool>>,
}

impl WebSocketManager {
    pub fn new() -> Self {
        Self {
            sender: Arc::new(Mutex::new(None)),
            connected: Arc::new(RwLock::new(false)),
        }
    }

    pub async fn is_connected(&self) -> bool {
        *self.connected.read().await
    }

    pub async fn connect(&self, app: AppHandle) -> Result<(), String> {
        if self.is_connected().await {
            return Ok(());
        }
        let connector = native_tls::TlsConnector::builder()
            .danger_accept_invalid_certs(true)
            .build()
            .map_err(|e| e.to_string())?;
        let connector = tokio_tungstenite::Connector::NativeTls(connector);
        let device_id = hwid::get_hwid();
        let client_version = app.package_info().version.to_string();
        let mut request = SERVER_URL.into_client_request().map_err(|e| e.to_string())?;
        let platform = if cfg!(target_os = "windows") {
            "windows"
        } else if cfg!(target_os = "macos") {
            "macos"
        } else {
            "linux"
        };
        let headers = request.headers_mut();
        headers.insert("Origin", "https://claude.nixdorfer.com".parse().unwrap());
        headers.insert("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36".parse().unwrap());
        headers.insert("X-Device-ID", device_id.parse().unwrap());
        headers.insert("X-Client-Version", client_version.parse().unwrap());
        headers.insert("X-Platform", platform.parse().unwrap());

        let (ws_stream, _) = connect_async_tls_with_config(request, None, false, Some(connector))
            .await
            .map_err(|e| format!("连接失败: {}", e))?;

        let (mut write, mut read) = ws_stream.split();

        let (tx, mut rx) = mpsc::channel::<String>(100);

        // Store sender
        *self.sender.lock().await = Some(tx);
        *self.connected.write().await = true;

        let connected = self.connected.clone();
        let sender = self.sender.clone();
        let app_clone = app.clone();

        // Emit connected event
        let _ = app.emit("connected", true);

        // Spawn write task
        let write_connected = connected.clone();
        tokio::spawn(async move {
            while let Some(msg) = rx.recv().await {
                if write.send(Message::Text(msg)).await.is_err() {
                    break;
                }
            }
            *write_connected.write().await = false;
        });

        // Spawn read task
        tokio::spawn(async move {
            while let Some(msg) = read.next().await {
                match msg {
                    Ok(Message::Text(text)) => {
                        if let Ok(ws_msg) = serde_json::from_str::<WSMessage>(&text) {
                            match ws_msg.msg_type.as_str() {
                                "connected" => {
                                    let _ = app_clone.emit("ws_connected", ws_msg.data);
                                }
                                "version_outdated" => {
                                    let _ = app_clone.emit("version_outdated", ws_msg.data);
                                }
                                "banned" => {
                                    let _ = app_clone.emit("device_banned", ws_msg.data);
                                }
                                "conversation_id" => {
                                    let _ = app_clone.emit("conversation_id", ws_msg.data);
                                }
                                "content" => {
                                    let _ = app_clone.emit("content", ws_msg.data);
                                }
                                "done" => {
                                    let _ = app_clone.emit("done", ws_msg.data);
                                }
                                "error" => {
                                    let _ = app_clone.emit("ws_error", ws_msg.data);
                                }
                                "usage_blocked" => {
                                    let _ = app_clone.emit("usage_blocked", ws_msg.data);
                                }
                                _ => {
                                    let _ = app_clone.emit("ws_message", ws_msg);
                                }
                            }
                        }
                    }
                    Ok(Message::Close(_)) => {
                        break;
                    }
                    Err(e) => {
                        let _ = app_clone.emit("connection_error", e.to_string());
                        break;
                    }
                    _ => {}
                }
            }

            *connected.write().await = false;
            *sender.lock().await = None;
            let _ = app_clone.emit("disconnected", true);
        });

        Ok(())
    }

    pub async fn disconnect(&self) {
        *self.sender.lock().await = None;
        *self.connected.write().await = false;
    }

    pub async fn send_message(&self, conversation_id: &str, message: &str) -> Result<(), String> {
        let sender = self.sender.lock().await;
        let tx = sender.as_ref().ok_or("Not connected")?;

        let req = WSMessage {
            msg_type: "dialogue".to_string(),
            data: serde_json::to_value(DialogueRequest {
                request: message.to_string(),
                conversation_id: conversation_id.to_string(),
                device_id: hwid::get_hwid().to_string(),
            }).map_err(|e| e.to_string())?,
        };

        let json = serde_json::to_string(&req).map_err(|e| e.to_string())?;
        tx.send(json).await.map_err(|e| e.to_string())?;

        Ok(())
    }
}
