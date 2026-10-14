package admin

import (
	"context"
)

// imageStudioPortalAuthMiddleware gates the public Image Studio portal:
// page must be enabled, and the request must present a valid API key.
// When imageProxy is available, reuse /v1 auth so limits/concurrency context is set.

// portalAPIKeyAuthFallback validates the API key when imageProxy is not wired.

// PublicImageStudioPageEnabled reports whether the public Image Studio portal is enabled.
func (h *Handler) PublicImageStudioPageEnabled(ctx context.Context) (bool, error) {
	if h == nil || h.db == nil {
		return false, nil
	}
	settings, err := h.db.GetSystemSettings(ctx)
	if err != nil {
		return false, err
	}
	if settings == nil {
		return true, nil
	}
	return settings.PublicImageStudioPageEnabled, nil
}

// portal never selects admin templates by id write

// Reserve the concurrency slot before accepting the job; see the same
// sequence in CreateExternalImageJob.
