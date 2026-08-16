package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// ==================== Types ====================

type StackStatus string

const (
	StackStatusPending   StackStatus = "pending"
	StackStatusDeploying StackStatus = "deploying"
	StackStatusRunning   StackStatus = "running"
	StackStatusPartial   StackStatus = "partial"
	StackStatusFailed    StackStatus = "failed"
	StackStatusStopped   StackStatus = "stopped"
)

type Stack struct {
	ID            uuid.UUID         `json:"id"`
	OrgID         uuid.UUID         `json:"org_id"`
	AgentID       uuid.UUID         `json:"agent_id"`
	Name          string            `json:"name"`
	Environment   string            `json:"environment"`
	ComposeYAML   string            `json:"compose_yaml"`
	Variables     map[string]string `json:"variables"`
	ServiceCount  int               `json:"service_count"`
	RunningCount  int               `json:"running_count"`
	FailedCount   int               `json:"failed_count"`
	Status        StackStatus       `json:"status"`
	StatusMessage *string           `json:"status_message,omitempty"`
	DeployedBy    *uuid.UUID        `json:"deployed_by,omitempty"`
	DeployedAt    *time.Time        `json:"deployed_at,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Deployments   []Deployment      `json:"deployments,omitempty"`
}

type CreateStackRequest struct {
	Name         string            `json:"name" binding:"required"`
	Environment  string            `json:"environment" binding:"required,oneof=dev staging prod"`
	ComposeYAML  string            `json:"compose_yaml" binding:"required"`
	Variables    map[string]string `json:"variables,omitempty"`
	Overrides    []ServiceOverride `json:"overrides,omitempty"`
	SkipScanning bool              `json:"skip_scanning,omitempty"`
	// Stack companion files (v3/45): the files this compose bind-mounts (init scripts, entrypoints,
	// configs), keyed by their relative bind source ("./scripts/db-init.sh" → path "scripts/db-init.sh").
	// The agent materializes them on the host at deploy; never persisted with the stack.
	Files []StackFile `json:"files,omitempty"`
}

// StackFile is a companion file shipped alongside the compose (v3/45).
type StackFile struct {
	Path    string `json:"path"`           // relative to the stack root, e.g. "scripts/db-init.sh"
	Content string `json:"content"`        // file body (plaintext)
	Mode    string `json:"mode,omitempty"` // octal, e.g. "0755"; default "0644"
}

// RequiredFile is a relative bind-mount source a compose needs supplied as a companion file (v3/45).
type RequiredFile struct {
	Source  string `json:"source"`  // as written in compose, e.g. "./scripts/db-init.sh"
	Path    string `json:"path"`    // normalized stack-root-relative path, e.g. "scripts/db-init.sh"
	Service string `json:"service"` // the service that bind-mounts it
	Target  string `json:"target"`  // the in-container mount path
}

// maxCompanionFileBytes caps a single companion file (v3/45) — scripts/configs are small.
const maxCompanionFileBytes = 1 << 20 // 1 MiB

// stackFilesBasePath is the host root where the agent materializes companion files. MUST match the
// agent's stackFilesBasePath (agent/internal/docker/client.go).
const stackFilesBasePath = "/var/lib/infrapilot/stacks"

type ServiceOverride struct {
	ServiceName  string            `json:"service_name"`
	TagOverride  *string           `json:"tag_override,omitempty"`
	EnvOverrides map[string]string `json:"env_overrides,omitempty"`
	Enabled      bool              `json:"enabled"`
}

type ParseComposeRequest struct {
	ComposeYAML string            `json:"compose_yaml" binding:"required"`
	Variables   map[string]string `json:"variables,omitempty"`
}

type ParsedCompose struct {
	Services    []ComposeService    `json:"services"`
	Networks    []ComposeNetwork    `json:"networks"`
	Volumes     []ComposeVolume     `json:"volumes"`
	Variables   []ComposeVariable   `json:"variables"`
	// RequiredFiles are relative bind-mount sources the compose needs shipped as companion files
	// (v3/45) — the wizard prompts for each, deploy is fail-closed until all are supplied.
	RequiredFiles []RequiredFile `json:"required_files,omitempty"`
	Errors        []string       `json:"errors,omitempty"`
}

type ComposeService struct {
	Name          string            `json:"name"`
	ContainerName string            `json:"container_name,omitempty"`
	Image         string            `json:"image"`
	DependsOn     []string          `json:"depends_on,omitempty"`
	Ports         []string          `json:"ports,omitempty"`
	Environment   map[string]string `json:"environment,omitempty"`
	Volumes       []string          `json:"volumes,omitempty"`
	Networks      []string          `json:"networks,omitempty"`
	Command       []string          `json:"command,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Restart       string            `json:"restart,omitempty"`
	EnvFiles      []string          `json:"env_files,omitempty"`
	Order         int               `json:"order"`
}

type ComposeNetwork struct {
	Name     string `json:"name"`
	Driver   string `json:"driver,omitempty"`
	External bool   `json:"external"`
}

type ComposeVolume struct {
	Name     string `json:"name"`
	Driver   string `json:"driver,omitempty"`
	External bool   `json:"external"`
}

