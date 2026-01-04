package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
)

// ContainerResponse represents a container in the API response
type ContainerResponse struct {
	ID            string   `json:"id"`
	ContainerID   string   `json:"container_id"`
	Name          string   `json:"name"`
	Image         string   `json:"image"`
	Status        string   `json:"status"`
	State         string   `json:"state"`
	StackName     string   `json:"stack_name,omitempty"`
	CPUPercent    float64  `json:"cpu_percent"`
	MemoryMB      int64    `json:"memory_mb"`
	MemoryLimitMB int64    `json:"memory_limit_mb"`
	Networks      []string `json:"networks"`
	CreatedAt     string   `json:"created_at"`
	RestartCount  int      `json:"restart_count"`
}

// ContainerDetailResponse represents detailed container information
type ContainerDetailResponse struct {
	ContainerResponse

	// Ports
	Ports []PortMapping `json:"ports"`

	// Mounts/Volumes
	Mounts []MountInfo `json:"mounts"`

	// Environment Variables
	Environment []EnvVar `json:"environment"`

	// Configuration
	Config ContainerConfig `json:"config"`

	// Health Check
	HealthCheck   *HealthCheckConfig `json:"health_check,omitempty"`
	HealthStatus  string             `json:"health_status,omitempty"`
	HealthLog     []HealthLogEntry   `json:"health_log,omitempty"`

	// Labels
	Labels map[string]string `json:"labels"`

	// Network details
	NetworkDetails []NetworkDetail `json:"network_details"`

	// Resource limits
	Resources ResourceLimits `json:"resources"`
}

type PortMapping struct {
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port"`
	Protocol      string `json:"protocol"`
	HostIP        string `json:"host_ip,omitempty"`
}

type MountInfo struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode"`
	ReadOnly    bool   `json:"read_only"`
}

type EnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ContainerConfig struct {
	Hostname      string   `json:"hostname"`
	Domainname    string   `json:"domainname"`
	User          string   `json:"user"`
	WorkingDir    string   `json:"working_dir"`
	Entrypoint    []string `json:"entrypoint"`
	Cmd           []string `json:"cmd"`
	RestartPolicy string   `json:"restart_policy"`
	Privileged    bool     `json:"privileged"`
	Tty           bool     `json:"tty"`
	OpenStdin     bool     `json:"open_stdin"`
}

type HealthCheckConfig struct {
	Test        []string `json:"test"`
	Interval    string   `json:"interval"`
	Timeout     string   `json:"timeout"`
	StartPeriod string   `json:"start_period"`
	Retries     int      `json:"retries"`
}

type HealthLogEntry struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
}

type NetworkDetail struct {
	Name       string   `json:"name"`
	NetworkID  string   `json:"network_id"`
	IPAddress  string   `json:"ip_address"`
	Gateway    string   `json:"gateway"`
	MacAddress string   `json:"mac_address"`
	Aliases    []string `json:"aliases,omitempty"`
}

type ResourceLimits struct {
	CPUShares   int64  `json:"cpu_shares"`
	CPUQuota    int64  `json:"cpu_quota"`
	CPUPeriod   int64  `json:"cpu_period"`
	CPUSetCPUs  string `json:"cpuset_cpus"`
	MemoryLimit int64  `json:"memory_limit"`
	MemorySwap  int64  `json:"memory_swap"`
	PidsLimit   int64  `json:"pids_limit"`
}

