#!/usr/bin/env python3
import json
import sys


def process(ctx):
    payload = ctx.get("payload") or {}
    request = payload.get("request") or {}
    params = request.get("Params") or {}
    arguments = params.get("arguments")

    if isinstance(arguments, dict):
        arguments["x-processor"] = "payload_transformer"
        params["arguments"] = arguments
        request["Params"] = params
        payload["request"] = request
        ctx["payload"] = payload

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
