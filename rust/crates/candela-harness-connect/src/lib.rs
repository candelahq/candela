//! ConnectRPC HTTP transport for the candela harness.
//!
//! Serves Connect, gRPC, and gRPC-Web protocols on a single port.

pub mod proto {
    connectrpc::include_generated!();
}

mod converters;

pub mod server;
pub mod service;
