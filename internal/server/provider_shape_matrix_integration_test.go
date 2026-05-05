package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"omnillm/internal/cif"
	"omnillm/internal/database"
	alibabapkg "omnillm/internal/providers/alibaba"
	azurepkg "omnillm/internal/providers/azure"
	kimipkg "omnillm/internal/providers/kimi"
	"omnillm/internal/providers/openaicompatprovider"
	providertypes "omnillm/internal/providers/types"
	"omnillm/internal/registry"
	"sync"
	"testing"
)

const (
	matrixInitialUserPrompt   = "Read README.md"
	matrixAssistantPrelude    = "I'll read it."
	matrixToolCallID          = "call_readme"
	matrixToolName            = "Read"
	matrixToolArgumentsJSON   = `{"file_path":"README.md"}`
	matrixToolResultText      = "README.md says OmniLLM normalizes requests into CIF before routing upstream."
	matrixFinalUserPrompt     = "Summarize that in one sentence."
	matrixAlibabaFinalReply   = "This codebase exposes an OpenAI-compatible and Anthropic-compatible proxy. It normalizes requests into CIF, routes by model, and adapts provider-specific upstreams."
	matrixOpenAICompatReply   = "This codebase exposes a proxy that normalizes requests into CIF and adapts multiple upstream APIs."
	matrixAzureResponsesReply = "Azure says hello from Responses."
	matrixCopilotStubReply    = "Copilot says hello from the stub provider."
	matrixAzureNormalizedCall = "fc_readme"
	matrixAlibabaAPIKey       = "sk-alibaba-matrix"
	matrixAzureAPIKey         = "sk-azure-matrix"
	matrixKimiAPIKey          = "sk-kimi-matrix"
	matrixOpenAICompatAPIKey  = "sk-openai-compat-matrix"
)

type ingressShapeCase struct {
	name             string
	endpoint         string
	body             func(model string) string
	headers          map[string]string
	expectedAPIShape string
}

type providerMatrixHarness struct {
	backend      *httptest.Server
	expectedText string
	assertLast   func(t *testing.T, shape ingressShapeCase)
}

type providerMatrixCase struct {
	name  string
	model string
	setup func(t *testing.T) providerMatrixHarness
}

type capturedAzureResponsesRequest struct {
	APIKey  string
	Payload map[string]interface{}
}

type fakeAzureResponsesUpstream struct {
	server *httptest.Server
	model  string

	mu       sync.Mutex
	requests []capturedAzureResponsesRequest
}

func newFakeAzureResponsesUpstream(t *testing.T, model string) *fakeAzureResponsesUpstream {
	t.Helper()

	upstream := &fakeAzureResponsesUpstream{model: model}
	upstream.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/openai/v1/responses" {
			http.NotFound(w, r)
			return
		}

		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, fmt.Sprintf("decode payload: %v", err), http.StatusBadRequest)
			return
		}

		upstream.mu.Lock()
		upstream.requests = append(upstream.requests, capturedAzureResponsesRequest{
			APIKey:  r.Header.Get("api-key"),
			Payload: payload,
		})
		upstream.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "resp_azure_matrix",
			"object": "response",
			"model":  model,
			"status": "completed",
			"output": []map[string]interface{}{
				{
					"type": "message",
					"role": "assistant",
					"content": []map[string]interface{}{
						{
							"type": "output_text",
							"text": matrixAzureResponsesReply,
						},
					},
				},
			},
			"usage": map[string]interface{}{
				"input_tokens":  42,
				"output_tokens": 9,
				"total_tokens":  51,
			},
		})
	}))

	t.Cleanup(upstream.server.Close)
	return upstream
}

func (u *fakeAzureResponsesUpstream) baseURL() string {
	return u.server.URL
}

func (u *fakeAzureResponsesUpstream) requestCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.requests)
}

