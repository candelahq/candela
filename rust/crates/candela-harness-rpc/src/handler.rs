//! JSON-RPC request handler — dispatches method calls.

use candela_core::harness::{ChatEvent, HarnessError, new_session};
use candela_harness_chat::ChatRuntime;
use candela_harness_storage::Database;
use std::sync::Arc;
use tokio::sync::mpsc;
use tracing::info;

use crate::protocol::*;

/// Handles JSON-RPC requests from IDE plugins.
pub struct RpcHandler {
    db: Arc<std::sync::Mutex<Database>>,
    chat: Arc<ChatRuntime>,
    notify_tx: mpsc::UnboundedSender<JsonRpcNotification>,
    default_model: String,
}

impl RpcHandler {
    pub fn new(
        db: Arc<std::sync::Mutex<Database>>,
        chat: Arc<ChatRuntime>,
        notify_tx: mpsc::UnboundedSender<JsonRpcNotification>,
        default_model: String,
    ) -> Self {
        Self {
            db,
            chat,
            notify_tx,
            default_model,
        }
    }

    /// Dispatch a JSON-RPC request to the appropriate handler.
    ///
    /// Returns `None` for JSON-RPC notifications (requests with no `id`),
    /// since the spec says notifications MUST NOT receive a response.
    pub async fn handle(&self, req: JsonRpcRequest) -> Option<JsonRpcResponse> {
        info!(method = %req.method, "rpc request");

        // JSON-RPC notifications have no id — never send a response.
        req.id.as_ref()?;

        let response = match req.method.as_str() {
            "initialize" => self.handle_initialize(req.id).await,
            "session.list" => self.handle_session_list(req.id, req.params).await,
            "session.create" => self.handle_session_create(req.id, req.params).await,
            "session.delete" => self.handle_session_delete(req.id, req.params).await,
            "chat.send" => self.handle_chat_send(req.id, req.params).await,
            _ => JsonRpcResponse::error(
                req.id,
                METHOD_NOT_FOUND,
                format!("Unknown method: {}", req.method),
            ),
        };
        Some(response)
    }

    async fn handle_initialize(&self, id: Option<serde_json::Value>) -> JsonRpcResponse {
        JsonRpcResponse::success(
            id,
            serde_json::json!({
                "name": "candela-harness",
                "version": env!("CARGO_PKG_VERSION"),
                "capabilities": {
                    "streaming": true,
                    "sessions": true,
                    "search": true,
                }
            }),
        )
    }

    async fn handle_session_list(
        &self,
        id: Option<serde_json::Value>,
        params: serde_json::Value,
    ) -> JsonRpcResponse {
        let limit = params
            .get("limit")
            .and_then(|v| v.as_i64())
            .unwrap_or(50)
            .max(0);
        let offset = params
            .get("offset")
            .and_then(|v| v.as_i64())
            .unwrap_or(0)
            .max(0);

        match self.db.lock().unwrap().list_sessions(limit, offset) {
            Ok(sessions) => JsonRpcResponse::success(
                id,
                serde_json::json!({
                    "sessions": sessions,
                    "total": sessions.len(),
                }),
            ),
            Err(e) => JsonRpcResponse::error(id, INTERNAL_ERROR, e.to_string()),
        }
    }

    async fn handle_session_create(
        &self,
        id: Option<serde_json::Value>,
        params: serde_json::Value,
    ) -> JsonRpcResponse {
        let model = params
            .get("model")
            .and_then(|v| v.as_str())
            .unwrap_or(&self.default_model);
        let device_id = params
            .get("device_id")
            .and_then(|v| v.as_str())
            .unwrap_or("");

        let session = new_session(model, device_id);
        match self.db.lock().unwrap().create_session(&session) {
            Ok(()) => JsonRpcResponse::success(id, serde_json::json!(session)),
            Err(e) => JsonRpcResponse::error(id, INTERNAL_ERROR, e.to_string()),
        }
    }

    async fn handle_session_delete(
        &self,
        id: Option<serde_json::Value>,
        params: serde_json::Value,
    ) -> JsonRpcResponse {
        let session_id = match params.get("session_id").and_then(|v| v.as_str()) {
            Some(sid) => sid,
            None => {
                return JsonRpcResponse::error(
                    id,
                    INVALID_PARAMS,
                    "session_id required".to_string(),
                );
            }
        };

        match self.db.lock().unwrap().delete_session(session_id) {
            Ok(()) => JsonRpcResponse::success(id, serde_json::json!({ "status": "deleted" })),
            Err(HarnessError::SessionNotFound(sid)) => {
                JsonRpcResponse::error(id, INVALID_PARAMS, format!("session not found: {sid}"))
            }
            Err(e) => JsonRpcResponse::error(id, INTERNAL_ERROR, e.to_string()),
        }
    }

