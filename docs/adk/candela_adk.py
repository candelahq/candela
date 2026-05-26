"""Candela ADK Integration — Tenant-Aware Observability Plugin.

Provides automatic propagation of tenant context (and optional job context)
through Google ADK agent traces via W3C Baggage. This ensures all LLM calls
made by an agent — including sub-agents and tool calls — are attributed to
the correct tenant and job for cost tracking in Candela.

Usage:
    from candela.adk import CandleaContextPlugin, CandelaContext

    context = CandelaContext(
        tenant_id="acme-corp",
        job_id="trial-NCT01750580",   # optional: maps to trial_id, campaign_id, etc.
        session_id="session-42",       # optional: groups related calls
    )
    plugin = CandleaContextPlugin(context)

    result = await agent.run_async(message, plugins=[plugin])

Installation:
    uv add opentelemetry-api opentelemetry-sdk

The plugin uses the standard OTel propagation mechanism — no Candela-specific
SDK is required. Any OTel-compatible propagator (W3C TraceContext + Baggage)
will correctly forward these headers.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from typing import Any

# OTel imports — required for Baggage propagation.
try:
    from opentelemetry import baggage, context
    from opentelemetry.propagate import inject

    _OTEL_AVAILABLE = True
except ImportError:
    _OTEL_AVAILABLE = False


# ── Validation ────────────────────────────────────────────────────────────────

# Must match the proxy-side tenantIDPattern in pkg/proxy/proxy.go
_TENANT_ID_PATTERN = re.compile(r"^[a-zA-Z0-9\-._]{1,128}$")

# job_id follows the same rules (will be added to the proxy in a future PR)
_JOB_ID_PATTERN = re.compile(r"^[a-zA-Z0-9\-._]{1,128}$")


def _validate_tenant_id(value: str | None, field_name: str = "tenant_id") -> str | None:
    """Validate a tenant/job ID against the allowed pattern.

    Returns the value unchanged if valid, or None (with a warning) if invalid.
    Never raises — invalid IDs are silently discarded, matching proxy behavior.
    """
    if value is None:
        return None
    if not _TENANT_ID_PATTERN.match(value):
        import warnings

        warnings.warn(
            f"candela: invalid {field_name}={value!r} — must match "
            f"[a-zA-Z0-9\\-._]{{1,128}}. Value will be ignored.",
            stacklevel=3,
        )
        return None
    return value


# ── Context Dataclass ─────────────────────────────────────────────────────────


@dataclass(frozen=True)
class CandelaContext:
    """Immutable attribution context for Candela observability.

    All fields are optional except that at least one should be set for
    cost attribution to be meaningful.

    Attributes:
        tenant_id: The downstream customer/tenant to attribute costs to.
            Maps to W3C Baggage key ``candela.tenant_id`` and the
            ``X-Candela-Tenant-Id`` header. Validated against
            ``[a-zA-Z0-9\\-._]{1,128}``.

        job_id: An optional job/trial/campaign identifier for second-dimension
            attribution. Useful when a single tenant runs many distinct jobs
            (e.g., clinical trial NCT IDs, campaign IDs, batch run IDs).
            Maps to W3C Baggage key ``candela.job_id``.
            **Note:** ``job_id`` storage support is planned — it is forwarded
            in Baggage today but not yet persisted as a first-class column.

        session_id: Optional session identifier for grouping related calls
            within a single user session or conversation. Maps to
            ``X-Session-Id``.

    Example:
        >>> ctx = CandelaContext(
        ...     tenant_id="acme-corp",
        ...     job_id="trial-NCT01750580",
        ...     session_id="conversation-abc123",
        ... )
        >>> ctx.to_baggage_headers()
        {'Baggage': 'candela.tenant_id=acme-corp,candela.job_id=trial-NCT01750580'}
    """

    tenant_id: str | None = None
    job_id: str | None = None
    session_id: str | None = None

    def __post_init__(self) -> None:
        # Validate at construction time so errors surface early.
        object.__setattr__(self, "tenant_id", _validate_tenant_id(self.tenant_id, "tenant_id"))
        object.__setattr__(self, "job_id", _validate_tenant_id(self.job_id, "job_id"))

    def to_baggage_dict(self) -> dict[str, str]:
        """Return the Candela-specific baggage entries as a plain dict.

        Keys are the W3C Baggage key names; values are the IDs.
        Only non-None, valid entries are included.
        """
        result: dict[str, str] = {}
        if self.tenant_id:
            result["candela.tenant_id"] = self.tenant_id
        if self.job_id:
            result["candela.job_id"] = self.job_id
        return result

    def to_baggage_headers(self) -> dict[str, str]:
        """Build HTTP headers dict with W3C Baggage for manual injection.

        Use this when you cannot use the OTel propagator directly (e.g., raw
        httpx/requests calls):

            headers = ctx.to_baggage_headers()
            resp = httpx.post(url, headers={**base_headers, **headers}, json=body)
        """
        if not _OTEL_AVAILABLE:
            # Fallback: build the Baggage header manually.
            entries = self.to_baggage_dict()
            if not entries:
                return {}
            header_value = ",".join(f"{k}={v}" for k, v in entries.items())
            return {"Baggage": header_value}

        entries = self.to_baggage_dict()
        if not entries:
            return {}

        ctx = context.get_current()
        for key, value in entries.items():
            ctx = baggage.set_baggage(key, value, context=ctx)

        headers: dict[str, str] = {}
        inject(headers, context=ctx)
        return headers

    def to_explicit_headers(self) -> dict[str, str]:
        """Build explicit Candela headers (non-OTel fallback).

        Use this for simple HTTP clients or test scripts that don't need
        OTel trace propagation but want tenant attribution:

            headers = ctx.to_explicit_headers()
            # {"X-Candela-Tenant-Id": "acme-corp"}
        """
        result: dict[str, str] = {}
        if self.tenant_id:
            result["X-Candela-Tenant-Id"] = self.tenant_id
        if self.session_id:
            result["X-Session-Id"] = self.session_id
        return result


# ── ADK Plugin ────────────────────────────────────────────────────────────────


class CandleaContextPlugin:
    """ADK plugin that injects Candela tenant context into every LLM call.

    Usage with Google ADK:

        from google.adk.agents import LlmAgent
        from candela.adk import CandleaContextPlugin, CandelaContext

        ctx = CandelaContext(tenant_id="acme-corp", job_id="trial-NCT01750580")
        plugin = CandleaContextPlugin(ctx)

        result = await agent.run_async(
            "Match this patient for NCT01750580",
            plugins=[plugin],
        )

    The plugin hooks into ADK's ``before_model_call`` lifecycle to inject
    W3C Baggage headers before each LLM HTTP request. This ensures:

    1. The Candela proxy captures tenant_id on every call.
    2. Baggage propagates through multi-hop agent traces (sub-agents, tool calls).
    3. No code changes are needed in agent tools or sub-agents.

    If ``opentelemetry-api`` is not installed, the plugin falls back to
    injecting the ``X-Candela-Tenant-Id`` explicit header instead.
    """

    def __init__(self, candela_context: CandelaContext) -> None:
        self._ctx = candela_context

    @property
    def name(self) -> str:
        return "CandleaContextPlugin"

    def before_model_call(self, request: Any) -> Any:
        """Inject Candela baggage headers before each LLM HTTP request.

        ADK calls this hook with the model request object. We inject headers
        into the request so the proxy can capture tenant attribution.

        Args:
            request: The ADK model request object (provider-specific).

        Returns:
            The modified request with injected headers.
        """
        headers_to_inject = self._ctx.to_baggage_headers()
        if not headers_to_inject and self._ctx.tenant_id:
            # OTel not available — fall back to explicit header.
            headers_to_inject = self._ctx.to_explicit_headers()

        if headers_to_inject:
            _inject_headers(request, headers_to_inject)

        return request


def _inject_headers(request: Any, headers: dict[str, str]) -> None:
    """Best-effort header injection into an ADK model request.

    ADK request objects vary by provider. We try the most common patterns.
    """
    # Pattern 1: request.headers is a dict-like object.
    if hasattr(request, "headers") and isinstance(request.headers, dict):
        request.headers.update(headers)
        return

    # Pattern 2: request has a config with extra_headers (Gemini SDK pattern).
    if hasattr(request, "config") and hasattr(request.config, "extra_headers"):
        existing = request.config.extra_headers or {}
        request.config.extra_headers = {**existing, **headers}
        return

    # Pattern 3: request.kwargs (httpx-style).
    if hasattr(request, "kwargs"):
        existing = request.kwargs.get("headers", {})
        request.kwargs["headers"] = {**existing, **headers}
        return

    # No-op: we couldn't find a headers attachment point.
    # The request proceeds without tenant context; this is non-fatal.
    import warnings

    warnings.warn(
        f"CandleaContextPlugin: could not inject headers into {type(request).__name__}. "
        "Tenant attribution may be missing for this call.",
        stacklevel=2,
    )


# ── Convenience Factory ───────────────────────────────────────────────────────


def make_candela_plugin(
    tenant_id: str | None = None,
    job_id: str | None = None,
    session_id: str | None = None,
) -> CandleaContextPlugin:
    """Convenience factory for creating a CandleaContextPlugin.

    Example:
        plugin = make_candela_plugin(tenant_id="acme-corp", job_id="NCT01750580")
        result = await agent.run_async(message, plugins=[plugin])
    """
    return CandleaContextPlugin(
        CandelaContext(tenant_id=tenant_id, job_id=job_id, session_id=session_id)
    )


# ── CTMS Adapter ─────────────────────────────────────────────────────────────


@dataclass(frozen=True)
class CTMSCandelaContext(CandelaContext):
    """Candela context specialized for CTMS / clinical trial workloads.

    Maps CTMS domain concepts to Candela attribution fields:
    - tenant_id → the CTMS tenant (healthcare organization)
    - job_id    → the trial NCT ID or campaign ID being processed

    Example (matching the FunnelRequest pattern in intelligence-service):
        ctx = CTMSCandelaContext.from_funnel_request(
            tenant_id=funnel_request.tenant_id,
            trial_id=funnel_request.trial_id,
            run_id=funnel_request.run_id,
        )
        plugin = CandleaContextPlugin(ctx)
    """

    trial_id: str | None = field(default=None)
    run_id: str | None = field(default=None)

    @classmethod
    def from_funnel_request(
        cls,
        tenant_id: str,
        trial_id: str | None = None,
        run_id: str | None = None,
        session_id: str | None = None,
    ) -> "CTMSCandelaContext":
        """Build a CTMSCandelaContext from FunnelRequest fields.

        The trial_id is used as the job_id for Candela cost attribution, so you
        can query "how much did this trial run cost?" via GetTenantLeaderboard
        once job_id is fully supported.

        Args:
            tenant_id: CTMS tenant identifier (e.g. "acme-health").
            trial_id: NCT ID or internal trial identifier (e.g. "NCT01750580").
            run_id: Optional run/campaign identifier for sub-attribution.
            session_id: Optional session ID for grouping calls.
        """
        return cls(
            tenant_id=tenant_id,
            job_id=trial_id,        # trial_id → job_id for Candela
            session_id=session_id,
            trial_id=trial_id,
            run_id=run_id,
        )
