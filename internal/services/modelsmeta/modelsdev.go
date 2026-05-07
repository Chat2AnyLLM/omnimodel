package modelsmeta

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultModelsDevURL = "https://models.dev/api.json"
	defaultCacheTTL     = 30 * time.Minute
)

// DefaultService is the package-level singleton used by adapters and routes.
var DefaultService = NewService()

type ModelMetadata struct {
	ID                        string   `json:"id"`
	Name                      string   `json:"name,omitempty"`
	ProviderID                string   `json:"provider_id"`
	ProviderName              string   `json:"provider_name,omitempty"`
	Family                    string   `json:"family,omitempty"`
	InputPriceUSDPer1MTokens  *float64 `json:"input_price_usd_per_1m_tokens,omitempty"`
	OutputPriceUSDPer1MTokens *float64 `json:"output_price_usd_per_1m_tokens,omitempty"`
	CacheReadUSDPer1MTokens   *float64 `json:"cache_read_usd_per_1m_tokens,omitempty"`
	ContextLimitTokens        *int     `json:"context_limit_tokens,omitempty"`
	InputLimitTokens          *int     `json:"input_limit_tokens,omitempty"`
	OutputLimitTokens         *int     `json:"output_limit_tokens,omitempty"`
	SupportsToolCall          *bool    `json:"supports_tool_call,omitempty"`
	SupportsStructuredOutput  *bool    `json:"supports_structured_output,omitempty"`
	SupportsReasoning         *bool    `json:"supports_reasoning,omitempty"`
	SupportsAttachments       *bool    `json:"supports_attachments,omitempty"`
	KnowledgeCutoff           string   `json:"knowledge_cutoff,omitempty"`
	ReleaseDate               string   `json:"release_date,omitempty"`
	LastUpdated               string   `json:"last_updated,omitempty"`
	InputModalities           []string `json:"input_modalities,omitempty"`
	OutputModalities          []string `json:"output_modalities,omitempty"`
	OpenWeights               *bool    `json:"open_weights,omitempty"`
}

type Result struct {
	FetchedAt time.Time       `json:"fetched_at"`
	Models    []ModelMetadata `json:"models"`
}

type Service struct {
	url      string
	cacheTTL time.Duration
	http     *http.Client

	cacheMu   sync.RWMutex
	cachedAt  time.Time
	cachedRes Result
	// index maps lowercase model ID to its metadata for O(1) lookups.
	index map[string]*ModelMetadata
}

func NewService() *Service {
	return &Service{
		url:      defaultModelsDevURL,
		cacheTTL: defaultCacheTTL,
		http: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (s *Service) Get(ctx context.Context, forceRefresh bool) (Result, error) {
	if !forceRefresh {
		if cached, ok := s.getFromCache(); ok {
			return cached, nil
		}
	}

	fetched, err := s.fetch(ctx)
	if err != nil {
		if cached, ok := s.getFromCache(); ok {
			return cached, nil
		}
		return Result{}, err
	}

	s.cacheMu.Lock()
	s.cachedAt = time.Now()
	s.cachedRes = fetched
	s.index = buildIndex(fetched.Models)
	s.cacheMu.Unlock()

	return fetched, nil
}

// LookupModel returns the ModelMetadata for modelID (case-insensitive) from
// the cached models.dev result, or nil if not found or the fetch fails.
func (s *Service) LookupModel(ctx context.Context, modelID string) *ModelMetadata {
	// Ensure the cache (and index) is warm.
	if _, err := s.Get(ctx, false); err != nil {
		return nil
	}
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	return s.index[strings.ToLower(strings.TrimSpace(modelID))]
}

func (s *Service) getFromCache() (Result, bool) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	if len(s.cachedRes.Models) == 0 {
		return Result{}, false
	}
	if time.Since(s.cachedAt) > s.cacheTTL {
		return Result{}, false
	}

	return s.cachedRes, true
}

func (s *Service) fetch(ctx context.Context) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return Result{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "omnillm-modelsmeta/1.0")

	resp, err := s.http.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("fetch models.dev metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("models.dev returned status %d", resp.StatusCode)
	}

	var raw map[string]modelsDevProvider
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Result{}, fmt.Errorf("decode models.dev payload: %w", err)
	}

	result := Result{
		FetchedAt: time.Now().UTC(),
		Models:    flatten(raw),
	}

	return result, nil
}