// listContainersReal fetches containers from Docker daemon with real-time stats
// NOTE: This is for local development only. In production, this should
// query the database which is populated by the agent via gRPC.
func (h *Handler) listContainersReal(c *gin.Context) {
	// agentID := c.Param("id") // Would use this to route to correct agent

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	// Connect to local Docker daemon
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to Docker: " + err.Error()})
		return
	}
	defer cli.Close()

	// List all containers (including stopped)
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list containers: " + err.Error()})
		return
	}

	// Convert to response format
	result := make([]ContainerResponse, 0, len(containers))
	for _, ctr := range containers {
		name := ""
		if len(ctr.Names) > 0 {
			name = strings.TrimPrefix(ctr.Names[0], "/")
		}

		// Determine status
		status := "unknown"
		if ctr.State == "running" {
			status = "running"
		} else if ctr.State == "exited" {
			status = "exited"
		} else if ctr.State == "paused" {
			status = "paused"
		} else {
			status = ctr.State
		}

		// Get stack name from labels (docker-compose)
		stackName := ""
		if project, ok := ctr.Labels["com.docker.compose.project"]; ok {
			stackName = project
		}

		// Get network names
		networks := make([]string, 0)
		for netName := range ctr.NetworkSettings.Networks {
			networks = append(networks, netName)
		}

		// Get real CPU/memory stats for running containers
		var cpuPercent float64
		var memoryMB int64
		var memoryLimitMB int64
		var restartCount int

		if ctr.State == "running" {
			cpuPercent, memoryMB, memoryLimitMB = getContainerStats(ctx, cli, ctr.ID)
		}

		// Get restart count from inspect
		inspect, err := cli.ContainerInspect(ctx, ctr.ID)
		if err == nil {
			restartCount = inspect.RestartCount
		}

		result = append(result, ContainerResponse{
			ID:            ctr.ID,
			ContainerID:   ctr.ID,
			Name:          name,
			Image:         ctr.Image,
			Status:        status,
			State:         ctr.State,
			StackName:     stackName,
			CPUPercent:    cpuPercent,
			MemoryMB:      memoryMB,
			MemoryLimitMB: memoryLimitMB,
			Networks:      networks,
			CreatedAt:     time.Unix(ctr.Created, 0).Format(time.RFC3339),
			RestartCount:  restartCount,
		})
	}

	c.JSON(http.StatusOK, result)
}

