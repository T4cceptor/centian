#!/usr/bin/env python3
"""
Demo processor: redact sensitive values in MCP response payload text.

This is a lightweight demonstration, not a complete DLP solution.
"""

import json
import re
import sys
from typing import Any, Dict, Tuple

from shared_event_model import DataContext


PATTERNS: Tuple[Tuple[re.Pattern[str], str], ...] = (
    (re.compile(r"\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b"), "[REDACTED_EMAIL]"),
    (re.compile(r"\b\d{3}-\d{2}-\d{4}\b"), "[REDACTED_SSN]"),
    (re.compile(r"\bAKIA[0-9A-Z]{16}\b"), "[REDACTED_AWS_ACCESS_KEY]"),
    (re.compile(r"(?i)\b(bearer\s+)[A-Za-z0-9\-._~+/]+=*\b"), r"\1[REDACTED_TOKEN]"),
)


def redact_string(value: str) -> str:
    redacted = value
    for regex, replacement in PATTERNS:
        redacted = regex.sub(replacement, redacted)
    return redacted


def redact_recursive(value: Any) -> Any:
    if isinstance(value, str):
        return redact_string(value)
    if isinstance(value, list):
        return [redact_recursive(item) for item in value]
    if isinstance(value, dict):
        return {key: redact_recursive(item) for key, item in value.items()}
    return value


def process(ctx: Dict[str, Any]) -> Dict[str, Any]:
    # Parse through the shared model so both demo processors use the same event scaffolding.
    parsed = DataContext.from_dict(ctx)
    if not parsed.payload or not parsed.payload.result:
        return ctx
    result_model = parsed.payload.result

    payload = ctx.get("payload")
    if not isinstance(payload, dict):
        return ctx

    result = payload.get("result")
    if not isinstance(result, dict):
        return ctx

    if isinstance(result_model.content, list):
        for item in result_model.content:
            if isinstance(item, dict) and item.get("type") == "text" and isinstance(item.get("text"), str):
                item["text"] = redact_string(item["text"])

    if result_model.structured_content is not None:
        result_model.structured_content = redact_recursive(result_model.structured_content)

    result.update(result_model.to_dict())
    payload["result"] = result
    ctx["payload"] = payload
    return ctx


def main():
    try:
        input_data = json.load(sys.stdin)
        output_data = process(input_data)
        output_data.pop("routing", None) # workaround for processing bug in proxy
        print(json.dumps(output_data))
        sys.exit(0)
    except Exception as exc:
        print(json.dumps({"event": {"status": 500, "error": str(exc), "success": False}}))
        sys.exit(0)


if __name__ == "__main__":
    main()