// buildIndex creates a lowercase-ID-keyed map for O(1) LookupModel calls.
func buildIndex(models []ModelMetadata) map[string]*ModelMetadata {
	idx := make(map[string]*ModelMetadata, len(models))
	for i := range models {
		key := strings.ToLower(models[i].ID)
		if _, exists := idx[key]; !exists {
			idx[key] = &models[i]
		}
	}
	return idx
}

type modelsDevProvider struct {
	ID     string                   `json:"id"`
	Name   string                   `json:"name"`
	Models map[string]modelsDevItem `json:"models"`
}

type modelsDevItem struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Family           string `json:"family"`
	Knowledge        string `json:"knowledge"`
	ReleaseDate      string `json:"release_date"`
	LastUpdated      string `json:"last_updated"`
	ToolCall         *bool  `json:"tool_call"`
	StructuredOutput *bool  `json:"structured_output"`
	Reasoning        *bool  `json:"reasoning"`
	Attachment       *bool  `json:"attachment"`
	OpenWeights      *bool  `json:"open_weights"`
	Cost             struct {
		Input     json.Number `json:"input"`
		Output    json.Number `json:"output"`
		CacheRead json.Number `json:"cache_read"`
	} `json:"cost"`
	Limit struct {
		Context json.Number `json:"context"`
		Input   json.Number `json:"input"`
		Output  json.Number `json:"output"`
	} `json:"limit"`
	Modalities struct {
		Input  []string `json:"input"`
		Output []string `json:"output"`
	} `json:"modalities"`
}

func flatten(raw map[string]modelsDevProvider) []ModelMetadata {
	all := make([]ModelMetadata, 0)

	for key, provider := range raw {
		providerID := firstNonEmpty(provider.ID, key)
		providerName := firstNonEmpty(provider.Name, providerID)

		for modelKey, item := range provider.Models {
			modelID := firstNonEmpty(item.ID, modelKey)
			if modelID == "" {
				continue
			}

			all = append(all, ModelMetadata{
				ID:                        modelID,
				Name:                      item.Name,
				ProviderID:                providerID,
				ProviderName:              providerName,
				Family:                    item.Family,
				InputPriceUSDPer1MTokens:  parseOptionalFloat(item.Cost.Input),
				OutputPriceUSDPer1MTokens: parseOptionalFloat(item.Cost.Output),
				CacheReadUSDPer1MTokens:   parseOptionalFloat(item.Cost.CacheRead),
				ContextLimitTokens:        parseOptionalInt(item.Limit.Context),
				InputLimitTokens:          parseOptionalInt(item.Limit.Input),
				OutputLimitTokens:         parseOptionalInt(item.Limit.Output),
				SupportsToolCall:          item.ToolCall,
				SupportsStructuredOutput:  item.StructuredOutput,
				SupportsReasoning:         item.Reasoning,
				SupportsAttachments:       item.Attachment,
				KnowledgeCutoff:           item.Knowledge,
				ReleaseDate:               item.ReleaseDate,
				LastUpdated:               item.LastUpdated,
				InputModalities:           item.Modalities.Input,
				OutputModalities:          item.Modalities.Output,
				OpenWeights:               item.OpenWeights,
			})
		}
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].ID == all[j].ID {
			return all[i].ProviderID < all[j].ProviderID
		}
		return all[i].ID < all[j].ID
	})

	return all
}

func parseOptionalFloat(n json.Number) *float64 {
	str := strings.TrimSpace(n.String())
	if str == "" {
		return nil
	}
	v, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return nil
	}
	return &v
}

func parseOptionalInt(n json.Number) *int {
	str := strings.TrimSpace(n.String())
	if str == "" {
		return nil
	}
	v, err := strconv.Atoi(str)
	if err != nil {
		f, ferr := strconv.ParseFloat(str, 64)
		if ferr != nil {
			return nil
		}
		i := int(f)
		return &i
	}
	return &v
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v != "" {
			return v
		}
	}
	return ""
}
