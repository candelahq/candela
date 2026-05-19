// Package transparent implements a transparent proxy listener for intercepting
// iptables-redirected TLS connections.
//
// This file provides a stub for non-Linux platforms where SO_ORIGINAL_DST
// is unavailable. On these platforms, the transparent proxy falls back to
// SNI-based hostname resolution (net.Dial to the SNI hostname).

//go:build !linux

package transparent

import (
	"fmt"
	"net"
	"runtime"
)

// GetOriginalDst is not supported on non-Linux platforms.
// iptables REDIRECT and SO_ORIGINAL_DST are Linux kernel features.
// The transparent proxy will fall back to SNI-based hostname resolution.
func GetOriginalDst(conn *net.TCPConn) (string, error) {
	return "", fmt.Errorf("SO_ORIGINAL_DST not supported on %s (Linux only)", runtime.GOOS)
}
