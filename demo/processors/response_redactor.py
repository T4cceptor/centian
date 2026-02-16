#!/usr/bin/env python3
"""
Demo processor: redact sensitive values in MCP response payload text.

This is a lightweight demonstration, not a complete DLP solution.
"""

import json
import re
import sys
from typing import Any, Dict, Tuple


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
    payload = ctx.get("payload")
    if not isinstance(payload, dict):
        return ctx

    result = payload.get("result")
    if not isinstance(result, dict):
        return ctx

    content = result.get("content")
    if isinstance(content, list):
        for item in content:
            if isinstance(item, dict) and item.get("type") == "text" and isinstance(item.get("text"), str):
                item["text"] = redact_string(item["text"])
        result["content"] = content

    if "structuredContent" in result:
        result["structuredContent"] = redact_recursive(result.get("structuredContent"))

    payload["result"] = result
    ctx["payload"] = payload
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
