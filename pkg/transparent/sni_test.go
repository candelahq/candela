package transparent

import (
	"crypto/tls"
	"net"
	"testing"
)

func TestParseClientHelloSNI(t *testing.T) {
	t.Run("real TLS ClientHello", func(t *testing.T) {
		// Generate a real ClientHello by dialing ourselves with a known SNI.
		clientHello := captureClientHello(t, "api.openai.com")
		sni, err := ParseClientHelloSNI(clientHello)
		if err != nil {
			t.Fatalf("ParseClientHelloSNI() error = %v", err)
		}
		if sni != "api.openai.com" {
			t.Errorf("SNI = %q, want %q", sni, "api.openai.com")
		}
	})

	t.Run("googleapis SNI", func(t *testing.T) {
		clientHello := captureClientHello(t, "generativelanguage.googleapis.com")
		sni, err := ParseClientHelloSNI(clientHello)
		if err != nil {
			t.Fatalf("ParseClientHelloSNI() error = %v", err)
		}
		if sni != "generativelanguage.googleapis.com" {
			t.Errorf("SNI = %q, want %q", sni, "generativelanguage.googleapis.com")
		}
	})

	t.Run("anthropic SNI", func(t *testing.T) {
		clientHello := captureClientHello(t, "api.anthropic.com")
		sni, err := ParseClientHelloSNI(clientHello)
		if err != nil {
			t.Fatalf("ParseClientHelloSNI() error = %v", err)
		}
		if sni != "api.anthropic.com" {
			t.Errorf("SNI = %q, want %q", sni, "api.anthropic.com")
		}
	})

	t.Run("not TLS", func(t *testing.T) {
		_, err := ParseClientHelloSNI([]byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))
		if err != ErrNotTLS {
			t.Errorf("expected ErrNotTLS, got %v", err)
		}
	})

	t.Run("too short", func(t *testing.T) {
		_, err := ParseClientHelloSNI([]byte{0x16, 0x03})
		if err != ErrNotTLS {
			t.Errorf("expected ErrNotTLS, got %v", err)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		_, err := ParseClientHelloSNI(nil)
		if err != ErrNotTLS {
			t.Errorf("expected ErrNotTLS, got %v", err)
		}
	})

	t.Run("wrong record type", func(t *testing.T) {
		// Alert record (0x15) instead of Handshake (0x16)
		_, err := ParseClientHelloSNI([]byte{0x15, 0x03, 0x01, 0x00, 0x02, 0x01, 0x00})
		if err != ErrNotTLS {
			t.Errorf("expected ErrNotTLS, got %v", err)
		}
	})
}

// captureClientHello generates a real TLS ClientHello by creating a local
// TCP listener and having a TLS client connect to it with the specified SNI.
// Returns the raw bytes of the ClientHello message.
func captureClientHello(t *testing.T, serverName string) []byte {
	t.Helper()

	// Start a TCP listener.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	// Channel to receive the ClientHello bytes.
	ch := make(chan []byte, 1)
	errCh := make(chan error, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer func() { _ = conn.Close() }()

		// Read up to 4KB — more than enough for a ClientHello.
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			errCh <- err
			return
		}
		ch <- buf[:n]
	}()

	// Connect with TLS and the specified SNI.
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Initiate TLS handshake (will fail because server isn't TLS, but we
	// only need the ClientHello to be sent).
	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: true, //nolint:gosec // test only
	})
	go func() {
		_ = tlsConn.Handshake() // will fail — that's fine
	}()

	select {
	case data := <-ch:
		return data
	case err := <-errCh:
		t.Fatalf("server error: %v", err)
		return nil
	}
}