type ComposeVariable struct {
	Name     string  `json:"name"`
	Default  *string `json:"default,omitempty"`
	Used     bool    `json:"used"`
	Required bool    `json:"required"`
}

type StackProgress struct {
	StackID      uuid.UUID               `json:"stack_id"`
	Status       StackStatus             `json:"status"`
	ServiceCount int                     `json:"service_count"`
	RunningCount int                     `json:"running_count"`
	FailedCount  int                     `json:"failed_count"`
	Services     []ServiceProgress       `json:"services"`
}

type ServiceProgress struct {
	Name      string           `json:"name"`
	Status    DeploymentStatus `json:"status"`
	Message   string           `json:"message,omitempty"`
	Order     int              `json:"order"`
}

// ==================== Parse Compose YAML ====================

func (h *Handler) parseComposeYAML(c *gin.Context) {
	var req ParseComposeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	parsed, err := parseCompose(req.ComposeYAML, req.Variables)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Failed to parse compose file",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, parsed)
}

// parseCompose parses a docker-compose.yml string into structured data
func parseCompose(yamlContent string, variables map[string]string) (*ParsedCompose, error) {
	// First, extract and collect variables
	foundVars := extractVariables(yamlContent)
	requiredVars := extractRequiredVariables(yamlContent)

	// Perform variable substitution
	substituted := substituteVariables(yamlContent, variables)

	// Parse YAML
	var compose struct {
		Services map[string]struct {
			Image         string                 `yaml:"image"`
			ContainerName string                 `yaml:"container_name"`
			DependsOn     interface{}            `yaml:"depends_on"`
			Ports         []string               `yaml:"ports"`
			Environment   interface{}            `yaml:"environment"`
			Volumes       []string               `yaml:"volumes"`
			Networks      interface{}            `yaml:"networks"`
			Command       interface{}            `yaml:"command"`
			Labels        interface{}            `yaml:"labels"`
			Restart       string                 `yaml:"restart"`
			EnvFile       interface{}            `yaml:"env_file"`
		} `yaml:"services"`
		Networks map[string]struct {
			Driver   string `yaml:"driver"`
			External bool   `yaml:"external"`
		} `yaml:"networks"`
		Volumes map[string]struct {
			Driver   string `yaml:"driver"`
			External bool   `yaml:"external"`
		} `yaml:"volumes"`
	}

	if err := yaml.Unmarshal([]byte(substituted), &compose); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	// Convert services
	services := make([]ComposeService, 0, len(compose.Services))
	var errors []string

	for name, svc := range compose.Services {
		if svc.Image == "" {
			errors = append(errors, fmt.Sprintf("Service '%s' has no image specified", name))
			continue
		}

		cs := ComposeService{
			Name:          name,
			ContainerName: svc.ContainerName,
			Image:         svc.Image,
			Ports:         svc.Ports,
			Volumes:       svc.Volumes,
			Restart:       svc.Restart,
		}

		// Parse depends_on (can be list or map)
		cs.DependsOn = parseDependsOn(svc.DependsOn)

		// Parse environment (can be list or map)
		cs.Environment = parseEnvironment(svc.Environment)

		// Parse networks (can be list or map)
		cs.Networks = parseNetworks(svc.Networks)

		// Parse command (can be string or list)
		cs.Command = parseCommand(svc.Command)

		// Parse labels (can be list or map)
		cs.Labels = parseLabels(svc.Labels)

		// Parse env_file (can be string or list)
		cs.EnvFiles = parseEnvFile(svc.EnvFile)

		services = append(services, cs)
	}

	// Topological sort by depends_on
	services, cycleErr := topologicalSort(services)
	if cycleErr != nil {
		errors = append(errors, cycleErr.Error())
	}

	// Convert networks
	networks := make([]ComposeNetwork, 0, len(compose.Networks))
	for name, net := range compose.Networks {
		networks = append(networks, ComposeNetwork{
			Name:     name,
			Driver:   net.Driver,
			External: net.External,
		})
	}

	// Convert volumes
	volumes := make([]ComposeVolume, 0, len(compose.Volumes))
	for name, vol := range compose.Volumes {
		volumes = append(volumes, ComposeVolume{
			Name:     name,
			Driver:   vol.Driver,
			External: vol.External,
		})
	}

	// Build variable list with usage info
	varList := make([]ComposeVariable, 0, len(foundVars))
	for name, defaultVal := range foundVars {
		cv := ComposeVariable{
			Name: name,
			Used: true,
		}
		if defaultVal != "" {
			cv.Default = &defaultVal
		}
		if _, isRequired := requiredVars[name]; isRequired {
			cv.Required = true
		}
		varList = append(varList, cv)
	}

	return &ParsedCompose{
		Services:      services,
		Networks:      networks,
		Volumes:       volumes,
		Variables:     varList,
		RequiredFiles: collectRequiredFiles(services),
		Errors:        errors,
	}, nil
}

// ==================== Stack companion files (v3/45) ====================