func (u *fakeAzureResponsesUpstream) lastRequest(t *testing.T) capturedAzureResponsesRequest {
	t.Helper()

	u.mu.Lock()
	defer u.mu.Unlock()

	if len(u.requests) == 0 {
		t.Fatal("expected at least one captured Azure Responses request")
	}
	return u.requests[len(u.requests)-1]
}

func TestProviderModelMessageMatrixAcrossIngressShapes(t *testing.T) {
	shapes := []ingressShapeCase{
		{
			name:             "chat completions",
			endpoint:         "/v1/chat/completions",
			body:             matrixChatCompletionsBody,
			expectedAPIShape: "openai",
		},
		{
			name:             "anthropic messages",
			endpoint:         "/v1/messages",
			body:             matrixAnthropicMessagesBody,
			headers:          map[string]string{"anthropic-version": "2023-06-01"},
			expectedAPIShape: "anthropic",
		},
		{
			name:             "responses api",
			endpoint:         "/v1/responses",
			body:             matrixResponsesBody,
			expectedAPIShape: "responses",
		},
	}

	providers := []providerMatrixCase{
		{
			name:  "alibaba/qwen3.6-plus",
			model: "qwen3.6-plus",
			setup: setupAlibabaProviderMatrixHarness,
		},
		{
			name:  "azure-openai/gpt-5.4",
			model: "gpt-5.4",
			setup: setupAzureProviderMatrixHarness,
		},
		{
			name:  "github-copilot/gpt-5-mini",
			model: "gpt-5-mini",
			setup: setupCopilotProviderMatrixHarness,
		},
		{
			name:  "kimi/kimi-k2.5",
			model: "kimi-k2.5",
			setup: setupKimiProviderMatrixHarness,
		},
		{
			name:  "openai-compatible-dashscope-aliyuncs-com-101ed4/qwen3.6-max-preview",
			model: "qwen3.6-max-preview",
			setup: setupOpenAICompatibleProviderMatrixHarness,
		},
	}

	for _, providerCase := range providers {
		t.Run(providerCase.name, func(t *testing.T) {
			harness := providerCase.setup(t)
			defer harness.backend.Close()

			for _, shape := range shapes {
				t.Run(shape.name, func(t *testing.T) {
					resp := postJSON(t, harness.backend.URL+shape.endpoint, shape.body(providerCase.model), shape.headers)
					body := readBody(t, resp)
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
					}
					assertEndpointResponseText(t, shape.endpoint, body, harness.expectedText)
					harness.assertLast(t, shape)
				})
			}
		})
	}
}

func setupAlibabaProviderMatrixHarness(t *testing.T) providerMatrixHarness {
	t.Helper()

	upstream := newFakeAlibabaQwenUpstream(t)
	provider := alibabapkg.NewProvider("alibaba-shape-matrix", "Alibaba Matrix Test")
	if err := provider.SetupAuth(&providertypes.AuthOptions{
		Method:   "api-key",
		APIKey:   matrixAlibabaAPIKey,
		Endpoint: upstream.baseURL(),
		Region:   "global",
		Plan:     "standard",
	}); err != nil {
		t.Fatalf("setup Alibaba provider: %v", err)
	}
	registerProviderAndActivate(t, provider)

	backend := newTestServer(t)
	requestsSeen := 0

	return providerMatrixHarness{
		backend:      backend,
		expectedText: matrixAlibabaFinalReply,
		assertLast: func(t *testing.T, shape ingressShapeCase) {
			t.Helper()

			if got := upstream.chatRequestCount(); got != requestsSeen+1 {
				t.Fatalf("%s: expected %d upstream requests, got %d", shape.name, requestsSeen+1, got)
			}
			requestsSeen++

			lastReq := upstream.lastChatRequest(t)
			if lastReq.Authorization != "Bearer "+matrixAlibabaAPIKey {
				t.Fatalf("%s: expected Alibaba bearer auth, got %q", shape.name, lastReq.Authorization)
			}
			if lastReq.Accept != "application/json" {
				t.Fatalf("%s: expected Alibaba Accept=application/json, got %q", shape.name, lastReq.Accept)
			}
			assertOpenAIHistoryPayload(t, shape.name, "qwen3.6-plus", lastReq.Payload)
		},
	}
}

