// Package telemetry emits anonymous, opt-out product-funnel events to infrapilot.org.
//
// PRIVACY CONTRACT (see v3/40): only an anonymous instance UUID + a funnel stage + coarse
// categorical metadata ever leave the box — never app names, repo URLs, env vars, or any
// user infra content. Disabled entirely with INFRAPILOT_TELEMETRY=off. Every send is
// fire-and-forget: it never blocks a request and silently drops on error.
//
// This channel is deliberately independent of the license-validate heartbeat, because
// keyless Community Edition never validates — so without this, the free funnel is invisible.
package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const defaultBaseURL = "https://infrapilot.org"

// Client is an async, best-effort telemetry emitter. A nil *Client is safe to call.
type Client struct {
	enabled    bool
	instanceID string
	edition    string
	version    string
	baseURL    string
	dataDir    string
	tierFn     func() string
	logger     *zap.Logger
}

// New builds a telemetry client. edition is the build edition ("community"); tierFn returns
// the current license tier at emit time (so heartbeats reflect a later paid upgrade).
func New(dataDir, edition, version string, tierFn func() string, logger *zap.Logger) *Client {
	enabled := true
	switch strings.ToLower(strings.TrimSpace(os.Getenv("INFRAPILOT_TELEMETRY"))) {
	case "off", "0", "false", "no", "disabled":
		enabled = false
	}
	base := strings.TrimRight(os.Getenv("INFRAPILOT_BASE_URL"), "/")
	if base == "" {
		base = defaultBaseURL
	}
	if tierFn == nil {
		tierFn = func() string { return "community" }
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	c := &Client{
		enabled:    enabled,
		instanceID: resolveInstanceID(dataDir),
		edition:    edition,
		version:    version,
		baseURL:    base,
		dataDir:    dataDir,
		tierFn:     tierFn,
		logger:     logger,
	}
	if !enabled {
		logger.Info("Telemetry disabled (INFRAPILOT_TELEMETRY=off)")
	}
	return c
}

// resolveInstanceID returns a stable anonymous id: the install-provided INSTANCE_ID env if
// present, else a UUID persisted under DataDir (so keyless installs are still stable).
func resolveInstanceID(dataDir string) string {
	if v := strings.TrimSpace(os.Getenv("INSTANCE_ID")); v != "" && v != "community" {
		return v
	}
	p := filepath.Join(dataDir, ".instance_id")
	if b, err := os.ReadFile(p); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	id := uuid.NewString()
	_ = os.MkdirAll(dataDir, 0o755)
	_ = os.WriteFile(p, []byte(id), 0o600)
	return id
}

// Emit sends one event, fire-and-forget.
func (c *Client) Emit(eventType string, meta map[string]any) {
	if c == nil || !c.enabled {
		return
	}
	p := map[string]any{
		"instance_id": c.instanceID,
		"type":        eventType,
		"edition":     c.edition,
		"tier":        c.tierFn(),
		"version":     c.version,
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
		"ts":          time.Now().UTC().Format(time.RFC3339),
	}
	if len(meta) > 0 {
		p["meta"] = meta
	}
	go c.post(p)
}

// EmitOnce emits an event at most once per instance, guarded by a marker file in DataDir.
// The marker's contents are the first-emit timestamp (used for days_since_install).
func (c *Client) EmitOnce(eventType string, meta map[string]any) {
	if c == nil || !c.enabled {
		return
	}
	marker := filepath.Join(c.dataDir, ".tel_"+sanitize(eventType))
	if _, err := os.Stat(marker); err == nil {
		return // already emitted on a prior boot
	}
	_ = os.WriteFile(marker, []byte(time.Now().UTC().Format(time.RFC3339)), 0o600)
	c.Emit(eventType, meta)
}

// StartHeartbeat emits one heartbeat immediately, then every 24h until ctx is cancelled.
func (c *Client) StartHeartbeat(ctx context.Context) {
	if c == nil || !c.enabled {
		return
	}
	c.heartbeat()
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.heartbeat()
		}
	}
}

func (c *Client) heartbeat() {
	c.Emit("heartbeat", map[string]any{"days_since_install": c.daysSinceInstall()})
}

func (c *Client) daysSinceInstall() int {
	marker := filepath.Join(c.dataDir, ".tel_installed")
	if b, err := os.ReadFile(marker); err == nil {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(b))); err == nil {
			return int(time.Since(t).Hours() / 24)
		}
	}
	return 0
}

func (c *Client) post(p map[string]any) {
	defer func() { _ = recover() }()
	body, err := json.Marshal(p)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/telemetry/event", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.logger.Debug("telemetry emit failed", zap.String("type", asString(p["type"])), zap.Error(err))
		return
	}
	_ = resp.Body.Close()
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, strings.ToLower(s))
}
