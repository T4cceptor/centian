package common

import (
	"encoding/json"
	"strconv"
	"strings"
)

// AnyMap returns value when it is already a JSON-like object map.
func AnyMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	typed, ok := value.(map[string]any)
	if ok {
		return typed
	}
	return nil
}

// StringValue converts string-like JSON fields without failing hard on other types.
func StringValue(value any) string {
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return typed
}

// IntPtrFromAny converts a loosely typed numeric field into *int.
func IntPtrFromAny(value any) *int {
	if parsed, ok := ParseInt64(value); ok {
		result := int(parsed)
		return &result
	}
	return nil
}

// Int64PtrFromAny converts a loosely typed numeric field into *int64.
func Int64PtrFromAny(value any) *int64 {
	if parsed, ok := ParseInt64(value); ok {
		return &parsed
	}
	return nil
}

// Float64PtrFromAny converts a loosely typed numeric field into *float64.
func Float64PtrFromAny(value any) *float64 {
	switch typed := value.(type) {
	case float64:
		return &typed
	case float32:
		result := float64(typed)
		return &result
	case int:
		result := float64(typed)
		return &result
	case int64:
		result := float64(typed)
		return &result
	case json.Number:
		parsed, err := typed.Float64()
		if err == nil {
			return &parsed
		}
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return &parsed
		}
	}
	return nil
}

// ParseInt64 accepts common JSON number encodings used in logs and payloads.
func ParseInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	case float32:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return parsed, true
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

// JSONGenericValue round-trips a typed Go value through JSON into generic maps/slices/scalars.
func JSONGenericValue(value any) (any, error) {
	content, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	var generic any
	if err := json.Unmarshal(content, &generic); err != nil {
		return nil, err
	}
	return generic, nil
}

// ConvertViaJSON converts one pointer value into another JSON-compatible shape via marshal/unmarshal.
func ConvertViaJSON[In any, Out any](value *In) (*Out, error) {
	if value == nil {
		return nil, nil
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var out Out
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