func setupAzureProviderMatrixHarness(t *testing.T) providerMatrixHarness {
	t.Helper()

	upstream := newFakeAzureResponsesUpstream(t, "gpt-5.4")
	provider := azurepkg.NewProvider("azure-openai-shape-matrix", "Azure Matrix Test")
	if err := provider.SetupAuth(&providertypes.AuthOptions{
		APIKey:   matrixAzureAPIKey,
		Endpoint: upstream.baseURL(),
	}); err != nil {
		t.Fatalf("setup Azure provider: %v", err)
	}
	if err := database.NewProviderConfigStore().Save(provider.GetInstanceID(), map[string]interface{}{
		"endpoint":    upstream.baseURL(),
		"deployments": []string{"gpt-5.4"},
	}); err != nil {
		t.Fatalf("save Azure deployments config: %v", err)
	}
	registerProviderAndActivate(t, provider)

	backend := newTestServer(t)
	requestsSeen := 0

	return providerMatrixHarness{
		backend:      backend,
		expectedText: matrixAzureResponsesReply,
		assertLast: func(t *testing.T, shape ingressShapeCase) {
			t.Helper()

			if got := upstream.requestCount(); got != requestsSeen+1 {
				t.Fatalf("%s: expected %d Azure upstream requests, got %d", shape.name, requestsSeen+1, got)
			}
			requestsSeen++

			lastReq := upstream.lastRequest(t)
			if lastReq.APIKey != matrixAzureAPIKey {
				t.Fatalf("%s: expected Azure api-key header %q, got %q", shape.name, matrixAzureAPIKey, lastReq.APIKey)
			}
			assertAzureResponsesHistoryPayload(t, shape.name, "gpt-5.4", lastReq.Payload)
		},
	}
}

func setupCopilotProviderMatrixHarness(t *testing.T) providerMatrixHarness {
	t.Helper()

	var captured []*cif.CanonicalRequest

	instanceID := registerStubProviderWithType(
		t,
		string(providertypes.ProviderGitHubCopilot),
		"gpt-5-mini",
		func(_ context.Context, req *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
			captured = append(captured, req)
			return &cif.CanonicalResponse{
				ID:         "resp_copilot_matrix",
				Model:      req.Model,
				Content:    []cif.CIFContentPart{cif.CIFTextPart{Type: "text", Text: matrixCopilotStubReply}},
				StopReason: cif.StopReasonEndTurn,
			}, nil
		},
		nil,
	)
	t.Cleanup(func() {
		_ = database.NewProviderInstanceStore().Delete(instanceID)
	})

	backend := newTestServer(t)
	requestsSeen := 0

	return providerMatrixHarness{
		backend:      backend,
		expectedText: matrixCopilotStubReply,
		assertLast: func(t *testing.T, shape ingressShapeCase) {
			t.Helper()

			if len(captured) != requestsSeen+1 {
				t.Fatalf("%s: expected %d Copilot requests, got %d", shape.name, requestsSeen+1, len(captured))
			}
			requestsSeen++

			lastReq := captured[len(captured)-1]
			if lastReq.Model != "gpt-5-mini" {
				t.Fatalf("%s: expected Copilot model gpt-5-mini, got %q", shape.name, lastReq.Model)
			}
			if lastReq.Extensions == nil || lastReq.Extensions.InboundAPIShape == nil || *lastReq.Extensions.InboundAPIShape != shape.expectedAPIShape {
				t.Fatalf("%s: expected inbound API shape %q, got %#v", shape.name, shape.expectedAPIShape, lastReq.Extensions)
			}
			assertCanonicalHistoryMessages(t, shape.name, lastReq.Messages)
		},
	}
}

