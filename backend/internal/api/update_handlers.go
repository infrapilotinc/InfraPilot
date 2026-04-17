package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const ceImage = "infrapilothq/infrapilot-ce"

type UpdateCheckResponse struct {
	CurrentVersion  string `json:"current_version"`
	LatestPushedAt  string `json:"latest_pushed_at,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	Error           string `json:"error,omitempty"`
}

type UpdateApplyResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// checkForUpdate compares the running image digest against the latest tag on Docker Hub.
func (h *Handler) checkForUpdate(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Fetch latest tag metadata from Docker Hub (no auth needed for public repos).
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://hub.docker.com/v2/repositories/"+ceImage+"/tags/latest/", nil)
	if err != nil {
		c.JSON(http.StatusOK, UpdateCheckResponse{
			CurrentVersion: h.version,
			Error:          "Could not build request: " + err.Error(),
		})
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusOK, UpdateCheckResponse{
			CurrentVersion: h.version,
			Error:          "Could not reach Docker Hub: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	var hubData struct {
		LastUpdated string `json:"last_updated"`
		Digest      string `json:"digest"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&hubData); err != nil {
		c.JSON(http.StatusOK, UpdateCheckResponse{
			CurrentVersion: h.version,
			Error:          "Could not parse Docker Hub response",
		})
		return
	}

	// Get the repo digest of the locally running image.
	currentDigest := ""
	out, err := exec.Command("docker", "inspect",
		"--format", "{{index .RepoDigests 0}}", ceImage+":latest").Output()
	if err == nil {
		parts := strings.SplitN(strings.TrimSpace(string(out)), "@", 2)
		if len(parts) == 2 {
			currentDigest = parts[1]
		}
	}

	updateAvailable := false
	if hubData.Digest != "" && currentDigest != "" {
		updateAvailable = hubData.Digest != currentDigest
	} else {
		// Can't compare digests — flag as possibly out of date.
		updateAvailable = true
	}

	c.JSON(http.StatusOK, UpdateCheckResponse{
		CurrentVersion:  h.version,
		LatestPushedAt:  hubData.LastUpdated,
		UpdateAvailable: updateAvailable,
	})
}

// applyUpdate pulls the latest CE image then triggers a zero-downtime compose restart
// by spawning a short-lived helper container that runs docker compose up -d after a delay.
func (h *Handler) applyUpdate(c *gin.Context) {
	h.logger.Info("System update requested")

	// Step 1: Pull the latest image.
	h.logger.Info("Pulling latest image", zap.String("image", ceImage+":latest"))
	pullOut, err := exec.Command("docker", "pull", ceImage+":latest").CombinedOutput()
	if err != nil {
		h.logger.Error("Failed to pull image", zap.Error(err), zap.String("output", string(pullOut)))
		c.JSON(http.StatusInternalServerError, UpdateApplyResponse{
			Status:  "error",
			Message: "Failed to pull image: " + strings.TrimSpace(string(pullOut)),
		})
		return
	}
	h.logger.Info("Image pulled", zap.String("output", strings.TrimSpace(string(pullOut))))

	// Step 2: Locate the Docker Compose project directory from our own container labels.
	containerID, _ := os.Hostname()
	composeDir := ""
	labelOut, err := exec.Command("docker", "inspect",
		"--format", `{{index .Config.Labels "com.docker.compose.project.working_dir"}}`,
		containerID).Output()
	if err == nil {
		composeDir = strings.TrimSpace(string(labelOut))
	}

	if composeDir == "" {
		// No compose context found — image is updated but needs a manual restart.
		h.logger.Warn("Compose project dir not found; image is pulled but restart must be manual")
		c.JSON(http.StatusOK, UpdateApplyResponse{
			Status:  "pulled",
			Message: "Image updated. Please restart your container to apply: docker compose up -d",
		})
		return
	}

	// Step 3: Spawn a one-shot helper container that waits briefly, then does
	// `docker compose up -d` which stops the old container and starts one with the new image.
	helperArgs := []string{
		"run", "--rm", "-d",
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"-v", composeDir + ":" + composeDir,
		"docker:cli",
		"sh", "-c",
		fmt.Sprintf("sleep 5 && docker compose --project-directory %s up -d --pull never 2>&1", composeDir),
	}
	helperOut, err := exec.Command("docker", helperArgs...).CombinedOutput()
	if err != nil {
		h.logger.Error("Failed to spawn update helper", zap.Error(err), zap.String("output", string(helperOut)))
		c.JSON(http.StatusOK, UpdateApplyResponse{
			Status:  "pulled",
			Message: "Image updated. Restart manually: docker compose --project-directory " + composeDir + " up -d",
		})
		return
	}

	h.logger.Info("Update helper spawned; container will restart in ~5 seconds")
	c.JSON(http.StatusOK, UpdateApplyResponse{
		Status:  "restarting",
		Message: "Update applied. InfraPilot is restarting — refresh in a few seconds.",
	})
}
