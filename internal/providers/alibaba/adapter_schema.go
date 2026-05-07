package alibaba

import "omnillm/internal/providers/openaicompat"

// sanitizeDashScopeTools strips JSON Schema keywords that DashScope's
// tool-call validation rejects. Applied in-place; nil slice is a no-op.
func sanitizeDashScopeTools(tools []openaicompat.Tool) {
	for i := range tools {
		tools[i].Function.Parameters = sanitizeDashScopeSchema(tools[i].Function.Parameters)
	}
}

// sanitizeDashScopeSchema returns a cleaned copy of schema:
//   - additionalProperties / unevaluatedProperties are dropped
//   - type arrays ["string","null"] are collapsed to the first non-null scalar
//
// Recursively processes properties, items, anyOf, oneOf, allOf.
func sanitizeDashScopeSchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}
	out := make(map[string]interface{}, len(schema))
	for k, v := range schema {
		switch k {
		case "additionalProperties", "unevaluatedProperties":
			// DashScope rejects these keywords; omit them.
		case "type":
			out[k] = collapseTypeArray(v)
		case "properties":
			out[k] = sanitizePropertyMap(v)
		case "items":
			out[k] = sanitizeSchemaOrValue(v)
		case "anyOf", "oneOf", "allOf":
			out[k] = sanitizeSchemaSlice(v)
		default:
			out[k] = v
		}
	}
	return out
}

// collapseTypeArray converts ["string","null"] → "string"; non-arrays pass through.
func collapseTypeArray(v interface{}) interface{} {
	arr, ok := v.([]interface{})
	if !ok {
		return v
	}
	for _, item := range arr {
		if s, ok := item.(string); ok && s != "null" {
			return s
		}
	}
	if len(arr) > 0 {
		return arr[0]
	}
	return v
}

func sanitizePropertyMap(v interface{}) interface{} {
	props, ok := v.(map[string]interface{})
	if !ok {
		return v
	}
	out := make(map[string]interface{}, len(props))
	for pk, pv := range props {
		if m, ok := pv.(map[string]interface{}); ok {
			out[pk] = sanitizeDashScopeSchema(m)
		} else {
			out[pk] = pv
		}
	}
	return out
}

func sanitizeSchemaOrValue(v interface{}) interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return sanitizeDashScopeSchema(m)
	}
	return v
}

func sanitizeSchemaSlice(v interface{}) interface{} {
	arr, ok := v.([]interface{})
	if !ok {
		return v
	}
	out := make([]interface{}, len(arr))
	for i, item := range arr {
		if m, ok := item.(map[string]interface{}); ok {
			out[i] = sanitizeDashScopeSchema(m)
		} else {
			out[i] = item
		}
	}
	return out
}
