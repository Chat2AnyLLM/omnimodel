package routes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"omnillm/internal/cif"
	"omnillm/internal/ingestion"
	"omnillm/internal/lib/modelrouting"
	"omnillm/internal/providerdispatch"
	"omnillm/internal/providers/types"
	"omnillm/internal/serialization"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func SetupResponseRoutes(router *gin.RouterGroup) {
	router.POST("/responses", handleResponses)
}

func handleResponses(c *gin.Context) {
	// Type assertion is zero-allocation vs fmt.Sprintf("%v", requestID)
	requestID, _ := c.Get("request_id")
	requestIDStr, _ := requestID.(string)
	startTime := time.Now()

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Error().Err(err).Str("request_id", requestIDStr).Msg("Failed to read request body")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request format",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Convert Responses API format to CIF.
	// json.Valid is omitted: ParseResponsesPayload calls json.Unmarshal which
	// already validates syntax and returns a clear error, avoiding a double parse pass.
	canonicalRequest, err := ingestion.ParseResponsesPayload(body)
	if err != nil {
		log.Error().Err(err).Str("request_id", requestIDStr).Msg("Failed to parse Responses API request")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": parseRequestMessage(err),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	originalModel := prepareCanonicalRequest(c, canonicalRequest, "responses")

	// Resolve providers
	resolvedModel, normalizedModel := resolveRequestedModel(requestIDStr, canonicalRequest.Model)
	canonicalRequest.Model = resolvedModel
	modelRoute, err := modelrouting.ResolveProvidersForModel(
		canonicalRequest.Model,
		normalizedModel,
		"",
		modelCache,
	)
	if err != nil {
		log.Error().Err(err).Str("request_id", requestIDStr).Str("model", canonicalRequest.Model).Msg("Failed to resolve providers")
		writeResolveProvidersError(c, err, "server_error")
		return
	}

	if len(modelRoute.CandidateProviders) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Model '%s' not found or no providers available", canonicalRequest.Model),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	if normalizedModel != canonicalRequest.Model {
		log.Debug().
			Str("request_id", requestIDStr).
			Str("from", canonicalRequest.Model).
			Str("to", normalizedModel).
			Msg("Normalized Responses API request model")
		canonicalRequest.Model = normalizedModel
	}

	// Try candidate providers
	executor := providerdispatch.NewExecutor(providerdispatch.ApplyGitHubCopilotSingleUpstreamMode, providerdispatch.DefaultUpstreamAPI)
	resolveFailed := false
	lastErr := executor.TryAttempts(
		[]providerdispatch.Attempt{{
			RequestedModel:  canonicalRequest.Model,
			NormalizedModel: normalizedModel,
			ProviderID:      "",
		}},
		canonicalRequest,
		modelCache,
		modelrouting.ResolveProvidersForModel,
		nil,
		func(attempt providerdispatch.Attempt, err error) {
			resolveFailed = true
			log.Error().Err(err).Str("request_id", requestIDStr).Str("model", attempt.RequestedModel).Msg("Failed to resolve providers")
		},
		func(candidate *providerdispatch.Candidate, providerID string) error {
			log.Debug().
				Str("request_id", requestIDStr).
				Str("model", candidate.CanonicalModel).
				Str("provider", providerID).
				Msg("Trying provider for Responses API request")

			log.Debug().
				Str("request_id", requestIDStr).
				Str("provider", providerID).
				Str("api_shape", "responses").
				Str("inbound_path", c.FullPath()).
				Str("upstream_api", candidate.UpstreamAPI).
				Str("canonical_model", candidate.CanonicalModel).
				Str("upstream_model", candidate.UpstreamModel).
				Msg("Converted CIF request to upstream model API")

			var err error
			if candidate.Request.Stream {
				err = handleResponsesStreamingResponse(c, candidate.Adapter, candidate.Request, requestIDStr, originalModel, providerID, startTime)
			} else {
				err = handleResponsesNonStreamingResponse(c, candidate.Adapter, candidate.Request, requestIDStr, originalModel, providerID, startTime)
			}
			if err != nil {
				log.Warn().Err(err).
					Str("request_id", requestIDStr).
					Str("provider", providerID).
					Str("upstream_model", candidate.UpstreamModel).
					Msg("Provider failed for Responses API request, trying next")
			}
			return err
		},
	)
	if lastErr == nil {
		return
	}
	if resolveFailed {
		writeResolveProvidersError(c, lastErr, "server_error")
		return
	}
	if strings.Contains(lastErr.Error(), "not found or no providers available") {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": lastErr.Error(),
				"type":    "invalid_request_error",
			},
		})
		return
	}
	writeProviderFailure(c, "server_error", lastErr)
}

