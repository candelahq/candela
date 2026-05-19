// Package transparent implements a transparent proxy listener for intercepting
// iptables-redirected TLS connections. It reads the TLS ClientHello to extract
// the SNI hostname, routes LLM traffic through the Candela proxy pipeline, and
// passes non-LLM traffic through unchanged.
//
// This package is used in sidecar mode when TRANSPARENT_PORT is set, alongside
// an iptables init container that redirects outbound port 443 traffic.
package transparent

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// TLS record types.
const (
	recordTypeHandshake = 0x16
	handshakeTypeClient = 0x01
)

// ErrNotTLS is returned when the peeked bytes are not a TLS handshake.
var ErrNotTLS = errors.New("not a TLS handshake")

// ErrNoSNI is returned when the ClientHello does not contain an SNI extension.
var ErrNoSNI = errors.New("no SNI extension found")

// ParseClientHelloSNI extracts the Server Name Indication (SNI) hostname
// from a raw TLS ClientHello message. The input should be the first bytes
// peeked from a TCP connection (typically 1024–4096 bytes is sufficient).
//
// Returns the SNI hostname or an error if the data is not a valid TLS
// ClientHello or does not contain an SNI extension.
func ParseClientHelloSNI(data []byte) (string, error) {
	// Minimum TLS record header: 5 bytes (type + version + length).
	if len(data) < 5 {
		return "", ErrNotTLS
	}

	// Check TLS record type: must be Handshake (0x16).
	if data[0] != recordTypeHandshake {
		return "", ErrNotTLS
	}

	// TLS record version (we accept any).
	// data[1:3] = version

	// TLS record length.
	recordLen := int(binary.BigEndian.Uint16(data[3:5]))
	recordData := data[5:]
	if len(recordData) < recordLen {
		// Truncated record — work with what we have.
	} else {
		recordData = recordData[:recordLen]
	}

	return parseHandshakeClientHello(recordData)
}

// parseHandshakeClientHello parses the Handshake layer to find the ClientHello
// and extract its SNI extension.
func parseHandshakeClientHello(data []byte) (string, error) {
	if len(data) < 4 {
		return "", ErrNotTLS
	}

	// Handshake type: must be ClientHello (0x01).
	if data[0] != handshakeTypeClient {
		return "", fmt.Errorf("handshake type %d, want ClientHello (1)", data[0])
	}

	// Handshake length (3 bytes, big-endian).
	handshakeLen := int(data[1])<<16 | int(data[2])<<8 | int(data[3])
	body := data[4:]
	if len(body) < handshakeLen {
		// Truncated — work with what we have (best effort).
	} else {
		body = body[:handshakeLen]
	}

	return parseClientHelloBody(body)
}

// parseClientHelloBody parses the ClientHello message body to find the SNI extension.
func parseClientHelloBody(data []byte) (string, error) {
	r := bytes.NewReader(data)

	// ClientVersion (2 bytes).
	if _, err := skip(r, 2); err != nil {
		return "", ErrNotTLS
	}

	// Random (32 bytes).
	if _, err := skip(r, 32); err != nil {
		return "", ErrNotTLS
	}

	// Session ID (variable length, 1 byte length prefix).
	sessionIDLen, err := readUint8(r)
	if err != nil {
		return "", ErrNotTLS
	}
	if _, err := skip(r, int(sessionIDLen)); err != nil {
		return "", ErrNotTLS
	}

	// Cipher Suites (variable length, 2 byte length prefix).
	cipherSuitesLen, err := readUint16(r)
	if err != nil {
		return "", ErrNotTLS
	}
	if _, err := skip(r, int(cipherSuitesLen)); err != nil {
		return "", ErrNotTLS
	}

	// Compression Methods (variable length, 1 byte length prefix).
	compressionLen, err := readUint8(r)
	if err != nil {
		return "", ErrNotTLS
	}
	if _, err := skip(r, int(compressionLen)); err != nil {
		return "", ErrNotTLS
	}

	// Extensions (variable length, 2 byte length prefix).
	if r.Len() < 2 {
		return "", ErrNoSNI // no extensions at all
	}
	extensionsLen, err := readUint16(r)
	if err != nil {
		return "", ErrNoSNI
	}

	// Parse extensions looking for SNI (type 0x0000).
	// SECURITY: cap allocation to actual available data to prevent OOM from
	// a malicious ClientHello with an inflated extensionsLen field.
	allocLen := int(extensionsLen)
	if allocLen > r.Len() {
		allocLen = r.Len()
	}
	extensionData := make([]byte, allocLen)
	if _, err := r.Read(extensionData); err != nil {
		return "", ErrNoSNI
	}

	return findSNIExtension(extensionData)
}

// findSNIExtension scans the extensions block for the SNI extension (type 0x0000)
// and returns the hostname.
func findSNIExtension(data []byte) (string, error) {
	for len(data) >= 4 {
		extType := binary.BigEndian.Uint16(data[0:2])
		extLen := int(binary.BigEndian.Uint16(data[2:4]))
		data = data[4:]

		if len(data) < extLen {
			break
		}

		if extType == 0x0000 { // SNI extension
			return parseSNIExtensionData(data[:extLen])
		}

		data = data[extLen:]
	}

	return "", ErrNoSNI
}

// parseSNIExtensionData parses the SNI extension payload and returns the hostname.
func parseSNIExtensionData(data []byte) (string, error) {
	if len(data) < 2 {
		return "", ErrNoSNI
	}

	// Server Name List length.
	listLen := int(binary.BigEndian.Uint16(data[0:2]))
	data = data[2:]
	if len(data) < listLen {
		return "", ErrNoSNI
	}
	data = data[:listLen]

	// Parse Server Name entries.
	for len(data) >= 3 {
		nameType := data[0]
		nameLen := int(binary.BigEndian.Uint16(data[1:3]))
		data = data[3:]

		if len(data) < nameLen {
			break
		}

		if nameType == 0x00 { // host_name type
			return string(data[:nameLen]), nil
		}

		data = data[nameLen:]
	}

	return "", ErrNoSNI
}

// Helper functions for reading binary data.

func readUint8(r *bytes.Reader) (uint8, error) {
	b, err := r.ReadByte()
	return b, err
}

func readUint16(r *bytes.Reader) (uint16, error) {
	var buf [2]byte
	if _, err := r.Read(buf[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(buf[:]), nil
}

func skip(r *bytes.Reader, n int) (int, error) {
	if n < 0 || int64(n) > int64(r.Len()) {
		return 0, fmt.Errorf("skip %d bytes: only %d available", n, r.Len())
	}
	_, err := r.Seek(int64(n), 1) // io.SeekCurrent = 1
	return n, err
}
