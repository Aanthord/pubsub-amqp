package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
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
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	httpSwagger "github.com/swaggo/http-swagger"
	_ "github.com/aanthord/pubsub-amqp/docs" // This line is necessary for swagger
)

// @title Pub/Sub AMQP API
// @version 1.0
// @description This is a Pub/Sub AMQP server with additional features.
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:8080
// @BasePath /api/v1
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
	if err := godotenv.Load(); err != nil {
		log.Warnw("Error loading .env file", "error", err)
	}

	// Initialize config
	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatalw("Failed to initialize config", "error", err)
	}
	defer cfg.TracerCloser.Close()

	// Initialize metrics
	metrics.Init()

	// Create router
	router := mux.NewRouter()

	// Setup middleware
	router.Use(loggingMiddleware(log))
	router.Use(recoveryMiddleware(log))
	router.Use(tracing.Middleware(cfg.Tracer))
	router.Use(metricsMiddleware)

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
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
			http.MethodHead,
		},
		AllowedHeaders: []string{"*"},
	})

	// Create server
	srv := &http.Server{
		Addr:         ":" + config.GetEnv("PORT", "8080"),
		Handler:      corsOpts.Handler(router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Use errgroup to manage goroutines
	g, ctx := errgroup.WithContext(context.Background())

	// Start server in a goroutine
	g.Go(func() error {
		log.Infow("Starting server", "port", config.GetEnv("PORT", "8080"))
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
		log.Fatalw("Error during server lifecycle", "error", err)
	}
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

func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		metrics.HTTPRequestDuration.WithLabelValues(r.URL.Path).Observe(duration.Seconds())
		metrics.HTTPRequestsTotal.WithLabelValues(r.URL.Path).Inc()
	})
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
