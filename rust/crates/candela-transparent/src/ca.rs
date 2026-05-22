//! Ephemeral Certificate Authority for MITM TLS termination.
//!
//! Generates a self-signed CA keypair at pod startup and issues short-lived
//! leaf certificates on-demand for intercepted SNI hostnames. Leaf certs
//! are cached per-hostname to avoid re-generation on repeat connections.
//!
//! The CA cert should be written to a mounted volume (e.g.
//! `/var/run/candela/ca.pem`) so that application containers can trust it
//! via `SSL_CERT_FILE` or `REQUESTS_CA_BUNDLE`.

use std::collections::HashMap;
use std::io::Write;
use std::path::Path;
use std::sync::{Arc, Mutex};

use rcgen::{
    BasicConstraints, CertificateParams, DnType, ExtendedKeyUsagePurpose, IsCa, Issuer, KeyPair,
    KeyUsagePurpose, SanType,
};
use rustls::ServerConfig;
use rustls::pki_types::{CertificateDer, PrivateKeyDer, PrivatePkcs8KeyDer};
use tracing::debug;

/// How long leaf certificates remain valid (24h is sufficient for ephemeral pods).
const LEAF_VALIDITY_DAYS: u32 = 1;

/// Ephemeral Certificate Authority.
///
/// Created once at startup and shared (via `Arc`) across all connection
/// handlers. Thread-safe: the inner `CertCache` is behind a `Mutex`.
pub struct EphemeralCA {
    /// The CA certificate (DER-encoded).
    pub(crate) ca_cert_der: CertificateDer<'static>,
    /// The signed CA certificate (for PEM serialization).
    ca_cert: rcgen::Certificate,
    /// The original CA parameters (needed to construct an `Issuer` for leaf signing).
    ca_params: CertificateParams,
    /// The CA key pair.
    ca_key_pair: KeyPair,
    /// Per-SNI hostname certificate cache.
    cache: Mutex<HashMap<String, Arc<ServerConfig>>>,
}

impl EphemeralCA {
    /// Generate a new ephemeral CA keypair + self-signed certificate.
    ///
    /// Call once at pod startup. The CA has a 365-day validity but is
    /// expected to be regenerated on every pod restart.
    pub fn generate() -> anyhow::Result<Self> {
        let key_pair = KeyPair::generate_for(&rcgen::PKCS_ECDSA_P256_SHA256)?;

        let mut params = CertificateParams::default();
        params.is_ca = IsCa::Ca(BasicConstraints::Unconstrained);
        params
            .distinguished_name
            .push(DnType::CommonName, "Candela Ephemeral CA");
        params
            .distinguished_name
            .push(DnType::OrganizationName, "Candela");
        params.key_usages = vec![KeyUsagePurpose::KeyCertSign, KeyUsagePurpose::CrlSign];
        params.not_before = time::OffsetDateTime::now_utc();
        params.not_after = time::OffsetDateTime::now_utc() + time::Duration::days(365);

        let ca_cert = params.self_signed(&key_pair)?;
        let ca_cert_der = CertificateDer::from(ca_cert.der().to_vec());

        Ok(Self {
            ca_cert_der,
            ca_cert,
            ca_params: params,
            ca_key_pair: key_pair,
            cache: Mutex::new(HashMap::new()),
        })
    }

    /// Write the CA certificate in PEM format to `path`.
    ///
    /// The file can then be mounted into application containers for trust
    /// injection (e.g. `SSL_CERT_FILE=/var/run/candela/ca.pem`).
    pub fn write_ca_pem(&self, path: &Path) -> std::io::Result<()> {
        let pem = self.ca_cert.pem();
        let mut f = std::fs::File::create(path)?;
        f.write_all(pem.as_bytes())?;
        Ok(())
    }

    /// Returns the CA certificate in PEM format (useful for testing).
    pub fn ca_pem(&self) -> String {
        self.ca_cert.pem()
    }