//nolint:dupl // structurally similar to chat.go but serves different API shape (responses vs chat)
func handleResponsesNonStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID, originalModel, providerID string, startTime time.Time) error {
	response, err := adapter.Execute(c.Request.Context(), canonicalRequest)
	if err != nil {
		return fmt.Errorf("adapter execute failed: %w", err)
	}

	responsesResp, err := serialization.SerializeToResponses(response)
	if err != nil {
		return fmt.Errorf("serialization failed: %w", err)
	}

	logCompletedResponse("responses", requestID, originalModel, response.Model, providerID, false, response.StopReason, response.Usage, startTime)
	recordUsage(requestID, originalModel, response.Model, providerID, normalizeMeteringClient(c.GetHeader("User-Agent")), "responses", response.Usage, time.Since(startTime).Milliseconds(), false, http.StatusOK, "")

	c.JSON(http.StatusOK, responsesResp)
	return nil
}

func handleResponsesStreamingResponse(c *gin.Context, adapter types.ProviderAdapter, canonicalRequest *cif.CanonicalRequest, requestID, originalModel, providerID string, startTime time.Time) error {
	eventCh, err := adapter.ExecuteStream(c.Request.Context(), canonicalRequest)
	if err != nil {
		if shouldFallbackToNonStreaming(err) {
			log.Warn().Err(err).Str("request_id", requestID).Msg("Streaming request failed before stream start, retrying as non-streaming")
			canonicalRequest.Stream = false
			return handleResponsesNonStreamingResponse(c, adapter, canonicalRequest, requestID, originalModel, providerID, startTime)
		}
		return err
	}

	setSSEHeaders(c, false)

	state := serialization.CreateResponsesStreamState()
	flusher, _ := c.Writer.(http.Flusher)
	modelUsed := canonicalRequest.Model
	ctx := c.Request.Context()

	c.Stream(func(w io.Writer) bool {
		select {
		case <-ctx.Done():
			return false
		case event, ok := <-eventCh:
			if !ok {
				return false
			}

			responsesEvents, err := serialization.ConvertCIFEventToResponsesSSE(event, state)
			if err != nil {
				log.Error().Err(err).Str("request_id", requestID).Msg("Failed to convert CIF event to Responses SSE")
				return false
			}

			var sb strings.Builder
			for _, evt := range responsesEvents {
				eventType, _ := evt["type"].(string)
				jsonBytes, err := json.Marshal(evt)
				if err != nil {
					continue
				}
				sb.WriteString("event: ")
				sb.WriteString(eventType)
				sb.WriteString("\ndata: ")
				sb.Write(jsonBytes)
				sb.WriteString("\n\n")
			}
			io.WriteString(w, sb.String())

			if flusher != nil {
				flusher.Flush()
			}

			if endEvt, isEnd := event.(cif.CIFStreamEnd); isEnd {
				inputTokens := 0
				outputTokens := 0
				if endEvt.Usage != nil {
					inputTokens = endEvt.Usage.InputTokens
					outputTokens = endEvt.Usage.OutputTokens
				}

				log.Info().
					Str("request_id", requestID).
					Str("api_shape", "responses").
					Str("model_requested", originalModel).
					Str("model_used", modelUsed).
					Str("provider", providerID).
					Str("stop_reason", string(endEvt.StopReason)).
					Bool("stream", true).
					Int("input_tokens", inputTokens).
					Int("output_tokens", outputTokens).
					Int64("latency_ms", time.Since(startTime).Milliseconds()).
					Msg("\x1b[32m<--\x1b[0m RESPONSE stream")
				recordUsage(requestID, originalModel, modelUsed, providerID, normalizeMeteringClient(c.GetHeader("User-Agent")), "responses", endEvt.Usage, time.Since(startTime).Milliseconds(), true, http.StatusOK, "")
				return false
			}

			if _, isErr := event.(cif.CIFStreamError); isErr {
				return false
			}

			return true
		}
	})

	return nil
}
