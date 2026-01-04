package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

type Client struct {
	cli *client.Client
}

type ContainerInfo struct {
	ID            string
	Name          string
	Image         string
	Status        string
	State         string
	StackName     string
	CPUPercent    float64
	MemoryMB      int64
	MemoryLimitMB int64
	RestartCount  int
}

// NetworkInfo represents Docker network information
type NetworkInfo struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Scope      string            `json:"scope"`
	Internal   bool              `json:"internal"`
	Containers map[string]string `json:"containers"` // containerID -> containerName
}

// ContainerNetworkInfo represents network membership for a container
type ContainerNetworkInfo struct {
	NetworkID   string `json:"network_id"`
	NetworkName string `json:"network_name"`
	IPAddress   string `json:"ip_address"`
}

// NetworkAttachResult contains the result of a network attach operation
type NetworkAttachResult struct {
	Success      bool   `json:"success"`
	NetworkID    string `json:"network_id"`
	NetworkName  string `json:"network_name"`
	ErrorMessage string `json:"error_message,omitempty"`
}

func NewClient() (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	return &Client{cli: cli}, nil
}

func (c *Client) Close() error {
	return c.cli.Close()
}

// Client returns the underlying Docker client for advanced operations
func (c *Client) Client() *client.Client {
	return c.cli
}

func (c *Client) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}

	result := make([]ContainerInfo, 0, len(containers))
	for _, cont := range containers {
		name := ""
		if len(cont.Names) > 0 {
			name = strings.TrimPrefix(cont.Names[0], "/")
		}

		// Extract stack name from labels (docker-compose)
		stackName := ""
		if project, ok := cont.Labels["com.docker.compose.project"]; ok {
			stackName = project
		}

		info := ContainerInfo{
			ID:        cont.ID[:12],
			Name:      name,
			Image:     cont.Image,
			Status:    cont.Status,
			State:     cont.State,
			StackName: stackName,
		}

		result = append(result, info)
	}

	return result, nil
}

func (c *Client) InspectContainer(ctx context.Context, containerID string) (*types.ContainerJSON, error) {
	info, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *Client) StartContainer(ctx context.Context, containerID string) error {
	return c.cli.ContainerStart(ctx, containerID, container.StartOptions{})
}

func (c *Client) StopContainer(ctx context.Context, containerID string, timeout *int) error {
	options := container.StopOptions{}
	if timeout != nil {
		options.Timeout = timeout
	}
	return c.cli.ContainerStop(ctx, containerID, options)
}

func (c *Client) RestartContainer(ctx context.Context, containerID string, timeout *int) error {
	options := container.StopOptions{}
	if timeout != nil {
		options.Timeout = timeout
	}
	return c.cli.ContainerRestart(ctx, containerID, options)
}

func (c *Client) GetContainerLogs(ctx context.Context, containerID string, tail string, follow bool) error {
	// Returns io.ReadCloser for streaming
	_, err := c.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
		Follow:     follow,
		Timestamps: true,
	})
	return err
}

func (c *Client) GetContainerStats(ctx context.Context, containerID string) (*container.StatsResponse, error) {
	stats, err := c.cli.ContainerStatsOneShot(ctx, containerID)
	if err != nil {
		return nil, err
	}
	defer stats.Body.Close()

	// Parse stats from response body
	// In production, decode JSON stats
	return nil, nil
}

func (c *Client) ExecCreate(ctx context.Context, containerID string, cmd []string) (string, error) {
	resp, err := c.cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  true,
		Tty:          true,
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// ============ Network Operations ============

// ListNetworks returns all Docker networks, filtering out unsafe networks (host, none, overlay)
func (c *Client) ListNetworks(ctx context.Context) ([]NetworkInfo, error) {
	networks, err := c.cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}

	result := make([]NetworkInfo, 0, len(networks))
	for _, net := range networks {
		// Filter out unsafe networks
		if net.Name == "host" || net.Name == "none" {
			continue
		}
		if net.Driver == "overlay" {
			continue
		}

		// Build container map
		containers := make(map[string]string)
		for id, ep := range net.Containers {
			shortID := id
			if len(id) > 12 {
				shortID = id[:12]
			}
			containers[shortID] = ep.Name
		}

		result = append(result, NetworkInfo{
			ID:         net.ID[:12],
			Name:       net.Name,
			Driver:     net.Driver,
			Scope:      net.Scope,
			Internal:   net.Internal,
			Containers: containers,
		})
	}

	return result, nil
}

