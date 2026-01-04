package logstreamer

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

// LogEntry represents a single log entry to send to backend
type LogEntry struct {
	Source     string            `json:"source"`
	SourceType string            `json:"source_type"`
	Stream     string            `json:"stream"`
	Level      string            `json:"level"`
	Message    string            `json:"message"`
	Timestamp  time.Time         `json:"timestamp"`
	Labels     map[string]string `json:"labels,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
}

// LogBatch represents a batch of log entries
type LogBatch struct {
	AgentID string     `json:"agent_id"`
	Entries []LogEntry `json:"entries"`
}

// Streamer handles collecting and forwarding logs to the backend
type Streamer struct {
	docker      *client.Client
	backendURL  string
	agentID     string
	httpClient  *http.Client
	logger      *zap.Logger

	// Buffering
	buffer     []LogEntry
	bufferMu   sync.Mutex
	bufferSize int
	flushInterval time.Duration

	// Container tracking
	containers   map[string]context.CancelFunc
	containersMu sync.Mutex

	// Level detection regex
	levelPatterns map[string]*regexp.Regexp
}

// NewStreamer creates a new log streamer
func NewStreamer(docker *client.Client, backendURL, agentID string, logger *zap.Logger) *Streamer {
	return &Streamer{
		docker:        docker,
		backendURL:    backendURL,
		agentID:       agentID,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		logger:        logger,
		buffer:        make([]LogEntry, 0, 100),
		bufferSize:    100,
		flushInterval: 5 * time.Second,
		containers:    make(map[string]context.CancelFunc),
		levelPatterns: map[string]*regexp.Regexp{
			"error": regexp.MustCompile(`(?i)\b(error|err|fatal|panic|exception|fail)\b`),
			"warn":  regexp.MustCompile(`(?i)\b(warn|warning)\b`),
			"debug": regexp.MustCompile(`(?i)\b(debug|trace)\b`),
		},
	}
}

// Start begins the log streaming process
func (s *Streamer) Start(ctx context.Context) error {
	s.logger.Info("Starting log streamer",
		zap.String("backend", s.backendURL),
		zap.String("agent_id", s.agentID),
	)

	// Start flush loop
	go s.flushLoop(ctx)

	// Start container watcher
	go s.watchContainers(ctx)

	return nil
}

// flushLoop periodically flushes buffered logs to the backend
func (s *Streamer) flushLoop(ctx context.Context) {
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final flush
			s.flush()
			s.logger.Info("Log streamer stopped")
			return
		case <-ticker.C:
			s.flush()
		}
	}
}

// flush sends buffered logs to the backend
func (s *Streamer) flush() {
	s.bufferMu.Lock()
	if len(s.buffer) == 0 {
		s.bufferMu.Unlock()
		return
	}

	// Copy and clear buffer
	entries := make([]LogEntry, len(s.buffer))
	copy(entries, s.buffer)
	s.buffer = s.buffer[:0]
	s.bufferMu.Unlock()

	// Send to backend
	batch := LogBatch{
		AgentID: s.agentID,
		Entries: entries,
	}

	data, err := json.Marshal(batch)
	if err != nil {
		s.logger.Error("Failed to marshal log batch", zap.Error(err))
		return
	}

	url := s.backendURL + "/api/v1/logs/ingest"
	resp, err := s.httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		s.logger.Error("Failed to send logs to backend",
			zap.Error(err),
			zap.Int("entries", len(entries)),
		)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error("Backend rejected logs",
			zap.Int("status", resp.StatusCode),
			zap.String("response", string(body)),
		)
		return
	}

	s.logger.Debug("Flushed logs to backend", zap.Int("count", len(entries)))
}

// addEntry adds a log entry to the buffer
func (s *Streamer) addEntry(entry LogEntry) {
	s.bufferMu.Lock()
	defer s.bufferMu.Unlock()

	s.buffer = append(s.buffer, entry)

	// Flush if buffer is full
	if len(s.buffer) >= s.bufferSize {
		go s.flush()
	}
}

// watchContainers monitors for container starts/stops and manages log collection
func (s *Streamer) watchContainers(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncContainers(ctx)
		}
	}
}

// syncContainers updates the set of containers we're collecting logs from
func (s *Streamer) syncContainers(ctx context.Context) {
	containers, err := s.docker.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		s.logger.Error("Failed to list containers", zap.Error(err))
		return
	}

	// Track current container IDs
	currentIDs := make(map[string]bool)
	for _, c := range containers {
		currentIDs[c.ID] = true

		s.containersMu.Lock()
		_, exists := s.containers[c.ID]
		s.containersMu.Unlock()

		if !exists {
			// Start collecting logs from this container
			logCtx, cancel := context.WithCancel(ctx)
			s.containersMu.Lock()
			s.containers[c.ID] = cancel
			s.containersMu.Unlock()

			go s.collectContainerLogs(logCtx, c.ID, strings.TrimPrefix(c.Names[0], "/"), c.Labels)
		}
	}

	// Stop collecting from removed containers
	s.containersMu.Lock()
	for id, cancel := range s.containers {
		if !currentIDs[id] {
			cancel()
			delete(s.containers, id)
		}
	}
	s.containersMu.Unlock()
}

// collectContainerLogs streams logs from a container
func (s *Streamer) collectContainerLogs(ctx context.Context, containerID, containerName string, labels map[string]string) {
	s.logger.Debug("Starting log collection", zap.String("container", containerName))

	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: true,
		Since:      "0s",
		Tail:       "100", // Start with last 100 lines
	}

	reader, err := s.docker.ContainerLogs(ctx, containerID, options)
	if err != nil {
		s.logger.Error("Failed to get container logs",
			zap.String("container", containerName),
			zap.Error(err),
		)
		return
	}
	defer reader.Close()

	buf := make([]byte, 8192)
	for {
		select {
		case <-ctx.Done():
			s.logger.Debug("Stopping log collection", zap.String("container", containerName))
			return
		default:
		}

		n, err := reader.Read(buf)
		if err != nil {
			if err == io.EOF {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			if ctx.Err() != nil {
				return
			}
			s.logger.Debug("Log read error", zap.String("container", containerName), zap.Error(err))
			return
		}

		if n == 0 {
			continue
		}

		// Parse log lines
		lines := strings.Split(string(buf[:n]), "\n")
		for _, line := range lines {
			if len(line) == 0 {
				continue
			}

			entry := s.parseLine(line, containerName, labels)
			if entry != nil {
				s.addEntry(*entry)
			}
		}
	}
}

// parseLine parses a Docker log line into a LogEntry
func (s *Streamer) parseLine(line, containerName string, labels map[string]string) *LogEntry {
	// Docker logs have 8-byte header for non-TTY containers
	// Format: [stream][0][0][0][size (4 bytes)][message]
	stream := "stdout"
	message := line

	if len(line) > 8 {
		header := line[0]
		if header == 1 {
			stream = "stdout"
			// Skip 8-byte header
			if len(line) > 8 {
				message = line[8:]
			}
		} else if header == 2 {
			stream = "stderr"
			if len(line) > 8 {
				message = line[8:]
			}
		}
	}

	// Try to parse timestamp from beginning of message
	// Docker format: 2006-01-02T15:04:05.999999999Z
	timestamp := time.Now()
	if len(message) > 30 && message[4] == '-' && message[7] == '-' {
		if t, err := time.Parse(time.RFC3339Nano, message[:30]); err == nil {
			timestamp = t
			message = strings.TrimSpace(message[31:])
		}
	}

	// Detect log level
	level := s.detectLevel(message)

	// Clean up labels for storage
	cleanLabels := make(map[string]string)
	for k, v := range labels {
		// Only include useful labels
		if strings.HasPrefix(k, "com.docker.compose") ||
			strings.HasPrefix(k, "maintainer") ||
			strings.HasPrefix(k, "org.") {
			cleanLabels[k] = v
		}
	}

	return &LogEntry{
		Source:     containerName,
		SourceType: "container",
		Stream:     stream,
		Level:      level,
		Message:    message,
		Timestamp:  timestamp,
		Labels:     cleanLabels,
	}
}

// detectLevel tries to detect the log level from the message
func (s *Streamer) detectLevel(message string) string {
	for level, pattern := range s.levelPatterns {
		if pattern.MatchString(message) {
			return level
		}
	}
	return "info"
}

// Stop stops the log streamer
func (s *Streamer) Stop() {
	s.containersMu.Lock()
	for _, cancel := range s.containers {
		cancel()
	}
	s.containers = make(map[string]context.CancelFunc)
	s.containersMu.Unlock()

	// Final flush
	s.flush()
}
