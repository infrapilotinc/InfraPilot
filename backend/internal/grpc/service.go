package grpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AgentConnection represents an active agent connection
type AgentConnection struct {
	AgentID   string
	SendCh    chan *BackendMessage
	ResponseCh map[string]chan *AgentMessage // requestID -> response channel
	mu        sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// ConnectedAgents tracks all connected agents
var connectedAgents = sync.Map{} // agentID -> *AgentConnection

// AgentService implements the gRPC AgentService
type AgentService struct {
	UnimplementedAgentServiceServer
	db     *pgxpool.Pool
	logger *zap.Logger
}

func NewAgentService(db *pgxpool.Pool, logger *zap.Logger) *AgentService {
	return &AgentService{
		db:     db,
		logger: logger,
	}
}

// GetConnectedAgent returns an agent connection if connected
func GetConnectedAgent(agentID string) (*AgentConnection, bool) {
	conn, ok := connectedAgents.Load(agentID)
	if !ok {
		return nil, false
	}
	return conn.(*AgentConnection), true
}

// SendCommand sends a command to an agent and waits for response
func SendCommand(agentID string, cmd *BackendMessage, timeout time.Duration) (*AgentMessage, error) {
	conn, ok := GetConnectedAgent(agentID)
	if !ok {
		return nil, status.Error(codes.Unavailable, "agent not connected")
	}

	// Create response channel
	responseCh := make(chan *AgentMessage, 1)
	conn.mu.Lock()
	conn.ResponseCh[cmd.RequestId] = responseCh
	conn.mu.Unlock()

	defer func() {
		conn.mu.Lock()
		delete(conn.ResponseCh, cmd.RequestId)
		conn.mu.Unlock()
	}()

	// Send command
	select {
	case conn.SendCh <- cmd:
	case <-time.After(timeout):
		return nil, status.Error(codes.DeadlineExceeded, "send timeout")
	case <-conn.ctx.Done():
		return nil, status.Error(codes.Unavailable, "agent disconnected")
	}

	// Wait for response
	select {
	case resp := <-responseCh:
		return resp, nil
	case <-time.After(timeout):
		return nil, status.Error(codes.DeadlineExceeded, "response timeout")
	case <-conn.ctx.Done():
		return nil, status.Error(codes.Unavailable, "agent disconnected")
	}
}

// SendCommandAsync sends a command without waiting for response
func SendCommandAsync(agentID string, cmd *BackendMessage) error {
	conn, ok := GetConnectedAgent(agentID)
	if !ok {
		return status.Error(codes.Unavailable, "agent not connected")
	}

	select {
	case conn.SendCh <- cmd:
		return nil
	default:
		return status.Error(codes.ResourceExhausted, "agent command buffer full")
	}
}

// RegisterAgentServiceServer registers the service with a gRPC server
func RegisterAgentServiceServer(s *grpc.Server, srv *AgentService) {
	// In production, use generated protobuf code
	// s.RegisterService(&AgentService_ServiceDesc, srv)
}

// Register handles agent registration
func (s *AgentService) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	s.logger.Info("Agent registration request",
		zap.String("hostname", req.Hostname),
		zap.String("version", req.AgentVersion),
	)

	// Validate enrollment token
	var agentID uuid.UUID
	var orgID uuid.UUID
	err := s.db.QueryRow(ctx, `
		SELECT id, org_id FROM agents
		WHERE enrollment_token = $1 AND status = 'pending'
	`, req.EnrollmentToken).Scan(&agentID, &orgID)

	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid enrollment token")
	}

	// Generate certificate fingerprint (placeholder - in production use real mTLS)
	fingerprint := sha256.Sum256([]byte(agentID.String() + req.Hostname))
	fingerprintHex := hex.EncodeToString(fingerprint[:])

	// Update agent status and clear enrollment token
	_, err = s.db.Exec(ctx, `
		UPDATE agents
		SET status = 'active',
		    hostname = $1,
		    fingerprint = $2,
		    version = $3,
		    enrollment_token = NULL,
		    last_seen_at = NOW()
		WHERE id = $4
	`, req.Hostname, fingerprintHex, req.AgentVersion, agentID)

	if err != nil {
		s.logger.Error("Failed to update agent", zap.Error(err))
		return nil, status.Error(codes.Internal, "registration failed")
	}

	s.logger.Info("Agent registered successfully",
		zap.String("agent_id", agentID.String()),
		zap.String("hostname", req.Hostname),
	)

	// In production, generate real mTLS certificates here
	return &RegisterResponse{
		AgentId:              agentID.String(),
		ClientCert:           []byte{}, // Placeholder
		ClientKey:            []byte{}, // Placeholder
		HeartbeatIntervalSec: 30,
	}, nil
}

