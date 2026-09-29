package config

import (
	"os"
	"strconv"
)

type Config struct {
	AgentID            string
	BackendGRPCAddr    string
	BackendHTTPURL     string // HTTP URL for API polling
	// InternalServiceToken must match the backend's INTERNAL_SERVICE_TOKEN for the
	// HTTP proxy poller to run. Empty disables the poller.
	InternalServiceToken string
	EnrollmentToken    string
	HeartbeatInterval  int
	NginxConfigPath    string
	NginxContainerName string
	CertPath           string
	KeyPath            string
	ProxyMode          string // "managed" (default) or "external"
	DataDir            string // Directory for agent data (credentials, state)
	LogPersistence     bool   // Enable log persistence (stream logs to backend)

	// SSL/ACME settings
	LetsEncryptDir   string // Directory for Let's Encrypt certs
	LetsEncryptEmail string // Email for Let's Encrypt account
	LetsEncryptStage bool   // Use staging server for testing
	ACMEWebRoot      string // Webroot for ACME HTTP-01 challenge

	// Nginx log analytics settings
	NginxLogAnalytics     bool   // Enable nginx access log analytics
	NginxAccessLogPath    string // Path to nginx access log
	NginxLogBufferSize    int    // Number of entries to buffer before flush
	NginxLogFlushInterval int    // Flush interval in seconds
	NginxLogSkipHealth    bool   // Skip health check endpoints
}

// IsManagedProxy returns true if InfraPilot should manage the proxy
func (c *Config) IsManagedProxy() bool {
	return c.ProxyMode != "external"
}

func Load() (*Config, error) {
	cfg := &Config{
		AgentID:            getEnv("AGENT_ID", ""),
		BackendGRPCAddr:    getEnv("BACKEND_GRPC_ADDR", "localhost:9090"),
		BackendHTTPURL:     getEnv("BACKEND_HTTP_URL", "http://backend:8080"),
		InternalServiceToken: getEnv("INTERNAL_SERVICE_TOKEN", ""),
		EnrollmentToken:    getEnv("ENROLLMENT_TOKEN", ""),
		HeartbeatInterval:  getEnvInt("HEARTBEAT_INTERVAL", 30),
		NginxConfigPath:    getEnv("NGINX_CONFIG_PATH", "/etc/nginx/conf.d"),
		NginxContainerName: getEnv("NGINX_CONTAINER_NAME", "infrapilot-nginx"),
		CertPath:           getEnv("CERT_PATH", ""),
		KeyPath:            getEnv("KEY_PATH", ""),
		ProxyMode:          getEnv("PROXY_MODE", "managed"), // "managed" or "external"
		DataDir:            getEnv("DATA_DIR", "/var/lib/infrapilot-agent"),

		LogPersistence: getEnvBool("LOG_PERSISTENCE", false),

		// SSL/ACME settings
		LetsEncryptDir:   getEnv("LETSENCRYPT_DIR", "/etc/letsencrypt"),
		LetsEncryptEmail: getEnv("LETSENCRYPT_EMAIL", ""),
		LetsEncryptStage: getEnvBool("LETSENCRYPT_STAGING", false),
		ACMEWebRoot:      getEnv("ACME_WEBROOT", "/var/www/acme-challenge"),

		// Nginx log analytics settings
		NginxLogAnalytics:     getEnvBool("NGINX_LOG_ANALYTICS", true),
		NginxAccessLogPath:    getEnv("NGINX_ACCESS_LOG_PATH", "/var/log/nginx/access.log"),
		NginxLogBufferSize:    getEnvInt("NGINX_LOG_BUFFER_SIZE", 500),
		NginxLogFlushInterval: getEnvInt("NGINX_LOG_FLUSH_INTERVAL", 5),
		NginxLogSkipHealth:    getEnvBool("NGINX_LOG_SKIP_HEALTH", true),
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}
