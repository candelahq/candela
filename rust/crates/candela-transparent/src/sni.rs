//! TLS ClientHello SNI parser.
//!
//! Extracts the Server Name Indication (SNI) hostname from a raw TLS
//! ClientHello message. The input should be the first bytes peeked from
//! a TCP connection (typically 1024–16384 bytes is sufficient).
//!
//! Ported from: `pkg/transparent/sni.go`

use std::fmt;

/// TLS record types.
const RECORD_TYPE_HANDSHAKE: u8 = 0x16;
const HANDSHAKE_TYPE_CLIENT_HELLO: u8 = 0x01;

/// SNI extension type.
const SNI_EXTENSION_TYPE: u16 = 0x0000;

/// SNI name type for hostnames.
const SNI_NAME_TYPE_HOSTNAME: u8 = 0x00;

/// Errors that can occur during SNI parsing.
#[derive(Debug, PartialEq, Eq)]
pub enum SniError {
    /// The data is not a TLS handshake.
    NotTls,
    /// The ClientHello does not contain an SNI extension.
    NoSni,
    /// The handshake type is not ClientHello.
    WrongHandshakeType(u8),
}

impl fmt::Display for SniError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            SniError::NotTls => write!(f, "not a TLS handshake"),
            SniError::NoSni => write!(f, "no SNI extension found"),
            SniError::WrongHandshakeType(t) => {
                write!(f, "handshake type {t}, want ClientHello (1)")
            }
        }
    }
}

impl std::error::Error for SniError {}

/// Extracts the SNI hostname from a raw TLS ClientHello message.
///
/// Returns the SNI hostname or an error if the data is not a valid TLS
/// ClientHello or does not contain an SNI extension.
pub fn parse_client_hello_sni(data: &[u8]) -> Result<String, SniError> {
    // Minimum TLS record header: 5 bytes (type + version + length).
    if data.len() < 5 {
        return Err(SniError::NotTls);
    }

    // Check TLS record type: must be Handshake (0x16).
    if data[0] != RECORD_TYPE_HANDSHAKE {
        return Err(SniError::NotTls);
    }

    // TLS record length (bytes 3..5).
    let record_len = u16::from_be_bytes([data[3], data[4]]) as usize;
    let record_data = &data[5..];

    // Work with what we have if truncated.
    let record_data = if record_data.len() < record_len {
        record_data
    } else {
        &record_data[..record_len]
    };

    parse_handshake_client_hello(record_data)
}

/// Parses the Handshake layer to find the ClientHello and extract its SNI.
fn parse_handshake_client_hello(data: &[u8]) -> Result<String, SniError> {
    if data.len() < 4 {
        return Err(SniError::NotTls);
    }

    // Handshake type: must be ClientHello (0x01).
    if data[0] != HANDSHAKE_TYPE_CLIENT_HELLO {
        return Err(SniError::WrongHandshakeType(data[0]));
    }

    // Handshake length (3 bytes, big-endian).
    let handshake_len = (data[1] as usize) << 16 | (data[2] as usize) << 8 | (data[3] as usize);
    let body = &data[4..];

    // Work with what we have if truncated.
    let body = if body.len() < handshake_len {
        body
    } else {
        &body[..handshake_len]
    };

    parse_client_hello_body(body)
}

/// Parses the ClientHello message body to find the SNI extension.
fn parse_client_hello_body(data: &[u8]) -> Result<String, SniError> {
    let mut pos: usize = 0;

    // ClientVersion (2 bytes).
    pos = skip(pos, 2, data.len())?;

    // Random (32 bytes).
    pos = skip(pos, 32, data.len())?;

    // Session ID (variable length, 1 byte length prefix).
    if pos >= data.len() {
        return Err(SniError::NotTls);
    }
    let session_id_len = data[pos] as usize;
    pos += 1;
    pos = skip(pos, session_id_len, data.len())?;

    // Cipher Suites (variable length, 2 byte length prefix).
    if pos + 2 > data.len() {
        return Err(SniError::NotTls);
    }
    let cipher_suites_len = u16::from_be_bytes([data[pos], data[pos + 1]]) as usize;
    pos += 2;
    pos = skip(pos, cipher_suites_len, data.len())?;

    // Compression Methods (variable length, 1 byte length prefix).
    if pos >= data.len() {
        return Err(SniError::NotTls);
    }
    let compression_len = data[pos] as usize;
    pos += 1;
    pos = skip(pos, compression_len, data.len())?;

    // Extensions (variable length, 2 byte length prefix).
    if pos + 2 > data.len() {
        return Err(SniError::NoSni); // no extensions at all
    }
    let extensions_len = u16::from_be_bytes([data[pos], data[pos + 1]]) as usize;
    pos += 2;

    // SECURITY: cap to actual available data to prevent OOM from a
    // malicious ClientHello with an inflated extensions_len field.
    let available = data.len() - pos;
    let ext_len = extensions_len.min(available);
    let extension_data = &data[pos..pos + ext_len];

    find_sni_extension(extension_data)
}