// getContainerStats fetches CPU and memory stats for a container
func getContainerStats(ctx context.Context, cli *client.Client, containerID string) (cpuPercent float64, memoryMB int64, memoryLimitMB int64) {
	statsCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	stats, err := cli.ContainerStatsOneShot(statsCtx, containerID)
	if err != nil {
		return 0, 0, 0
	}
	defer stats.Body.Close()

	// Parse stats JSON
	var statsJSON ContainerStatsJSON
	if err := json.NewDecoder(stats.Body).Decode(&statsJSON); err != nil {
		return 0, 0, 0
	}

	// Calculate CPU percentage
	cpuDelta := float64(statsJSON.CPUStats.CPUUsage.TotalUsage - statsJSON.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(statsJSON.CPUStats.SystemCPUUsage - statsJSON.PreCPUStats.SystemCPUUsage)
	numCPUs := float64(statsJSON.CPUStats.OnlineCPUs)
	if numCPUs == 0 {
		numCPUs = float64(len(statsJSON.CPUStats.CPUUsage.PercpuUsage))
	}

	if systemDelta > 0 && cpuDelta > 0 {
		cpuPercent = (cpuDelta / systemDelta) * numCPUs * 100.0
	}

	// Calculate memory in MB
	memoryMB = int64(statsJSON.MemoryStats.Usage / (1024 * 1024))
	memoryLimitMB = int64(statsJSON.MemoryStats.Limit / (1024 * 1024))

	return cpuPercent, memoryMB, memoryLimitMB
}

// ContainerStatsJSON represents Docker stats response
type ContainerStatsJSON struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     int    `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
}

// getDockerClient creates a Docker client for local development
func getDockerClient() (*client.Client, error) {
	return client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
}

// evaluateContainerPolicy checks policies before container actions (community edition - no policy engine)
func (h *Handler) evaluateContainerPolicy(c *gin.Context, containerID string, action string) (bool, string) {
	// Community edition: no policy engine, always allow
	return false, ""
}

// startContainerReal starts a Docker container
func (h *Handler) startContainerReal(c *gin.Context) {
	containerID := c.Param("cid")

	// Check policies before action
	if blocked, message := h.evaluateContainerPolicy(c, containerID, "start"); blocked {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "Action blocked by policy",
			"message": message,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	cli, err := getDockerClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to Docker"})
		return
	}
	defer cli.Close()

	if err := cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start container: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "container started", "container_id": containerID})
}

// stopContainerReal stops a Docker container
func (h *Handler) stopContainerReal(c *gin.Context) {
	containerID := c.Param("cid")

	// Check policies before action
	if blocked, message := h.evaluateContainerPolicy(c, containerID, "stop"); blocked {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "Action blocked by policy",
			"message": message,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	cli, err := getDockerClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to Docker"})
		return
	}
	defer cli.Close()

	timeout := 10 // seconds
	if err := cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stop container: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "container stopped", "container_id": containerID})
}

// restartContainerReal restarts a Docker container
func (h *Handler) restartContainerReal(c *gin.Context) {
	containerID := c.Param("cid")

	// Check policies before action
	if blocked, message := h.evaluateContainerPolicy(c, containerID, "restart"); blocked {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "Action blocked by policy",
			"message": message,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	cli, err := getDockerClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to Docker"})
		return
	}
	defer cli.Close()

	timeout := 10 // seconds
	if err := cli.ContainerRestart(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to restart container: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "container restarted", "container_id": containerID})
}

// deleteContainerReal removes a Docker container
func (h *Handler) deleteContainerReal(c *gin.Context) {
	containerID := c.Param("cid")

	// Check policies before action
	if blocked, message := h.evaluateContainerPolicy(c, containerID, "delete"); blocked {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "Action blocked by policy",
			"message": message,
		})
		return
	}

	// Require confirmation name in request body
	var req struct {
		ConfirmName string `json:"confirm_name" binding:"required"`
		Force       bool   `json:"force"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confirm_name is required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	cli, err := getDockerClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to Docker"})
		return
	}
	defer cli.Close()

	// Get container info to verify name
	info, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "container not found"})
		return
	}

	containerName := strings.TrimPrefix(info.Name, "/")
	if req.ConfirmName != containerName {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":         "container name does not match",
			"expected_name": containerName,
		})
		return
	}

	// Stop container first if running and force is true
	if info.State.Running && req.Force {
		timeout := 10
		if err := cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stop container: " + err.Error()})
			return
		}
	} else if info.State.Running {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container is running, set force=true to stop and delete"})
		return
	}

	// Remove container
	if err := cli.ContainerRemove(ctx, containerID, container.RemoveOptions{
		RemoveVolumes: false, // Don't remove volumes by default for safety
		Force:         req.Force,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete container: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "container deleted",
		"container_id": containerID,
		"name":         containerName,
	})
}

// getContainerLogsReal fetches logs from a Docker container
func (h *Handler) getContainerLogsReal(c *gin.Context) {
	containerID := c.Param("cid")

	// Query params
	tail := c.DefaultQuery("tail", "100")
	since := c.DefaultQuery("since", "")
	timestamps := c.DefaultQuery("timestamps", "true")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	cli, err := getDockerClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to Docker"})
		return
	}
	defer cli.Close()

	showTimestamps, _ := strconv.ParseBool(timestamps)

	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
		Timestamps: showTimestamps,
	}
	if since != "" {
		options.Since = since
	}

	reader, err := cli.ContainerLogs(ctx, containerID, options)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get container logs: " + err.Error()})
		return
	}
	defer reader.Close()

	// Read logs
	logs, err := io.ReadAll(reader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read logs: " + err.Error()})
		return
	}

	// Return as JSON with the raw log content
	c.JSON(http.StatusOK, gin.H{
		"container_id": containerID,
		"logs":         string(logs),
	})
}