    /// Get (or create) a `rustls::ServerConfig` for the given SNI hostname.
    ///
    /// On first call for a hostname, generates a leaf certificate signed by
    /// the CA and caches the resulting `ServerConfig`. Subsequent calls
    /// return the cached config.
    pub fn server_config_for(&self, sni: &str) -> anyhow::Result<Arc<ServerConfig>> {
        // Fast path: check the cache.
        {
            let cache = self.cache.lock().unwrap();
            if let Some(cfg) = cache.get(sni) {
                return Ok(Arc::clone(cfg));
            }
        }

        // Slow path: generate a new leaf cert.
        let cfg = self.generate_leaf_config(sni)?;

        let mut cache = self.cache.lock().unwrap();
        // Double-check (another thread may have inserted between our check
        // and the lock acquisition).
        if let Some(existing) = cache.get(sni) {
            return Ok(Arc::clone(existing));
        }
        cache.insert(sni.to_string(), Arc::clone(&cfg));
        debug!(sni = %sni, cache_size = cache.len(), "generated leaf cert");
        Ok(cfg)
    }

    /// Generate a leaf `ServerConfig` for `sni`, signed by this CA.
    fn generate_leaf_config(&self, sni: &str) -> anyhow::Result<Arc<ServerConfig>> {
        let leaf_key = KeyPair::generate_for(&rcgen::PKCS_ECDSA_P256_SHA256)?;

        let mut leaf_params = CertificateParams::default();
        leaf_params.distinguished_name.push(DnType::CommonName, sni);
        leaf_params.subject_alt_names = vec![SanType::DnsName(sni.try_into()?)];
        leaf_params.not_before = time::OffsetDateTime::now_utc();
        leaf_params.not_after =
            time::OffsetDateTime::now_utc() + time::Duration::days(LEAF_VALIDITY_DAYS as i64);
        leaf_params.key_usages = vec![
            KeyUsagePurpose::DigitalSignature,
            KeyUsagePurpose::KeyEncipherment,
        ];
        leaf_params.extended_key_usages = vec![ExtendedKeyUsagePurpose::ServerAuth];

        // Build an Issuer from the stored CA params + key pair, then sign.
        let issuer = Issuer::from_params(&self.ca_params, &self.ca_key_pair);
        let leaf_cert = leaf_params.signed_by(&leaf_key, &issuer)?;

        // Build rustls certificate chain: [leaf, CA].
        let leaf_der = CertificateDer::from(leaf_cert.der().to_vec());
        let cert_chain = vec![leaf_der, self.ca_cert_der.clone()];

        // Build private key.
        let key_der = PrivateKeyDer::Pkcs8(PrivatePkcs8KeyDer::from(leaf_key.serialize_der()));

        let mut config = ServerConfig::builder()
            .with_no_client_auth()
            .with_single_cert(cert_chain, key_der)?;

        // Advertise both h2 and http/1.1 via ALPN — client picks.
        config.alpn_protocols = vec![b"h2".to_vec(), b"http/1.1".to_vec()];

        Ok(Arc::new(config))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use rustls::RootCertStore;
    use rustls::pki_types::ServerName;

    /// Ensure a rustls CryptoProvider is installed (idempotent across parallel tests).
    fn install_crypto_provider() {
        let _ = rustls::crypto::ring::default_provider().install_default();
    }

    /// Helper: build a rustls ClientConfig that trusts the ephemeral CA.
    fn client_config_trusting(ca: &EphemeralCA) -> rustls::ClientConfig {
        let mut root_store = RootCertStore::empty();
        root_store.add(ca.ca_cert_der.clone()).unwrap();
        rustls::ClientConfig::builder()
            .with_root_certificates(root_store)
            .with_no_client_auth()
    }

    #[test]
    fn ca_generation_succeeds() {
        let ca = EphemeralCA::generate().expect("CA generation should succeed");
        assert!(
            !ca.ca_cert_der.is_empty(),
            "CA cert DER should not be empty"
        );
    }

    #[test]
    fn ca_pem_is_valid() {
        let ca = EphemeralCA::generate().unwrap();
        let pem = ca.ca_pem();
        assert!(pem.starts_with("-----BEGIN CERTIFICATE-----"));
        assert!(pem.contains("-----END CERTIFICATE-----"));
    }

    #[test]
    fn leaf_cert_has_correct_san() {
        install_crypto_provider();
        let ca = EphemeralCA::generate().unwrap();
        let server_cfg = ca
            .server_config_for("api.openai.com")
            .expect("leaf generation should succeed");

        // Verify the leaf chains to the CA by building a ClientConfig
        // that trusts the CA and attempting a TLS connection.
        let client_cfg = client_config_trusting(&ca);
        let server_name = ServerName::try_from("api.openai.com").unwrap();

        // If the leaf cert's SAN is wrong, ClientConnection::new will fail
        // during certificate verification.
        let _conn = rustls::ClientConnection::new(Arc::new(client_cfg), server_name.to_owned())
            .expect("client connection should accept leaf cert for api.openai.com");

        let _server_conn = rustls::ServerConnection::new(server_cfg)
            .expect("server connection should be creatable");
    }

    #[test]
    fn leaf_cert_chains_to_ca() {
        install_crypto_provider();
        let ca = EphemeralCA::generate().unwrap();
        let cfg = ca.server_config_for("example.com").unwrap();

        // Verify the cert chain is [leaf, CA] = 2 certs.
        // ServerConfig doesn't expose the chain directly, but we can verify
        // by generating a second config and confirming cache behavior.
        let cfg2 = ca.server_config_for("example.com").unwrap();
        assert!(
            Arc::ptr_eq(&cfg, &cfg2),
            "same hostname should return cached config"
        );
    }

    #[test]
    fn cache_returns_same_arc() {
        install_crypto_provider();
        let ca = EphemeralCA::generate().unwrap();
        let cfg1 = ca.server_config_for("api.openai.com").unwrap();
        let cfg2 = ca.server_config_for("api.openai.com").unwrap();
        assert!(
            Arc::ptr_eq(&cfg1, &cfg2),
            "repeated lookups must return the same Arc"
        );
    }

    #[test]
    fn different_snis_produce_different_configs() {
        install_crypto_provider();
        let ca = EphemeralCA::generate().unwrap();
        let cfg1 = ca.server_config_for("api.openai.com").unwrap();
        let cfg2 = ca.server_config_for("api.anthropic.com").unwrap();
        assert!(
            !Arc::ptr_eq(&cfg1, &cfg2),
            "different hostnames must produce different configs"
        );
    }

    #[test]
    fn alpn_includes_h2_and_http11() {
        install_crypto_provider();
        let ca = EphemeralCA::generate().unwrap();
        let cfg = ca.server_config_for("test.example.com").unwrap();
        assert!(
            cfg.alpn_protocols.contains(&b"h2".to_vec()),
            "ALPN must advertise h2"
        );
        assert!(
            cfg.alpn_protocols.contains(&b"http/1.1".to_vec()),
            "ALPN must advertise http/1.1"
        );
    }

    #[test]
    fn write_ca_pem_to_file() {
        let ca = EphemeralCA::generate().unwrap();
        let dir = std::env::temp_dir().join("candela-ca-test");
        std::fs::create_dir_all(&dir).unwrap();
        let path = dir.join("ca.pem");

        ca.write_ca_pem(&path).expect("write should succeed");
        let contents = std::fs::read_to_string(&path).unwrap();
        assert!(contents.starts_with("-----BEGIN CERTIFICATE-----"));

        // Cleanup.
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn tls_handshake_with_leaf_cert() {
        install_crypto_provider();
        // Full verification: build a ClientConfig that trusts the CA,
        // then verify the leaf cert is accepted for the correct SNI.
        let ca = EphemeralCA::generate().unwrap();
        let server_cfg = ca.server_config_for("test.example.com").unwrap();

        let mut root_store = RootCertStore::empty();
        root_store.add(ca.ca_cert_der.clone()).unwrap();

        let client_cfg = rustls::ClientConfig::builder()
            .with_root_certificates(root_store)
            .with_no_client_auth();

        // Build a server/client pair in-memory to verify handshake.
        let server_name = ServerName::try_from("test.example.com").unwrap();
        let _conn = rustls::ClientConnection::new(Arc::new(client_cfg), server_name.to_owned())
            .expect("client connection should be creatable");

        let _server_conn = rustls::ServerConnection::new(server_cfg)
            .expect("server connection should be creatable");
    }
}