/// Scans the extensions block for the SNI extension (type 0x0000).
fn find_sni_extension(mut data: &[u8]) -> Result<String, SniError> {
    while data.len() >= 4 {
        let ext_type = u16::from_be_bytes([data[0], data[1]]);
        let ext_len = u16::from_be_bytes([data[2], data[3]]) as usize;
        data = &data[4..];

        if data.len() < ext_len {
            break;
        }

        if ext_type == SNI_EXTENSION_TYPE {
            return parse_sni_extension_data(&data[..ext_len]);
        }

        data = &data[ext_len..];
    }

    Err(SniError::NoSni)
}

/// Parses the SNI extension payload and returns the hostname.
fn parse_sni_extension_data(data: &[u8]) -> Result<String, SniError> {
    if data.len() < 2 {
        return Err(SniError::NoSni);
    }

    // Server Name List length.
    let list_len = u16::from_be_bytes([data[0], data[1]]) as usize;
    let mut data = &data[2..];
    if data.len() < list_len {
        return Err(SniError::NoSni);
    }
    data = &data[..list_len];

    // Parse Server Name entries.
    while data.len() >= 3 {
        let name_type = data[0];
        let name_len = u16::from_be_bytes([data[1], data[2]]) as usize;
        data = &data[3..];

        if data.len() < name_len {
            break;
        }

        if name_type == SNI_NAME_TYPE_HOSTNAME {
            return String::from_utf8(data[..name_len].to_vec()).map_err(|_| SniError::NoSni);
        }

        data = &data[name_len..];
    }

    Err(SniError::NoSni)
}

