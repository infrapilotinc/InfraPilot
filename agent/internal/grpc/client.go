package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/infrapilot/agent/internal/docker"
	"github.com/infrapilot/agent/internal/metrics"
)

const agentVersion = "0.1.0"

type Client struct {
	conn            *grpc.ClientConn
	logger          *zap.Logger
	enrollmentToken string
	agentID         string
	backendAddr     string // Backend gRPC/WS address (e.g., "backend:9090")

	// Command stream
	sendCh   chan *AgentMessage
	recvCh   chan *BackendMessage
	streamMu sync.Mutex
}

// ============ Command Types (matching backend) ============

type BackendMessage struct {
	RequestId string          `json:"request_id"`
	Command   json.RawMessage `json:"command"`
	Type      string          `json:"type"` // "nginx", "docker", "network"
}

type AgentMessage struct {
	RequestId string      `json:"request_id"`
	Response  interface{} `json:"response"`
}

type NginxCommand struct {
	Action        string `json:"action"`
	ConfigContent string `json:"config_content"`
	ConfigPath    string `json:"config_path"`
	Domain        string `json:"domain"`
	Email         string `json:"email"`
	DNSProvider   string `json:"dns_provider"`
}

type NetworkCommand struct {
	Action      string `json:"action"`
	NetworkID   string `json:"network_id"`
	ContainerID string `json:"container_id"`
}

type CommandResult struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func NewClient(addr, enrollmentToken string, logger *zap.Logger) (*Client, error) {
	// In production, use mTLS credentials
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:            conn,
		logger:          logger,
		enrollmentToken: enrollmentToken,
		backendAddr:     addr,
		sendCh:          make(chan *AgentMessage, 100),
		recvCh:          make(chan *BackendMessage, 100),
	}, nil
}

func (c *Client) Close() error {
	close(c.sendCh)
	return c.conn.Close()
}

// SetAgentID sets the agent ID after registration
func (c *Client) SetAgentID(agentID string) {
	c.agentID = agentID
}

// GetRecvChannel returns the channel for receiving commands from backend
func (c *Client) GetRecvChannel() <-chan *BackendMessage {
	return c.recvCh
}

// SendResponse sends a response back to the backend
func (c *Client) SendResponse(requestID string, result *CommandResult) {
	c.sendCh <- &AgentMessage{
		RequestId: requestID,
		Response:  result,
	}
}

// SendError sends an error response back to the backend
func (c *Client) SendError(requestID string, code int, message string) {
	c.sendCh <- &AgentMessage{
		RequestId: requestID,
		Response: ErrorResponse{
			Code:    code,
			Message: message,
		},
	}
}

func (c *Client) Register(ctx context.Context, hostname string) (string, error) {
	// In production, use generated protobuf client
	c.logger.Info("Registering agent",
		zap.String("hostname", hostname),
		zap.String("version", agentVersion),
	)

	osInfo := runtime.GOOS + "/" + runtime.GOARCH

	// Placeholder for actual gRPC call
	// In production:
	// client := pb.NewAgentServiceClient(c.conn)
	// resp, err := client.Register(ctx, &pb.RegisterRequest{...})

	_ = osInfo
	_ = c.enrollmentToken

	// Return placeholder agent ID
	return "", nil
}

func (c *Client) Heartbeat(ctx context.Context, agentID string, sysMetrics *metrics.SystemMetrics, containers []docker.ContainerInfo) error {
	// In production, use generated protobuf client
	c.logger.Debug("Sending heartbeat",
		zap.String("agent_id", agentID),
		zap.Float64("cpu_percent", sysMetrics.CPUPercent),
		zap.Int64("memory_used_mb", sysMetrics.MemoryUsedMB),
		zap.Int("container_count", len(containers)),
	)

	// Placeholder for actual gRPC call
	return nil
}