// InspectNetwork returns detailed info about a specific network
func (c *Client) InspectNetwork(ctx context.Context, networkID string) (*NetworkInfo, error) {
	net, err := c.cli.NetworkInspect(ctx, networkID, network.InspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to inspect network: %w", err)
	}

	// Build container map
	containers := make(map[string]string)
	for id, ep := range net.Containers {
		shortID := id
		if len(id) > 12 {
			shortID = id[:12]
		}
		containers[shortID] = ep.Name
	}

	return &NetworkInfo{
		ID:         net.ID[:12],
		Name:       net.Name,
		Driver:     net.Driver,
		Scope:      net.Scope,
		Internal:   net.Internal,
		Containers: containers,
	}, nil
}

// GetContainerNetworks returns all networks a container is connected to
func (c *Client) GetContainerNetworks(ctx context.Context, containerID string) ([]ContainerNetworkInfo, error) {
	info, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	if info.NetworkSettings == nil || info.NetworkSettings.Networks == nil {
		return []ContainerNetworkInfo{}, nil
	}

	result := make([]ContainerNetworkInfo, 0, len(info.NetworkSettings.Networks))
	for networkName, netSettings := range info.NetworkSettings.Networks {
		networkID := netSettings.NetworkID
		if len(networkID) > 12 {
			networkID = networkID[:12]
		}

		result = append(result, ContainerNetworkInfo{
			NetworkID:   networkID,
			NetworkName: networkName,
			IPAddress:   netSettings.IPAddress,
		})
	}

	return result, nil
}

// ConnectNetwork attaches a container to a network
func (c *Client) ConnectNetwork(ctx context.Context, networkID, containerID string) error {
	err := c.cli.NetworkConnect(ctx, networkID, containerID, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to network: %w", err)
	}
	return nil
}

// DisconnectNetwork detaches a container from a network
func (c *Client) DisconnectNetwork(ctx context.Context, networkID, containerID string) error {
	err := c.cli.NetworkDisconnect(ctx, networkID, containerID, false)
	if err != nil {
		return fmt.Errorf("failed to disconnect from network: %w", err)
	}
	return nil
}

// IsNetworkSafe validates if a network is safe to attach nginx to
// Returns (true, "") if safe, or (false, reason) if not
func (c *Client) IsNetworkSafe(ctx context.Context, networkID string) (bool, string) {
	net, err := c.cli.NetworkInspect(ctx, networkID, network.InspectOptions{})
	if err != nil {
		return false, "network not found"
	}

	// BLOCKED: host network
	if net.Name == "host" {
		return false, "cannot attach to host network"
	}

	// BLOCKED: none network
	if net.Name == "none" {
		return false, "cannot attach to none network"
	}

	// BLOCKED: overlay networks (Swarm mode)
	if net.Driver == "overlay" {
		return false, "overlay networks not supported (requires Swarm mode)"
	}

	// BLOCKED: non-local scope
	if net.Scope != "local" {
		return false, "only local scope networks are supported"
	}

	return true, ""
}

// IsContainerOnNetwork checks if a container is connected to a specific network
func (c *Client) IsContainerOnNetwork(ctx context.Context, containerID, networkID string) (bool, error) {
	networks, err := c.GetContainerNetworks(ctx, containerID)
	if err != nil {
		return false, err
	}

	for _, net := range networks {
		if net.NetworkID == networkID || net.NetworkName == networkID {
			return true, nil
		}
	}

	return false, nil
}
