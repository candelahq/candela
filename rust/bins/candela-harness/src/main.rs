//! candela-harness — IDE sidecar for AI-assisted development.

use std::io::Write;
use std::path::PathBuf;
use std::sync::Arc;

use candela_core::harness::{HarnessConfig, TransportMode};
use candela_harness_chat::ChatRuntime;
use candela_harness_rpc::{
    RpcHandler,
    protocol::{JsonRpcNotification, JsonRpcRequest},
};
use candela_harness_storage::{Database, SearchIndex};
use clap::Parser;
use tokio::sync::mpsc;
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

    /// Default model
    #[arg(long, default_value = "gemini-2.0-flash")]
    model: String,

    /// LLM API base URL (e.g. http://localhost:8080/proxy/openai or https://api.openai.com)
    #[arg(long, env = "CANDELA_PROXY_URL")]
    proxy_url: Option<String>,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Initialize tracing
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::from_default_env().add_directive("harness=info".parse()?))
        .with_writer(std::io::stderr)
        .init();

    let cli = Cli::parse();

    let transport = match cli.transport.as_str() {
        "stdio" => TransportMode::Stdio,
        "http" => TransportMode::Http,
        other => anyhow::bail!("unsupported transport: {other:?} (expected \"stdio\" or \"http\")"),
    };

    let config = HarnessConfig {
        save_dir: cli.save_dir.unwrap_or_else(|| {
            dirs::home_dir()
                .unwrap_or_default()
                .join(".candela")
                .join("sessions")
        }),
        model: cli.model,
        transport,
        http_port: cli.port,
        proxy_url: cli.proxy_url,
        ..Default::default()
    };

    // Ensure save directory exists
    std::fs::create_dir_all(&config.save_dir)?;

    // Open database
    let db_path = config.save_dir.join("harness.db");
    let db = Database::open(&db_path)?;
    let db = Arc::new(std::sync::Mutex::new(db));

    // Open search index
    let search_path = config.save_dir.join("search.db");
    let search = SearchIndex::open(&search_path)?;
    let search = Arc::new(std::sync::Mutex::new(search));

    // Create notification channel for streaming events (used by stdio mode)
    let (notify_tx, notify_rx) = mpsc::unbounded_channel::<JsonRpcNotification>();

    // Create chat runtime
    let chat = Arc::new(ChatRuntime::new(config.clone(), db.clone(), search.clone()));

    info!(
        transport = ?config.transport,
        model = %config.model,
        save_dir = %config.save_dir.display(),
        proxy_url = ?config.proxy_url,
        "candela-harness started"
    );

    match config.transport {
        TransportMode::Stdio => {
            let handler = RpcHandler::new(db, chat, notify_tx, config.model.clone());
            serve_stdio(handler, notify_rx).await?;
        }
        TransportMode::Http => {
            candela_harness_connect::server::serve(config.http_port, chat, db, search)
                .await
                .map_err(|e| anyhow::anyhow!("ConnectRPC server error: {e}"))?;
        }
    }

    Ok(())
}

/// Read JSON-RPC from stdin, write responses + notifications to stdout.
async fn serve_stdio(
    handler: RpcHandler,
    mut notify_rx: mpsc::UnboundedReceiver<JsonRpcNotification>,
) -> anyhow::Result<()> {
    use tokio::io::{AsyncBufReadExt, BufReader};

    let stdin = tokio::io::stdin();
    let reader = BufReader::new(stdin);
    let mut lines = reader.lines();

    // Spawn a task to write notifications to stdout.
    // This runs independently of the request/response loop so streaming
    // events can be emitted while the main loop waits for the next request.
    let stdout_notify = tokio::spawn(async move {
        while let Some(notif) = notify_rx.recv().await {
            if let Ok(json) = serde_json::to_string(&notif) {
                let _ = writeln!(std::io::stdout(), "{json}");
                let _ = std::io::stdout().flush();
            }
        }
    });

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
                std::io::stdout().flush()?;
                continue;
            }
        };

        if let Some(response) = handler.handle(request).await {
            println!("{}", serde_json::to_string(&response)?);
            std::io::stdout().flush()?;
        }
    }

    // Clean up notification task
    stdout_notify.abort();

    Ok(())
}