// ConnectCommandStream establishes a bidirectional command stream with the backend
// Uses WebSocket for real-time communication
func (c *Client) ConnectCommandStream(ctx context.Context, handler CommandHandler) error {
	c.logger.Info("Connecting command stream via WebSocket", zap.String("agent_id", c.agentID))

	// Build WebSocket URL from backend address
	// The backend HTTP server typically runs on port 8080, gRPC on 9090
	// We connect to the HTTP server's WebSocket endpoint
	backendHTTPURL := os.Getenv("BACKEND_HTTP_URL")
	if backendHTTPURL == "" {
		backendHTTPURL = "http://backend:8080"
	}

	// Parse URL and convert to WebSocket
	parsedURL, err := url.Parse(backendHTTPURL)
	if err != nil {
		return fmt.Errorf("failed to parse backend URL: %w", err)
	}

	wsScheme := "ws"
	if parsedURL.Scheme == "https" {
		wsScheme = "wss"
	}
	wsURL := fmt.Sprintf("%s://%s/api/v1/agents/%s/ws/commands", wsScheme, parsedURL.Host, c.agentID)

	c.logger.Info("Connecting to WebSocket", zap.String("url", wsURL))

	// Connection retry loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Connect to WebSocket
		conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
		if err != nil {
			c.logger.Error("WebSocket connection failed, retrying in 5s",
				zap.Error(err),
				zap.String("url", wsURL),
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				continue
			}
		}

		c.logger.Info("WebSocket connected successfully", zap.String("agent_id", c.agentID))

		// Handle the connection
		err = c.handleWebSocketConnection(ctx, conn, handler)
		conn.Close()

		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.logger.Error("WebSocket connection lost, reconnecting in 5s", zap.Error(err))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				continue
			}
		}
	}
}

// handleWebSocketConnection manages a single WebSocket connection
func (c *Client) handleWebSocketConnection(ctx context.Context, conn *websocket.Conn, handler CommandHandler) error {
	// Channel for coordinating goroutines
	done := make(chan error, 2)

	// Start goroutine to read commands from WebSocket
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				done <- fmt.Errorf("read error: %w", err)
				return
			}

			var cmd BackendMessage
			if err := json.Unmarshal(msg, &cmd); err != nil {
				c.logger.Error("Failed to unmarshal command", zap.Error(err))
				continue
			}

			c.logger.Info("Received command from backend",
				zap.String("request_id", cmd.RequestId),
				zap.String("type", cmd.Type),
			)

			// Process command and send response
			go func(cmd BackendMessage) {
				result := handler.HandleCommand(ctx, &cmd)

				response := AgentMessage{
					RequestId: cmd.RequestId,
					Response:  result,
				}

				data, err := json.Marshal(response)
				if err != nil {
					c.logger.Error("Failed to marshal response", zap.Error(err))
					return
				}

				c.streamMu.Lock()
				err = conn.WriteMessage(websocket.TextMessage, data)
				c.streamMu.Unlock()

				if err != nil {
					c.logger.Error("Failed to send response", zap.Error(err))
					return
				}

				c.logger.Info("Sent response to backend",
					zap.String("request_id", cmd.RequestId),
					zap.Bool("success", result.Success),
				)
			}(cmd)
		}
	}()

	// Start goroutine to send ping/keepalive
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				done <- ctx.Err()
				return
			case <-ticker.C:
				c.streamMu.Lock()
				err := conn.WriteMessage(websocket.PingMessage, nil)
				c.streamMu.Unlock()

				if err != nil {
					done <- fmt.Errorf("ping error: %w", err)
					return
				}
				c.logger.Debug("WebSocket ping sent")
			}
		}
	}()

	// Wait for error or context cancellation
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CommandHandler interface for processing backend commands
type CommandHandler interface {
	HandleCommand(ctx context.Context, cmd *BackendMessage) *CommandResult
}

// QueueCommand adds a command to the receive queue (for testing/simulation)
func (c *Client) QueueCommand(cmd *BackendMessage) {
	select {
	case c.recvCh <- cmd:
	default:
		c.logger.Warn("Command queue full, dropping command",
			zap.String("request_id", cmd.RequestId),
		)
	}
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}