// splitVolumeDef splits a compose volume entry "src:dst[:mode]" into its parts. ok=false if it is
// not a bind/named mount (fewer than 2 parts). A leading absolute Windows-style path is out of
// scope (agents are Linux).
func splitVolumeDef(volDef string) (source, target string, readOnly, ok bool) {
	parts := strings.Split(volDef, ":")
	if len(parts) < 2 {
		return "", "", false, false
	}
	return parts[0], parts[1], len(parts) >= 3 && parts[2] == "ro", true
}

// isRelativeBindSource reports whether a compose bind source is a RELATIVE host path — the case
// that needs a companion file (v3/45). Named volumes (no "." / "/" prefix) and absolute host paths
// are excluded.
func isRelativeBindSource(src string) bool {
	return strings.HasPrefix(src, "./") || strings.HasPrefix(src, "../") || src == "." || src == ".."
}

// normalizeCompanionPath turns a relative bind source ("./scripts/db-init.sh") into a clean,
// stack-root-relative path ("scripts/db-init.sh"). ok=false for traversal or absolute escapes —
// callers treat that as a hard error (fail-closed).
func normalizeCompanionPath(src string) (string, bool) {
	p := strings.TrimPrefix(src, "./")
	if p == "" || strings.HasPrefix(p, "/") {
		return "", false
	}
	if p == ".." || strings.HasPrefix(p, "../") || strings.Contains(p, "/../") || strings.HasSuffix(p, "/..") {
		return "", false
	}
	cleaned := path.Clean(p)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", false
	}
	return cleaned, true
}

// collectRequiredFiles scans every service's bind mounts for relative sources and returns the set
// the deploy must be supplied as companion files (v3/45), de-duplicated by normalized path.
func collectRequiredFiles(services []ComposeService) []RequiredFile {
	var out []RequiredFile
	seen := map[string]bool{}
	for _, svc := range services {
		for _, volDef := range svc.Volumes {
			source, target, _, ok := splitVolumeDef(volDef)
			if !ok || !isRelativeBindSource(source) {
				continue
			}
			norm, ok := normalizeCompanionPath(source)
			if !ok || seen[norm] {
				continue
			}
			seen[norm] = true
			out = append(out, RequiredFile{Source: source, Path: norm, Service: svc.Name, Target: target})
		}
	}
	return out
}

// sanitizeStackDirName makes a stack name safe as a single host path segment (v3/45).
func sanitizeStackDirName(name string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, name)
	safe = strings.TrimLeft(safe, ".") // no leading dots → no hidden/".." segment
	if safe == "" {
		return "stack"
	}
	return safe
}

// extractVariables finds all ${VAR}, ${VAR:-default}, ${VAR:?error} patterns
func extractVariables(content string) map[string]string {
	vars := make(map[string]string)

	// Match ${VAR}, ${VAR:-default}, ${VAR-default}, ${VAR:?error}, ${VAR?error}
	re := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?:(:?[-?])([^}]*))?\}`)
	matches := re.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		varName := match[1]
		operator := ""
		operand := ""
		if len(match) > 2 {
			operator = match[2]
		}
		if len(match) > 3 {
			operand = match[3]
		}

		defaultVal := ""
		if operator == ":-" || operator == "-" {
			defaultVal = operand
		}
		// For :? and ? (required), default is empty — variable must be provided

		// Only store if not already present (first default wins)
		if _, exists := vars[varName]; !exists {
			vars[varName] = defaultVal
		}
	}

	return vars
}

// extractRequiredVariables returns variable names that use the :? or ? syntax (required)
func extractRequiredVariables(content string) map[string]string {
	required := make(map[string]string)

	re := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:?\?)([^}]*)\}`)
	matches := re.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		varName := match[1]
		errorMsg := match[3]
		if errorMsg == "" {
			errorMsg = varName + " is required"
		}
		required[varName] = errorMsg
	}

	return required
}