// Heartbeat handles agent heartbeats
func (s *AgentService) Heartbeat(ctx context.Context, req *HeartbeatRequest) (*HeartbeatResponse, error) {
	agentID, err := uuid.Parse(req.AgentId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid agent ID")
	}

	// Update last seen
	_, err = s.db.Exec(ctx, `
		UPDATE agents SET last_seen_at = NOW() WHERE id = $1
	`, agentID)

	if err != nil {
		return nil, status.Error(codes.Internal, "heartbeat failed")
	}

	// Sync container states
	if len(req.Containers) > 0 {
		for _, container := range req.Containers {
			_, err := s.db.Exec(ctx, `
				INSERT INTO containers (agent_id, container_id, name, image, stack_name, status,
				                         cpu_percent, memory_mb, memory_limit_mb, restart_count, last_synced_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
				ON CONFLICT (agent_id, container_id)
				DO UPDATE SET
					name = EXCLUDED.name,
					image = EXCLUDED.image,
					stack_name = EXCLUDED.stack_name,
					status = EXCLUDED.status,
					cpu_percent = EXCLUDED.cpu_percent,
					memory_mb = EXCLUDED.memory_mb,
					memory_limit_mb = EXCLUDED.memory_limit_mb,
					restart_count = EXCLUDED.restart_count,
					last_synced_at = NOW()
			`, agentID, container.ContainerId, container.Name, container.Image,
				nullString(container.StackName), container.Status,
				container.CpuPercent, container.MemoryMb, container.MemoryLimitMb, container.RestartCount)

			if err != nil {
				s.logger.Error("Failed to sync container", zap.Error(err))
			}
		}
	}

	return &HeartbeatResponse{
		Acknowledged:    true,
		PendingCommands: nil, // Would fetch from command queue
	}, nil
}

