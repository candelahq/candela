//! Transparent proxy listener for iptables-redirected TLS connections.
//!
//! Intercepts outbound TCP connections redirected via iptables REDIRECT,
//! peeks the TLS ClientHello to extract the SNI hostname, and routes
//! matched LLM traffic through the Candela proxy pipeline.
//!
//! Ported from: `pkg/transparent/`

pub mod listener;
pub mod origdst;
pub mod sni;
