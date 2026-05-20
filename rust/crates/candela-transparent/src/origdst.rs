//! SO_ORIGINAL_DST for iptables-redirected TCP connections.
//!
//! Ported from: `pkg/transparent/origdst_linux.go` and `origdst_other.go`

use std::io;
use std::net::SocketAddr;

/// Retrieves the original destination of an iptables-redirected connection.
///
/// - **Linux**: Uses `getsockopt(SOL_IP, SO_ORIGINAL_DST)`.
/// - **Other**: Returns an error (SNI-based fallback should be used).
#[cfg(target_os = "linux")]
pub fn get_original_dst(stream: &tokio::net::TcpStream) -> io::Result<SocketAddr> {
    use std::mem;
    use std::os::unix::io::AsRawFd;

    const SO_ORIGINAL_DST: libc::c_int = 80;
    let fd = stream.as_raw_fd();

    // Try IPv4.
    unsafe {
        let mut addr: libc::sockaddr_in = mem::zeroed();
        let mut len = mem::size_of::<libc::sockaddr_in>() as libc::socklen_t;
        let ret = libc::getsockopt(
            fd,
            libc::SOL_IP,
            SO_ORIGINAL_DST,
            &mut addr as *mut _ as *mut libc::c_void,
            &mut len,
        );
        if ret == 0 {
            let ip = std::net::Ipv4Addr::from(u32::from_be(addr.sin_addr.s_addr));
            let port = u16::from_be(addr.sin_port);
            return Ok(SocketAddr::new(ip.into(), port));
        }
    }

    // Try IPv6.
    unsafe {
        let mut addr: libc::sockaddr_in6 = mem::zeroed();
        let mut len = mem::size_of::<libc::sockaddr_in6>() as libc::socklen_t;
        let ret = libc::getsockopt(
            fd,
            libc::SOL_IPV6,
            SO_ORIGINAL_DST,
            &mut addr as *mut _ as *mut libc::c_void,
            &mut len,
        );
        if ret == 0 {
            let ip = std::net::Ipv6Addr::from(addr.sin6_addr.s6_addr);
            let port = u16::from_be(addr.sin6_port);
            return Ok(SocketAddr::new(ip.into(), port));
        }
    }

    Err(io::Error::other("getsockopt SO_ORIGINAL_DST failed"))
}

/// Stub for non-Linux platforms.
#[cfg(not(target_os = "linux"))]
pub fn get_original_dst(_stream: &tokio::net::TcpStream) -> io::Result<SocketAddr> {
    Err(io::Error::new(
        io::ErrorKind::Unsupported,
        format!(
            "SO_ORIGINAL_DST not supported on {} (Linux only)",
            std::env::consts::OS
        ),
    ))
}
