package api

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/infrapilot/backend/internal/license"
)

// Edition identifies this build at runtime. The Community image reports "community";
// the Enterprise build ships its own value. The frontend uses it to decide whether a
// paid key still needs a CE→EE image switch, and as the success signal after the swap.
const Edition = "community"

// upgradeStatusFile is written by the detached switch helper into the shared data
// volume (readable by whichever backend is running after the restart) so the frontend
// can distinguish a successful switch from an auto-reverted failure.
const upgradeStatusFile = ".infrapilot-upgrade.json"

// upgradeBackupDir (relative to the compose project dir) holds the pre-switch
// docker-compose.yml + .env so the helper can revert on failure.
const upgradeBackupDir = ".infrapilot-upgrade-backup"

func infrapilotBaseURL() string {
	if v := strings.TrimRight(os.Getenv("INFRAPILOT_BASE_URL"), "/"); v != "" {
		return v
	}
	return "https://infrapilot.org"
}

type eePullToken struct {
	Token    string `json:"token"`
	Registry string `json:"registry"`
	Username string `json:"username"`
	Image    string `json:"image"`   // repository WITHOUT tag, e.g. ghcr.io/infrapilothq/infrapilot-ee
	Version  string `json:"version"` // tag, e.g. v2.9.80
	Tier     string `json:"tier"`
	Error    string `json:"error"`
}

