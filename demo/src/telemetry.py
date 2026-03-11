#!/usr/bin/env python3
"""
Shared OpenTelemetry helpers for the Centian PoC processors.
"""

import os
from pathlib import Path
import sys
from typing import Any, Dict, Optional, Tuple

try:
    from shared_event_model import CallToolParamsRaw, DataContext
except ImportError:  # pragma: no cover - package import fallback
    from .shared_event_model import CallToolParamsRaw, DataContext


def load_local_env_file() -> None:
    candidate_paths = (
        Path(__file__).with_name(".env.local"),
        Path(__file__).resolve().parent.parent / ".env.local",
    )

    for env_path in candidate_paths:
        if not env_path.exists():
            continue

        try:
            for raw_line in env_path.read_text(encoding="utf-8").splitlines():
                line = raw_line.strip()
                if not line or line.startswith("#") or "=" not in line:
                    continue
                key, value = line.split("=", 1)
                key = key.strip()
                if key:
                    os.environ.setdefault(key, value.strip())
            return
        except Exception:
            continue


def apply_local_otel_env_defaults() -> None:
    os.environ.setdefault("OTEL_SERVICE_NAME", "centian-demo")
    os.environ.setdefault("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
    os.environ.setdefault("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://localhost:4318/v1/traces")
    os.environ.setdefault("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "http://localhost:4318/v1/metrics")

    resource_attributes = os.environ.get("OTEL_RESOURCE_ATTRIBUTES", "").strip()
    service_name = os.environ["OTEL_SERVICE_NAME"]
    if "service.name=" not in resource_attributes:
        if resource_attributes:
            resource_attributes = f"{resource_attributes},service.name={service_name}"
        else:
            resource_attributes = f"service.name={service_name}"
        os.environ["OTEL_RESOURCE_ATTRIBUTES"] = resource_attributes


def get_trace_endpoint() -> str:
    traces_endpoint = os.getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
    base_endpoint = os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT")

    if traces_endpoint:
        return traces_endpoint
    if base_endpoint:
        return f"{base_endpoint.rstrip('/')}/v1/traces"
    return "http://localhost:4318/v1/traces"


def get_metrics_endpoint() -> str:
    metrics_endpoint = os.getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT")
    base_endpoint = os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT")

    if metrics_endpoint:
        return metrics_endpoint
    if base_endpoint:
        return f"{base_endpoint.rstrip('/')}/v1/metrics"
    return "http://localhost:4318/v1/metrics"


def get_export_timeout_seconds() -> float:
    raw_timeout = os.getenv("OTEL_EXPORTER_OTLP_TIMEOUT", "").strip()
    if not raw_timeout:
        return 2.0

    try:
        parsed = float(raw_timeout)
    except ValueError:
        return 2.0

    if parsed <= 0:
        return 2.0
    return parsed


def should_force_flush() -> bool:
    return os.getenv("CENTIAN_OTEL_FORCE_FLUSH", "").strip().lower() in {"1", "true", "yes", "on"}


def get_force_flush_timeout_millis() -> int:
    raw_timeout = os.getenv("CENTIAN_OTEL_FORCE_FLUSH_TIMEOUT_MS", "").strip()
    if not raw_timeout:
        return 750

    try:
        parsed = int(raw_timeout)
    except ValueError:
        return 750

    if parsed <= 0:
        return 750
    return parsed


load_local_env_file()
apply_local_otel_env_defaults()

OTEL_TRACE_READY = True
OTEL_TRACE_IMPORT_ERROR = None
OTEL_METRICS_READY = True
OTEL_METRICS_IMPORT_ERROR = None

try:
    from opentelemetry import trace
    from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
    from opentelemetry.sdk.resources import Resource
    from opentelemetry.sdk.trace import TracerProvider
    from opentelemetry.sdk.trace.export import BatchSpanProcessor
    from opentelemetry.sdk.trace.export import ConsoleSpanExporter
    from opentelemetry.sdk.trace.export import SimpleSpanProcessor
except Exception as exc:  # pragma: no cover - runtime environment dependent
    OTEL_TRACE_READY = False
    OTEL_TRACE_IMPORT_ERROR = str(exc)

try:
    from opentelemetry import metrics
    from opentelemetry.exporter.otlp.proto.http.metric_exporter import OTLPMetricExporter
    from opentelemetry.sdk.metrics import MeterProvider
    from opentelemetry.sdk.metrics.export import PeriodicExportingMetricReader
except Exception as exc:  # pragma: no cover - runtime environment dependent
    OTEL_METRICS_READY = False
    OTEL_METRICS_IMPORT_ERROR = str(exc)


def parse_key_value_csv(raw: str) -> Dict[str, str]:
    pairs: Dict[str, str] = {}
    if not raw:
        return pairs

    for chunk in raw.split(","):
        item = chunk.strip()
        if not item or "=" not in item:
            continue
        key, value = item.split("=", 1)
        key = key.strip()
        value = value.strip()
        if key:
            pairs[key] = value

    return pairs


def build_resource(service_name: str) -> "Resource":
    attributes = parse_key_value_csv(os.getenv("OTEL_RESOURCE_ATTRIBUTES", ""))
    attributes.setdefault("service.name", service_name)
    return Resource.create(attributes)


def build_tracer():
    if not OTEL_TRACE_READY:
        return None

    service_name = os.getenv("OTEL_SERVICE_NAME", "centian-demo")
    otel_headers = parse_key_value_csv(os.getenv("OTEL_EXPORTER_OTLP_HEADERS", ""))
    traces_exporter_mode = os.getenv("OTEL_TRACES_EXPORTER", "").strip().lower()
    exporters = {item.strip() for item in traces_exporter_mode.split(",") if item.strip()}
    span_processor_mode = os.getenv("CENTIAN_OTEL_TRACE_PROCESSOR", "simple").strip().lower()

    provider = TracerProvider(resource=build_resource(service_name))
    use_otlp = not exporters or "otlp" in exporters
    use_console = "console" in exporters

    if use_otlp:
        exporter = OTLPSpanExporter(
            endpoint=get_trace_endpoint(),
            headers=otel_headers or None,
            timeout=get_export_timeout_seconds(),
        )
        if span_processor_mode == "batch":
            provider.add_span_processor(BatchSpanProcessor(exporter))
        else:
            provider.add_span_processor(SimpleSpanProcessor(exporter))

    if use_console:
        provider.add_span_processor(SimpleSpanProcessor(ConsoleSpanExporter(out=sys.stderr)))

    trace.set_tracer_provider(provider)
    return trace.get_tracer("centian.demo.processor.telemetry")


TRACER = build_tracer()


def build_meter_provider():
    if not OTEL_METRICS_READY:
        return None

    service_name = os.getenv("OTEL_SERVICE_NAME", "centian-demo")
    otel_headers = parse_key_value_csv(os.getenv("OTEL_EXPORTER_OTLP_HEADERS", ""))
    metrics_exporter_mode = os.getenv("OTEL_METRICS_EXPORTER", "").strip().lower()
    exporters = {item.strip() for item in metrics_exporter_mode.split(",") if item.strip()}
    use_otlp = "otlp" in exporters

    readers = []
    if use_otlp:
        exporter = OTLPMetricExporter(
            endpoint=get_metrics_endpoint(),
            headers=otel_headers or None,
            timeout=get_export_timeout_seconds(),
        )
        readers.append(PeriodicExportingMetricReader(exporter, export_interval_millis=5000))

    if not readers:
        return None

    provider = MeterProvider(resource=build_resource(service_name), metric_readers=readers)
    metrics.set_meter_provider(provider)
    return provider


METER_PROVIDER = build_meter_provider()
METER = metrics.get_meter("centian.demo.processor.telemetry") if OTEL_METRICS_READY and METER_PROVIDER else None
DECISION_COUNTER = (
    METER.create_counter(
        "centian_requests_total",
        description="Total processed requests by governance decision",
        unit="1",
    )
    if METER is not None
    else None
)


def get_request_params(ctx: DataContext) -> CallToolParamsRaw:
    payload = ctx.payload
    if payload and payload.request and payload.request.params:
        return payload.request.params
    return CallToolParamsRaw()


def get_tool_name(ctx: DataContext) -> str:
    if ctx.routing and ctx.routing.tool_name:
        return ctx.routing.tool_name

    params = get_request_params(ctx)
    if params.name:
        return params.name

    return "unknown"


def get_server_name(ctx: DataContext) -> str:
    if ctx.routing and ctx.routing.server_name:
        return ctx.routing.server_name

    tool_name = get_tool_name(ctx)
    if "___" in tool_name:
        return tool_name.split("___", 1)[0]

    return "unknown"


def get_arg_count(arguments: Any) -> int:
    if isinstance(arguments, dict):
        return len(arguments)
    if isinstance(arguments, list):
        return len(arguments)
    if arguments is None:
        return 0
    return 1


def get_argument_keys_summary(arguments: Any) -> str:
    if isinstance(arguments, dict):
        keys = sorted(str(key) for key in arguments.keys())
        return ",".join(keys)[:200]
    if isinstance(arguments, list):
        return "list"
    if arguments is None:
        return "none"
    return "scalar"


def get_decision_meta(ctx: DataContext) -> Dict[str, Any]:
    payload = ctx.payload
    candidates = []

    if payload and payload.request and payload.request.params and isinstance(payload.request.params.meta, dict):
        candidates.append(payload.request.params.meta)

    if payload and payload.result and isinstance(payload.result.meta, dict):
        candidates.append(payload.result.meta)

    for meta in candidates:
        centian_meta = meta.get("centian")
        if isinstance(centian_meta, dict):
            return centian_meta

    return {}


def get_original_request_params(ctx: DataContext) -> CallToolParamsRaw:
    payload = ctx.payload
    if payload and payload.original_request and payload.original_request.params:
        return payload.original_request.params
    return CallToolParamsRaw()


def get_result_summary(ctx: DataContext) -> str:
    payload = ctx.payload
    if payload and payload.result is not None:
        return str(payload.result)
    return ""


def get_is_error(ctx: DataContext) -> bool:
    payload = ctx.payload
    if payload and payload.result is not None:
        return bool(payload.result.is_error)
    return False


def infer_decision(ctx: DataContext, decision_meta: Dict[str, Any]) -> Tuple[str, Optional[str], Optional[str]]:
    decision = decision_meta.get("decision")
    reason = decision_meta.get("reason")
    risk_level = decision_meta.get("risk_level")

    if decision:
        return str(decision), (str(reason) if reason else None), (str(risk_level) if risk_level else None)

    if ctx.event and ctx.event.get("message_type") == "request":
        payload = ctx.payload
        if payload and payload.result:
            return "deny", (str(reason) if reason else None), (str(risk_level) if risk_level else None)

        current_params = get_request_params(ctx)
        original_params = get_original_request_params(ctx)
        same_name = current_params.name == original_params.name
        same_args = str(current_params.arguments) == str(original_params.arguments)
        if not same_name or not same_args:
            return "modify", (str(reason) if reason else None), (str(risk_level) if risk_level else None)

    return "allow", (str(reason) if reason else None), (str(risk_level) if risk_level else None)


def emit_decision_metric(metric_attributes: Dict[str, Any]) -> None:
    if DECISION_COUNTER is None:
        if OTEL_METRICS_IMPORT_ERROR:
            print(f"[telemetry] metrics unavailable: {OTEL_METRICS_IMPORT_ERROR}", file=sys.stderr)
        return

    try:
        DECISION_COUNTER.add(1, metric_attributes)
        if should_force_flush() and METER_PROVIDER is not None and hasattr(METER_PROVIDER, "force_flush"):
            flush_timeout = get_force_flush_timeout_millis()
            try:
                METER_PROVIDER.force_flush(timeout_millis=flush_timeout)
            except TypeError:
                METER_PROVIDER.force_flush()
    except Exception as exc:  # pragma: no cover - demo safety net
        print(f"[telemetry] metric export failed: {exc}", file=sys.stderr)


def build_metric_attributes(ctx: DataContext, decision: str) -> Dict[str, Any]:
    params = get_request_params(ctx)
    event = ctx.event or {}

    return {
        "decision": decision,
        "tool_name": get_tool_name(ctx),
        "message_type": str(event.get("message_type", "unknown")),
        "direction": str(event.get("direction", "unknown")),
        "is_error": str(get_is_error(ctx)).lower(),
        "arguments_keys": get_argument_keys_summary(params.arguments),
        "arguments_count": get_arg_count(params.arguments),
    }


def emit_tool_call_span(ctx: DataContext) -> None:
    params = get_request_params(ctx)
    original_params = get_original_request_params(ctx)
    event = ctx.event or {}
    decision_meta = get_decision_meta(ctx)
    decision, reason, risk_level = infer_decision(ctx, decision_meta)
    is_error = get_is_error(ctx)

    emit_decision_metric(build_metric_attributes(ctx, decision))

    if TRACER is None:
        if OTEL_TRACE_IMPORT_ERROR:
            print(f"[telemetry] OTEL unavailable: {OTEL_TRACE_IMPORT_ERROR}", file=sys.stderr)
        return

    try:
        tool_name = get_tool_name(ctx)
        server_name = get_server_name(ctx)
        status = int(event.get("status", 200))
        span_name = f"centian.mcp.tool_call/{tool_name}"

        with TRACER.start_as_current_span(span_name) as span:
            span.set_attribute("centian.span_name", span_name)
            span.set_attribute("centian.server_name", server_name)
            span.set_attribute("centian.direction", str(event.get("direction", "unknown")))
            span.set_attribute("centian.status", status)
            span.set_attribute("centian.is_error", is_error)
            span.set_attribute("centian.arguments_count", get_arg_count(params.arguments))
            span.set_attribute("centian.tool_name", tool_name)
            span.set_attribute("centian.arguments", str(params.arguments)[:1000])  # Truncate long arguments for safety
            if original_params.name:
                span.set_attribute("centian.original_tool_name", original_params.name)
            if original_params.arguments is not None:
                span.set_attribute("centian.original_arguments", str(original_params.arguments)[:1000])  # Truncate long arguments for safety
            result_summary = get_result_summary(ctx)
            if result_summary:
                span.set_attribute("centian.result", result_summary[:10000])
            span.set_attribute("centian.decision", decision)

            if ctx.version:
                span.set_attribute("centian.version", ctx.version)
            if event.get("request_id"):
                span.set_attribute("centian.request_id", str(event["request_id"]))
            if event.get("session_id"):
                span.set_attribute("centian.session_id", str(event["session_id"]))
            if event.get("message_type"):
                span.set_attribute("centian.message_type", str(event["message_type"]))
            if reason:
                span.set_attribute("centian.reason", reason)
            if risk_level:
                span.set_attribute("centian.risk_level", risk_level)

        if should_force_flush():
            provider = trace.get_tracer_provider()
            if hasattr(provider, "force_flush"):
                flush_timeout = get_force_flush_timeout_millis()
                try:
                    provider.force_flush(timeout_millis=flush_timeout)
                except TypeError:
                    provider.force_flush()
    except Exception as exc:  # pragma: no cover - demo safety net
        print(f"[telemetry] export failed: {exc}", file=sys.stderr)
