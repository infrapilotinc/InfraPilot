package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/infrapilot/backend/internal/alerts"
	"github.com/infrapilot/backend/internal/api"
	"github.com/infrapilot/backend/internal/auth"
	"github.com/infrapilot/backend/internal/config"
	"github.com/infrapilot/backend/internal/crypto"
	"github.com/infrapilot/backend/internal/db"
	agentgrpc "github.com/infrapilot/backend/internal/grpc"
	"github.com/infrapilot/backend/internal/license"
	"github.com/infrapilot/backend/internal/telemetry"
)

// version is injected at build time via -ldflags.
var version = "dev"

func main() {
	// Initialize logger
	logger, _ := zap.NewProduction()
	if os.Getenv("ENV") == "development" {
		logger, _ = zap.NewDevelopment()
	}
	defer logger.Sync()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	// Connect to PostgreSQL with connection pool configuration
	ctx := context.Background()
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("Failed to parse database URL", zap.Error(err))
	}

	// Configure connection pool limits to prevent exhaustion
	poolConfig.MaxConns = 20                      // Max connections (leave room for other clients)
	poolConfig.MinConns = 2                       // Keep some connections warm
	poolConfig.MaxConnLifetime = 30 * time.Minute // Recycle connections periodically
	poolConfig.MaxConnIdleTime = 5 * time.Minute  // Close idle connections
	poolConfig.HealthCheckPeriod = time.Minute    // Check connection health

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer pool.Close()

	// Verify database connection
	if err := pool.Ping(ctx); err != nil {
		logger.Fatal("Failed to ping database", zap.Error(err))
	}
	logger.Info("Connected to PostgreSQL")

	// Run database migrations
	if err := db.RunMigrations(ctx, pool, logger); err != nil {
		logger.Fatal("Failed to run migrations", zap.Error(err))
	}

	// ---------------------------------------------------------------
	// License validation
	// ---------------------------------------------------------------
	var licenseClient *license.Client
	if os.Getenv("LICENSE_OFFLINE") == "true" && cfg.Env != "production" {
		licenseClient = license.NewOfflineClient(logger)
	} else if cfg.LicenseKey != "" {
		// ENV var set — validate immediately, but fall back to setup mode on failure
		// so the operator can enter a new key via the UI rather than getting a crash loop.
		licenseClient, err = license.NewClient(cfg.LicenseKey, cfg.DataDir, version, logger)
		if err != nil {
			logger.Warn("Failed to initialize license client — starting in setup mode",
				zap.Error(err))
			licenseClient = nil
		} else {
			resp, validateErr := licenseClient.Validate()
			if validateErr != nil {
				// Network error reaching infrapilot.org — keep the client so it can
				// retry in the background; features will be restricted until reachable.
				logger.Warn("License validation failed at startup — features restricted until infrapilot.org is reachable",
					zap.Error(validateErr))
			} else if !resp.Valid {
				// Key is explicitly rejected — drop to setup mode so the user can
				// enter a valid key via the web UI instead of crash-looping.
				logger.Warn("LICENSE_KEY is invalid — starting in setup mode",
					zap.String("error", resp.Error),
					zap.String("hint", "Visit http://localhost/setup to enter a valid license key"),
				)
				licenseClient = nil
			} else {
				logger.Info("License validated",
					zap.String("tier", resp.Tier),
					zap.Int("max_agents", resp.MaxAgents),
				)
			}
		}
		if licenseClient == nil {
			// No usable license → keyless Community Edition (valid, 1 agent, community
			// features). NOT setup mode: setup mode grants ALL features, which after
			// initial setup is both wrong (keyless CE is community) and a privilege
			// hole. This is what Settings → License reflects.
			licenseClient = license.NewCommunityModeClient(logger)
		}
	} else {
		// Try to load license key from system_settings (saved via setup wizard)
		var savedKey string
		pool.QueryRow(ctx, `
			SELECT setting_value->>'key' FROM system_settings
			WHERE org_id = '00000000-0000-0000-0000-000000000001'
			AND setting_key = 'license_key'
		`).Scan(&savedKey)

		if savedKey != "" {
			licenseClient, err = license.NewClient(savedKey, cfg.DataDir, version, logger)
			if err == nil {
				resp, validateErr := licenseClient.Validate()
				if validateErr == nil && resp != nil && resp.Valid {
					logger.Info("License loaded from database", zap.String("tier", resp.Tier))
				} else if validateErr != nil {
					// Network error reaching infrapilot.org — keep the client.
					// HasFeature() returns false until validation succeeds (safe default).
					// Retries happen automatically, throttled to once per 5 minutes.
					logger.Warn("License validation failed at startup — features restricted until infrapilot.org is reachable",
						zap.Error(validateErr))
				} else {
					// resp.Valid == false: key is explicitly invalid
					logger.Warn("License key stored in database is invalid — starting in setup mode",
						zap.String("reason", resp.Error))
					licenseClient = nil
				}
			} else {
				logger.Error("Failed to create license client", zap.Error(err))
				licenseClient = nil
			}
		}

		if licenseClient == nil {
			// No usable license → keyless Community Edition (valid, 1 agent, community
			// features). NOT setup mode: setup mode grants ALL features, which after
			// initial setup is both wrong (keyless CE is community) and a privilege
			// hole. This is what Settings → License reflects.
			licenseClient = license.NewCommunityModeClient(logger)
		}
	}

	logger.Info("InfraPilot started", zap.String("version", version), zap.String("tier", licenseClient.Tier()))

	// Anonymous, opt-out product-funnel telemetry (v3/40). Keyless CE never validates, so
	// this is the only channel that makes the free funnel visible. Best-effort, non-blocking.
	tel := telemetry.New(cfg.DataDir, "community", version, licenseClient.Tier, logger)
	tel.EmitOnce("installed", nil)
	go tel.StartHeartbeat(ctx)

	// Initialize encryption service (optional but recommended)
	var encryptionSvc *crypto.EncryptionService
	if cfg.EncryptionKey != "" {
		var err error
		encryptionSvc, err = crypto.NewEncryptionService(cfg.EncryptionKey)
		if err != nil {
			logger.Fatal("Failed to initialize encryption service", zap.Error(err))
		}
		logger.Info("Encryption service initialized")
	} else {
		logger.Warn("ENCRYPTION_KEY not set - webhook signature verification and credential encryption will be disabled")
	}

	// Initialize auth service
	authService := auth.NewService(cfg.JWTSecret, cfg.JWTExpiry)

	// Initialize HTTP server (Gin)
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(api.LoggerMiddleware(logger))
	router.Use(api.CORSMiddleware(cfg.AllowedOrigins))

	// Setup API routes
	apiHandler := api.NewHandler(pool, authService, logger, encryptionSvc, licenseClient, cfg, version, tel)
	apiHandler.RegisterRoutes(router)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Initialize gRPC server for agents
	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(16 * 1024 * 1024), // 16MB
		grpc.MaxSendMsgSize(16 * 1024 * 1024),
	)
	agentService := agentgrpc.NewAgentService(pool, logger)
	agentgrpc.RegisterAgentServiceServer(grpcServer, agentService)

	// Start gRPC server
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		logger.Fatal("Failed to listen on gRPC port", zap.Error(err))
	}

	go func() {
		logger.Info("Starting gRPC server", zap.Int("port", cfg.GRPCPort))
		if err := grpcServer.Serve(grpcListener); err != nil {
			logger.Fatal("gRPC server failed", zap.Error(err))
		}
	}()

	// Start HTTP server
	go func() {
		logger.Info("Starting HTTP server", zap.Int("port", cfg.HTTPPort))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP server failed", zap.Error(err))
		}
	}()

	// Start background tasks (like dispatching default page config on startup)
	apiHandler.StartBackgroundTasks(ctx)

	// Start alert evaluator
	var alertEvaluator *alerts.AlertEvaluator
	alertEvaluator, err = alerts.NewAlertEvaluator(pool, logger)
	if err != nil {
		logger.Warn("Failed to initialize alert evaluator", zap.Error(err))
	} else {
		alertEvaluator.Start(ctx, 30*time.Second) // Check every 30 seconds
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down servers...")

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("HTTP server forced to shutdown", zap.Error(err))
	}

	// Shutdown gRPC server
	grpcServer.GracefulStop()

	// Stop alert evaluator
	if alertEvaluator != nil {
		alertEvaluator.Stop()
	}

	logger.Info("Servers stopped")
}
