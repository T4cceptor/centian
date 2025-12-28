#!/usr/bin/env python3
"""
Centian Processor: passthrough
Type: passthrough
Generated: 2025-12-28T11:45:56Z
"""

import sys
import json

def process(event):
    """
    Process an MCP message.

    Args:
        event: Input event with structure:
            - type: "request" or "response"
            - timestamp: ISO 8601 timestamp
            - connection: {server_name, transport, session_id}
            - payload: MCP message payload
            - metadata: {processor_chain, original_payload}

    Returns:
        dict: Output with structure:
            - status: HTTP status code (200, 40x, 50x)
            - payload: Modified or original payload
            - error: Error message or None
            - metadata: {processor_name, modifications}
    """
    payload = event["payload"]

    # Passthrough: return payload unchanged

    # Return success
    return {
        "status": 200,
        "payload": payload,
        "error": None,
        "metadata": {
            "processor_name": "passthrough",
            "modifications": []
        }
    }

def main():
    try:
        # Read input from stdin
        event = json.load(sys.stdin)

        # Process the event
        result = process(event)

        # Write result to stdout
        print(json.dumps(result))
        sys.exit(0)

    except Exception as e:
        # Return internal error
        result = {
            "status": 500,
            "payload": {},
            "error": str(e),
            "metadata": {"processor_name": "passthrough"}
        }
        print(json.dumps(result))
        sys.exit(0)  # Exit 0 even on error

if __name__ == "__main__":
    main()
