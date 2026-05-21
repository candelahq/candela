package transparent_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/proxy"
	"github.com/candelahq/candela/pkg/transparent"
)

// TestListenerSNIInterception is an integration test that verifies the
// transparent listener correctly identifies LLM traffic by SNI and counts
// interception events.
func TestListenerSNIInterception(t *testing.T) {
	// Build an SNI map with test providers.
	providers := []proxy.Provider{
		{Name: "openai", UpstreamURL: "https://api.openai.com"},
		{Name: "anthropic-direct", UpstreamURL: "https://api.anthropic.com"},
	}
	sniMap := proxy.BuildSNIMap(providers)

	// Start the transparent listener on a random port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	listener := transparent.NewListener(transparent.Config{
		ListenAddr: ln.Addr().String(),
		SNIMap:     sniMap,
		ProxyAddr:  "127.0.0.1:8080", // not used in SNI-only mode
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = listener.ListenAndServeOnListener(ctx, ln)
	}()

	// Allow listener to start.
	time.Sleep(50 * time.Millisecond)

	// Connect with an LLM SNI — should be counted as intercepted.
	// The connection will fail at the tunnel stage (no real upstream),
	// but the SNI detection and stats should still work.
	sendTLSClientHello(t, ln.Addr().String(), "api.openai.com")
	time.Sleep(100 * time.Millisecond)

	intercepted, _, _ := listener.Stats().Snapshot()
	if intercepted != 1 {
		t.Errorf("intercepted = %d, want 1", intercepted)
	}

	// Connect with a non-LLM SNI — should be counted as passthrough.
	sendTLSClientHello(t, ln.Addr().String(), "example.com")
	time.Sleep(100 * time.Millisecond)

	intercepted2, passthrough, _ := listener.Stats().Snapshot()
	if intercepted2 != 1 {
		t.Errorf("intercepted = %d, want 1 (unchanged)", intercepted2)
	}
	if passthrough != 1 {
		t.Errorf("passthrough = %d, want 1", passthrough)
	}

	// Connect with another LLM SNI.
	sendTLSClientHello(t, ln.Addr().String(), "api.anthropic.com")
	time.Sleep(100 * time.Millisecond)

	intercepted3, _, _ := listener.Stats().Snapshot()
	if intercepted3 != 2 {
		t.Errorf("intercepted = %d, want 2", intercepted3)
	}
}

// TestListenerNonTLSPassthrough verifies that non-TLS traffic is passed through.
func TestListenerNonTLSPassthrough(t *testing.T) {
	providers := []proxy.Provider{
		{Name: "openai", UpstreamURL: "https://api.openai.com"},
	}
	sniMap := proxy.BuildSNIMap(providers)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	listener := transparent.NewListener(transparent.Config{
		ListenAddr: ln.Addr().String(),
		SNIMap:     sniMap,
		ProxyAddr:  "127.0.0.1:8080",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = listener.ListenAndServeOnListener(ctx, ln)
	}()
	time.Sleep(50 * time.Millisecond)

	// Send raw HTTP (not TLS) — should be counted as passthrough.
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	_, _ = conn.Write([]byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	_ = conn.Close()

	time.Sleep(100 * time.Millisecond)

	_, passthrough, _ := listener.Stats().Snapshot()
	if passthrough != 1 {
		t.Errorf("passthrough = %d, want 1", passthrough)
	}
}

// TestListenerWildcardSNI verifies that wildcard patterns match correctly.
func TestListenerWildcardSNI(t *testing.T) {
	providers := []proxy.Provider{
		{
			Name:        "anthropic",
			UpstreamURL: "https://us-central1-aiplatform.googleapis.com",
			HostPattern: "*-aiplatform.googleapis.com",
		},
	}
	sniMap := proxy.BuildSNIMap(providers)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	listener := transparent.NewListener(transparent.Config{
		ListenAddr: ln.Addr().String(),
		SNIMap:     sniMap,
		ProxyAddr:  "127.0.0.1:8080",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = listener.ListenAndServeOnListener(ctx, ln)
	}()
	time.Sleep(50 * time.Millisecond)

	// us-central1 should match the wildcard.
	sendTLSClientHello(t, ln.Addr().String(), "us-central1-aiplatform.googleapis.com")
	time.Sleep(100 * time.Millisecond)

	intercepted, _, _ := listener.Stats().Snapshot()
	if intercepted != 1 {
		t.Errorf("intercepted = %d, want 1 (wildcard match)", intercepted)
	}

	// europe-west4 should also match.
	sendTLSClientHello(t, ln.Addr().String(), "europe-west4-aiplatform.googleapis.com")
	time.Sleep(100 * time.Millisecond)

	intercepted, _, _ = listener.Stats().Snapshot()
	if intercepted != 2 {
		t.Errorf("intercepted = %d, want 2 (wildcard match)", intercepted)
	}
}

// sendTLSClientHello connects to the given address and sends a TLS ClientHello
// with the specified server name. The handshake will fail (no TLS server),
// but the ClientHello bytes are what we need.
// Returns true if the dial succeeded (connection reached the listener).
func sendTLSClientHello(t *testing.T, addr, serverName string) bool {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Logf("sendTLSClientHello: dial %s failed (expected in some cases): %v", addr, err)
		return false
	}
	defer func() { _ = conn.Close() }()

	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: true, //nolint:gosec // test only
	})

	// Start the handshake — will fail because remote isn't a TLS server.
	// But this sends the ClientHello.
	done := make(chan struct{})
	go func() {
		_ = tlsConn.Handshake()
		close(done)
	}()

	// Wait briefly for the ClientHello to be sent.
	select {
	case <-done:
	case <-time.After(1 * time.Second):
	}
	return true
}

