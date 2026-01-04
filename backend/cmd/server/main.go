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
	"github.com/infrapilot/backend/internal/db"
	"github.com/infrapilot/backend/internal/enterprise/license"
	agentgrpc "github.com/infrapilot/backend/internal/grpc"
)

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

	// Connect to PostgreSQL
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
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

	// Initialize license
	if err := license.Init(); err != nil {
		logger.Warn("Failed to initialize license", zap.Error(err))
	}
	logger.Info("InfraPilot Community Edition started")

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
	apiHandler := api.NewHandler(pool, authService, logger)
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
