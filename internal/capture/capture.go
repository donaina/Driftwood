package capture

import (
	"strings"

	"github.com/donaina/driftwood/pkg/types"
)

// SanitizeTraffic removes sensitive header tokens (Bearer, Authorization) before storage
func SanitizeTraffic(traffic *types.CapturedTraffic) {
	if traffic == nil {
		return
	}

	for k := range traffic.RequestHeaders {
		if strings.EqualFold(k, "authorization") || strings.EqualFold(k, "cookie") {
			traffic.RequestHeaders[k] = "[REDACTED]"
		}
	}
}