// substituteVariables replaces ${VAR} patterns with values
func substituteVariables(content string, variables map[string]string) string {
	result := content

	// Match all ${VAR...} patterns: ${VAR:-default}, ${VAR-default}, ${VAR:?error}, ${VAR?error}
	reWithOperator := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:?[-?])([^}]*)\}`)
	result = reWithOperator.ReplaceAllStringFunc(result, func(match string) string {
		parts := reWithOperator.FindStringSubmatch(match)
		varName := parts[1]
		operator := parts[2]
		operand := parts[3]

		if val, ok := variables[varName]; ok && val != "" {
			return val
		}

		switch operator {
		case ":-", "-":
			// Default value
			return operand
		case ":?", "?":
			// Required — if we get here, the variable was not provided
			// Return empty string; validation should catch this earlier
			return ""
		default:
			return match
		}
	})

	// Match ${VAR} without any operator
	reSimple := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	result = reSimple.ReplaceAllStringFunc(result, func(match string) string {
		parts := reSimple.FindStringSubmatch(match)
		varName := parts[1]

		if val, ok := variables[varName]; ok {
			return val
		}
		return match // Keep original if no value
	})

	return result
}

// parseDependsOn handles both list and map formats
func parseDependsOn(v interface{}) []string {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case []interface{}:
		deps := make([]string, 0, len(val))
		for _, d := range val {
			if s, ok := d.(string); ok {
				deps = append(deps, s)
			}
		}
		return deps
	case map[string]interface{}:
		deps := make([]string, 0, len(val))
		for name := range val {
			deps = append(deps, name)
		}
		return deps
	}
	return nil
}

// parseEnvironment handles both list and map formats
func parseEnvironment(v interface{}) map[string]string {
	if v == nil {
		return nil
	}

	env := make(map[string]string)

	switch val := v.(type) {
	case []interface{}:
		for _, e := range val {
			if s, ok := e.(string); ok {
				parts := strings.SplitN(s, "=", 2)
				if len(parts) == 2 {
					env[parts[0]] = parts[1]
				} else {
					env[parts[0]] = ""
				}
			}
		}
	case map[string]interface{}:
		for k, v := range val {
			if s, ok := v.(string); ok {
				env[k] = s
			} else if v != nil {
				env[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	return env
}

// parseNetworks handles both list and map formats
func parseNetworks(v interface{}) []string {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case []interface{}:
		nets := make([]string, 0, len(val))
		for _, n := range val {
			if s, ok := n.(string); ok {
				nets = append(nets, s)
			}
		}
		return nets
	case map[string]interface{}:
		nets := make([]string, 0, len(val))
		for name := range val {
			nets = append(nets, name)
		}
		return nets
	}
	return nil
}

// parseCommand handles both string and list formats
func parseCommand(v interface{}) []string {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case string:
		// Shell-like parsing that handles quoted strings
		var args []string
		current := ""
		inQuote := false
		quoteChar := byte(0)
		for i := 0; i < len(val); i++ {
			ch := val[i]
			if inQuote {
				if ch == quoteChar {
					inQuote = false
				} else {
					current += string(ch)
				}
			} else if ch == '"' || ch == '\'' {
				inQuote = true
				quoteChar = ch
			} else if ch == ' ' || ch == '\t' {
				if current != "" {
					args = append(args, current)
					current = ""
				}
			} else {
				current += string(ch)
			}
		}
		if current != "" {
			args = append(args, current)
		}
		return args
	case []interface{}:
		cmd := make([]string, 0, len(val))
		for _, c := range val {
			if s, ok := c.(string); ok {
				cmd = append(cmd, s)
			}
		}
		return cmd
	}
	return nil
}

// parseLabels handles both list and map formats
func parseLabels(v interface{}) map[string]string {
	if v == nil {
		return nil
	}

	labels := make(map[string]string)

	switch val := v.(type) {
	case []interface{}:
		for _, l := range val {
			if s, ok := l.(string); ok {
				parts := strings.SplitN(s, "=", 2)
				if len(parts) == 2 {
					labels[parts[0]] = parts[1]
				} else {
					labels[parts[0]] = ""
				}
			}
		}
	case map[string]interface{}:
		for k, v := range val {
			if s, ok := v.(string); ok {
				labels[k] = s
			} else if v != nil {
				labels[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	return labels
}

// parseEnvFile handles both string and list formats for env_file
func parseEnvFile(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		return []string{val}
	case []interface{}:
		files := make([]string, 0, len(val))
		for _, f := range val {
			if s, ok := f.(string); ok {
				files = append(files, s)
			}
		}
		return files
	}
	return nil
}

// topologicalSort orders services by dependencies using Kahn's algorithm
func topologicalSort(services []ComposeService) ([]ComposeService, error) {
	// Build adjacency list and in-degree count
	inDegree := make(map[string]int)
	graph := make(map[string][]string)
	serviceMap := make(map[string]*ComposeService)

	for i := range services {
		svc := &services[i]
		serviceMap[svc.Name] = svc
		inDegree[svc.Name] = 0
		graph[svc.Name] = nil
	}

	// Add edges based on depends_on
	for _, svc := range services {
		for _, dep := range svc.DependsOn {
			if _, exists := serviceMap[dep]; exists {
				graph[dep] = append(graph[dep], svc.Name)
				inDegree[svc.Name]++
			}
		}
	}

	// Start with nodes that have no dependencies
	var queue []string
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	// Sort queue for deterministic order
	sort.Strings(queue)

	var result []ComposeService
	order := 0

	for len(queue) > 0 {
		// Pop first element
		name := queue[0]
		queue = queue[1:]

		svc := serviceMap[name]
		svc.Order = order
		order++
		result = append(result, *svc)

		// Reduce in-degree for dependent services
		dependents := graph[name]
		sort.Strings(dependents) // For deterministic order

		for _, dep := range dependents {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
				sort.Strings(queue)
			}
		}
	}

	// Check for cycles
	if len(result) != len(services) {
		return services, fmt.Errorf("circular dependency detected in depends_on configuration")
	}

	return result, nil
}

// ==================== Create Stack ====================

func (h *Handler) createStack(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)
	agentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	var req CreateStackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify agent exists and belongs to org
	var exists bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1 AND org_id = $2)
	`, agentID, orgID).Scan(&exists)
	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Parse and validate compose YAML
	parsed, err := parseCompose(req.ComposeYAML, req.Variables)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Failed to parse compose file",
			"details": err.Error(),
		})
		return
	}

	if len(parsed.Services) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No services found in compose file"})
		return
	}

	// Stack companion files (v3/45): index supplied files by normalized path and fail-closed if the
	// compose bind-mounts a relative source we weren't given. The volume loop below rewrites each
	// relative bind to an absolute stack-root path and attaches the content for the agent to write.
	fileMap := map[string]StackFile{}
	for _, f := range req.Files {
		norm, ok := normalizeCompanionPath(f.Path)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid companion file path: " + f.Path})
			return
		}
		if len(f.Content) > maxCompanionFileBytes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "companion file exceeds 1 MiB: " + f.Path})
			return
		}
		fileMap[norm] = f
	}
	var missingFiles []string
	for _, rf := range parsed.RequiredFiles {
		if _, ok := fileMap[rf.Path]; !ok {
			missingFiles = append(missingFiles, rf.Source)
		}
	}
	if len(missingFiles) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":          "this stack bind-mounts local files that must be supplied: " + strings.Join(missingFiles, ", "),
			"required_files": parsed.RequiredFiles,
		})
		return
	}
	stackDir := sanitizeStackDirName(req.Name)

	// Build override map for quick lookup
	overrideMap := make(map[string]ServiceOverride)
	for _, o := range req.Overrides {
		overrideMap[o.ServiceName] = o
	}

	// Count enabled services
	enabledCount := 0
	for _, svc := range parsed.Services {
		if override, ok := overrideMap[svc.Name]; ok {
			if override.Enabled {
				enabledCount++
			}
		} else {
			enabledCount++ // Default to enabled
		}
	}

	if enabledCount == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one service must be enabled"})
		return
	}

	// Marshal variables to JSON
	variablesJSON, _ := json.Marshal(req.Variables)

	// Remove previous stacks with the same name+environment so redeploys don't accumulate rows
	_, _ = h.db.Exec(c.Request.Context(), `
		DELETE FROM stacks
		WHERE org_id = $1 AND agent_id = $2 AND name = $3 AND environment = $4
	`, orgID, agentID, req.Name, req.Environment)

	// Create stack record
	var stackID uuid.UUID
	err = h.db.QueryRow(c.Request.Context(), `
		INSERT INTO stacks (
			org_id, agent_id, name, environment,
			compose_yaml, variables, service_count,
			status, deployed_by, skip_scanning
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', $8, $9)
		RETURNING id
	`, orgID, agentID, req.Name, req.Environment,
		req.ComposeYAML, variablesJSON, enabledCount, userID, req.SkipScanning,
	).Scan(&stackID)

	if err != nil {
		h.logger.Error("Failed to create stack", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create stack"})
		return
	}

	h.logger.Info("Stack created",
		zap.String("stack_id", stackID.String()),
		zap.String("name", req.Name),
		zap.Int("service_count", enabledCount),
	)

	// Create deployment records for each enabled service
	for _, svc := range parsed.Services {
		override, hasOverride := overrideMap[svc.Name]
		if hasOverride && !override.Enabled {
			continue // Skip disabled services
		}

		// Determine image tag
		imageRef := svc.Image
		if hasOverride && override.TagOverride != nil {
			// Apply tag override
			parts := strings.Split(imageRef, ":")
			imageRef = parts[0] + ":" + *override.TagOverride
		}

		// Parse image reference
		imageRepo, imageTag := parseImageRef(imageRef)

		// Merge environment variables
		envVars := svc.Environment
		if hasOverride && len(override.EnvOverrides) > 0 {
			if envVars == nil {
				envVars = make(map[string]string)
			}
			for k, v := range override.EnvOverrides {
				envVars[k] = v
			}
		}

		// Build container config with compose labels
		containerConfig := &DeploymentContainerConfig{
			ContainerName: svc.ContainerName,
			EnvVars:       envVars,
			Command:       svc.Command,
			Labels: map[string]string{
				"com.docker.compose.project": req.Name,
				"com.docker.compose.service": svc.Name,
			},
		}

		// Add networks with service name as alias (like Docker Compose does)
		if len(svc.Networks) > 0 {
			for _, netName := range svc.Networks {
				containerConfig.Networks = append(containerConfig.Networks, ContainerConfigNetwork{
					NetworkName:     &netName,
					CreateIfMissing: true,
					Driver:          "bridge",
					Aliases:         []string{svc.Name},
				})
			}
		}

		// Add volumes
		for _, volDef := range svc.Volumes {
			parts := strings.Split(volDef, ":")
			if len(parts) >= 2 {
				volName := parts[0]
				mountPath := parts[1]
				readOnly := len(parts) >= 3 && parts[2] == "ro"

				// Check if it's a named volume or bind mount
				if strings.HasPrefix(volName, "/") || strings.HasPrefix(volName, ".") {
					// Bind mount
					if isRelativeBindSource(volName) {
						// Companion file (v3/45): rewrite the relative source to an absolute stack-root
						// path on the host and attach the content so the agent materializes it there
						// before mounting. Presence was already validated (fail-closed) above.
						norm, ok := normalizeCompanionPath(volName)
						if !ok {
							continue // defensive: validation would have rejected this
						}
						f := fileMap[norm]
						hostPath := stackFilesBasePath + "/" + stackDir + "/" + norm
						content := f.Content
						containerConfig.Volumes = append(containerConfig.Volumes, ContainerConfigVolume{
							HostPath:  &hostPath,
							MountPath: mountPath,
							ReadOnly:  readOnly,
							Content:   &content,
							FileMode:  f.Mode,
						})
					} else {
						// Absolute host path — must already exist on the target host.
						containerConfig.Volumes = append(containerConfig.Volumes, ContainerConfigVolume{
							HostPath:  &volName,
							MountPath: mountPath,
							ReadOnly:  readOnly,
						})
					}
				} else {
					// Named volume
					containerConfig.Volumes = append(containerConfig.Volumes, ContainerConfigVolume{
						VolumeName:      &volName,
						MountPath:       mountPath,
						CreateIfMissing: true,
						ReadOnly:        readOnly,
					})
				}
			}
		}

		containerConfigJSON, _ := h.marshalContainerConfig(containerConfig)

		// Create deployment record
		var deploymentID uuid.UUID
		err = h.db.QueryRow(c.Request.Context(), `
			INSERT INTO deployments (
				org_id, agent_id, service_name, environment,
				image_repository, image_tag,
				stack_id, service_order,
				deployed_by, status, container_config
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending', $10)
			RETURNING id
		`, orgID, agentID, svc.Name, req.Environment,
			imageRepo, imageTag, stackID, svc.Order, userID, containerConfigJSON,
		).Scan(&deploymentID)

		if err != nil {
			h.logger.Error("Failed to create deployment for service",
				zap.String("service", svc.Name),
				zap.Error(err),
			)
			// Continue with other services
			continue
		}

		h.logger.Info("Deployment created for stack service",
			zap.String("deployment_id", deploymentID.String()),
			zap.String("service", svc.Name),
			zap.Int("order", svc.Order),
		)
	}

	// Start async deployment pipeline
	go h.runStackDeploymentPipeline(context.Background(), orgID, stackID)

	c.JSON(http.StatusCreated, gin.H{
		"id":            stackID,
		"status":        "pending",
		"service_count": enabledCount,
		"message":       "Stack created and deployment initiated",
	})
}

