package modelsmeta

import (
	"encoding/json"
	"testing"
)

func TestFlattenModelsDevPayload(t *testing.T) {
	rawJSON := `{
		"openai": {
			"id": "openai",
			"name": "OpenAI",
			"models": {
				"gpt-5-mini": {
					"id": "gpt-5-mini",
					"name": "GPT-5 Mini",
					"family": "gpt",
					"tool_call": true,
					"structured_output": true,
					"reasoning": true,
					"attachment": true,
					"open_weights": false,
					"cost": {"input": 0.25, "output": 2, "cache_read": 0.025},
					"limit": {"context": 400000, "input": 272000, "output": 128000},
					"modalities": {"input": ["text", "image"], "output": ["text"]}
				}
			}
		}
	}`

	var raw map[string]modelsDevProvider
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	models := flatten(raw)
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}

	got := models[0]
	if got.ID != "gpt-5-mini" {
		t.Fatalf("expected model ID gpt-5-mini, got %q", got.ID)
	}
	if got.ProviderID != "openai" {
		t.Fatalf("expected provider openai, got %q", got.ProviderID)
	}
	if got.ProviderName != "OpenAI" {
		t.Fatalf("expected provider name OpenAI, got %q", got.ProviderName)
	}
	if got.ContextLimitTokens == nil || *got.ContextLimitTokens != 400000 {
		t.Fatalf("expected context limit 400000, got %#v", got.ContextLimitTokens)
	}
	if got.InputLimitTokens == nil || *got.InputLimitTokens != 272000 {
		t.Fatalf("expected input limit 272000, got %#v", got.InputLimitTokens)
	}
	if got.OutputLimitTokens == nil || *got.OutputLimitTokens != 128000 {
		t.Fatalf("expected output limit 128000, got %#v", got.OutputLimitTokens)
	}
	if got.InputPriceUSDPer1MTokens == nil || *got.InputPriceUSDPer1MTokens != 0.25 {
		t.Fatalf("expected input price 0.25, got %#v", got.InputPriceUSDPer1MTokens)
	}
	if got.OutputPriceUSDPer1MTokens == nil || *got.OutputPriceUSDPer1MTokens != 2 {
		t.Fatalf("expected output price 2, got %#v", got.OutputPriceUSDPer1MTokens)
	}
	if got.SupportsToolCall == nil || !*got.SupportsToolCall {
		t.Fatalf("expected supports_tool_call=true, got %#v", got.SupportsToolCall)
	}
}

func TestParseOptionalIntAndFloat(t *testing.T) {
	i := parseOptionalInt(json.Number("16384"))
	if i == nil || *i != 16384 {
		t.Fatalf("expected 16384, got %#v", i)
	}

	f := parseOptionalFloat(json.Number("1.75"))
	if f == nil || *f != 1.75 {
		t.Fatalf("expected 1.75, got %#v", f)
	}

	if parseOptionalInt(json.Number("")) != nil {
		t.Fatal("expected nil for empty int value")
	}
	if parseOptionalFloat(json.Number("")) != nil {
		t.Fatal("expected nil for empty float value")
	}
}
