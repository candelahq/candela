//! ConnectRPC HTTP transport for the candela harness.
//!
//! Serves Connect, gRPC, and gRPC-Web protocols on a single port.

pub mod proto {
    include!(concat!(env!("OUT_DIR"), "/_connectrpc.rs"));
}

pub mod server;
pub mod service;
