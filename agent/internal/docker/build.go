package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BuildImageConfig describes a source build request from the backend.
type BuildImageConfig struct {
	RepoURL        string            // git clone URL
	Ref            string            // branch/tag
	Commit         string            // commit SHA (optional)
	DockerfilePath string            // optional, relative to repo root
	BuildArgs      map[string]string // optional build args / nixpacks env
	ImageTag       string            // local tag to produce, e.g. infrapilot-build/web-prod:abc1234
	GitToken       string            // optional, for private repos (masked in logs)
	WorkDir        string            // base dir for checkouts (agent DataDir/builds)
}

// BuildImageResult is returned to the backend (also carried on failure for logs).
type BuildImageResult struct {
	ImageRef string `json:"image_ref"`
	ImageID  string `json:"image_id"`
	Log      string `json:"log"`
	Success  bool   `json:"success"`
	Message  string `json:"message"`
}

// BuildImage clones a git repo and builds a LOCAL image: Dockerfile if present,
// otherwise Nixpacks. The image is tagged locally (registry-less) so the deploy
// step runs it without a registry pull. The git token, if any, is masked in logs.
func (c *Client) BuildImage(ctx context.Context, cfg BuildImageConfig) (*BuildImageResult, error) {
	res := &BuildImageResult{ImageRef: cfg.ImageTag}
	var log strings.Builder

	if cfg.RepoURL == "" || cfg.ImageTag == "" {
		return nil, fmt.Errorf("repo_url and image_tag are required")
	}

	base := cfg.WorkDir
	if base == "" {
		base = filepath.Join(os.TempDir(), "infrapilot-builds")
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, fmt.Errorf("create build dir: %w", err)
	}
	tmp, err := os.MkdirTemp(base, "build-")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	cloneURL := cfg.RepoURL
	if cfg.GitToken != "" {
		cloneURL = injectGitToken(cfg.RepoURL, cfg.GitToken)
	}

	// 1. Shallow clone (token masked in the recorded log).
	cloneArgs := []string{"clone", "--depth", "1"}
	if cfg.Ref != "" {
		cloneArgs = append(cloneArgs, "--branch", cfg.Ref)
	}
	cloneArgs = append(cloneArgs, cloneURL, tmp)
	if err := runCmd(ctx, &log, base, cfg.GitToken, "git", cloneArgs...); err != nil {
		res.Log, res.Message = log.String(), "git clone failed"
		return res, fmt.Errorf("git clone failed: %w", err)
	}
	if cfg.Commit != "" {
		// Shallow clone may not contain the commit; best-effort fetch + checkout.
		_ = runCmd(ctx, &log, tmp, cfg.GitToken, "git", "fetch", "--depth", "1", "origin", cfg.Commit)
		if err := runCmd(ctx, &log, tmp, cfg.GitToken, "git", "checkout", cfg.Commit); err != nil {
			log.WriteString(fmt.Sprintf("(warning) could not checkout %s; using %s HEAD\n", cfg.Commit, cfg.Ref))
		}
	}

	// 2. Build: Dockerfile if present, else Nixpacks.
	dockerfile := cfg.DockerfilePath
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	if _, statErr := os.Stat(filepath.Join(tmp, dockerfile)); statErr == nil {
		args := []string{"build", "-t", cfg.ImageTag, "-f", filepath.Join(tmp, dockerfile)}
		for k, v := range cfg.BuildArgs {
			args = append(args, "--build-arg", fmt.Sprintf("%s=%s", k, v))
		}
		args = append(args, tmp)
		log.WriteString("Building with Dockerfile\n")
		if err := runCmd(ctx, &log, tmp, cfg.GitToken, "docker", args...); err != nil {
			res.Log, res.Message = log.String(), "docker build failed"
			return res, fmt.Errorf("docker build failed: %w", err)
		}
	} else {
		args := []string{"build", tmp, "--name", cfg.ImageTag}
		for k, v := range cfg.BuildArgs {
			args = append(args, "--env", fmt.Sprintf("%s=%s", k, v))
		}
		log.WriteString("No Dockerfile found — building with Nixpacks\n")
		if err := runCmd(ctx, &log, tmp, cfg.GitToken, "nixpacks", args...); err != nil {
			res.Log, res.Message = log.String(), "nixpacks build failed"
			return res, fmt.Errorf("nixpacks build failed: %w", err)
		}
	}

	// 3. Resolve the built image ID.
	if info, err := c.InspectImage(ctx, cfg.ImageTag); err == nil && info != nil {
		res.ImageID = info.ID
	}

	res.Log, res.Success, res.Message = log.String(), true, "image built"
	return res, nil
}

// runCmd runs a command in dir, appending a token-masked transcript to log.
func runCmd(ctx context.Context, log *strings.Builder, dir, token, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	log.WriteString("$ " + maskToken(name+" "+strings.Join(args, " "), token) + "\n")
	if len(out) > 0 {
		log.WriteString(maskToken(string(out), token))
		if !strings.HasSuffix(string(out), "\n") {
			log.WriteString("\n")
		}
	}
	if err != nil {
		log.WriteString(fmt.Sprintf("(error: %v)\n", err))
	}
	return err
}

func maskToken(s, token string) string {
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, "***")
}

func injectGitToken(repoURL, token string) string {
	if strings.HasPrefix(repoURL, "https://") {
		return "https://x-access-token:" + token + "@" + strings.TrimPrefix(repoURL, "https://")
	}
	return repoURL
}