type upgradeStatus struct {
	State   string `json:"state"` // switching | completed | failed
	Tier    string `json:"tier,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	At      string `json:"at"`
}

// sseEvent writes one Server-Sent Event frame and flushes it to the client.
func sseEvent(c *gin.Context, event string, payload any) {
	b, _ := json.Marshal(payload)
	fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, b)
	if f, ok := c.Writer.(http.Flusher); ok {
		f.Flush()
	}
}

// upgradeToEnterprise performs an in-place CE→EE upgrade, streaming progress over SSE.
// The slow, failure-prone steps (validate → authenticate → pull) run while the CE
// container is still alive, so any failure aborts with nothing changed. The actual
// file edits (compose swap + .env rewrite) and the restart are delegated to a detached
// helper container, because THIS backend runs inside the all-in-one container and does
// not have the host compose directory mounted — only the helper does. The helper reverts
// to Community if the Enterprise container fails to come up.
func (h *Handler) upgradeToEnterprise(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	// The server sets a 15s WriteTimeout globally; the EE image pull takes minutes.
	rc := http.NewResponseController(c.Writer)
	_ = rc.SetWriteDeadline(time.Time{})
	c.Writer.WriteHeader(http.StatusOK)

	fail := func(step, msg string) {
		h.logger.Warn("CE→EE upgrade failed", zap.String("step", step), zap.String("error", msg))
		sseEvent(c, "error", gin.H{"step": step, "error": msg})
	}

	if h.cfg.LicenseKey != "" {
		fail("validate", "License is locked via the LICENSE_KEY environment variable. Remove it and re-run the installer with --license to upgrade.")
		return
	}

	// Resolve the paid key the operator already saved.
	key := h.license.GetKey()
	if key == "" || key == license.CommunityModeKey || key == license.SetupModeKey {
		_ = h.db.QueryRow(c.Request.Context(), `
			SELECT setting_value->>'key' FROM system_settings
			WHERE org_id = '00000000-0000-0000-0000-000000000001' AND setting_key = 'license_key'
		`).Scan(&key)
	}
	if key == "" {
		fail("validate", "No license key found. Enter your paid license key and save it first.")
		return
	}

	// Step 1 — validate the key and obtain a short-lived EE registry pull token.
	sseEvent(c, "step", gin.H{"step": "validate", "status": "running", "label": "Validating license"})
	tok, err := h.requestEEPullToken(c.Request.Context(), key)
	if err != nil {
		fail("validate", err.Error())
		return
	}
	if tok.Tier == "community" || tok.Tier == "" {
		fail("validate", "This key is a Community key — there is nothing to upgrade. Enter a paid (Pro/Business/Enterprise) key.")
		return
	}
	version := tok.Version
	if version == "" {
		version = "latest"
	}
	fullImage := tok.Image + ":" + version
	sseEvent(c, "step", gin.H{"step": "validate", "status": "done", "tier": tok.Tier})

	// Step 2 — authenticate to the private registry.
	sseEvent(c, "step", gin.H{"step": "authenticate", "status": "running", "label": "Authenticating with registry"})
	login := exec.Command("docker", "login", tok.Registry, "-u", tok.Username, "--password-stdin")
	login.Stdin = strings.NewReader(tok.Token)
	if out, lerr := login.CombinedOutput(); lerr != nil {
		fail("authenticate", "registry login failed: "+strings.TrimSpace(string(out)))
		return
	}
	sseEvent(c, "step", gin.H{"step": "authenticate", "status": "done"})

	// Step 3 — pull the Enterprise image (streamed; nothing swapped yet).
	sseEvent(c, "step", gin.H{"step": "download", "status": "running", "label": "Downloading Enterprise image"})
	if perr := h.streamDockerPull(c, fullImage); perr != nil {
		fail("download", "image download failed: "+perr.Error())
		return
	}
	sseEvent(c, "step", gin.H{"step": "download", "status": "done"})

	// Step 4 — swap compose to Enterprise and restart via a detached, self-reverting helper.
	sseEvent(c, "step", gin.H{"step": "switch", "status": "running", "label": "Switching to Enterprise"})
	composeDir, derr := h.composeProjectDir()
	if derr != nil || composeDir == "" {
		fail("switch", "Enterprise image is downloaded, but the compose project directory could not be located. Finish by re-running: curl -fsSL https://infrapilot.sh | bash -s -- --license "+maskLicenseKey(key))
		return
	}
	eeCompose, cerr := httpGetBytes(c.Request.Context(), infrapilotBaseURL()+"/docker-compose.install.ee.yml")
	if cerr != nil || len(eeCompose) == 0 {
		fail("switch", "could not download the Enterprise compose file. Finish by re-running: curl -fsSL https://infrapilot.sh | bash -s -- --license "+maskLicenseKey(key))
		return
	}

	// Record "switching" in the shared data volume so a reverted CE can report failure.
	_ = writeStatus(filepath.Join(h.cfg.DataDir, upgradeStatusFile), upgradeStatus{
		State: "switching", Tier: tok.Tier, At: time.Now().UTC().Format(time.RFC3339),
	})

	if serr := h.spawnSwitchHelper(composeDir, key, tok.Image, version, tok.Tier, eeCompose); serr != nil {
		fail("switch", "could not launch the restart helper: "+serr.Error())
		return
	}

	sseEvent(c, "done", gin.H{
		"tier":    tok.Tier,
		"message": "Enterprise image ready. InfraPilot is switching over and will restart — reconnecting…",
	})
}

// getUpgradeStatus returns the outcome recorded by the switch helper (via the shared
// data volume). The frontend polls this across the restart to distinguish a successful
// switch from an auto-reverted failure. On the Enterprise image this route does not
// exist (404), which is fine — success is confirmed via the public /version endpoint.
func (h *Handler) getUpgradeStatus(c *gin.Context) {
	b, rerr := os.ReadFile(filepath.Join(h.cfg.DataDir, upgradeStatusFile))
	if rerr != nil {
		c.JSON(http.StatusOK, gin.H{"state": "none", "edition": Edition})
		return
	}
	var st upgradeStatus
	if json.Unmarshal(b, &st) != nil {
		c.JSON(http.StatusOK, gin.H{"state": "none", "edition": Edition})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"state": st.State, "tier": st.Tier, "message": st.Message,
		"error": st.Error, "at": st.At, "edition": Edition,
	})
}

func (h *Handler) requestEEPullToken(ctx context.Context, key string) (*eePullToken, error) {
	instanceID := os.Getenv("INSTANCE_ID")
	hostname, _ := os.Hostname()
	if instanceID == "" {
		instanceID = hostname
	}
	body, _ := json.Marshal(map[string]string{
		"key": key, "instance_id": instanceID, "hostname": hostname, "version": h.version,
	})

	reqCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		infrapilotBaseURL()+"/api/license/ee-pull-token", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach the license server: %w", err)
	}
	defer resp.Body.Close()

	var tok eePullToken
	if derr := json.NewDecoder(resp.Body).Decode(&tok); derr != nil {
		return nil, fmt.Errorf("unexpected response from the license server")
	}
	if tok.Token == "" {
		if tok.Error != "" {
			return nil, fmt.Errorf("%s", tok.Error)
		}
		return nil, fmt.Errorf("license did not grant Enterprise access (is the subscription active?)")
	}
	if tok.Image == "" {
		return nil, fmt.Errorf("license server did not return an Enterprise image reference")
	}
	return &tok, nil
}

// streamDockerPull runs `docker pull` and forwards its per-layer status lines as SSE
// progress events (throttled). Docker renders live progress bars with carriage returns,
// so the newline-based scanner mostly surfaces layer completions — enough for the UI.
func (h *Handler) streamDockerPull(c *gin.Context, image string) error {
	cmd := exec.Command("docker", "pull", image)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if serr := cmd.Start(); serr != nil {
		return serr
	}

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	last := time.Time{}
	lastLine := ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		lastLine = line // keep the tail so a failure surfaces docker's real message
		if time.Since(last) > 350*time.Millisecond {
			sseEvent(c, "progress", gin.H{"step": "download", "line": line})
			last = time.Now()
		}
	}
	if werr := cmd.Wait(); werr != nil {
		if lastLine != "" {
			return fmt.Errorf("%s (%v)", lastLine, werr)
		}
		return werr
	}
	return nil
}

// composeProjectDir reads the compose working dir (the HOST path) from our own
// container labels, matching how applyUpdate locates it. This path is only usable
// from a helper that bind-mounts it — not from inside this container.
func (h *Handler) composeProjectDir() (string, error) {
	id, _ := os.Hostname()
	out, err := exec.Command("docker", "inspect",
		"--format", `{{index .Config.Labels "com.docker.compose.project.working_dir"}}`, id).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// dataVolumeName finds the Docker volume backing our /data mount, so the helper can
// bind it and write the upgrade status where the (post-restart) backend can read it.
func (h *Handler) dataVolumeName() string {
	id, _ := os.Hostname()
	out, err := exec.Command("docker", "inspect", "--format",
		`{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}`, id).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// spawnSwitchHelper launches a short-lived docker:cli container that (after a brief
// delay so this response can flush) backs up the current compose/.env, installs the
// Enterprise compose + .env, and runs `docker compose up -d`. If the Enterprise
// container fails to start it restores the backup and brings Community back, recording
// the outcome in the shared data volume for the frontend to read after reconnecting.
func (h *Handler) spawnSwitchHelper(dir, key, eeImage, version, tier string, eeCompose []byte) error {
	now := time.Now().UTC().Format(time.RFC3339)
	composeB64 := base64.StdEncoding.EncodeToString(eeCompose)

	// Where the helper writes the final status. If we can bind the data volume, write
	// there (readable by the post-restart backend); otherwise fall back to the compose
	// dir (success is still confirmed via /version regardless).
	dataVol := h.dataVolumeName()
	statusOut := filepath.Join(dir, upgradeStatusFile)
	statusMounts := []string{}
	if dataVol != "" {
		statusOut = "/upgrade-status/" + upgradeStatusFile
		statusMounts = []string{"-v", dataVol + ":/upgrade-status"}
	}

	// The helper swaps compose/.env, brings up Enterprise, then HEALTH-GATES the switch:
	// it waits up to ~150s for the service to report healthy. A container that starts but
	// then crash-loops (e.g. a failed migration) stays "unhealthy" — so we revert to the
	// backed-up Community compose/.env instead of leaving a broken box. Backups are made
	// world-readable so an operator can also revert by hand without root.
	revert := `cp ` + upgradeBackupDir + `/docker-compose.yml docker-compose.yml 2>/dev/null || true
  cp ` + upgradeBackupDir + `/.env .env 2>/dev/null || true
  docker compose up -d || true`

	script := fmt.Sprintf(`set -e
sleep 3
cd %[1]q
mkdir -p %[2]s
cp docker-compose.yml %[2]s/ 2>/dev/null || true
cp .env %[2]s/ 2>/dev/null || true
chmod -R a+r %[2]s 2>/dev/null || true
echo %[3]q | base64 -d > docker-compose.yml
touch .env
grep -vE '^(EE_IMAGE|LICENSE_KEY|INFRAPILOT_VERSION)=' .env > .env.next 2>/dev/null || true
mv .env.next .env 2>/dev/null || true
printf 'EE_IMAGE=%%s\nLICENSE_KEY=%%s\nINFRAPILOT_VERSION=%%s\n' %[4]q %[5]q %[6]q >> .env
if docker compose up -d --pull never; then
  cid=$(docker compose ps -q infrapilot 2>/dev/null)
  ok=0
  n=0
  while [ $n -lt 30 ]; do
    sleep 5
    st=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$cid" 2>/dev/null || echo gone)
    if [ "$st" = "healthy" ] || [ "$st" = "running" ]; then ok=1; break; fi
    n=$((n+1))
  done
  if [ "$ok" = "1" ]; then
    printf '{"state":"completed","tier":%[7]q,"at":%[8]q}' > %[9]s
  else
    %[10]s
    printf '{"state":"failed","tier":%[7]q,"error":"Enterprise did not become healthy in time (likely a database/migration incompatibility) — reverted to Community Edition.","at":%[8]q}' > %[9]s
  fi
else
  %[10]s
  printf '{"state":"failed","tier":%[7]q,"error":"Enterprise container failed to start; reverted to Community Edition.","at":%[8]q}' > %[9]s
fi`,
		dir, upgradeBackupDir, composeB64, eeImage, key, version, tier, now, statusOut, revert)

	helperArgs := []string{
		"run", "--rm", "-d",
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"-v", dir + ":" + dir,
	}
	helperArgs = append(helperArgs, statusMounts...)
	helperArgs = append(helperArgs, "docker:cli", "sh", "-c", script)

	if out, err := exec.Command("docker", helperArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	h.logger.Info("CE→EE switch helper spawned; container will restart shortly", zap.String("tier", tier))
	return nil
}

// ---- small helpers -------------------------------------------------------

func httpGetBytes(ctx context.Context, url string) ([]byte, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func writeStatus(path string, st upgradeStatus) error {
	b, _ := json.Marshal(st)
	return os.WriteFile(path, b, 0o600)
}
