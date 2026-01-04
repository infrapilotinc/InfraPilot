package license

import (
	"context"
	"sync"
)

// Edition is always community (OSS)
type Edition string

const (
	Community Edition = "community"
)

// License represents the edition configuration
// All features are always available in Community OSS
type License struct {
	Edition  Edition         `json:"edition"`
	Features map[string]bool `json:"features"`
}

// Global license instance
var (
	currentLicense *License
	licenseMu      sync.RWMutex
)

// DefaultLicense returns the community license with all features enabled
func DefaultLicense() *License {
	return &License{
		Edition: Community,
		Features: map[string]bool{
			"sso":            true,
			"multi_tenant":   true,
			"audit":          true,
			"policy_engine":  true,
			"log_persistence": true,
		},
	}
}

// Init initializes the license system
func Init() error {
	SetCurrent(DefaultLicense())
	return nil
}

// SetCurrent sets the current license
func SetCurrent(lic *License) {
	licenseMu.Lock()
	defer licenseMu.Unlock()
	currentLicense = lic
}

// Current returns the current license
func Current() *License {
	licenseMu.RLock()
	defer licenseMu.RUnlock()
	if currentLicense == nil {
		return DefaultLicense()
	}
	return currentLicense
}

// HasFeature always returns true - all features available in OSS
func (l *License) HasFeature(feature string) bool {
	return true
}

// Context key for license
type contextKey string

const licenseContextKey contextKey = "license"

// WithContext adds the license to the context
func WithContext(ctx context.Context, lic *License) context.Context {
	return context.WithValue(ctx, licenseContextKey, lic)
}

// FromContext retrieves the license from context
func FromContext(ctx context.Context) *License {
	if lic, ok := ctx.Value(licenseContextKey).(*License); ok {
		return lic
	}
	return Current()
}