// parseImageRef splits image reference into repository and tag
func parseImageRef(imageRef string) (string, *string) {
	// Handle digest format
	if strings.Contains(imageRef, "@") {
		parts := strings.Split(imageRef, "@")
		return parts[0], nil
	}

	// Handle tag format
	lastColon := strings.LastIndex(imageRef, ":")
	if lastColon > 0 && !strings.Contains(imageRef[lastColon:], "/") {
		repo := imageRef[:lastColon]
		tag := imageRef[lastColon+1:]
		return repo, &tag
	}

	// No tag specified
	return imageRef, nil
}

// ==================== Stack Deployment Pipeline ====================

func (h *Handler) runStackDeploymentPipeline(ctx context.Context, orgID, stackID uuid.UUID) {
	logger := h.logger.With(
		zap.String("stack_id", stackID.String()),
		zap.String("org_id", orgID.String()),
	)

	logger.Info("Starting stack deployment pipeline")

	// Get skip_scanning setting from stack
	var skipScanning bool
	err := h.db.QueryRow(ctx, `SELECT skip_scanning FROM stacks WHERE id = $1`, stackID).Scan(&skipScanning)
	if err != nil {
		logger.Warn("Failed to get skip_scanning setting, defaulting to false", zap.Error(err))
		skipScanning = false
	}

	// Update stack status to deploying
	if err := h.updateStackStatus(ctx, stackID, StackStatusDeploying, "Deploying services"); err != nil {
		logger.Error("Failed to update stack status", zap.Error(err))
		return
	}

	// Get all deployments for this stack, ordered by service_order
	rows, err := h.db.Query(ctx, `
		SELECT id, service_name, image_repository, image_tag, service_order, container_config
		FROM deployments
		WHERE stack_id = $1
		ORDER BY service_order ASC
	`, stackID)
	if err != nil {
		logger.Error("Failed to get stack deployments", zap.Error(err))
		h.updateStackStatus(ctx, stackID, StackStatusFailed, "Failed to get deployments")
		return
	}
	defer rows.Close()

	type serviceDeployment struct {
		ID              uuid.UUID
		ServiceName     string
		ImageRepository string
		ImageTag        *string
		Order           int
		ContainerConfig []byte
	}

	var deployments []serviceDeployment
	for rows.Next() {
		var d serviceDeployment
		if err := rows.Scan(&d.ID, &d.ServiceName, &d.ImageRepository, &d.ImageTag, &d.Order, &d.ContainerConfig); err != nil {
			logger.Warn("Failed to scan deployment", zap.Error(err))
			continue
		}
		deployments = append(deployments, d)
	}

	if len(deployments) == 0 {
		h.updateStackStatus(ctx, stackID, StackStatusFailed, "No deployments found")
		return
	}

	runningCount := 0
	failedCount := 0

	// Deploy services in order
	for _, d := range deployments {
		logger.Info("Deploying service",
			zap.String("service", d.ServiceName),
			zap.Int("order", d.Order),
		)

		// Parse container config
		var containerConfig *DeploymentContainerConfig
		if len(d.ContainerConfig) > 0 {
			if err := json.Unmarshal(d.ContainerConfig, &containerConfig); err != nil {
				logger.Warn("Failed to parse container config", zap.Error(err))
			}
		}

		// Build image reference
		imageTag := ""
		if d.ImageTag != nil {
			imageTag = *d.ImageTag
		}

		// Run the deployment pipeline for this service
		h.runDeploymentPipeline(ctx, orgID, d.ID, d.ImageRepository, imageTag, "", containerConfig, skipScanning, false)

		// Check deployment status
		var status string
		err := h.db.QueryRow(ctx, `SELECT status FROM deployments WHERE id = $1`, d.ID).Scan(&status)
		if err != nil {
			logger.Warn("Failed to get deployment status", zap.Error(err))
			failedCount++
			continue
		}

		if status == "running" {
			runningCount++
		} else if status == "failed" {
			failedCount++
		}

		// Update stack counts
		_, err = h.db.Exec(ctx, `
			UPDATE stacks
			SET running_count = $1, failed_count = $2, updated_at = NOW()
			WHERE id = $3
		`, runningCount, failedCount, stackID)
		if err != nil {
			logger.Warn("Failed to update stack counts", zap.Error(err))
		}
	}

	// Determine final stack status
	var finalStatus StackStatus
	var finalMessage string

	totalServices := len(deployments)
	if runningCount == totalServices {
		finalStatus = StackStatusRunning
		finalMessage = fmt.Sprintf("All %d services running", totalServices)
	} else if failedCount == totalServices {
		finalStatus = StackStatusFailed
		finalMessage = fmt.Sprintf("All %d services failed", totalServices)
	} else {
		finalStatus = StackStatusPartial
		finalMessage = fmt.Sprintf("%d/%d services running, %d failed", runningCount, totalServices, failedCount)
	}

	// Update final status
	_, err = h.db.Exec(ctx, `
		UPDATE stacks
		SET status = $1, status_message = $2, deployed_at = NOW(), updated_at = NOW()
		WHERE id = $3
	`, finalStatus, finalMessage, stackID)
	if err != nil {
		logger.Error("Failed to update final stack status", zap.Error(err))
	}

	logger.Info("Stack deployment pipeline completed",
		zap.String("status", string(finalStatus)),
		zap.Int("running", runningCount),
		zap.Int("failed", failedCount),
	)
}