/// Advances position by `n` bytes, returning the new position or an error.
fn skip(pos: usize, n: usize, len: usize) -> Result<usize, SniError> {
    let new_pos = pos + n;
    if new_pos > len {
        return Err(SniError::NotTls);
    }
    Ok(new_pos)
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Builds a minimal TLS 1.2 ClientHello with the given SNI hostname.
    fn build_client_hello(server_name: &str) -> Vec<u8> {
        // SNI extension payload.
        let name_bytes = server_name.as_bytes();
        let sni_entry_len = 1 + 2 + name_bytes.len(); // type(1) + len(2) + name
        let sni_list_len = sni_entry_len;
        let sni_ext_data_len = 2 + sni_list_len; // list_len(2) + list

        let mut sni_ext = Vec::new();
        // Extension type: SNI (0x0000).
        sni_ext.extend_from_slice(&0u16.to_be_bytes());
        // Extension data length.
        sni_ext.extend_from_slice(&(sni_ext_data_len as u16).to_be_bytes());
        // Server Name List length.
        sni_ext.extend_from_slice(&(sni_list_len as u16).to_be_bytes());
        // Name type: host_name (0x00).
        sni_ext.push(0x00);
        // Name length.
        sni_ext.extend_from_slice(&(name_bytes.len() as u16).to_be_bytes());
        // Name.
        sni_ext.extend_from_slice(name_bytes);

        // ClientHello body.
        let mut body = Vec::new();
        // Client version: TLS 1.2 (0x0303).
        body.extend_from_slice(&[0x03, 0x03]);
        // Random (32 bytes of zeros).
        body.extend_from_slice(&[0u8; 32]);
        // Session ID length: 0.
        body.push(0x00);
        // Cipher suites: 1 suite (2 bytes length + 2 bytes suite).
        body.extend_from_slice(&2u16.to_be_bytes());
        body.extend_from_slice(&[0x00, 0x2f]); // TLS_RSA_WITH_AES_128_CBC_SHA
        // Compression methods: 1 method (1 byte length + 1 byte method).
        body.push(0x01);
        body.push(0x00); // null compression
        // Extensions length.
        body.extend_from_slice(&(sni_ext.len() as u16).to_be_bytes());
        // Extensions.
        body.extend_from_slice(&sni_ext);

        // Handshake header.
        let mut handshake = Vec::new();
        // Handshake type: ClientHello (0x01).
        handshake.push(0x01);
        // Handshake length (3 bytes).
        let body_len = body.len();
        handshake.push((body_len >> 16) as u8);
        handshake.push((body_len >> 8) as u8);
        handshake.push(body_len as u8);
        handshake.extend_from_slice(&body);

        // TLS record header.
        let mut record = Vec::new();
        // Record type: Handshake (0x16).
        record.push(0x16);
        // Version: TLS 1.0 (0x0301) — record layer version.
        record.extend_from_slice(&[0x03, 0x01]);
        // Record length.
        record.extend_from_slice(&(handshake.len() as u16).to_be_bytes());
        record.extend_from_slice(&handshake);

        record
    }

    #[test]
    fn parse_valid_sni() {
        let data = build_client_hello("api.openai.com");
        let sni = parse_client_hello_sni(&data).unwrap();
        assert_eq!(sni, "api.openai.com");
    }

    #[test]
    fn parse_long_hostname() {
        let hostname = "us-central1-aiplatform.googleapis.com";
        let data = build_client_hello(hostname);
        let sni = parse_client_hello_sni(&data).unwrap();
        assert_eq!(sni, hostname);
    }

    #[test]
    fn parse_too_short() {
        assert_eq!(parse_client_hello_sni(&[0x16, 0x03]), Err(SniError::NotTls));
    }

    #[test]
    fn parse_not_tls() {
        // HTTP request instead of TLS.
        let data = b"GET / HTTP/1.1\r\nHost: example.com\r\n\r\n";
        assert_eq!(parse_client_hello_sni(data), Err(SniError::NotTls));
    }

    #[test]
    fn parse_empty() {
        assert_eq!(parse_client_hello_sni(&[]), Err(SniError::NotTls));
    }

    #[test]
    fn parse_wrong_handshake_type() {
        let mut data = build_client_hello("example.com");
        // Change handshake type from ClientHello (0x01) to ServerHello (0x02).
        data[5] = 0x02;
        assert_eq!(
            parse_client_hello_sni(&data),
            Err(SniError::WrongHandshakeType(0x02))
        );
    }

    #[test]
    fn parse_no_extensions() {
        // Build a ClientHello with no extensions.
        let mut body = Vec::new();
        body.extend_from_slice(&[0x03, 0x03]); // version
        body.extend_from_slice(&[0u8; 32]); // random
        body.push(0x00); // session ID len
        body.extend_from_slice(&2u16.to_be_bytes()); // cipher suites len
        body.extend_from_slice(&[0x00, 0x2f]); // cipher suite
        body.push(0x01); // compression len
        body.push(0x00); // null compression
        // No extensions.

        let mut handshake = vec![0x01];
        let body_len = body.len();
        handshake.push((body_len >> 16) as u8);
        handshake.push((body_len >> 8) as u8);
        handshake.push(body_len as u8);
        handshake.extend_from_slice(&body);

        let mut record = vec![0x16, 0x03, 0x01];
        record.extend_from_slice(&(handshake.len() as u16).to_be_bytes());
        record.extend_from_slice(&handshake);

        assert_eq!(parse_client_hello_sni(&record), Err(SniError::NoSni));
    }

    #[test]
    fn parse_truncated_record_still_works() {
        let mut data = build_client_hello("api.anthropic.com");
        // Inflate the TLS record length by 10 so parser thinks there
        // should be more data, but still parses what we have.
        let rec_len = u16::from_be_bytes([data[3], data[4]]);
        let inflated = (rec_len + 10).to_be_bytes();
        data[3] = inflated[0];
        data[4] = inflated[1];
        let sni = parse_client_hello_sni(&data).unwrap();
        assert_eq!(sni, "api.anthropic.com");
    }

    #[test]
    fn error_display() {
        assert_eq!(format!("{}", SniError::NotTls), "not a TLS handshake");
        assert_eq!(format!("{}", SniError::NoSni), "no SNI extension found");
        assert_eq!(
            format!("{}", SniError::WrongHandshakeType(2)),
            "handshake type 2, want ClientHello (1)"
        );
    }

    #[test]
    fn parse_wildcard_subdomain() {
        let hostname = "deep.sub.api.anthropic.com";
        let data = build_client_hello(hostname);
        let sni = parse_client_hello_sni(&data).unwrap();
        assert_eq!(sni, hostname);
    }

    #[test]
    fn parse_single_byte_host() {
        // Edge case: single-character hostname.
        let data = build_client_hello("x");
        let sni = parse_client_hello_sni(&data).unwrap();
        assert_eq!(sni, "x");
    }

    #[test]
    fn parse_max_label_hostname() {
        // DNS label max is 63 chars; a max-ish hostname.
        let long_label = "a".repeat(63);
        let hostname = format!("{long_label}.example.com");
        let data = build_client_hello(&hostname);
        let sni = parse_client_hello_sni(&data).unwrap();
        assert_eq!(sni, hostname);
    }
}