// getContainerReal fetches detailed info for a single container
func (h *Handler) getContainerReal(c *gin.Context) {
	containerID := c.Param("cid")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	cli, err := getDockerClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to Docker"})
		return
	}
	defer cli.Close()

	info, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "container not found: " + err.Error()})
		return
	}

	// Get network names
	networks := make([]string, 0)
	for netName := range info.NetworkSettings.Networks {
		networks = append(networks, netName)
	}

	// Get stack name
	stackName := ""
	if project, ok := info.Config.Labels["com.docker.compose.project"]; ok {
		stackName = project
	}

	// Get stats if running
	var cpuPercent float64
	var memoryMB, memoryLimitMB int64
	if info.State.Running {
		cpuPercent, memoryMB, memoryLimitMB = getContainerStats(ctx, cli, containerID)
	}

	// Parse ports
	ports := make([]PortMapping, 0)
	for portProto, bindings := range info.NetworkSettings.Ports {
		parts := strings.Split(string(portProto), "/")
		containerPort, _ := strconv.Atoi(parts[0])
		protocol := "tcp"
		if len(parts) > 1 {
			protocol = parts[1]
		}

		for _, binding := range bindings {
			hostPort, _ := strconv.Atoi(binding.HostPort)
			ports = append(ports, PortMapping{
				ContainerPort: containerPort,
				HostPort:      hostPort,
				Protocol:      protocol,
				HostIP:        binding.HostIP,
			})
		}
	}

	// Parse mounts
	mounts := make([]MountInfo, 0)
	for _, m := range info.Mounts {
		mounts = append(mounts, MountInfo{
			Type:        string(m.Type),
			Source:      m.Source,
			Destination: m.Destination,
			Mode:        m.Mode,
			ReadOnly:    !m.RW,
		})
	}

	// Parse environment variables
	environment := make([]EnvVar, 0)
	for _, env := range info.Config.Env {
		parts := strings.SplitN(env, "=", 2)
		key := parts[0]
		value := ""
		if len(parts) > 1 {
			value = parts[1]
		}
		environment = append(environment, EnvVar{
			Key:   key,
			Value: value,
		})
	}

	// Build config
	restartPolicy := info.HostConfig.RestartPolicy.Name
	config := ContainerConfig{
		Hostname:      info.Config.Hostname,
		Domainname:    info.Config.Domainname,
		User:          info.Config.User,
		WorkingDir:    info.Config.WorkingDir,
		Entrypoint:    info.Config.Entrypoint,
		Cmd:           info.Config.Cmd,
		RestartPolicy: string(restartPolicy),
		Privileged:    info.HostConfig.Privileged,
		Tty:           info.Config.Tty,
		OpenStdin:     info.Config.OpenStdin,
	}

	// Parse health check
	var healthCheck *HealthCheckConfig
	if info.Config.Healthcheck != nil && len(info.Config.Healthcheck.Test) > 0 {
		healthCheck = &HealthCheckConfig{
			Test:        info.Config.Healthcheck.Test,
			Interval:    info.Config.Healthcheck.Interval.String(),
			Timeout:     info.Config.Healthcheck.Timeout.String(),
			StartPeriod: info.Config.Healthcheck.StartPeriod.String(),
			Retries:     info.Config.Healthcheck.Retries,
		}
	}

	// Health status and log
	healthStatus := ""
	healthLog := make([]HealthLogEntry, 0)
	if info.State.Health != nil {
		healthStatus = info.State.Health.Status
		for _, entry := range info.State.Health.Log {
			healthLog = append(healthLog, HealthLogEntry{
				Start:    entry.Start.Format(time.RFC3339),
				End:      entry.End.Format(time.RFC3339),
				ExitCode: entry.ExitCode,
				Output:   entry.Output,
			})
		}
	}

	// Network details
	networkDetails := make([]NetworkDetail, 0)
	for netName, netSettings := range info.NetworkSettings.Networks {
		networkDetails = append(networkDetails, NetworkDetail{
			Name:       netName,
			NetworkID:  netSettings.NetworkID,
			IPAddress:  netSettings.IPAddress,
			Gateway:    netSettings.Gateway,
			MacAddress: netSettings.MacAddress,
			Aliases:    netSettings.Aliases,
		})
	}

	// Resource limits
	var pidsLimit int64
	if info.HostConfig.PidsLimit != nil {
		pidsLimit = *info.HostConfig.PidsLimit
	}
	resources := ResourceLimits{
		CPUShares:   info.HostConfig.CPUShares,
		CPUQuota:    info.HostConfig.CPUQuota,
		CPUPeriod:   info.HostConfig.CPUPeriod,
		CPUSetCPUs:  info.HostConfig.CpusetCpus,
		MemoryLimit: info.HostConfig.Memory,
		MemorySwap:  info.HostConfig.MemorySwap,
		PidsLimit:   pidsLimit,
	}

	response := ContainerDetailResponse{
		ContainerResponse: ContainerResponse{
			ID:            info.ID,
			ContainerID:   info.ID,
			Name:          strings.TrimPrefix(info.Name, "/"),
			Image:         info.Config.Image,
			Status:        info.State.Status,
			State:         info.State.Status,
			StackName:     stackName,
			CPUPercent:    cpuPercent,
			MemoryMB:      memoryMB,
			MemoryLimitMB: memoryLimitMB,
			Networks:      networks,
			CreatedAt:     info.Created,
			RestartCount:  info.RestartCount,
		},
		Ports:          ports,
		Mounts:         mounts,
		Environment:    environment,
		Config:         config,
		HealthCheck:    healthCheck,
		HealthStatus:   healthStatus,
		HealthLog:      healthLog,
		Labels:         info.Config.Labels,
		NetworkDetails: networkDetails,
		Resources:      resources,
	}

	c.JSON(http.StatusOK, response)
}

