package transparent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/candelahq/candela/pkg/proxy"
)

// peekBufPool reuses 16KB buffers for TLS ClientHello peeking,
// avoiding per-connection allocations under high concurrency.
var peekBufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 16384)
		return &buf
	},
}

// upstreamDialTimeout is the maximum time to wait for an upstream TCP
// connection. Prevents goroutine leaks against unresponsive destinations.
const upstreamDialTimeout = 10 * time.Second

// Listener accepts connections on a port that receives iptables-redirected
// traffic (typically port 15001). For each connection it:
//
//  1. Peeks the TLS ClientHello to extract the SNI hostname.
//  2. Looks up the SNI in the provider map.
//  3. If matched: records the interception event and tunnels the TCP
//     connection to the original destination. (Future phase: MITM TLS
//     termination will route through the Candela HTTP proxy for
//     request-level budget enforcement and token counting.)
//  4. If not matched: tunnels directly to the original destination (passthrough).
//
// Upstream resolution priority: SO_ORIGINAL_DST (iptables conntrack) → SNI DNS.
//
// This implements SNI-based interception without requiring application
// configuration changes (no SDK base_url changes needed).
type Listener struct {
	// listenAddr is the address to listen on (e.g. ":15001").
	listenAddr string

	// sniMap maps SNI hostnames to provider names.
	sniMap *proxy.SNIMap

	// proxyAddr is the address of the Candela HTTP proxy (e.g. "127.0.0.1:8080").
	// Matched connections are forwarded here.
	proxyAddr string

	// stats tracks interception metrics.
	stats Stats
}

// Stats tracks transparent proxy interception statistics.
// Stats are safe for concurrent use and can be exposed via HTTP for
// production monitoring (e.g. /debug/transparent/stats).
type Stats struct {
	mu          sync.Mutex
	Intercepted int64
	Passthrough int64
	Errors      int64
}

// Snapshot returns a copy of the current stats.
func (s *Stats) Snapshot() (intercepted, passthrough, errors int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Intercepted, s.Passthrough, s.Errors
}