// CommandStream handles bidirectional command streaming
// This is called when an agent connects and maintains the stream
func (s *AgentService) CommandStream(agentID string, recvCh <-chan *AgentMessage, sendCh chan<- *BackendMessage) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := &AgentConnection{
		AgentID:    agentID,
		SendCh:     make(chan *BackendMessage, 100),
		ResponseCh: make(map[string]chan *AgentMessage),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Register connection
	connectedAgents.Store(agentID, conn)
	defer connectedAgents.Delete(agentID)

	s.logger.Info("Agent connected to command stream",
		zap.String("agent_id", agentID),
	)

	// Update agent status to connected
	s.db.Exec(ctx, `UPDATE agents SET status = 'active', last_seen_at = NOW() WHERE id = $1`, agentID)

	// Error channel for goroutines
	errCh := make(chan error, 2)

	// Goroutine to send commands to agent
	go func() {
		for {
			select {
			case cmd := <-conn.SendCh:
				select {
				case sendCh <- cmd:
					s.logger.Debug("Sent command to agent",
						zap.String("agent_id", agentID),
						zap.String("request_id", cmd.RequestId),
					)
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Goroutine to receive responses from agent
	go func() {
		for {
			select {
			case msg, ok := <-recvCh:
				if !ok {
					errCh <- nil // Channel closed normally
					return
				}

				s.logger.Debug("Received message from agent",
					zap.String("agent_id", agentID),
					zap.String("request_id", msg.RequestId),
				)

				// Route response to waiting handler
				conn.mu.Lock()
				if ch, exists := conn.ResponseCh[msg.RequestId]; exists {
					select {
					case ch <- msg:
					default:
						s.logger.Warn("Response channel full, dropping message",
							zap.String("request_id", msg.RequestId),
						)
					}
				}
				conn.mu.Unlock()

			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for error or context cancellation
	select {
	case err := <-errCh:
		s.logger.Info("Agent disconnected from command stream",
			zap.String("agent_id", agentID),
			zap.Error(err),
		)
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// StreamLogs handles log streaming from agent
// In production, this would use the generated protobuf streaming interface
func (s *AgentService) StreamLogs(ctx context.Context, req *LogStreamRequest) error {
	// In production, stream logs to Redis pub/sub
	return status.Error(codes.Unimplemented, "log streaming not implemented")
}

// PushMetrics handles metrics push from agent
func (s *AgentService) PushMetrics(ctx context.Context, req *MetricsRequest) (*MetricsResponse, error) {
	// In production, store metrics in time-series database or Redis
	return &MetricsResponse{Acknowledged: true}, nil
}

// Helper functions
func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Temporary types until protobuf is generated
type RegisterRequest struct {
	EnrollmentToken string
	Hostname        string
	AgentVersion    string
	OsInfo          string
}

type RegisterResponse struct {
	AgentId              string
	ClientCert           []byte
	ClientKey            []byte
	HeartbeatIntervalSec int32
}

type HeartbeatRequest struct {
	AgentId    string
	Metrics    *SystemMetrics
	Containers []*ContainerState
}

type HeartbeatResponse struct {
	Acknowledged    bool
	PendingCommands []*PendingCommand
}

type PendingCommand struct {
	RequestId   string
	CommandType string
	Payload     []byte
}

type SystemMetrics struct {
	CpuPercent    float64
	MemoryUsedMb  int64
	MemoryTotalMb int64
	DiskUsedMb    int64
	DiskTotalMb   int64
	UptimeSeconds int64
}

type ContainerState struct {
	ContainerId   string
	Name          string
	Image         string
	Status        string
	StackName     string
	CpuPercent    float64
	MemoryMb      int64
	MemoryLimitMb int64
	RestartCount  int32
}

type LogStreamRequest struct {
	Source      string
	ContainerID string
	Since       int64
	Follow      bool
}

type LogEntry struct {
	Timestamp int64
	Source    string
	Level     string
	Message   string
	Metadata  map[string]string
}

type MetricsRequest struct{}
type MetricsResponse struct {
	Acknowledged bool
}

// ============ Command Stream Types ============

type BackendMessage struct {
	RequestId string      `json:"request_id"`
	Command   interface{} `json:"command"` // One of: NginxCommand, DockerCommand, NetworkCommand, etc.
	Type      string      `json:"type"`    // "nginx", "docker", "network"
}

type AgentMessage struct {
	RequestId string      `json:"request_id"`
	Response  interface{} `json:"response"` // One of: CommandResult, ErrorResponse, etc.
}

// NginxCommand actions (string-based for JSON serialization)
const (
	NginxActionGetConfig   = "get_config"
	NginxActionWriteConfig = "write_config"
	NginxActionTestConfig  = "test_config"
	NginxActionReload      = "reload"
	NginxActionGetStatus   = "get_status"
	NginxActionRequestSSL  = "request_ssl"
)

type NginxCommand struct {
	Action        string `json:"action"`
	ConfigContent string `json:"config_content,omitempty"`
	ConfigPath    string `json:"config_path,omitempty"`
	Domain        string `json:"domain,omitempty"`
	Email         string `json:"email,omitempty"`
	DNSProvider   string `json:"dns_provider,omitempty"`
}

// DockerCommand actions
type DockerAction int

const (
	DockerActionUnspecified DockerAction = iota
	DockerActionList
	DockerActionStart
	DockerActionStop
	DockerActionRestart
	DockerActionInspect
	DockerActionLogs
	DockerActionStats
)

type DockerCommand struct {
	Action      DockerAction
	ContainerID string
	Options     map[string]string
}

// NetworkCommand actions (string-based for JSON serialization)
const (
	NetworkActionListNetworks         = "list_networks"
	NetworkActionGetContainerNetworks = "get_container_networks"
	NetworkActionCheckNginxNetwork    = "check_nginx"
	NetworkActionAttachNginxNetwork   = "attach_nginx"
	NetworkActionDetachNginxNetwork   = "detach_nginx"
)

type NetworkCommand struct {
	Action      string `json:"action"`
	NetworkID   string `json:"network_id,omitempty"`
	ContainerID string `json:"container_id,omitempty"`
}

// Response types
type CommandResult struct {
	Success bool
	Message string
	Data    json.RawMessage
}

type ErrorResponse struct {
	Code    int
	Message string
}

type NetworkInfo struct {
	ID         string
	Name       string
	Driver     string
	Scope      string
	Internal   bool
	Containers map[string]string
}

type NetworkListResponse struct {
	Networks []NetworkInfo
}

type NetworkAttachResult struct {
	Success      bool
	NetworkID    string
	NetworkName  string
	ErrorMessage string
}

type UnimplementedAgentServiceServer struct{}

// ============ Command Helpers ============

// NewNginxWriteConfigCommand creates a command to write nginx config
func NewNginxWriteConfigCommand(configContent, configPath string) *BackendMessage {
	return &BackendMessage{
		RequestId: uuid.New().String(),
		Type:      "nginx",
		Command: NginxCommand{
			Action:        NginxActionWriteConfig,
			ConfigContent: configContent,
			ConfigPath:    configPath,
		},
	}
}

// NewNginxTestConfigCommand creates a command to test nginx config
func NewNginxTestConfigCommand() *BackendMessage {
	return &BackendMessage{
		RequestId: uuid.New().String(),
		Type:      "nginx",
		Command: NginxCommand{
			Action: NginxActionTestConfig,
		},
	}
}

// NewNginxReloadCommand creates a command to reload nginx
func NewNginxReloadCommand() *BackendMessage {
	return &BackendMessage{
		RequestId: uuid.New().String(),
		Type:      "nginx",
		Command: NginxCommand{
			Action: NginxActionReload,
		},
	}
}

// NewNginxSSLCommand creates a command to request SSL certificate
func NewNginxSSLCommand(domain, email, dnsProvider string) *BackendMessage {
	return &BackendMessage{
		RequestId: uuid.New().String(),
		Type:      "nginx",
		Command: NginxCommand{
			Action:      NginxActionRequestSSL,
			Domain:      domain,
			Email:       email,
			DNSProvider: dnsProvider,
		},
	}
}

// NewNetworkAttachCommand creates a command to attach nginx to a network
func NewNetworkAttachCommand(networkID string) *BackendMessage {
	return &BackendMessage{
		RequestId: uuid.New().String(),
		Type:      "network",
		Command: NetworkCommand{
			Action:    NetworkActionAttachNginxNetwork,
			NetworkID: networkID,
		},
	}
}

// IsAgentConnected checks if an agent is currently connected
func IsAgentConnected(agentID string) bool {
	_, ok := connectedAgents.Load(agentID)
	return ok
}