// StackResponse represents a docker-compose stack
type StackResponse struct {
	Name          string              `json:"name"`
	ContainerCount int                `json:"container_count"`
	RunningCount  int                 `json:"running_count"`
	Status        string              `json:"status"`
	Containers    []ContainerResponse `json:"containers"`
}

// listStacksReal fetches docker-compose stacks from Docker daemon
func (h *Handler) listStacksReal(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	cli, err := getDockerClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to Docker"})
		return
	}
	defer cli.Close()

	// List all containers
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list containers: " + err.Error()})
		return
	}

	// Group containers by stack (compose project)
	stackMap := make(map[string]*StackResponse)

	for _, ctr := range containers {
		stackName := ""
		if project, ok := ctr.Labels["com.docker.compose.project"]; ok {
			stackName = project
		}

		// Skip containers not part of a compose stack
		if stackName == "" {
			continue
		}

		// Create stack entry if not exists
		if _, exists := stackMap[stackName]; !exists {
			stackMap[stackName] = &StackResponse{
				Name:       stackName,
				Containers: make([]ContainerResponse, 0),
			}
		}

		stack := stackMap[stackName]

		// Convert container to response format
		name := ""
		if len(ctr.Names) > 0 {
			name = strings.TrimPrefix(ctr.Names[0], "/")
		}

		status := ctr.State
		if ctr.State == "running" {
			stack.RunningCount++
		}

		// Get network names
		networks := make([]string, 0)
		for netName := range ctr.NetworkSettings.Networks {
			networks = append(networks, netName)
		}

		stack.Containers = append(stack.Containers, ContainerResponse{
			ID:          ctr.ID,
			ContainerID: ctr.ID,
			Name:        name,
			Image:       ctr.Image,
			Status:      status,
			State:       ctr.State,
			StackName:   stackName,
			Networks:    networks,
			CreatedAt:   time.Unix(ctr.Created, 0).Format(time.RFC3339),
		})

		stack.ContainerCount = len(stack.Containers)
	}

	// Convert map to slice and determine stack status
	result := make([]StackResponse, 0, len(stackMap))
	for _, stack := range stackMap {
		if stack.RunningCount == stack.ContainerCount {
			stack.Status = "running"
		} else if stack.RunningCount > 0 {
			stack.Status = "partial"
		} else {
			stack.Status = "stopped"
		}
		result = append(result, *stack)
	}

	c.JSON(http.StatusOK, result)
}
