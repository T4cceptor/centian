#!/usr/bin/env python3
"""
Demo processor: emit MCP tool-call spans via OpenTelemetry OTLP.

This script is intentionally minimal and best-effort for demo purposes.
If OTEL setup or export fails, traffic passes through unchanged.
"""

import json
import os
import sys
from typing import Any, Dict

OTEL_READY = True
OTEL_IMPORT_ERROR = None

try:
    from opentelemetry import trace
    from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
    from opentelemetry.sdk.resources import Resource
    from opentelemetry.sdk.trace import TracerProvider
    from opentelemetry.sdk.trace.export import BatchSpanProcessor
except Exception as exc:  # pragma: no cover - runtime environment dependent
    OTEL_READY = False
    OTEL_IMPORT_ERROR = str(exc)


def build_tracer():
    if not OTEL_READY:
        return None

    endpoint = os.getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://localhost:4318/v1/traces")
    service_name = os.getenv("OTEL_SERVICE_NAME", "centian-demo")

    provider = TracerProvider(resource=Resource.create({"service.name": service_name}))
    exporter = OTLPSpanExporter(endpoint=endpoint)
    provider.add_span_processor(BatchSpanProcessor(exporter))
    trace.set_tracer_provider(provider)
    return trace.get_tracer("centian.demo.processor.otel_span_logger")


TRACER = build_tracer()


def get_request_params(ctx: Dict[str, Any]) -> Dict[str, Any]:
    payload = ctx.get("payload") or {}
    request = payload.get("request") or {}
    return request.get("Params") or request.get("params") or {}


def get_arg_count(params: Dict[str, Any]) -> int:
    arguments = params.get("arguments")
    if isinstance(arguments, dict):
        return len(arguments)
    if isinstance(arguments, list):
        return len(arguments)
    if arguments is None:
        return 0
    return 1


def process(ctx: Dict[str, Any]) -> Dict[str, Any]:
    if TRACER is None:
        if OTEL_IMPORT_ERROR:
            print(f"[otel_span_logger] OTEL unavailable: {OTEL_IMPORT_ERROR}", file=sys.stderr)
        return ctx

    try:
        params = get_request_params(ctx)
        routing = ctx.get("routing") or {}
        event = ctx.get("event") or {}

        tool_name = params.get("name") or routing.get("tool_name") or "unknown"
        server_name = routing.get("server_name") or "unknown"
        status = int(event.get("status", 200))

        with TRACER.start_as_current_span("centian.mcp.tool_call") as span:
            span.set_attribute("centian.tool_name", tool_name)
            span.set_attribute("centian.server_name", server_name)
            span.set_attribute("centian.direction", str(event.get("direction", "unknown")))
            span.set_attribute("centian.status", status)
            span.set_attribute("centian.arguments_count", get_arg_count(params))

        provider = trace.get_tracer_provider()
        if hasattr(provider, "force_flush"):
            provider.force_flush()
    except Exception as exc:  # pragma: no cover - demo safety net
        print(f"[otel_span_logger] export failed: {exc}", file=sys.stderr)

    return ctx


def main():
    try:
        input_data = json.load(sys.stdin)
        print(json.dumps(process(input_data)))
        sys.exit(0)
    except Exception as exc:
        print(json.dumps({"event": {"status": 500, "error": str(exc), "success": False}}))
        sys.exit(0)


if __name__ == "__main__":
    main()
