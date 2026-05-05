package routes

import (
	"omnillm/internal/cif"
	"omnillm/internal/providerdispatch"
)

var applyGitHubCopilotSingleUpstreamMode = providerdispatch.ApplyGitHubCopilotSingleUpstreamMode

func allowStreamingFallback(request *cif.CanonicalRequest) bool {
	if request == nil || request.Extensions == nil || request.Extensions.DisableStreamingFallback == nil {
		return true
	}

	return !*request.Extensions.DisableStreamingFallback
}