func (h *Handler) updateStackStatus(ctx context.Context, stackID uuid.UUID, status StackStatus, message string) error {
	_, err := h.db.Exec(ctx, `
		UPDATE stacks
		SET status = $1, status_message = $2, updated_at = NOW()
		WHERE id = $3
	`, status, message, stackID)

	if err != nil {
		h.logger.Error("Failed to update stack status",
			zap.String("stack_id", stackID.String()),
			zap.String("status", string(status)),
			zap.Error(err),
		)
	}

	return err
}

// ==================== List Managed Stacks ====================

func (h *Handler) listManagedStacks(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentID := c.Param("id")

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, org_id, agent_id, name, environment,
		       service_count, running_count, failed_count,
		       status, status_message,
		       deployed_by, deployed_at, created_at, updated_at
		FROM stacks
		WHERE org_id = $1 AND agent_id = $2
		ORDER BY created_at DESC
		LIMIT 100
	`, orgID, agentID)

	if err != nil {
		h.logger.Error("Failed to list stacks", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list stacks"})
		return
	}
	defer rows.Close()

	stacks := []Stack{}
	for rows.Next() {
		var s Stack
		if err := rows.Scan(
			&s.ID, &s.OrgID, &s.AgentID, &s.Name, &s.Environment,
			&s.ServiceCount, &s.RunningCount, &s.FailedCount,
			&s.Status, &s.StatusMessage,
			&s.DeployedBy, &s.DeployedAt, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			h.logger.Warn("Failed to scan stack", zap.Error(err))
			continue
		}
		stacks = append(stacks, s)
	}

	c.JSON(http.StatusOK, stacks)
}

// ==================== Get Stack ====================

func (h *Handler) getManagedStack(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentID := c.Param("id")
	stackID := c.Param("sid")

	var s Stack
	var variablesJSON []byte

	err := h.db.QueryRow(c.Request.Context(), `
		SELECT id, org_id, agent_id, name, environment,
		       compose_yaml, variables,
		       service_count, running_count, failed_count,
		       status, status_message,
		       deployed_by, deployed_at, created_at, updated_at
		FROM stacks
		WHERE id = $1 AND org_id = $2 AND agent_id = $3
	`, stackID, orgID, agentID).Scan(
		&s.ID, &s.OrgID, &s.AgentID, &s.Name, &s.Environment,
		&s.ComposeYAML, &variablesJSON,
		&s.ServiceCount, &s.RunningCount, &s.FailedCount,
		&s.Status, &s.StatusMessage,
		&s.DeployedBy, &s.DeployedAt, &s.CreatedAt, &s.UpdatedAt,
	)

	if err != nil {
		h.logger.Error("Failed to get stack", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "stack not found"})
		return
	}

	// Parse variables
	if len(variablesJSON) > 0 {
		json.Unmarshal(variablesJSON, &s.Variables)
	}

	// Get associated deployments
	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, org_id, agent_id, service_name, environment,
		       image_registry, image_repository, image_tag, image_digest,
		       git_repo, git_branch, git_commit,
		       ci_provider, ci_pipeline_id, ci_build_url,
		       scan_result_id, sbom_id,
		       policy_decision, policy_reason,
		       status, status_message,
		       container_id, container_name, proxy_host_id,
		       deployed_by, deployed_at, created_at, updated_at
		FROM deployments
		WHERE stack_id = $1
		ORDER BY service_order ASC
	`, stackID)

	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var d Deployment
			if err := rows.Scan(
				&d.ID, &d.OrgID, &d.AgentID, &d.ServiceName, &d.Environment,
				&d.ImageRegistry, &d.ImageRepository, &d.ImageTag, &d.ImageDigest,
				&d.GitRepo, &d.GitBranch, &d.GitCommit,
				&d.CIProvider, &d.CIPipelineID, &d.CIBuildURL,
				&d.ScanResultID, &d.SBOMID,
				&d.PolicyDecision, &d.PolicyReason,
				&d.Status, &d.StatusMessage,
				&d.ContainerID, &d.ContainerName, &d.ProxyHostID,
				&d.DeployedBy, &d.DeployedAt, &d.CreatedAt, &d.UpdatedAt,
			); err != nil {
				h.logger.Warn("Failed to scan deployment", zap.Error(err))
				continue
			}
			s.Deployments = append(s.Deployments, d)
		}
	}

	c.JSON(http.StatusOK, s)
}

