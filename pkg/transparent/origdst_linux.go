// Package transparent implements a transparent proxy listener for intercepting
// iptables-redirected TLS connections.
//
// This file contains the Linux-specific SO_ORIGINAL_DST implementation for
// retrieving the original destination address of iptables REDIRECT'd connections.

//go:build linux

package transparent

import (
	"fmt"
	"net"
	"syscall"
	"unsafe"
)

// SO_ORIGINAL_DST is the Linux socket option for retrieving the original
// destination of a connection that was redirected by iptables REDIRECT/DNAT.
// It is defined in the kernel as 80 (include/uapi/linux/netfilter_ipv4.h).
const SO_ORIGINAL_DST = 80

// SO_ORIGINAL_DST for IPv6 (ip6_tables).
const IP6T_SO_ORIGINAL_DST = 80

// GetOriginalDst retrieves the original destination address of an
// iptables-redirected TCP connection. This is used in transparent proxy
// mode where outbound port 443 traffic is redirected to the proxy port
// (e.g. 15001) via:
//
//	iptables -t nat -A OUTPUT -p tcp --dport 443 -j REDIRECT --to-port 15001
//
// The kernel stores the original destination in the conntrack entry, which
// can be retrieved via the SO_ORIGINAL_DST socket option.
//
// Returns the original destination as "host:port".
func GetOriginalDst(conn *net.TCPConn) (string, error) {
	// Use SyscallConn to access the file descriptor without duplicating it
	// or switching to blocking mode (unlike conn.File()).
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return "", fmt.Errorf("get raw conn: %w", err)
	}

	var (
		result string
		opErr  error
	)

	ctrlErr := rawConn.Control(func(fd uintptr) {
		// Try IPv4 first.
		addr4, err := getOriginalDst4(int(fd))
		if err == nil {
			result = addr4
			return
		}

		// Fall back to IPv6.
		addr6, err := getOriginalDst6(int(fd))
		if err == nil {
			result = addr6
			return
		}

		opErr = fmt.Errorf("getsockopt SO_ORIGINAL_DST failed (ipv4 and ipv6)")
	})
	if ctrlErr != nil {
		return "", fmt.Errorf("raw conn control: %w", ctrlErr)
	}
	if opErr != nil {
		return "", opErr
	}
	return result, nil
}

// getOriginalDst4 retrieves the IPv4 original destination.
func getOriginalDst4(fd int) (string, error) {
	var addr syscall.RawSockaddrInet4
	size := uint32(unsafe.Sizeof(addr))

	_, _, errno := syscall.Syscall6(
		syscall.SYS_GETSOCKOPT,
		uintptr(fd),
		uintptr(syscall.SOL_IP),
		uintptr(SO_ORIGINAL_DST),
		uintptr(unsafe.Pointer(&addr)),
		uintptr(unsafe.Pointer(&size)),
		0,
	)
	if errno != 0 {
		return "", fmt.Errorf("ipv4 getsockopt: %v", errno)
	}

	ip := net.IPv4(addr.Addr[0], addr.Addr[1], addr.Addr[2], addr.Addr[3])
	port := ntohs(addr.Port)
	return fmt.Sprintf("%s:%d", ip.String(), port), nil
}

// getOriginalDst6 retrieves the IPv6 original destination.
func getOriginalDst6(fd int) (string, error) {
	var addr syscall.RawSockaddrInet6
	size := uint32(unsafe.Sizeof(addr))

	_, _, errno := syscall.Syscall6(
		syscall.SYS_GETSOCKOPT,
		uintptr(fd),
		uintptr(syscall.SOL_IPV6),
		uintptr(IP6T_SO_ORIGINAL_DST),
		uintptr(unsafe.Pointer(&addr)),
		uintptr(unsafe.Pointer(&size)),
		0,
	)
	if errno != 0 {
		return "", fmt.Errorf("ipv6 getsockopt: %v", errno)
	}

	ip := net.IP(addr.Addr[:])
	port := ntohs(addr.Port)
	return fmt.Sprintf("[%s]:%d", ip.String(), port), nil
}

// ntohs converts a uint16 from network byte order (big-endian) to host byte order.
func ntohs(n uint16) int {
	return int(n>>8) | int(n&0xff)<<8
}
