package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/aanthord/pubsub-amqp/internal/config"
	"github.com/aanthord/pubsub-amqp/internal/handlers"
	"github.com/aanthord/pubsub-amqp/internal/metrics"
	"github.com/aanthord/pubsub-amqp/internal/tracing"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	httpSwagger "github.com/swaggo/http-swagger"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Initialize structured logging
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()
	log := logger.Sugar()

	// Load environment variables
	if err := loadEnv(); err != nil {
		log.Fatalf("Failed to load environment variables: %v", err)
	}

	// Print version information
	log.Infow("Starting application",
		"version", version,
		"commit", commit,
		"build_date", date,
	)

	// Initialize config
	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	// Initialize tracing
	tracer, closer, err := tracing.InitJaeger("pubsub-amqp")
	if err != nil {
		log.Fatalf("Could not initialize Jaeger tracer: %v", err)
	}
	defer closer.Close()

	// Initialize metrics
	metrics.Init()

	// Create router
	router := mux.NewRouter()

	// Setup middleware
	router.Use(loggingMiddleware(log))
	router.Use(recoveryMiddleware(log))
	router.Use(tracing.Middleware(tracer))
	router.Use(metrics.Middleware)

	// Setup API routes
	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	apiRouter.HandleFunc("/publish", handlers.NewPublishHandler(cfg.AMQPService, log).Handle).Methods("POST")
	apiRouter.HandleFunc("/subscribe", handlers.NewSubscribeHandler(cfg.AMQPService, log).Handle).Methods("GET")
	apiRouter.HandleFunc("/uuid", handlers.NewUUIDHandler(cfg.UUIDService, log).Handle).Methods("GET")
	apiRouter.HandleFunc("/search", handlers.NewSearchHandler(cfg.SearchService, log).Handle).Methods("GET")

	// Health check endpoint
	router.HandleFunc("/healthz", healthCheckHandler).Methods("GET")

	// Prometheus metrics endpoint
	router.Handle("/metrics", promhttp.Handler())

	// Swagger documentation
	router.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)

	// Setup CORS
	corsOpts := cors.New(cors.Options{
		AllowedOrigins: strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ","),
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
	})

	// Create server
	srv := &http.Server{
		Addr:         ":" + os.Getenv("PORT"),
		Handler:      corsOpts.Handler(router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Use errgroup to manage goroutines
	g, ctx := errgroup.WithContext(context.Background())

	// Start server in a goroutine
	g.Go(func() error {
		log.Infow("Starting server", "port", os.Getenv("PORT"))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server failed to start: %w", err)
		}
		return nil
	})

	// Graceful shutdown
	g.Go(func() error {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		select {
		case <-sigint:
			log.Info("Received interrupt signal, shutting down...")
		case <-ctx.Done():
			log.Info("Shutting down due to cancelled context...")
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server forced to shutdown: %w", err)
		}

		log.Info("Server exited gracefully")
		return nil
	})

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		log.Fatalf("Error during server lifecycle: %v", err)
	}
}

func loadEnv() error {
	err := godotenv.Load()
	if err != nil {
		return fmt.Errorf("error loading .env file: %w", err)
	}
	return nil
}

func loggingMiddleware(log *zap.SugaredLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			log.Infow("Request processed",
				"method", r.Method,
				"path", r.URL.Path,
				"duration", time.Since(start),
			)
		})
	}
}

func recoveryMiddleware(log *zap.SugaredLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Errorw("Panic occurred",
						"error", err,
						"stacktrace", string(debug.Stack()),
					)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
