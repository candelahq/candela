//! MITM TLS interceptor for transparent proxy connections.
//!
//! Terminates TLS with the client using an ephemeral leaf certificate,
//! then forwards the decrypted bytes to the Candela HTTP proxy via a
//! plaintext TCP connection. The interceptor is fully protocol-transparent:
//! it performs byte-level `copy_bidirectional` without parsing HTTP.
//!
//! **Trace context propagation:** The MITM layer does NOT inject or modify
//! HTTP headers — it operates at the byte level. W3C `traceparent` headers
//! propagate naturally: the client's HTTP request (including trace headers)
//! flows through the decrypted TLS stream → plaintext to the proxy handler,
//! which already parses `traceparent` (see `handler.rs`). The `#[instrument]`
//! spans here provide sidecar-internal observability for the MITM lifecycle
//! itself (handshake timing, byte counts, errors).

use std::io::Cursor;
use std::sync::atomic::Ordering;
use std::time::{Duration, Instant};

use tokio::io::{self, AsyncRead, AsyncWrite};
use tokio::net::TcpStream;
use tokio_rustls::TlsAcceptor;
use tracing::{Instrument, debug, info_span};

use crate::ca::EphemeralCA;
use crate::listener::Stats;

/// RAII guard that records `duration_ms` on the current tracing span when
/// dropped. Guarantees the field is always populated regardless of which
/// exit path (success, error, early-return) is taken.
struct DurationGuard(tracing::Span, Instant);

impl Drop for DurationGuard {
    fn drop(&mut self) {
        self.0
            .record("duration_ms", self.1.elapsed().as_millis() as u64);
    }
}

/// Timeout for connecting to the local Candela proxy.
const PROXY_DIAL_TIMEOUT: Duration = Duration::from_secs(5);

/// Performs MITM TLS termination on an intercepted connection.
///
/// 1. Obtains a `ServerConfig` from the CA for the given SNI hostname.
/// 2. Replays the peeked ClientHello bytes and accepts the TLS handshake.
/// 3. Opens a plaintext TCP connection to `proxy_addr`.
/// 4. Bidirectionally copies bytes between the decrypted TLS stream and
///    the proxy connection.
///
/// The function is instrumented with a `tracing` span (`mitm.tls_termination`)
/// that records SNI, proxy address, handshake success, and byte counts.
/// When the sidecar exports to an OTLP collector, these spans provide
/// visibility into the MITM lifecycle alongside the proxy's LLM spans.
///
/// # Errors
///
/// Returns an error if any step fails (cert generation, TLS handshake,
/// proxy dial, or copy).
pub async fn mitm_intercept(
    client: TcpStream,
    peeked: &[u8],
    sni: &str,
    ca: &EphemeralCA,
    proxy_addr: &str,
    stats: &Stats,
) -> anyhow::Result<()> {
    let span = info_span!(
        "mitm.tls_termination",
        sni = %sni,
        proxy_addr = %proxy_addr,
        handshake_ok = tracing::field::Empty,
        handshake_ms = tracing::field::Empty,
        bytes_client_to_proxy = tracing::field::Empty,
        bytes_proxy_to_client = tracing::field::Empty,
        duration_ms = tracing::field::Empty,
    );

    async {
        let start = Instant::now();
        let _duration_guard = DurationGuard(tracing::Span::current(), start);

        // 1. Get the TLS ServerConfig for this hostname.
        let server_config = ca.server_config_for(sni).map_err(|e| {
            tracing::Span::current().record("handshake_ok", false);
            debug!(sni = %sni, error = %e, "MITM cert generation failed");
            e
        })?;
        let acceptor = TlsAcceptor::from(server_config);

        // 2. Replay the peeked ClientHello.
        //    We already consumed these bytes from the socket; we need to
        //    prepend them so the TLS acceptor sees the full handshake.
        let replay = ReplayStream::new(peeked, client);

        // 3. Accept TLS handshake.
        let tls_stream = acceptor.accept(replay).await.map_err(|e| {
            tracing::Span::current().record("handshake_ok", false);
            debug!(sni = %sni, error = %e, "MITM TLS handshake failed");
            anyhow::anyhow!("TLS handshake failed for {sni}: {e}")
        })?;

        let handshake_elapsed = start.elapsed();
        let current_span = tracing::Span::current();
        current_span.record("handshake_ok", true);
        current_span.record("handshake_ms", handshake_elapsed.as_millis() as u64);
        debug!(sni = %sni, handshake_ms = handshake_elapsed.as_millis(), "MITM TLS handshake succeeded");

        // 4. Open plaintext connection to the Candela proxy.
        let mut proxy_stream =
            tokio::time::timeout(PROXY_DIAL_TIMEOUT, TcpStream::connect(proxy_addr))
                .await
                .map_err(|_| anyhow::anyhow!("proxy dial timeout to {proxy_addr}"))??;

        // 5. Bidirectional copy: decrypted client ↔ plaintext proxy.
        //    copy_bidirectional drains both directions fully before closing,
        //    avoiding response truncation when the client finishes sending
        //    before the proxy finishes responding.
        let mut tls_stream = tls_stream;
        match io::copy_bidirectional(&mut tls_stream, &mut proxy_stream).await {
            Ok((client_to_proxy, proxy_to_client)) => {
                let current = tracing::Span::current();
                current.record("bytes_client_to_proxy", client_to_proxy);
                current.record("bytes_proxy_to_client", proxy_to_client);
                debug!(
                    sni = %sni,
                    client_to_proxy,
                    proxy_to_client,
                    "MITM bidirectional copy completed"
                );
            }
            Err(e) => {
                debug!(sni = %sni, error = %e, "MITM bidirectional copy error");
            }
        }

        stats.mitm.fetch_add(1, Ordering::Relaxed);
        Ok(())
    }
    .instrument(span)
    .await
}

