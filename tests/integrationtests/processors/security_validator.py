#!/usr/bin/env python3
import json
import sys


def process(ctx):
    payload = ctx.get("payload") or {}
    request = payload.get("request") or {}
    params = request.get("Params") or {}
    tool_name = (params.get("name") or "")

    if "delete" in tool_name.lower():
        event = ctx.get("event") or {}
        event["status"] = 403
        event["error"] = "Delete operations not allowed"
        event["success"] = False
        ctx["event"] = event

    return ctx


def main():
    try:
        ctx = json.load(sys.stdin)
        print(json.dumps(process(ctx)))
        sys.exit(0)
    except Exception as e:
        print(json.dumps({"error": str(e)}))
        sys.exit(0)


if __name__ == "__main__":
    main()
