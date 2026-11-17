//! Cache-token parity with the Go proxy, driven by the shared fixture.
//!
//! `test/fixtures/cache_tokens.json` is the single source of truth for what a
//! provider's cache numbers are. The Go proxy reads the same file, so a
//! provider measured differently by the two binaries fails one of them.

use std::collections::BTreeSet;
use std::path::PathBuf;

use candela_proxy::parsers::{
    extract_cache_tokens, extract_streaming_cache_tokens, parser_family, registered_providers,
};
use serde::Deserialize;

#[derive(Deserialize)]
struct Fixture {
    cases: Vec<Case>,
}

#[derive(Deserialize)]
struct Case {
    provider: String,
    standard_body: serde_json::Value,
    streaming_body: String,
    cache_read_tokens: i64,
    cache_creation_tokens: i64,
}

fn fixture() -> Fixture {
    let path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../../test/fixtures/cache_tokens.json");
    let data = std::fs::read(&path)
        .unwrap_or_else(|e| panic!("read {}: {e}", path.display()));
    let fixture: Fixture = serde_json::from_slice(&data)
        .unwrap_or_else(|e| panic!("parse {}: {e}", path.display()));
    assert!(!fixture.cases.is_empty(), "{} has no cases", path.display());
    fixture
}

#[test]
fn standard_bodies_match_fixture() {
    for case in fixture().cases {
        let body = serde_json::to_vec(&case.standard_body).unwrap();
        let cache = extract_cache_tokens(&case.provider, &body);
        assert_eq!(
            (cache.cache_read_tokens, cache.cache_creation_tokens),
            (case.cache_read_tokens, case.cache_creation_tokens),
            "{} standard body",
            case.provider
        );
    }
}

#[test]
fn streaming_bodies_match_fixture() {
    for case in fixture().cases {
        let cache = extract_streaming_cache_tokens(&case.provider, case.streaming_body.as_bytes());
        assert_eq!(
            (cache.cache_read_tokens, cache.cache_creation_tokens),
            (case.cache_read_tokens, case.cache_creation_tokens),
            "{} streaming body",
            case.provider
        );
    }
}

#[test]
fn registry_covers_fixture_providers_exactly() {
    let mut seen = BTreeSet::new();
    for case in fixture().cases {
        assert!(
            parser_family(&case.provider).is_some(),
            "provider {} has a fixture case but is not registered",
            case.provider
        );
        assert!(
            seen.insert(case.provider.clone()),
            "provider {} appears twice in the fixture",
            case.provider
        );
    }
    for provider in registered_providers() {
        assert!(
            seen.contains(provider),
            "provider {provider} is registered but has no fixture case"
        );
    }
}
