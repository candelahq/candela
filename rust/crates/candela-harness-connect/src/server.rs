//! Axum + ConnectRPC server setup.

use std::sync::{Arc, Mutex};

use candela_harness_chat::ChatRuntime;
use candela_harness_storage::{Database, SearchIndex};
use tower_http::cors::CorsLayer;
use tower_http::trace::TraceLayer;
use tracing::info;

use crate::proto::candela::harness::v1::HarnessServiceExt;
use crate::service::HarnessServiceImpl;

/// Start the ConnectRPC server.
///
/// Serves Connect, gRPC, and gRPC-Web protocols on the given port.
pub async fn serve(
    port: u16,
    chat: Arc<ChatRuntime>,
    db: Arc<Mutex<Database>>,
    search: Arc<Mutex<SearchIndex>>,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let service = Arc::new(HarnessServiceImpl { chat, db, search });

    // Register the service with a ConnectRPC router, then convert to Axum
    let connect_router = service.register(connectrpc::Router::new());

    let app = axum::Router::new()
        .merge(connect_router.into_axum_router())
        .layer(CorsLayer::permissive())
        .layer(TraceLayer::new_for_http());

    let addr = format!("0.0.0.0:{port}");
    let listener = tokio::net::TcpListener::bind(&addr).await?;
    info!(addr = %addr, "harness ConnectRPC server started");

    axum::serve(listener, app).await?;

    Ok(())
}
