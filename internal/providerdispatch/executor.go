package providerdispatch

import (
	"context"
	"fmt"
	"strings"

	"omnillm/internal/cif"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
)

type upstreamAPIReporter interface {
	UpstreamAPI(request *cif.CanonicalRequest, model string) string
}

type Candidate struct {
	Provider       types.Provider
	Adapter        types.ProviderAdapter
	Request        *cif.CanonicalRequest
	CanonicalModel string
	UpstreamModel  string
	UpstreamAPI    string
}

type PrepareFunc func(provider types.Provider, request *cif.CanonicalRequest)
type FallbackFunc func(request *cif.CanonicalRequest) bool

type Executor struct {
	PrepareRequest       PrepareFunc
	DefaultUpstreamAPI   func(providerID, model string) string
}

func NewExecutor(prepare PrepareFunc, defaultUpstreamAPI func(providerID, model string) string) *Executor {
	return &Executor{
		PrepareRequest:     prepare,
		DefaultUpstreamAPI: defaultUpstreamAPI,
	}
}

func (e *Executor) BuildCandidate(provider types.Provider, request *cif.CanonicalRequest) (*Candidate, error) {
	if provider == nil {
		return nil, fmt.Errorf("provider is nil")
	}
	adapter := provider.GetAdapter()
	if adapter == nil {
		return nil, fmt.Errorf("provider %q has no adapter", provider.GetInstanceID())
	}

	providerRequest := *request
	if e.PrepareRequest != nil {
		e.PrepareRequest(provider, &providerRequest)
	}

	canonicalModel := providerRequest.Model
	upstreamModel := adapter.RemapModel(providerRequest.Model)
	providerRequest.Model = upstreamModel

	return &Candidate{
		Provider:       provider,
		Adapter:        adapter,
		Request:        &providerRequest,
		CanonicalModel: canonicalModel,
		UpstreamModel:  upstreamModel,
		UpstreamAPI:    e.detectUpstreamAPI(provider.GetID(), adapter, &providerRequest, upstreamModel),
	}, nil
}

func (e *Executor) Execute(ctx context.Context, candidate *Candidate) (*cif.CanonicalResponse, error) {
	response, err := candidate.Adapter.Execute(ctx, candidate.Request)
	if err != nil {
		return nil, fmt.Errorf("adapter execute failed: %w", err)
	}
	return response, nil
}

func (e *Executor) ExecuteStream(ctx context.Context, candidate *Candidate) (<-chan cif.CIFStreamEvent, error) {
	eventCh, err := candidate.Adapter.ExecuteStream(ctx, candidate.Request)
	if err != nil {
		return nil, err
	}
	return eventCh, nil
}

func (e *Executor) detectUpstreamAPI(providerID string, adapter types.ProviderAdapter, request *cif.CanonicalRequest, model string) string {
	if reporter, ok := adapter.(upstreamAPIReporter); ok {
		return reporter.UpstreamAPI(request, model)
	}
	if e.DefaultUpstreamAPI != nil {
		return e.DefaultUpstreamAPI(providerID, model)
	}
	return "chat.completions"
}

func DefaultUpstreamAPI(providerID, model string) string {
	if providerID == "azure-openai" && strings.Contains(strings.ToLower(model), "gpt-5.4") {
		return "responses"
	}
	return "chat.completions"
}

func ApplyGitHubCopilotSingleUpstreamMode(provider types.Provider, request *cif.CanonicalRequest) {
	if provider == nil || request == nil || provider.GetID() != string(types.ProviderGitHubCopilot) {
		return
	}

	if request.Extensions == nil {
		request.Extensions = &cif.Extensions{}
	}

	trueValue := true
	if !shared.IsGPT5Family(request.Model) {
		request.Extensions.ForceChatCompletions = &trueValue
	}
	request.Extensions.DisableAuthRetry = &trueValue
	request.Extensions.DisableStreamingFallback = &trueValue
}