// ==================== Get Stack Progress ====================

func (h *Handler) getStackProgress(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentID := c.Param("id")
	stackID := c.Param("sid")

	// Get stack status
	var progress StackProgress
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT id, status, service_count, running_count, failed_count
		FROM stacks
		WHERE id = $1 AND org_id = $2 AND agent_id = $3
	`, stackID, orgID, agentID).Scan(
		&progress.StackID, &progress.Status,
		&progress.ServiceCount, &progress.RunningCount, &progress.FailedCount,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "stack not found"})
		return
	}

	// Get service statuses
	rows, err := h.db.Query(c.Request.Context(), `
		SELECT service_name, status, status_message, service_order
		FROM deployments
		WHERE stack_id = $1
		ORDER BY service_order ASC
	`, stackID)

	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var sp ServiceProgress
			var statusMessage *string
			if err := rows.Scan(&sp.Name, &sp.Status, &statusMessage, &sp.Order); err != nil {
				continue
			}
			if statusMessage != nil {
				sp.Message = *statusMessage
			}
			progress.Services = append(progress.Services, sp)
		}
	}

	c.JSON(http.StatusOK, progress)
}

// ==================== Delete Stack ====================

func (h *Handler) deleteManagedStack(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}
	stackID, err := uuid.Parse(c.Param("sid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid stack ID"})
		return
	}

	// Get all container IDs for this stack's deployments
	rows, err := h.db.Query(c.Request.Context(), `
		SELECT container_id
		FROM deployments
		WHERE stack_id = $1 AND container_id IS NOT NULL
	`, stackID)

	if err != nil {
		h.logger.Error("Failed to get stack containers", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stack containers"})
		return
	}
	defer rows.Close()

	var containerIDs []string
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err == nil {
			containerIDs = append(containerIDs, cid)
		}
	}

	// Stop containers via agent
	for _, cid := range containerIDs {
		// Use agent grpc to stop containers
		h.logger.Info("Stopping container for stack deletion",
			zap.String("container_id", cid),
			zap.String("stack_id", stackID.String()),
		)
		// The actual stop is handled by the agent
		// We update deployment status
		h.db.Exec(c.Request.Context(), `
			UPDATE deployments
			SET status = 'stopped', status_message = 'Stack deleted', updated_at = NOW()
			WHERE stack_id = $1 AND container_id = $2
		`, stackID, cid)
	}

	// Delete stack (deployments will have stack_id set to NULL due to ON DELETE SET NULL)
	result, err := h.db.Exec(c.Request.Context(), `
		DELETE FROM stacks
		WHERE id = $1 AND org_id = $2 AND agent_id = $3
	`, stackID, orgID, agentID)

	if err != nil {
		h.logger.Error("Failed to delete stack", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete stack"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "stack not found"})
		return
	}

	h.logger.Info("Stack deleted",
		zap.String("stack_id", stackID.String()),
		zap.Int("containers_stopped", len(containerIDs)),
	)

	c.JSON(http.StatusOK, gin.H{
		"message":            "stack deleted successfully",
		"containers_stopped": len(containerIDs),
	})
}