// ServeHTTP exposes stats as JSON for health checks and monitoring.
// Register with: http.Handle("/debug/transparent/stats", listener.Stats())
func (s *Stats) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	intercepted, passthrough, errors := s.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"intercepted":%d,"passthrough":%d,"errors":%d}`,
		intercepted, passthrough, errors)
}

// LogStats emits the current stats to the structured logger. Call periodically
// (e.g. every 30s) from the sidecar main loop for production visibility.
func (s *Stats) LogStats() {
	intercepted, passthrough, errors := s.Snapshot()
	slog.Info("transparent proxy stats",
		"intercepted", intercepted,
		"passthrough", passthrough,
		"errors", errors,
		"total", intercepted+passthrough+errors,
	)
}

func (s *Stats) incIntercepted() {
	s.mu.Lock()
	s.Intercepted++
	s.mu.Unlock()
}

func (s *Stats) incPassthrough() {
	s.mu.Lock()
	s.Passthrough++
	s.mu.Unlock()
}

func (s *Stats) incErrors() {
	s.mu.Lock()
	s.Errors++
	s.mu.Unlock()
}

// Config holds transparent listener configuration.
type Config struct {
	// ListenAddr is the address to listen on (e.g. ":15001").
	ListenAddr string

	// SNIMap maps SNI hostnames to provider names for routing decisions.
	SNIMap *proxy.SNIMap

	// ProxyAddr is the address of the Candela HTTP proxy listener.
	// Intercepted connections are forwarded here.
	ProxyAddr string
}

// NewListener creates a transparent proxy listener.
func NewListener(cfg Config) *Listener {
	return &Listener{
		listenAddr: cfg.ListenAddr,
		sniMap:     cfg.SNIMap,
		proxyAddr:  cfg.ProxyAddr,
	}
}

// Stats returns the listener's interception statistics.
func (l *Listener) Stats() *Stats {
	return &l.stats
}

// ListenAndServe starts accepting connections. It blocks until the context
// is cancelled or an unrecoverable error occurs.
func (l *Listener) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", l.listenAddr)
	if err != nil {
		return fmt.Errorf("transparent listen %s: %w", l.listenAddr, err)
	}
	defer func() { _ = ln.Close() }()

	slog.Info("🔍 transparent proxy listening",
		"addr", l.listenAddr,
		"proxy_addr", l.proxyAddr,
		"sni_hosts", l.sniMap.Hosts())

	// Close listener when context is cancelled.
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			// Check if we're shutting down.
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			slog.Warn("transparent accept error", "error", err)
			continue
		}

		go l.handleConn(conn)
	}
}

// ListenAndServeOnListener is like ListenAndServe but uses an existing
// net.Listener. This is primarily for testing.
func (l *Listener) ListenAndServeOnListener(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			return err
		}

		go l.handleConn(conn)
	}
}

// handleConn processes a single intercepted connection.
func (l *Listener) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	// Peek the first bytes to extract the TLS ClientHello SNI.
	// The peeked bytes are replayed to the upstream connection.
	// 16KB is sufficient for TLS 1.3 ClientHello with ECH + GREASE extensions.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	peekBufPtr := peekBufPool.Get().(*[]byte)
	defer peekBufPool.Put(peekBufPtr)
	peekBuf := *peekBufPtr
	n, err := conn.Read(peekBuf)
	_ = conn.SetReadDeadline(time.Time{}) // clear deadline for tunnel phase
	if err != nil {
		slog.Debug("transparent: failed to read ClientHello", "error", err)
		l.stats.incErrors()
		return
	}
	peeked := peekBuf[:n]

	// Try to extract SNI.
	sni, err := ParseClientHelloSNI(peeked)
	if err != nil {
		slog.Debug("transparent: not TLS or no SNI, passthrough",
			"error", err,
			"remote", conn.RemoteAddr())
		l.tunnelPassthrough(conn, peeked)
		return
	}

	// Look up SNI in provider map.
	provider, matched := l.sniMap.Lookup(sni)
	if !matched {
		slog.Debug("transparent: SNI not in provider map, passthrough",
			"sni", sni,
			"remote", conn.RemoteAddr())
		l.stats.incPassthrough()
		l.tunnelToOrigDest(conn, peeked, sni)
		return
	}

	slog.Info("transparent: intercepting LLM connection",
		"sni", sni,
		"provider", provider,
		"remote", conn.RemoteAddr())
	l.stats.incIntercepted()

	// For Phase 3 (SNI-only routing without MITM), we tunnel to the
	// original destination through the proxy. Full MITM with cert
	// generation comes in a future phase.
	//
	// For now: tunnel to the original destination (the proxy sees the
	// connection in its stats, and we record the interception event).
	l.tunnelToOrigDest(conn, peeked, sni)
}

// tunnelPassthrough tunnels a non-TLS connection to its original destination.
func (l *Listener) tunnelPassthrough(clientConn net.Conn, peeked []byte) {
	l.stats.incPassthrough()

	// Try SO_ORIGINAL_DST to find the real destination.
	origDst := l.resolveOrigDst(clientConn, "")
	if origDst == "" {
		// Can't determine destination without SNI or SO_ORIGINAL_DST.
		slog.Debug("transparent: non-TLS with no original destination, dropping")
		return
	}

	upstream, err := (&net.Dialer{Timeout: upstreamDialTimeout}).Dial("tcp", origDst)
	if err != nil {
		slog.Warn("transparent: passthrough dial failed",
			"dest", origDst, "error", err)
		l.stats.incErrors()
		return
	}
	defer func() { _ = upstream.Close() }()

	// Replay peeked bytes and tunnel.
	if _, err := upstream.Write(peeked); err != nil {
		slog.Warn("transparent: passthrough write failed",
			"dest", origDst, "error", err)
		l.stats.incErrors()
		return
	}
	tunnel(clientConn, upstream)
}

// tunnelToOrigDest creates a TCP tunnel between the client and the original
// destination, replaying the peeked bytes.
func (l *Listener) tunnelToOrigDest(clientConn net.Conn, peeked []byte, sni string) {
	origDst := l.resolveOrigDst(clientConn, sni)
	if origDst == "" {
		slog.Warn("transparent: cannot resolve original destination",
			"sni", sni)
		l.stats.incErrors()
		return
	}

	upstream, err := (&net.Dialer{Timeout: upstreamDialTimeout}).Dial("tcp", origDst)
	if err != nil {
		slog.Warn("transparent: failed to connect to upstream",
			"sni", sni, "dest", origDst, "error", err)
		l.stats.incErrors()
		return
	}
	defer func() { _ = upstream.Close() }()

	// Write the peeked ClientHello to the upstream.
	if _, err := upstream.Write(peeked); err != nil {
		slog.Warn("transparent: failed to write ClientHello to upstream",
			"sni", sni, "error", err)
		l.stats.incErrors()
		return
	}

	// Bidirectional tunnel.
	tunnel(clientConn, upstream)
}

// resolveOrigDst determines the upstream address for a connection.
// Priority:
//  1. SO_ORIGINAL_DST (iptables REDIRECT — Linux only)
//  2. SNI hostname + port 443 (DNS resolution fallback)
func (l *Listener) resolveOrigDst(conn net.Conn, sni string) string {
	// Try SO_ORIGINAL_DST first (works when traffic was iptables-redirected).
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		if origDst, err := GetOriginalDst(tcpConn); err == nil {
			// Guard against self-connection: when there is no iptables
			// REDIRECT rule, SO_ORIGINAL_DST returns the actual destination
			// the client connected to — which is this listener itself.
			// Tunneling back to ourselves creates an infinite accept loop
			// that burns through file descriptors until the process crashes.
			if origDst == conn.LocalAddr().String() {
				slog.Debug("transparent: SO_ORIGINAL_DST returned self, skipping",
					"dest", origDst, "sni", sni)
			} else {
				slog.Debug("transparent: resolved via SO_ORIGINAL_DST",
					"dest", origDst, "sni", sni)
				return origDst
			}
		}
	}

	// Fallback: use SNI hostname.
	if sni != "" {
		return sni + ":443"
	}

	return ""
}

// tunnel copies data bidirectionally between two connections.
func tunnel(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(b, a)
		// Signal EOF to peer.
		if tc, ok := b.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(a, b)
		if tc, ok := a.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()

	wg.Wait()
}
