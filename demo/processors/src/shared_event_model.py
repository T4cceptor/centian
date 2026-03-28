#!/usr/bin/env python3
"""
Shared event model for the Centian PoC processors.
"""

from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional


def compact_dict(value: Dict[str, Any]) -> Dict[str, Any]:
    return {key: item for key, item in value.items() if item is not None}


@dataclass
class CallToolParamsRaw:
    name: str = ""
    arguments: Any = None
    meta: Optional[Dict[str, Any]] = None

    @staticmethod
    def from_dict(data: Optional[Dict[str, Any]]) -> "CallToolParamsRaw":
        source = data or {}
        return CallToolParamsRaw(
            name=source.get("name", ""),
            arguments=source.get("arguments"),
            meta=source.get("_meta"),
        )

    def to_dict(self) -> Dict[str, Any]:
        return compact_dict({
            "name": self.name,
            "arguments": self.arguments,
            "_meta": self.meta,
        })


@dataclass
class CallToolRequest:
    params: Optional[CallToolParamsRaw] = None

    @staticmethod
    def from_dict(data: Optional[Dict[str, Any]]) -> "CallToolRequest":
        source = data or {}
        params_source = source.get("Params")
        if params_source is None:
            params_source = source.get("params")

        params = None
        if isinstance(params_source, dict):
            params = CallToolParamsRaw.from_dict(params_source)
        return CallToolRequest(params=params)

    def to_dict(self) -> Dict[str, Any]:
        return compact_dict({
            "Params": self.params.to_dict() if self.params else None,
        })


@dataclass
class CallToolResult:
    content: List[Any] = field(default_factory=list)
    structured_content: Any = None
    is_error: bool = False
    meta: Optional[Dict[str, Any]] = None

    @staticmethod
    def from_dict(data: Optional[Dict[str, Any]]) -> "CallToolResult":
        source = data or {}
        raw_content = source.get("content")
        content = raw_content if isinstance(raw_content, list) else []
        return CallToolResult(
            content=content,
            structured_content=source.get("structuredContent"),
            is_error=bool(source.get("isError", False)),
            meta=source.get("_meta"),
        )

    def to_dict(self) -> Dict[str, Any]:
        return compact_dict({
            "content": self.content,
            "structuredContent": self.structured_content,
            "isError": True if self.is_error else None,
            "_meta": self.meta,
        })


@dataclass
class PayloadPart:
    request: Optional[CallToolRequest] = None
    original_request: Optional[CallToolRequest] = None
    result: Optional[CallToolResult] = None
    original_result: Optional[CallToolResult] = None

    @staticmethod
    def from_dict(data: Optional[Dict[str, Any]]) -> "PayloadPart":
        source = data or {}
        return PayloadPart(
            request=CallToolRequest.from_dict(source["request"]) if isinstance(source.get("request"), dict) else None,
            original_request=CallToolRequest.from_dict(source["original_request"]) if isinstance(source.get("original_request"), dict) else None,
            result=CallToolResult.from_dict(source["result"]) if isinstance(source.get("result"), dict) else None,
            original_result=CallToolResult.from_dict(source["original_result"]) if isinstance(source.get("original_result"), dict) else None,
        )

    def to_dict(self) -> Dict[str, Any]:
        return compact_dict({
            "request": self.request.to_dict() if self.request else None,
            "original_request": self.original_request.to_dict() if self.original_request else None,
            "result": self.result.to_dict() if self.result else None,
            "original_result": self.original_result.to_dict() if self.original_result else None,
        })


@dataclass
class RoutingPart:
    server_name: str = ""
    tool_name: str = ""
    original_server_name: str = ""
    original_tool_name: str = ""

    @staticmethod
    def from_dict(data: Optional[Dict[str, Any]]) -> "RoutingPart":
        source = data or {}
        return RoutingPart(
            server_name=source.get("server_name", ""),
            tool_name=source.get("tool_name", ""),
            original_server_name=source.get("original_server_name", ""),
            original_tool_name=source.get("original_tool_name", ""),
        )

    def to_dict(self) -> Dict[str, Any]:
        return compact_dict({
            "server_name": self.server_name,
            "tool_name": self.tool_name,
            "original_server_name": self.original_server_name,
            "original_tool_name": self.original_tool_name,
        })


@dataclass
class DataContext:
    version: str = ""
    event: Optional[Dict[str, Any]] = None
    payload: Optional[PayloadPart] = None
    routing: Optional[RoutingPart] = None

    @staticmethod
    def from_dict(data: Dict[str, Any]) -> "DataContext":
        payload_source = data.get("payload")
        routing_source = data.get("routing")
        return DataContext(
            version=data.get("version", ""),
            event=data.get("event"),
            payload=PayloadPart.from_dict(payload_source) if isinstance(payload_source, dict) else None,
            routing=RoutingPart.from_dict(routing_source) if isinstance(routing_source, dict) else None,
        )

    def to_dict(self) -> Dict[str, Any]:
        return compact_dict({
            "version": self.version,
            "event": self.event,
            "payload": self.payload.to_dict() if self.payload else None,
            "routing": self.routing.to_dict() if self.routing else None,
        })
