package license

import (
	"context"
)

// RequireFeature always succeeds - all features available in OSS
func RequireFeature(ctx context.Context, feature string) error {
	return nil
}

// CheckFeature always returns allowed - all features available in OSS
func CheckFeature(ctx context.Context, feature string) bool {
	return true
}
