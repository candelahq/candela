"""Candela Enrichment SDK for Python.

Lightweight, zero-dependency middleware for propagating tenant and job
metadata to Candela AI observability proxies via W3C Baggage headers.

Usage with OpenAI::

    import openai
    from candela import CandelaSession

    session = CandelaSession(tenant_id="acme-corp", job_id="training-v3")
    client = openai.OpenAI(
        http_client=session.httpx_client(),  # httpx
    )

Usage with requests::

    import requests
    from candela import CandelaSession

    session = CandelaSession(tenant_id="acme-corp", job_id="eval-42")
    resp = requests.post(url, headers=session.headers())

Usage with httpx (manual)::

    import httpx
    from candela import CandelaSession

    session = CandelaSession(tenant_id="acme-corp")
    client = httpx.Client(headers=session.headers())
"""

from candela.sdk import CandelaSession, inject_headers, validate_id

__all__ = ["CandelaSession", "inject_headers", "validate_id"]
__version__ = "0.1.0"