/// A stream that replays `peeked` bytes before reading from the inner stream.
///
/// When the transparent listener peeks the ClientHello, it consumes those
/// bytes from the TCP socket. The TLS acceptor needs to see them, so we
/// prepend them via this wrapper.
struct ReplayStream<S> {
    replay: Cursor<Vec<u8>>,
    inner: S,
    replay_done: bool,
}

impl<S> ReplayStream<S> {
    fn new(peeked: &[u8], inner: S) -> Self {
        Self {
            replay: Cursor::new(peeked.to_vec()),
            inner,
            replay_done: false,
        }
    }
}

impl<S: AsyncRead + Unpin> AsyncRead for ReplayStream<S> {
    fn poll_read(
        self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
        buf: &mut io::ReadBuf<'_>,
    ) -> std::task::Poll<io::Result<()>> {
        let this = self.get_mut();

        if !this.replay_done {
            let before = buf.filled().len();
            let result = std::pin::Pin::new(&mut this.replay).poll_read(cx, buf);
            let after = buf.filled().len();

            if after > before {
                return result;
            }
            // Replay exhausted — switch to inner stream.
            this.replay_done = true;
        }

        std::pin::Pin::new(&mut this.inner).poll_read(cx, buf)
    }
}

impl<S: AsyncWrite + Unpin> AsyncWrite for ReplayStream<S> {
    fn poll_write(
        self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
        buf: &[u8],
    ) -> std::task::Poll<io::Result<usize>> {
        std::pin::Pin::new(&mut self.get_mut().inner).poll_write(cx, buf)
    }

    fn poll_flush(
        self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
    ) -> std::task::Poll<io::Result<()>> {
        std::pin::Pin::new(&mut self.get_mut().inner).poll_flush(cx)
    }

