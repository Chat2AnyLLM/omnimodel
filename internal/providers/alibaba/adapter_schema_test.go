package alibaba

import (
	"encoding/json"
	"testing"

	"omnillm/internal/providers/openaicompat"
)

func TestSanitizeDashScopeSchema(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]interface{}
		want  map[string]interface{}
	}{
		{
			name:  "nil passthrough",
			input: nil,
			want:  nil,
		},
		{
			name: "removes additionalProperties",
			input: map[string]interface{}{
				"type":                 "object",
				"properties":          map[string]interface{}{},
				"additionalProperties": false,
			},
			want: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			name: "removes unevaluatedProperties",
			input: map[string]interface{}{
				"type":                  "object",
				"unevaluatedProperties": false,
			},
			want: map[string]interface{}{
				"type": "object",
			},
		},
		{
			name: "collapses nullable type array",
			input: map[string]interface{}{
				"type": []interface{}{"string", "null"},
			},
			want: map[string]interface{}{
				"type": "string",
			},
		},
		{
			name: "collapses null-first type array",
			input: map[string]interface{}{
				"type": []interface{}{"null", "integer"},
			},
			want: map[string]interface{}{
				"type": "integer",
			},
		},
		{
			name: "preserves scalar type",
			input: map[string]interface{}{
				"type": "string",
			},
			want: map[string]interface{}{
				"type": "string",
			},
		},
		{
			name: "recursively sanitizes properties",
			input: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":                 []interface{}{"string", "null"},
						"additionalProperties": false,
					},
				},
				"additionalProperties": false,
			},
			want: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
		{
			name: "recursively sanitizes items",
			input: map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type":                 []interface{}{"string", "null"},
					"additionalProperties": false,
				},
			},
			want: map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
		},
		{
			name: "recursively sanitizes anyOf",
			input: map[string]interface{}{
				"anyOf": []interface{}{
					map[string]interface{}{
						"type":                 "string",
						"additionalProperties": false,
					},
					map[string]interface{}{
						"type": "null",
					},
				},
			},
			want: map[string]interface{}{
				"anyOf": []interface{}{
					map[string]interface{}{"type": "string"},
					map[string]interface{}{"type": "null"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeDashScopeSchema(tt.input)
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tt.want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("sanitizeDashScopeSchema() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestSanitizeDashScopeTools(t *testing.T) {
	tools := []openaicompat.Tool{
		{
			Type: "function",
			Function: openaicompat.FunctionSpec{
				Name: "read_file",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":                 []interface{}{"string", "null"},
							"additionalProperties": false,
						},
					},
					"additionalProperties": false,
				},
			},
		},
	}

	sanitizeDashScopeTools(tools)

	params := tools[0].Function.Parameters
	if _, ok := params["additionalProperties"]; ok {
		t.Error("expected additionalProperties to be removed from top-level schema")
	}
	props, _ := params["properties"].(map[string]interface{})
	path, _ := props["path"].(map[string]interface{})
	if _, ok := path["additionalProperties"]; ok {
		t.Error("expected additionalProperties to be removed from nested schema")
	}
	if path["type"] != "string" {
		t.Errorf("expected type array collapsed to string, got %#v", path["type"])
	}
}
