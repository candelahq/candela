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

	t.Run("oversized record length", func(t *testing.T) {
		// TLS record header claims 65535 bytes but only 2 bytes follow.
		// Parser should handle gracefully (truncated = best effort).
		_, err := ParseClientHelloSNI([]byte{0x16, 0x03, 0x01, 0xff, 0xff, 0x01, 0x00})
		if err == nil {
			t.Error("expected error for oversized record with insufficient data")
		}
	})

	t.Run("handshake but not ClientHello", func(t *testing.T) {
		// Handshake type 0x02 = ServerHello, not ClientHello.
		data := []byte{
			0x16, 0x03, 0x01, 0x00, 0x05, // TLS record: Handshake, 5 bytes
			0x02, 0x00, 0x00, 0x01, 0x00, // ServerHello handshake
		}
		_, err := ParseClientHelloSNI(data)
		if err == nil {
			t.Error("expected error for ServerHello, got nil")
		}
	})

	t.Run("valid handshake with no extensions", func(t *testing.T) {
		// Minimal ClientHello with no extensions — should return ErrNoSNI.
		clientHello := buildMinimalClientHello(nil)
		_, err := ParseClientHelloSNI(clientHello)
		if err != ErrNoSNI {
			t.Errorf("expected ErrNoSNI for extensionless ClientHello, got %v", err)
		}
	})

	t.Run("SNI with zero-length hostname", func(t *testing.T) {
		// SNI extension present but hostname length is 0.
		sniExt := []byte{
			0x00, 0x00, // extension type: SNI
			0x00, 0x05, // extension data length: 5
			0x00, 0x03, // server name list length: 3
			0x00,       // name type: host_name
			0x00, 0x00, // name length: 0
		}
		clientHello := buildMinimalClientHello(sniExt)
		sni, err := ParseClientHelloSNI(clientHello)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sni != "" {
			t.Errorf("expected empty SNI for zero-length hostname, got %q", sni)
		}
	})

	t.Run("long SNI hostname", func(t *testing.T) {
		// 253-character hostname (DNS max).
		longHost := ""
		for i := 0; i < 25; i++ {
			if i > 0 {
				longHost += "."
			}
			longHost += "abcdefghij"
		}
		clientHello := captureClientHello(t, longHost)
		sni, err := ParseClientHelloSNI(clientHello)
		if err != nil {
			t.Fatalf("ParseClientHelloSNI() error = %v", err)
		}
		if sni != longHost {
			t.Errorf("SNI = %q, want %q", sni, longHost)
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

// buildMinimalClientHello creates a synthetic TLS ClientHello with the given
// extensions block. Used for testing edge cases that can't be triggered
// with Go's standard TLS client.
func buildMinimalClientHello(extensions []byte) []byte {
	// ClientHello body:
	//   client_version (2) + random (32) + session_id_len (1) +
	//   cipher_suites_len (2) + one suite (2) + compression_len (1) + null (1)
	bodyLen := 2 + 32 + 1 + 2 + 2 + 1 + 1
	if len(extensions) > 0 {
		bodyLen += 2 + len(extensions) // extensions length prefix + data
	}

	body := make([]byte, 0, bodyLen)
	body = append(body, 0x03, 0x03)          // TLS 1.2
	body = append(body, make([]byte, 32)...) // random
	body = append(body, 0x00)                // session ID length: 0
	body = append(body, 0x00, 0x02)          // cipher suites length: 2
	body = append(body, 0x00, 0x2f)          // TLS_RSA_WITH_AES_128_CBC_SHA
	body = append(body, 0x01, 0x00)          // compression: 1 method, null

	if len(extensions) > 0 {
		extLen := len(extensions)
		body = append(body, byte(extLen>>8), byte(extLen)) // extensions length
		body = append(body, extensions...)
	}

	// Handshake header.
	handshake := make([]byte, 0, 4+len(body))
	handshake = append(handshake, 0x01) // ClientHello
	handshakeLen := len(body)
	handshake = append(handshake, byte(handshakeLen>>16), byte(handshakeLen>>8), byte(handshakeLen))
	handshake = append(handshake, body...)

	// TLS record header.
	record := make([]byte, 0, 5+len(handshake))
	record = append(record, 0x16)       // Handshake
	record = append(record, 0x03, 0x01) // TLS 1.0
	recordLen := len(handshake)
	record = append(record, byte(recordLen>>8), byte(recordLen))
	record = append(record, handshake...)

	return record
}
