#!/usr/bin/env python3
"""
Demo processor: emit MCP tool-call spans via OpenTelemetry OTLP.

This script is intentionally minimal and best-effort for demo purposes.
If OTEL setup or export fails, traffic passes through unchanged.
"""

import json
import sys
from typing import Any, Dict

from shared_event_model import DataContext
from telemetry import emit_tool_call_span


def process(ctx: Dict[str, Any]) -> Dict[str, Any]:
    try:
        data_ctx = DataContext.from_dict(ctx)
        emit_tool_call_span(data_ctx)
    except Exception as exc:  # pragma: no cover - demo safety net
        print(f"[otel_span_logger] export failed: {exc}", file=sys.stderr)

    # This processor is observability-only; do not write routing back.
    # In aggregated gateways, writing de-namespaced routing tool names can
    # accidentally alter downstream dispatch behavior.
    output = dict(ctx)
    output.pop("routing", None)
    return output


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