    async fn handle_chat_send(
        &self,
        id: Option<serde_json::Value>,
        params: serde_json::Value,
    ) -> JsonRpcResponse {
        let session_id = match params.get("session_id").and_then(|v| v.as_str()) {
            Some(sid) => sid.to_string(),
            None => {
                return JsonRpcResponse::error(
                    id,
                    INVALID_PARAMS,
                    "session_id required".to_string(),
                );
            }
        };
        let content = match params.get("content").and_then(|v| v.as_str()) {
            Some(c) => c.to_string(),
            None => {
                return JsonRpcResponse::error(id, INVALID_PARAMS, "content required".to_string());
            }
        };

        let stream_id = uuid::Uuid::new_v4().to_string();
        let chat = self.chat.clone();
        let tx = self.notify_tx.clone();
        let sid = stream_id.clone();

        // Spawn the streaming task — response comes back immediately
        tokio::spawn(async move {
            let notify_tx = tx.clone();
            let on_event = move |event: ChatEvent| {
                let notif = JsonRpcNotification::new(
                    "chat.event",
                    serde_json::to_value(&event).unwrap_or_default(),
                );
                let _ = notify_tx.send(notif);
            };

            if let Err(e) = chat.send_message(&session_id, &content, on_event).await {
                let error_notif = JsonRpcNotification::new(
                    "chat.event",
                    serde_json::json!({
                        "type": "error",
                        "stream_id": sid,
                        "message": e.to_string(),
                    }),
                );
                let _ = tx.send(error_notif);
            }
        });

        JsonRpcResponse::success(
            id,
            serde_json::json!({
                "status": "accepted",
                "stream_id": stream_id,
            }),
        )
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use candela_core::harness::HarnessConfig;
    use candela_harness_storage::SearchIndex;

    fn test_handler() -> (RpcHandler, mpsc::UnboundedReceiver<JsonRpcNotification>) {
        let db = Database::open_in_memory().unwrap();
        let db = Arc::new(std::sync::Mutex::new(db));
        let search = SearchIndex::open_in_memory().unwrap();
        let search = Arc::new(std::sync::Mutex::new(search));
        let config = HarnessConfig {
            model: "test-model".to_string(),
            ..Default::default()
        };
        let chat = Arc::new(ChatRuntime::new(config, db.clone(), search));
        let (tx, rx) = mpsc::unbounded_channel();
        let handler = RpcHandler::new(db, chat, tx, "test-model".to_string());
        (handler, rx)
    }

    #[tokio::test]
    async fn test_initialize() {
        let (handler, _rx) = test_handler();
        let req = JsonRpcRequest {
            jsonrpc: "2.0".to_string(),
            id: Some(serde_json::json!(1)),
            method: "initialize".to_string(),
            params: serde_json::Value::Null,
        };
        let resp = handler.handle(req).await.expect("should return response");
        assert!(resp.result.is_some());
        assert!(resp.error.is_none());
        let result = resp.result.unwrap();
        assert_eq!(result["name"], "candela-harness");
    }

    #[tokio::test]
    async fn test_notification_returns_none() {
        let (handler, _rx) = test_handler();
        let req = JsonRpcRequest {
            jsonrpc: "2.0".to_string(),
            id: None,
            method: "initialize".to_string(),
            params: serde_json::Value::Null,
        };
        let resp = handler.handle(req).await;
        assert!(resp.is_none(), "notifications must not produce a response");
    }

    #[tokio::test]
    async fn test_session_lifecycle() {
        let (handler, _rx) = test_handler();

        // Create
        let req = JsonRpcRequest {
            jsonrpc: "2.0".to_string(),
            id: Some(serde_json::json!(1)),
            method: "session.create".to_string(),
            params: serde_json::json!({ "model": "test-model" }),
        };
        let resp = handler.handle(req).await.unwrap();
        let session_id = resp.result.unwrap()["id"].as_str().unwrap().to_string();

        // List
        let req = JsonRpcRequest {
            jsonrpc: "2.0".to_string(),
            id: Some(serde_json::json!(2)),
            method: "session.list".to_string(),
            params: serde_json::json!({}),
        };
        let resp = handler.handle(req).await.unwrap();
        let sessions = resp.result.unwrap();
        assert_eq!(sessions["sessions"].as_array().unwrap().len(), 1);

        // Delete
        let req = JsonRpcRequest {
            jsonrpc: "2.0".to_string(),
            id: Some(serde_json::json!(3)),
            method: "session.delete".to_string(),
            params: serde_json::json!({ "session_id": session_id }),
        };
        let resp = handler.handle(req).await.unwrap();
        assert!(resp.error.is_none());
    }

    #[tokio::test]
    async fn test_session_delete_not_found() {
        let (handler, _rx) = test_handler();
        let req = JsonRpcRequest {
            jsonrpc: "2.0".to_string(),
            id: Some(serde_json::json!(1)),
            method: "session.delete".to_string(),
            params: serde_json::json!({ "session_id": "nonexistent" }),
        };
        let resp = handler.handle(req).await.unwrap();
        assert!(resp.error.is_some());
        assert_eq!(resp.error.unwrap().code, INVALID_PARAMS);
    }

    #[tokio::test]
    async fn test_unknown_method() {
        let (handler, _rx) = test_handler();
        let req = JsonRpcRequest {
            jsonrpc: "2.0".to_string(),
            id: Some(serde_json::json!(1)),
            method: "nonexistent".to_string(),
            params: serde_json::Value::Null,
        };
        let resp = handler.handle(req).await.unwrap();
        assert!(resp.error.is_some());
        assert_eq!(resp.error.unwrap().code, METHOD_NOT_FOUND);
    }

    #[tokio::test]
    async fn test_session_create_uses_default_model() {
        let (handler, _rx) = test_handler();
        let req = JsonRpcRequest {
            jsonrpc: "2.0".to_string(),
            id: Some(serde_json::json!(1)),
            method: "session.create".to_string(),
            params: serde_json::json!({}),
        };
        let resp = handler.handle(req).await.unwrap();
        let model = resp.result.unwrap()["model"].as_str().unwrap().to_string();
        assert_eq!(model, "test-model");
    }

    #[tokio::test]
    async fn test_negative_limit_clamped() {
        let (handler, _rx) = test_handler();
        let req = JsonRpcRequest {
            jsonrpc: "2.0".to_string(),
            id: Some(serde_json::json!(1)),
            method: "session.list".to_string(),
            params: serde_json::json!({ "limit": -5, "offset": -10 }),
        };
        let resp = handler.handle(req).await.unwrap();
        // Should not error — clamped to 0
        assert!(resp.error.is_none());
    }

    #[tokio::test]
    async fn test_chat_send_validates_params() {
        let (handler, _rx) = test_handler();

        // Missing session_id
        let req = JsonRpcRequest {
            jsonrpc: "2.0".to_string(),
            id: Some(serde_json::json!(1)),
            method: "chat.send".to_string(),
            params: serde_json::json!({ "content": "hello" }),
        };
        let resp = handler.handle(req).await.unwrap();
        assert!(resp.error.is_some());
        assert_eq!(resp.error.unwrap().code, INVALID_PARAMS);

        // Missing content
        let req = JsonRpcRequest {
            jsonrpc: "2.0".to_string(),
            id: Some(serde_json::json!(2)),
            method: "chat.send".to_string(),
            params: serde_json::json!({ "session_id": "test" }),
        };
        let resp = handler.handle(req).await.unwrap();
        assert!(resp.error.is_some());
        assert_eq!(resp.error.unwrap().code, INVALID_PARAMS);
    }

    #[tokio::test]
    async fn test_chat_send_returns_accepted() {
        let (handler, _rx) = test_handler();

        // Create a session first
        let req = JsonRpcRequest {
            jsonrpc: "2.0".to_string(),
            id: Some(serde_json::json!(1)),
            method: "session.create".to_string(),
            params: serde_json::json!({}),
        };
        let resp = handler.handle(req).await.unwrap();
        let session_id = resp.result.unwrap()["id"].as_str().unwrap().to_string();

        // Send chat — should immediately return accepted (even though model call will fail)
        let req = JsonRpcRequest {
            jsonrpc: "2.0".to_string(),
            id: Some(serde_json::json!(2)),
            method: "chat.send".to_string(),
            params: serde_json::json!({
                "session_id": session_id,
                "content": "hello"
            }),
        };
        let resp = handler.handle(req).await.unwrap();
        assert!(resp.error.is_none());
        let result = resp.result.unwrap();
        assert_eq!(result["status"], "accepted");
        assert!(result["stream_id"].is_string());
    }
}
