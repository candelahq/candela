"""ADK agent with Candela tracing — proxy + OTel unified traces.

Routes LLM calls through candela-sidecar for cost/token monitoring,
and exports OTel spans to Candela's collector for full agent DAG
visibility. Trace context is propagated automatically via traceparent
headers, creating a unified trace tree.

Usage:
    # Quick start (proxy only — no OTel):
    CANDELA_SIDECAR_URL=http://localhost:8080/proxy/google adk web .

    # Full observability (proxy + OTel):
    ./launch.sh

See docs/adk-integration.md for the complete guide.
"""

import os

# OTel environment — ADK reads these natively.
# Set BEFORE importing ADK so that maybe_set_otel_providers() picks them up.
os.environ.setdefault(
    "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://localhost:4318/v1/traces"
)
os.environ.setdefault("OTEL_SERVICE_NAME", "adk-candela-demo")
os.environ.setdefault("OTEL_SEMCONV_STABILITY_OPT_IN", "gen_ai_latest_experimental")
os.environ.setdefault(
    "OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT", "EVENT_ONLY"
)

from google.adk.agents import Agent  # noqa: E402
from google.adk.models import Gemini  # noqa: E402

SIDECAR = os.environ.get(
    "CANDELA_SIDECAR_URL", "http://localhost:8080/proxy/google"
)

root_agent = Agent(
    model=Gemini(
        model="gemini-2.0-flash",
        base_url=SIDECAR,
    ),
    name="candela_demo_agent",
    instruction="You are a helpful assistant.",
)
