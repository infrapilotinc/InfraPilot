package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	agentgrpc "github.com/infrapilot/backend/internal/grpc"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins in development
		// TODO: Restrict in production
		return true
	},
}

// execContainer handles WebSocket exec sessions to containers
func (h *Handler) execContainer(c *gin.Context) {
	containerId := c.Param("cid")

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("Failed to upgrade WebSocket", zap.Error(err))
		return
	}
	defer conn.Close()

	h.logger.Info("WebSocket exec session started",
		zap.String("container", containerId))

	// Create Docker client
	docker, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		h.sendWSError(conn, "Failed to connect to Docker")
		return
	}
	defer docker.Close()

	ctx := context.Background()

	// Create exec instance
	execConfig := container.ExecOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          []string{"/bin/sh"},
	}

	execResp, err := docker.ContainerExecCreate(ctx, containerId, execConfig)
	if err != nil {
		h.logger.Error("Failed to create exec", zap.Error(err))
		h.sendWSError(conn, "Failed to create exec session: "+err.Error())
		return
	}

	// Attach to exec
	attachResp, err := docker.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{
		Tty: true,
	})
	if err != nil {
		h.logger.Error("Failed to attach to exec", zap.Error(err))
		h.sendWSError(conn, "Failed to attach to exec session: "+err.Error())
		return
	}
	defer attachResp.Close()

	// Create done channel
	done := make(chan struct{})
	var once sync.Once

	// Forward Docker output to WebSocket
	go func() {
		defer once.Do(func() { close(done) })
		buf := make([]byte, 1024)
		for {
			n, err := attachResp.Reader.Read(buf)
			if err != nil {
				if err != io.EOF {
					h.logger.Debug("Docker read error", zap.Error(err))
				}
				return
			}
			if n > 0 {
				if err := conn.WriteMessage(websocket.TextMessage, buf[:n]); err != nil {
					h.logger.Debug("WebSocket write error", zap.Error(err))
					return
				}
			}
		}
	}()

	// Forward WebSocket input to Docker
	go func() {
		defer once.Do(func() { close(done) })
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					h.logger.Debug("WebSocket read error", zap.Error(err))
				}
				return
			}
			if _, err := attachResp.Conn.Write(msg); err != nil {
				h.logger.Debug("Docker write error", zap.Error(err))
				return
			}
		}
	}()

	// Keep connection alive with pings
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	// Wait for session to end
	<-done

	h.logger.Info("WebSocket exec session ended",
		zap.String("container", containerId))
}

// streamContainerLogs handles WebSocket log streaming
func (h *Handler) streamContainerLogs(c *gin.Context) {
	containerId := c.Param("cid")

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("Failed to upgrade WebSocket", zap.Error(err))
		return
	}
	defer conn.Close()

	h.logger.Info("WebSocket log stream started",
		zap.String("container", containerId))

	// Create Docker client
	docker, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		h.sendWSError(conn, "Failed to connect to Docker")
		return
	}
	defer docker.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Get container logs with follow
	logs, err := docker.ContainerLogs(ctx, containerId, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       "100",
		Timestamps: true,
	})
	if err != nil {
		h.logger.Error("Failed to get container logs", zap.Error(err))
		h.sendWSError(conn, "Failed to get logs: "+err.Error())
		return
	}
	defer logs.Close()

	done := make(chan struct{})

	// Forward logs to WebSocket
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := logs.Read(buf)
			if err != nil {
				if err != io.EOF {
					h.logger.Debug("Log read error", zap.Error(err))
				}
				return
			}
			if n > 0 {
				// Docker log format has 8-byte header for multiplexed streams
				// For TTY containers, no header; for non-TTY, strip header
				data := buf[:n]
				if len(data) > 8 && (data[0] == 1 || data[0] == 2) {
					// Has header, strip it
					data = data[8:]
				}
				if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
					h.logger.Debug("WebSocket write error", zap.Error(err))
					return
				}
			}
		}
	}()

	// Handle WebSocket close
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				cancel()
				return
			}
		}
	}()

	// Keep connection alive
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	<-done

	h.logger.Info("WebSocket log stream ended",
		zap.String("container", containerId))
}

func (h *Handler) sendWSError(conn *websocket.Conn, msg string) {
	conn.WriteMessage(websocket.TextMessage, []byte("\x1b[31mError: "+msg+"\x1b[0m\r\n"))
}

// agentCommandStream handles WebSocket command streaming for agents
// This replaces the gRPC bidirectional stream with a WebSocket-based approach
func (h *Handler) agentCommandStream(c *gin.Context) {
	agentIDStr := c.Param("id")

	// Validate agent ID
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Verify agent exists
	var exists bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1)
	`, agentID).Scan(&exists)
	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("Failed to upgrade WebSocket for agent", zap.Error(err))
		return
	}
	defer conn.Close()

	h.logger.Info("Agent connected via WebSocket",
		zap.String("agent_id", agentIDStr))

	// Create channels for command stream
	recvCh := make(chan *agentgrpc.AgentMessage, 100)
	sendCh := make(chan *agentgrpc.BackendMessage, 100)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the backend's command stream handler in a goroutine
	go func() {
		agentService := agentgrpc.NewAgentService(h.db, h.logger)
		err := agentService.CommandStream(agentIDStr, recvCh, sendCh)
		if err != nil {
			h.logger.Debug("Agent command stream ended", zap.Error(err))
		}
		cancel()
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine to send commands to agent via WebSocket
	go func() {
		defer wg.Done()
		for {
			select {
			case cmd, ok := <-sendCh:
				if !ok {
					return
				}
				data, err := json.Marshal(cmd)
				if err != nil {
					h.logger.Error("Failed to marshal command", zap.Error(err))
					continue
				}
				if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
					h.logger.Debug("WebSocket write error", zap.Error(err))
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Goroutine to receive responses from agent via WebSocket
	go func() {
		defer wg.Done()
		defer close(recvCh)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					h.logger.Debug("WebSocket read error", zap.Error(err))
				}
				cancel()
				return
			}

			var agentMsg agentgrpc.AgentMessage
			if err := json.Unmarshal(msg, &agentMsg); err != nil {
				h.logger.Error("Failed to unmarshal agent message", zap.Error(err))
				continue
			}

			select {
			case recvCh <- &agentMsg:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Keep connection alive with pings
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					cancel()
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for context to be cancelled
	<-ctx.Done()
	wg.Wait()

	h.logger.Info("Agent disconnected from WebSocket",
		zap.String("agent_id", agentIDStr))
}
