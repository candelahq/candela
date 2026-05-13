"""Tests for the Candela enrichment SDK."""

import pytest

from candela import CandelaSession, inject_headers, validate_id


class TestValidateId:
    def test_valid_ids(self):
        assert validate_id("acme-corp") == "acme-corp"
        assert validate_id("run_42") == "run_42"
        assert validate_id("a.b.c") == "a.b.c"
        assert validate_id("A" * 128) == "A" * 128

    def test_invalid_empty(self):
        with pytest.raises(ValueError, match="invalid"):
            validate_id("")

    def test_invalid_spaces(self):
        with pytest.raises(ValueError, match="invalid"):
            validate_id("has spaces")

    def test_invalid_too_long(self):
        with pytest.raises(ValueError, match="invalid"):
            validate_id("A" * 129)


class TestInjectHeaders:
    def test_both_ids(self):
        h = inject_headers({}, tenant_id="acme", job_id="run-1")
        assert h["X-Candela-Tenant-Id"] == "acme"
        assert h["X-Candela-Job-Id"] == "run-1"
        assert "candela.tenant_id=acme" in h["Baggage"]
        assert "candela.job_id=run-1" in h["Baggage"]

    def test_preserves_existing_baggage(self):
        h = inject_headers({"Baggage": "foo=bar"}, tenant_id="t1")
        assert h["Baggage"].startswith("foo=bar,")
        assert "candela.tenant_id=t1" in h["Baggage"]

    def test_no_ids_noop(self):
        h = inject_headers({})
        assert "Baggage" not in h
        assert "X-Candela-Tenant-Id" not in h

    def test_invalid_id_raises(self):
        with pytest.raises(ValueError):
            inject_headers({}, tenant_id="bad id!")


class TestCandelaSession:
    def test_headers(self):
        s = CandelaSession(tenant_id="acme", job_id="j1")
        h = s.headers()
        assert h["X-Candela-Tenant-Id"] == "acme"
        assert h["X-Candela-Job-Id"] == "j1"
        assert "candela.tenant_id=acme" in h["Baggage"]

    def test_invalid_on_init(self):
        with pytest.raises(ValueError):
            CandelaSession(tenant_id="bad spaces")

    def test_headers_are_fresh(self):
        s = CandelaSession(tenant_id="t1")
        h1 = s.headers()
        h2 = s.headers()
        assert h1 is not h2
        assert h1 == h2
