// @title           QueueForge API
// @version         1.0
// @description     Distributed job processing system
// @host            localhost:8080
// @BasePath        /
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/ulasdursun/queueforge/config"
	api "github.com/ulasdursun/queueforge/internal/api"
	"github.com/ulasdursun/queueforge/internal/metrics"
	"github.com/ulasdursun/queueforge/internal/queue"
	"github.com/ulasdursun/queueforge/internal/repository"
	"github.com/ulasdursun/queueforge/internal/worker"
)

func main() {
	// Structured logging setup
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Config
	cfg := config.Load()
	switch cfg.AppMode {
	case "all", "api", "worker":
	default:
		log.Fatal().Str("app_mode", cfg.AppMode).Msg("Invalid APP_MODE; allowed values are: all, api, worker")
	}
	log.Info().Str("mode", cfg.AppMode).Msg("Starting QueueForge")

	// Prometheus metrics
	metrics.Register()

	// PostgreSQL
	db, err := repository.NewPostgresDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to PostgreSQL")
	}
	defer db.Close()
	log.Info().Msg("Connected to PostgreSQL")

	// Redis
	redisClient, err := queue.NewRedisClient(cfg.RedisURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis")
	}
	defer redisClient.Close()
	log.Info().Msg("Connected to Redis")

	// Repository + Queue
	jobRepo := repository.NewJobRepository(db)
	redisQueue := queue.NewRedisQueue(redisClient)

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Worker pool (started if mode is "all" or "worker")
	poolDone := make(chan struct{})
	if cfg.AppMode == "all" || cfg.AppMode == "worker" {
		pool := worker.NewPool(cfg.WorkerCount, redisQueue, jobRepo)
		go func() {
			pool.Start(ctx)
			close(poolDone)
		}()
	} else {
		close(poolDone)
	}

	// HTTP router configured according to running mode
	var httpHandler http.Handler
	if cfg.AppMode == "worker" {
		httpHandler = api.NewWorkerRouter(db, redisClient)
	} else {
		httpHandler = api.NewRouter(jobRepo, redisQueue, cfg.MaxRetries, db, redisClient)
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.ServerPort),
		Handler:      httpHandler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start server in background
	go func() {
		log.Info().Msg(fmt.Sprintf("QueueForge (%s mode) listening at http://127.0.0.1:%s", cfg.AppMode, cfg.ServerPort))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed")
		}
	}()

	// Wait for SIGTERM or SIGINT
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	log.Info().Msg("Shutdown signal received")

	// 1. Stop accepting new HTTP requests
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}

	// 2. Signal workers to stop taking new jobs and drain in-flight jobs
	cancel()

	select {
	case <-poolDone:
		log.Info().Msg("All workers drained cleanly")
	case <-time.After(10 * time.Second):
		log.Warn().Msg("Worker shutdown timed out after 10s")
	}

	log.Info().Msg("QueueForge stopped cleanly")
}
