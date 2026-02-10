#!/usr/bin/env python3
import json
import sys


def process(ctx):
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