func setupKimiProviderMatrixHarness(t *testing.T) providerMatrixHarness {
	t.Helper()

	upstream := newFakeOpenAICompatUpstream(t, "kimi-k2.5")
	provider := kimipkg.NewProvider("kimi-shape-matrix", "Kimi Matrix Test")
	if err := provider.SetupAuth(&providertypes.AuthOptions{
		Method:   "api-key",
		APIKey:   matrixKimiAPIKey,
		Endpoint: upstream.baseURL(),
		Region:   "global",
	}); err != nil {
		t.Fatalf("setup Kimi provider: %v", err)
	}
	registerProviderAndActivate(t, provider)

	backend := newTestServer(t)
	requestsSeen := 0

	return providerMatrixHarness{
		backend:      backend,
		expectedText: matrixOpenAICompatReply,
		assertLast: func(t *testing.T, shape ingressShapeCase) {
			t.Helper()

			if got := upstream.chatRequestCount(); got != requestsSeen+1 {
				t.Fatalf("%s: expected %d Kimi upstream requests, got %d", shape.name, requestsSeen+1, got)
			}
			requestsSeen++

			lastReq := upstream.lastChatRequest(t)
			if lastReq.Authorization != "Bearer "+matrixKimiAPIKey {
				t.Fatalf("%s: expected Kimi bearer auth, got %q", shape.name, lastReq.Authorization)
			}
			if lastReq.Accept != "application/json" {
				t.Fatalf("%s: expected Kimi Accept=application/json, got %q", shape.name, lastReq.Accept)
			}
			assertOpenAIHistoryPayload(t, shape.name, "kimi-k2.5", lastReq.Payload)
		},
	}
}

func setupOpenAICompatibleProviderMatrixHarness(t *testing.T) providerMatrixHarness {
	t.Helper()

	upstream := newFakeOpenAICompatUpstream(t, "qwen3.6-max-preview")
	provider := openaicompatprovider.NewProvider(
		"openai-compatible-dashscope-aliyuncs-com-101ed4",
		"OpenAI-Compatible DashScope Matrix Test",
	)
	if err := provider.SetupAuth(&providertypes.AuthOptions{
		APIKey:              matrixOpenAICompatAPIKey,
		Endpoint:            upstream.baseURL(),
		Models:              `["qwen3.6-max-preview"]`,
		AllowLocalEndpoints: true,
	}); err != nil {
		t.Fatalf("setup openai-compatible provider: %v", err)
	}
	registerProviderAndActivate(t, provider)

	backend := newTestServer(t)
	requestsSeen := 0

	return providerMatrixHarness{
		backend:      backend,
		expectedText: matrixOpenAICompatReply,
		assertLast: func(t *testing.T, shape ingressShapeCase) {
			t.Helper()

			if got := upstream.chatRequestCount(); got != requestsSeen+1 {
				t.Fatalf("%s: expected %d openai-compatible upstream requests, got %d", shape.name, requestsSeen+1, got)
			}
			requestsSeen++

			lastReq := upstream.lastChatRequest(t)
			if lastReq.Authorization != "Bearer "+matrixOpenAICompatAPIKey {
				t.Fatalf("%s: expected openai-compatible bearer auth, got %q", shape.name, lastReq.Authorization)
			}
			if lastReq.Accept != "application/json" {
				t.Fatalf("%s: expected openai-compatible Accept=application/json, got %q", shape.name, lastReq.Accept)
			}
			assertOpenAIHistoryPayload(t, shape.name, "qwen3.6-max-preview", lastReq.Payload)
		},
	}
}