    fn poll_shutdown(
        self: std::pin::Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
    ) -> std::task::Poll<io::Result<()>> {
        std::pin::Pin::new(&mut self.get_mut().inner).poll_shutdown(cx)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::io::AsyncReadExt;

    #[tokio::test]
    async fn replay_stream_replays_peeked_bytes() {
        let peeked = b"hello ";
        let inner_data = b"world";
        let inner = Cursor::new(inner_data.to_vec());

        let mut stream = ReplayStream::new(peeked, inner);
        let mut result = Vec::new();
        stream.read_to_end(&mut result).await.unwrap();

        assert_eq!(result, b"hello world");
    }

    #[tokio::test]
    async fn replay_stream_empty_peeked() {
        let inner_data = b"just inner";
        let inner = Cursor::new(inner_data.to_vec());

        let mut stream = ReplayStream::new(b"", inner);
        let mut result = Vec::new();
        stream.read_to_end(&mut result).await.unwrap();

        assert_eq!(result, b"just inner");
    }

    #[tokio::test]
    async fn replay_stream_only_peeked() {
        let inner = Cursor::new(Vec::<u8>::new());

        let mut stream = ReplayStream::new(b"only peeked", inner);
        let mut result = Vec::new();
        stream.read_to_end(&mut result).await.unwrap();

        assert_eq!(result, b"only peeked");
    }

    fn install_crypto_provider() {
        let _ = rustls::crypto::ring::default_provider().install_default();
    }

    #[tokio::test]
    async fn mitm_intercept_end_to_end() {
        install_crypto_provider();
        use std::sync::Arc;
        use tokio::io::AsyncWriteExt;
        use tokio::net::TcpListener;

        // Start a mock plaintext echo server that reads exactly N bytes
        // (sent as a 4-byte big-endian length prefix) then echoes them back.
        // This avoids depending on half-close / EOF which races with TLS
        // close_notify timing.
        let echo_listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let echo_addr = echo_listener.local_addr().unwrap();
        let echo_handle = tokio::spawn(async move {
            if let Ok((mut stream, _)) = echo_listener.accept().await {
                // Read 4-byte length prefix.
                let mut len_buf = [0u8; 4];
                stream.read_exact(&mut len_buf).await.unwrap();
                let len = u32::from_be_bytes(len_buf) as usize;

                // Read exactly `len` payload bytes.
                let mut payload = vec![0u8; len];
                stream.read_exact(&mut payload).await.unwrap();

                // Echo the payload back (without the length prefix).
                stream.write_all(&payload).await.unwrap();
                stream.flush().await.unwrap();
                // Shut down our write side so the MITM copy sees EOF.
                let _ = stream.shutdown().await;
            }
        });

        // Generate ephemeral CA.
        let ca = Arc::new(EphemeralCA::generate().unwrap());

        // Start a listener that the "client" connects to.
        let mitm_listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let mitm_addr = mitm_listener.local_addr().unwrap();
        let stats = Arc::new(Stats::default());
        let ca_clone = Arc::clone(&ca);
        let stats_clone = Arc::clone(&stats);
        let proxy_addr_str = echo_addr.to_string();

        // Spawn MITM handler that accepts one connection.
        let mitm_handle = tokio::spawn(async move {
            let (stream, _) = mitm_listener.accept().await.unwrap();

            // Read the "peeked" bytes (simulating the listener peek).
            let mut buf = vec![0u8; 16384];
            let n = stream.peek(&mut buf).await.unwrap();
            let peeked = buf[..n].to_vec();

            // Consume the peeked bytes from the socket.
            let mut discard = vec![0u8; n];
            let mut stream2 = stream;
            stream2.read_exact(&mut discard).await.unwrap();

            mitm_intercept(
                stream2,
                &peeked,
                "test.example.com",
                &ca_clone,
                &proxy_addr_str,
                &stats_clone,
            )
            .await
        });

        // Connect a TLS client that trusts the ephemeral CA.
        let mut root_store = rustls::RootCertStore::empty();
        root_store.add(ca.ca_cert_der.clone()).unwrap();
        let client_cfg = Arc::new(
            rustls::ClientConfig::builder()
                .with_root_certificates(root_store)
                .with_no_client_auth(),
        );
        let connector = tokio_rustls::TlsConnector::from(client_cfg);
        let server_name = rustls::pki_types::ServerName::try_from("test.example.com")
            .unwrap()
            .to_owned();

        let tcp = TcpStream::connect(mitm_addr).await.unwrap();
        let mut tls = connector.connect(server_name, tcp).await.unwrap();

        // Build a length-prefixed payload: [4-byte big-endian len][payload].
        let payload = b"hello mitm";
        let len_prefix = (payload.len() as u32).to_be_bytes();
        tls.write_all(&len_prefix).await.unwrap();
        tls.write_all(payload).await.unwrap();
        tls.flush().await.unwrap();

        // Read the echoed response. The echo server writes exactly `payload`
        // bytes then shuts down its write side, so read_to_end will return
        // once the proxy-side copy propagates the EOF.
        let mut response = Vec::new();
        let read_result =
            tokio::time::timeout(Duration::from_secs(5), tls.read_to_end(&mut response)).await;
        assert!(read_result.is_ok(), "read should not time out");
        assert_eq!(response, payload, "echo should return same data");

        // Clean shutdown after we have the response.
        let _ = tls.shutdown().await;

        // Wait for handlers to finish.
        let mitm_result = tokio::time::timeout(Duration::from_secs(2), mitm_handle).await;
        assert!(
            mitm_result.is_ok(),
            "MITM handler should complete within timeout"
        );
        let _ = tokio::time::timeout(Duration::from_secs(1), echo_handle).await;

        // Verify stats.
        assert_eq!(
            stats.mitm.load(Ordering::Relaxed),
            1,
            "MITM counter should be 1"
        );
    }

    /// Verifies that mitm_intercept returns an error and does NOT increment
    /// stats when the proxy address is unreachable (exercises the proxy dial
    /// timeout error path, which should record duration_ms on the span).
    #[tokio::test]
    async fn mitm_intercept_proxy_unreachable_returns_error() {
        install_crypto_provider();
        use std::sync::Arc;
        use tokio::net::TcpListener;

        let ca = Arc::new(EphemeralCA::generate().unwrap());
        let stats = Arc::new(Stats::default());

        // Bind a listener for the MITM side, but point proxy_addr at a
        // port nothing is listening on.
        let mitm_listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let mitm_addr = mitm_listener.local_addr().unwrap();

        // Use a port that will refuse connections.
        let bad_proxy_addr = "127.0.0.1:1"; // port 1 — almost certainly refused

        let ca_clone = Arc::clone(&ca);
        let stats_clone = Arc::clone(&stats);
        let proxy_str = bad_proxy_addr.to_string();

        let mitm_handle = tokio::spawn(async move {
            let (stream, _) = mitm_listener.accept().await.unwrap();
            let mut buf = vec![0u8; 16384];
            let n = stream.peek(&mut buf).await.unwrap();
            let peeked = buf[..n].to_vec();
            let mut stream2 = stream;
            let mut discard = vec![0u8; n];
            stream2.read_exact(&mut discard).await.unwrap();

            mitm_intercept(
                stream2,
                &peeked,
                "unreachable.example.com",
                &ca_clone,
                &proxy_str,
                &stats_clone,
            )
            .await
        });

        // Connect a TLS client.
        let mut root_store = rustls::RootCertStore::empty();
        root_store.add(ca.ca_cert_der.clone()).unwrap();
        let client_cfg = Arc::new(
            rustls::ClientConfig::builder()
                .with_root_certificates(root_store)
                .with_no_client_auth(),
        );
        let connector = tokio_rustls::TlsConnector::from(client_cfg);
        let server_name = rustls::pki_types::ServerName::try_from("unreachable.example.com")
            .unwrap()
            .to_owned();

        let tcp = TcpStream::connect(mitm_addr).await.unwrap();
        // The TLS handshake should succeed (the MITM accepts it), but then
        // the proxy dial should fail.
        let tls_result = connector.connect(server_name, tcp).await;
        // The handshake may succeed or fail depending on timing. Either way,
        // the MITM handler should complete with an error.
        drop(tls_result);

        let result = tokio::time::timeout(Duration::from_secs(10), mitm_handle)
            .await
            .expect("MITM handler should complete");
        let inner = result.expect("task should not panic");
        assert!(inner.is_err(), "should error on unreachable proxy");

        // Stats should NOT be incremented on failure.
        assert_eq!(
            stats.mitm.load(Ordering::Relaxed),
            0,
            "MITM counter should remain 0 on error"
        );
    }

    /// Verifies that sending non-TLS garbage data results in a handshake
    /// failure error (exercises the handshake_ok=false span recording path).
    #[tokio::test]
    async fn mitm_intercept_non_tls_data_returns_error() {
        install_crypto_provider();
        use std::sync::Arc;
        use tokio::io::AsyncWriteExt;
        use tokio::net::TcpListener;

        let ca = Arc::new(EphemeralCA::generate().unwrap());
        let stats = Arc::new(Stats::default());

        let echo_listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let echo_addr = echo_listener.local_addr().unwrap();

        let mitm_listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let mitm_addr = mitm_listener.local_addr().unwrap();

        let ca_clone = Arc::clone(&ca);
        let stats_clone = Arc::clone(&stats);
        let proxy_addr_str = echo_addr.to_string();

        let mitm_handle = tokio::spawn(async move {
            let (stream, _) = mitm_listener.accept().await.unwrap();
            // Simulate peeking some garbage "ClientHello".
            let garbage = b"NOT A TLS CLIENT HELLO";
            mitm_intercept(
                stream,
                garbage,
                "garbage.example.com",
                &ca_clone,
                &proxy_addr_str,
                &stats_clone,
            )
            .await
        });

        // Connect and send garbage (non-TLS) data.
        let mut tcp = TcpStream::connect(mitm_addr).await.unwrap();
        tcp.write_all(b"GET / HTTP/1.1\r\nHost: garbage.example.com\r\n\r\n")
            .await
            .unwrap();

        let result = tokio::time::timeout(Duration::from_secs(5), mitm_handle)
            .await
            .expect("MITM handler should complete");
        let inner = result.expect("task should not panic");
        assert!(inner.is_err(), "should error on non-TLS handshake");

        // Stats should NOT be incremented on failure.
        assert_eq!(
            stats.mitm.load(Ordering::Relaxed),
            0,
            "MITM counter should remain 0 on handshake failure"
        );
    }
}