// TestListenerConcurrentRace hammers the listener with many concurrent
// connections to verify the Stats counters are race-free.
// Run with: go test -race -count=1 ./pkg/transparent/...
func TestListenerConcurrentRace(t *testing.T) {
	providers := []proxy.Provider{
		{Name: "openai", UpstreamURL: "https://api.openai.com"},
	}
	sniMap := proxy.BuildSNIMap(providers)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	listener := transparent.NewListener(transparent.Config{
		ListenAddr: ln.Addr().String(),
		SNIMap:     sniMap,
		ProxyAddr:  "127.0.0.1:0", // won't connect; that's fine
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = listener.ListenAndServeOnListener(ctx, ln)
	}()
	time.Sleep(50 * time.Millisecond)

	// Send 50 concurrent TLS connections.
	const N = 50
	var wg sync.WaitGroup
	var dialFailures int64
	var mu sync.Mutex
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			if !sendTLSClientHello(t, ln.Addr().String(), "api.openai.com") {
				mu.Lock()
				dialFailures++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Give listener time to process.
	time.Sleep(200 * time.Millisecond)

	intercepted, passthrough, errors := listener.Stats().Snapshot()
	total := intercepted + passthrough + errors

	// Connections that failed to dial (e.g. "too many open files" in CI)
	// never reach the listener, so subtract them from the expected count.
	expected := int64(N) - dialFailures
	if total != expected {
		t.Errorf("total stats (%d intercepted + %d passthrough + %d errors = %d) != %d expected (%d sent - %d dial failures)",
			intercepted, passthrough, errors, total, expected, int64(N), dialFailures)
	}
}

func TestStatsServeHTTP(t *testing.T) {
	// Set up a listener to get a Stats instance with known values.
	providers := []proxy.Provider{
		{Name: "test", UpstreamURL: "https://test.example.com"},
	}
	sniMap := proxy.BuildSNIMap(providers)
	listener := transparent.NewListener(transparent.Config{
		ListenAddr: "127.0.0.1:0",
		SNIMap:     sniMap,
		ProxyAddr:  "127.0.0.1:8080",
	})

	// Hit the stats HTTP handler.
	rec := httptest.NewRecorder()
	listener.Stats().ServeHTTP(rec, httptest.NewRequest("GET", "/debug/transparent/stats", nil))

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]int64
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v — body: %s", err, rec.Body.String())
	}

	// Fresh listener — all counters should be 0.
	for _, key := range []string{"intercepted", "passthrough", "errors"} {
		if body[key] != 0 {
			t.Errorf("%s = %d, want 0", key, body[key])
		}
	}
}

func TestStatsLogStats(t *testing.T) {
	// Smoke test: LogStats should not panic on a fresh stats instance.
	providers := []proxy.Provider{
		{Name: "test", UpstreamURL: "https://test.example.com"},
	}
	sniMap := proxy.BuildSNIMap(providers)
	listener := transparent.NewListener(transparent.Config{
		ListenAddr: "127.0.0.1:0",
		SNIMap:     sniMap,
		ProxyAddr:  "127.0.0.1:8080",
	})

	// Should not panic.
	listener.Stats().LogStats()
}
