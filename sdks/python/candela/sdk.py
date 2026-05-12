"""Core enrichment SDK logic — zero external dependencies."""

from __future__ import annotations

import re
from typing import Dict, Optional

# Same pattern as the Go server-side validation.
_ID_PATTERN = re.compile(r"^[a-zA-Z0-9\-._]{1,128}$")


def validate_id(value: str, name: str = "id") -> str:
    """Validate a tenant or job ID against the allowed pattern.

    Raises ValueError if the ID is invalid.
    """
    if not _ID_PATTERN.match(value):
        raise ValueError(
            f"candela: invalid {name} {value!r} — "
            "must be 1-128 chars of [a-zA-Z0-9._-]"
        )
    return value


def inject_headers(
    headers: Dict[str, str],
    *,
    tenant_id: Optional[str] = None,
    job_id: Optional[str] = None,
) -> Dict[str, str]:
    """Add Candela enrichment headers to a dict, returning the mutated dict.

    Injects both W3C Baggage entries and explicit X-Candela-* fallback headers.
    Existing Baggage entries are preserved.
    """
    parts: list[str] = []

    if tenant_id:
        validate_id(tenant_id, "tenant_id")
        parts.append(f"candela.tenant_id={tenant_id}")
        headers["X-Candela-Tenant-Id"] = tenant_id

    if job_id:
        validate_id(job_id, "job_id")
        parts.append(f"candela.job_id={job_id}")
        headers["X-Candela-Job-Id"] = job_id

    if parts:
        existing = headers.get("Baggage", "")
        baggage = ",".join(parts)
        if existing:
            baggage = f"{existing},{baggage}"
        headers["Baggage"] = baggage

    return headers


class CandelaSession:
    """Reusable session that generates enrichment headers for all requests.

    Args:
        tenant_id: Tenant identifier for cost attribution.
        job_id: Job/experiment identifier for cost attribution.
    """

    def __init__(
        self,
        *,
        tenant_id: Optional[str] = None,
        job_id: Optional[str] = None,
    ) -> None:
        if tenant_id:
            validate_id(tenant_id, "tenant_id")
        if job_id:
            validate_id(job_id, "job_id")
        self._tenant_id = tenant_id
        self._job_id = job_id

    def headers(self) -> Dict[str, str]:
        """Return a fresh dict of enrichment headers."""
        return inject_headers(
            {}, tenant_id=self._tenant_id, job_id=self._job_id
        )

    def httpx_client(self, **kwargs):
        """Create an httpx.Client pre-configured with enrichment headers.

        Requires httpx to be installed (optional dependency).
        """
        import httpx  # noqa: delay import — httpx is optional

        existing = dict(kwargs.pop("headers", {}) or {})
        merged = inject_headers(
            existing, tenant_id=self._tenant_id, job_id=self._job_id
        )
        return httpx.Client(headers=merged, **kwargs)