func registerProviderAndActivate(t *testing.T, provider providertypes.Provider) {
	t.Helper()

	reg := registry.GetProviderRegistry()
	if err := reg.Register(provider, false); err != nil {
		t.Fatalf("register provider %s: %v", provider.GetInstanceID(), err)
	}
	if _, err := reg.AddActive(provider.GetInstanceID()); err != nil {
		t.Fatalf("activate provider %s: %v", provider.GetInstanceID(), err)
	}

	t.Cleanup(func() {
		_ = reg.Remove(provider.GetInstanceID())
		_ = database.NewProviderConfigStore().Delete(provider.GetInstanceID())
		_ = database.NewProviderInstanceStore().Delete(provider.GetInstanceID())
		_ = database.NewTokenStore().Delete(provider.GetInstanceID())
	})
}

func matrixChatCompletionsBody(model string) string {
	return fmt.Sprintf(
		`{"model":"%s","messages":[{"role":"user","content":%q},{"role":"assistant","content":%q,"tool_calls":[{"id":"%s","type":"function","function":{"name":"%s","arguments":%q}}]},{"role":"tool","tool_call_id":"%s","content":%q},{"role":"user","content":%q}]}`,
		model,
		matrixInitialUserPrompt,
		matrixAssistantPrelude,
		matrixToolCallID,
		matrixToolName,
		matrixToolArgumentsJSON,
		matrixToolCallID,
		matrixToolResultText,
		matrixFinalUserPrompt,
	)
}

func matrixAnthropicMessagesBody(model string) string {
	return fmt.Sprintf(
		`{"model":"%s","max_tokens":128,"messages":[{"role":"user","content":%q},{"role":"assistant","content":[{"type":"text","text":%q},{"type":"tool_use","id":"%s","name":"%s","input":{"file_path":"README.md"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"%s","content":%q}]},{"role":"user","content":%q}]}`,
		model,
		matrixInitialUserPrompt,
		matrixAssistantPrelude,
		matrixToolCallID,
		matrixToolName,
		matrixToolCallID,
		matrixToolResultText,
		matrixFinalUserPrompt,
	)
}

func matrixResponsesBody(model string) string {
	return fmt.Sprintf(
		`{"model":"%s","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":%q}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":%q}]},{"type":"function_call","id":"%s","call_id":"%s","name":"%s","arguments":%q},{"type":"function_call_output","call_id":"%s","output":%q},{"type":"message","role":"user","content":[{"type":"input_text","text":%q}]}]}`,
		model,
		matrixInitialUserPrompt,
		matrixAssistantPrelude,
		matrixToolCallID,
		matrixToolCallID,
		matrixToolName,
		matrixToolArgumentsJSON,
		matrixToolCallID,
		matrixToolResultText,
		matrixFinalUserPrompt,
	)
}

