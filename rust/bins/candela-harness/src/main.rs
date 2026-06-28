//! candela-harness — IDE sidecar for AI-assisted development.

use std::path::PathBuf;
use std::sync::Arc;

use candela_core::harness::{HarnessConfig, TransportMode};
use candela_harness_rpc::{RpcHandler, protocol::JsonRpcRequest};
use candela_harness_storage::Database;
use clap::Parser;
use tracing::info;
use tracing_subscriber::EnvFilter;

#[derive(Parser)]
#[command(
    name = "candela-harness",
    version,
    about = "IDE sidecar for AI-assisted development"
)]
struct Cli {
    /// Transport mode: stdio (default) or http
    #[arg(long, default_value = "stdio")]
    transport: String,

    /// HTTP port for daemon mode
    #[arg(long, default_value_t = 8200)]
    port: u16,

    /// Session storage directory
    #[arg(long)]
    save_dir: Option<PathBuf>,

    /// Resume a conversation by ID
    #[arg(long)]
    conversation_id: Option<String>,

    /// Default model
    #[arg(long, default_value = "gemini-2.0-flash")]
    model: String,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Initialize tracing
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::from_default_env().add_directive("harness=info".parse()?))
        .with_writer(std::io::stderr)
        .init();

    let cli = Cli::parse();

    let config = HarnessConfig {
        save_dir: cli.save_dir.unwrap_or_else(|| {
            dirs::home_dir()
                .unwrap_or_default()
                .join(".candela")
                .join("sessions")
        }),
        model: cli.model,
        transport: match cli.transport.as_str() {
            "http" => TransportMode::Http,
            _ => TransportMode::Stdio,
        },
        http_port: cli.port,
        ..Default::default()
    };

    // Ensure save directory exists
    std::fs::create_dir_all(&config.save_dir)?;

    // Open database
    let db_path = config.save_dir.join("harness.db");
    let db = Database::open(&db_path)?;
    let db = Arc::new(std::sync::Mutex::new(db));

    let handler = RpcHandler::new(db);

    info!(
        transport = ?config.transport,
        model = %config.model,
        save_dir = %config.save_dir.display(),
        "candela-harness started"
    );

    match config.transport {
        TransportMode::Stdio => serve_stdio(handler).await?,
        TransportMode::Http => {
            info!(port = config.http_port, "HTTP mode not yet implemented");
            // TODO: Axum HTTP server
        }
    }

    Ok(())
}

/// Read JSON-RPC from stdin, write responses to stdout.
async fn serve_stdio(handler: RpcHandler) -> anyhow::Result<()> {
    use tokio::io::{AsyncBufReadExt, BufReader};

    let stdin = tokio::io::stdin();
    let reader = BufReader::new(stdin);
    let mut lines = reader.lines();

    while let Some(line) = lines.next_line().await? {
        if line.trim().is_empty() {
            continue;
        }

        let request: JsonRpcRequest = match serde_json::from_str(&line) {
            Ok(req) => req,
            Err(_) => {
                let resp = candela_harness_rpc::protocol::JsonRpcResponse::error(
                    None,
                    candela_harness_rpc::protocol::PARSE_ERROR,
                    "Parse error".to_string(),
                );
                println!("{}", serde_json::to_string(&resp)?);
                continue;
            }
        };

        let response = handler.handle(request).await;
        println!("{}", serde_json::to_string(&response)?);
    }

    Ok(())
}