func assertEndpointResponseText(t *testing.T, endpoint, body, want string) {
	t.Helper()

	switch endpoint {
	case "/v1/chat/completions":
		var payload struct {
			Choices []struct {
				Message struct {
					Content *string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("invalid chat completions JSON: %v", err)
		}
		if len(payload.Choices) != 1 || payload.Choices[0].Message.Content == nil || *payload.Choices[0].Message.Content != want {
			t.Fatalf("unexpected chat completions payload: %#v", payload)
		}

	case "/v1/messages":
		var payload struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("invalid messages JSON: %v", err)
		}
		if len(payload.Content) != 1 || payload.Content[0].Type != "text" || payload.Content[0].Text != want {
			t.Fatalf("unexpected messages payload: %#v", payload)
		}

	case "/v1/responses":
		var payload struct {
			Output []struct {
				Type    string `json:"type"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"output"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("invalid responses JSON: %v", err)
		}
		if len(payload.Output) != 1 || payload.Output[0].Type != "message" || len(payload.Output[0].Content) != 1 || payload.Output[0].Content[0].Text != want {
			t.Fatalf("unexpected responses payload: %#v", payload)
		}

	default:
		t.Fatalf("unsupported endpoint %q", endpoint)
	}
}

func assertNoUnexpectedSplitAssistantTurns(t *testing.T, shapeName string, messages []map[string]interface{}) {
	t.Helper()

	if len(messages) >= 3 {
		secondRole, _ := messages[1]["role"].(string)
		thirdRole, _ := messages[2]["role"].(string)
		if secondRole == "assistant" && thirdRole == "assistant" {
			t.Fatalf("%s: upstream chat payload split assistant tool history across adjacent assistant turns: %#v", shapeName, messages)
		}
	}
}

func assertOpenAIHistoryPayload(t *testing.T, shapeName, model string, payload map[string]interface{}) {
	t.Helper()

	if got, _ := payload["model"].(string); got != model {
		t.Fatalf("%s: expected upstream model %q, got %#v", shapeName, model, payload["model"])
	}

	upstreamMessages := asInterfaceSlice(payload["messages"])
	if len(upstreamMessages) != 4 {
		t.Fatalf("%s: expected 4 upstream messages, got %#v", shapeName, upstreamMessages)
	}

	messageMaps := make([]map[string]interface{}, 0, len(upstreamMessages))
	for _, raw := range upstreamMessages {
		msg, _ := raw.(map[string]interface{})
		messageMaps = append(messageMaps, msg)
	}
	assertNoUnexpectedSplitAssistantTurns(t, shapeName, messageMaps)

	firstUser := messageMaps[0]
	if role, _ := firstUser["role"].(string); role != "user" {
		t.Fatalf("%s: expected first upstream role=user, got %#v", shapeName, firstUser)
	}
	if content, _ := firstUser["content"].(string); content != matrixInitialUserPrompt {
		t.Fatalf("%s: unexpected first upstream content: %#v", shapeName, firstUser)
	}

	assistant := messageMaps[1]
	if role, _ := assistant["role"].(string); role != "assistant" {
		t.Fatalf("%s: expected assistant upstream message, got %#v", shapeName, assistant)
	}
	if content, _ := assistant["content"].(string); content != matrixAssistantPrelude {
		t.Fatalf("%s: expected assistant prelude %q, got %#v", shapeName, matrixAssistantPrelude, assistant["content"])
	}
	toolCalls := asInterfaceSlice(assistant["tool_calls"])
	if len(toolCalls) != 1 {
		t.Fatalf("%s: expected one upstream tool_call, got %#v", shapeName, assistant)
	}
	toolCall, _ := toolCalls[0].(map[string]interface{})
	if id, _ := toolCall["id"].(string); id != matrixToolCallID {
		t.Fatalf("%s: unexpected upstream tool call id: %#v", shapeName, toolCall)
	}
	functionMap, _ := toolCall["function"].(map[string]interface{})
	if name, _ := functionMap["name"].(string); name != matrixToolName {
		t.Fatalf("%s: unexpected upstream tool function: %#v", shapeName, toolCall)
	}
	if args, _ := functionMap["arguments"].(string); args != matrixToolArgumentsJSON {
		t.Fatalf("%s: unexpected upstream tool arguments: %#v", shapeName, toolCall)
	}

	toolMessage := messageMaps[2]
	if role, _ := toolMessage["role"].(string); role != "tool" {
		t.Fatalf("%s: expected upstream tool result role=tool, got %#v", shapeName, toolMessage)
	}
	if callID, _ := toolMessage["tool_call_id"].(string); callID != matrixToolCallID {
		t.Fatalf("%s: unexpected upstream tool_call_id: %#v", shapeName, toolMessage)
	}
	if content, _ := toolMessage["content"].(string); content != matrixToolResultText {
		t.Fatalf("%s: unexpected upstream tool result content: %#v", shapeName, toolMessage)
	}

	finalUser := messageMaps[3]
	if role, _ := finalUser["role"].(string); role != "user" {
		t.Fatalf("%s: expected final upstream role=user, got %#v", shapeName, finalUser)
	}
	if content, _ := finalUser["content"].(string); content != matrixFinalUserPrompt {
		t.Fatalf("%s: unexpected final upstream content: %#v", shapeName, finalUser)
	}
}

func assertAzureResponsesHistoryPayload(t *testing.T, shapeName, model string, payload map[string]interface{}) {
	t.Helper()

	if got, _ := payload["model"].(string); got != model {
		t.Fatalf("%s: expected Azure model %q, got %#v", shapeName, model, payload["model"])
	}

	input := asInterfaceSlice(payload["input"])
	if len(input) != 5 {
		t.Fatalf("%s: expected 5 Azure input items, got %#v", shapeName, input)
	}

	firstUser, _ := input[0].(map[string]interface{})
	if itemType, _ := firstUser["type"].(string); itemType != "message" {
		t.Fatalf("%s: expected first Azure item to be message, got %#v", shapeName, firstUser)
	}
	if role, _ := firstUser["role"].(string); role != "user" {
		t.Fatalf("%s: expected first Azure role=user, got %#v", shapeName, firstUser)
	}
	assertAzureTextBlock(t, shapeName, firstUser["content"], "input_text", matrixInitialUserPrompt)

	assistant, _ := input[1].(map[string]interface{})
	if itemType, _ := assistant["type"].(string); itemType != "message" {
		t.Fatalf("%s: expected second Azure item to be assistant message, got %#v", shapeName, assistant)
	}
	if role, _ := assistant["role"].(string); role != "assistant" {
		t.Fatalf("%s: expected assistant Azure role=assistant, got %#v", shapeName, assistant)
	}
	assertAzureTextBlock(t, shapeName, assistant["content"], "output_text", matrixAssistantPrelude)

	functionCall, _ := input[2].(map[string]interface{})
	if itemType, _ := functionCall["type"].(string); itemType != "function_call" {
		t.Fatalf("%s: expected third Azure item to be function_call, got %#v", shapeName, functionCall)
	}
	if id, _ := functionCall["id"].(string); id != matrixAzureNormalizedCall {
		t.Fatalf("%s: unexpected Azure function_call id: %#v", shapeName, functionCall)
	}
	if callID, _ := functionCall["call_id"].(string); callID != matrixAzureNormalizedCall {
		t.Fatalf("%s: unexpected Azure function_call call_id: %#v", shapeName, functionCall)
	}
	if name, _ := functionCall["name"].(string); name != matrixToolName {
		t.Fatalf("%s: unexpected Azure function_call name: %#v", shapeName, functionCall)
	}
	if args, _ := functionCall["arguments"].(string); args != matrixToolArgumentsJSON {
		t.Fatalf("%s: unexpected Azure function_call arguments: %#v", shapeName, functionCall)
	}

	functionOutput, _ := input[3].(map[string]interface{})
	if itemType, _ := functionOutput["type"].(string); itemType != "function_call_output" {
		t.Fatalf("%s: expected fourth Azure item to be function_call_output, got %#v", shapeName, functionOutput)
	}
	if callID, _ := functionOutput["call_id"].(string); callID != matrixAzureNormalizedCall {
		t.Fatalf("%s: unexpected Azure function_call_output call_id: %#v", shapeName, functionOutput)
	}
	if output, _ := functionOutput["output"].(string); output != matrixToolResultText {
		t.Fatalf("%s: unexpected Azure function_call_output output: %#v", shapeName, functionOutput)
	}

	finalUser, _ := input[4].(map[string]interface{})
	if itemType, _ := finalUser["type"].(string); itemType != "message" {
		t.Fatalf("%s: expected final Azure item to be message, got %#v", shapeName, finalUser)
	}
	if role, _ := finalUser["role"].(string); role != "user" {
		t.Fatalf("%s: expected final Azure role=user, got %#v", shapeName, finalUser)
	}
	assertAzureTextBlock(t, shapeName, finalUser["content"], "input_text", matrixFinalUserPrompt)
}

func assertAzureTextBlock(t *testing.T, shapeName string, rawContent interface{}, wantType, wantText string) {
	t.Helper()

	content := asInterfaceSlice(rawContent)
	if len(content) != 1 {
		t.Fatalf("%s: expected one Azure text block, got %#v", shapeName, rawContent)
	}
	block, _ := content[0].(map[string]interface{})
	if blockType, _ := block["type"].(string); blockType != wantType {
		t.Fatalf("%s: expected Azure block type %q, got %#v", shapeName, wantType, block)
	}
	if text, _ := block["text"].(string); text != wantText {
		t.Fatalf("%s: expected Azure block text %q, got %#v", shapeName, wantText, block)
	}
}

func assertCanonicalHistoryMessages(t *testing.T, shapeName string, messages []cif.CIFMessage) {
	t.Helper()

	if len(messages) != 4 {
		t.Fatalf("%s: expected 4 canonical messages, got %d", shapeName, len(messages))
	}

	firstUser, ok := messages[0].(cif.CIFUserMessage)
	if !ok {
		t.Fatalf("%s: expected first canonical message to be user, got %T", shapeName, messages[0])
	}
	if len(firstUser.Content) != 1 {
		t.Fatalf("%s: expected one part in first user message, got %#v", shapeName, firstUser.Content)
	}
	if text, ok := firstUser.Content[0].(cif.CIFTextPart); !ok || text.Text != matrixInitialUserPrompt {
		t.Fatalf("%s: unexpected first user content: %#v", shapeName, firstUser.Content)
	}

	assistant, ok := messages[1].(cif.CIFAssistantMessage)
	if !ok {
		t.Fatalf("%s: expected second canonical message to be assistant, got %T", shapeName, messages[1])
	}
	if len(assistant.Content) != 2 {
		t.Fatalf("%s: expected assistant text+tool_call in one turn, got %#v", shapeName, assistant.Content)
	}
	if text, ok := assistant.Content[0].(cif.CIFTextPart); !ok || text.Text != matrixAssistantPrelude {
		t.Fatalf("%s: unexpected assistant text content: %#v", shapeName, assistant.Content)
	}
	toolCall, ok := assistant.Content[1].(cif.CIFToolCallPart)
	if !ok {
		t.Fatalf("%s: expected assistant tool_call part, got %#v", shapeName, assistant.Content)
	}
	if toolCall.ToolCallID != matrixToolCallID || toolCall.ToolName != matrixToolName {
		t.Fatalf("%s: unexpected assistant tool_call: %#v", shapeName, toolCall)
	}
	if got := toolCall.ToolArguments["file_path"]; got != "README.md" {
		t.Fatalf("%s: unexpected tool_call arguments: %#v", shapeName, toolCall.ToolArguments)
	}

	toolResultMsg, ok := messages[2].(cif.CIFUserMessage)
	if !ok {
		t.Fatalf("%s: expected third canonical message to be tool_result user wrapper, got %T", shapeName, messages[2])
	}
	if len(toolResultMsg.Content) != 1 {
		t.Fatalf("%s: expected one tool_result part, got %#v", shapeName, toolResultMsg.Content)
	}
	toolResult, ok := toolResultMsg.Content[0].(cif.CIFToolResultPart)
	if !ok {
		t.Fatalf("%s: expected tool_result content part, got %#v", shapeName, toolResultMsg.Content)
	}
	if toolResult.ToolCallID != matrixToolCallID || toolResult.Content != matrixToolResultText {
		t.Fatalf("%s: unexpected tool_result content: %#v", shapeName, toolResult)
	}

	finalUser, ok := messages[3].(cif.CIFUserMessage)
	if !ok {
		t.Fatalf("%s: expected final canonical message to be user, got %T", shapeName, messages[3])
	}
	if len(finalUser.Content) != 1 {
		t.Fatalf("%s: expected one final user part, got %#v", shapeName, finalUser.Content)
	}
	if text, ok := finalUser.Content[0].(cif.CIFTextPart); !ok || text.Text != matrixFinalUserPrompt {
		t.Fatalf("%s: unexpected final user content: %#v", shapeName, finalUser.Content)
	}
}

