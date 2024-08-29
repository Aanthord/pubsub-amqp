// @title PubSub AMQP API
// @version 1.0
// @description This is a PubSub service using AMQP.
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host txanunxlbapd512:8080
// @BasePath /api/v1

package main

import (
    "context"
    "encoding/json"
    "encoding/xml"
    "fmt"
    "net/http"
    "os"
    "os/signal"
    "runtime/debug"
    "strconv"
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
    "go.uber.org/zap"
    "golang.org/x/sync/errgroup"
    _ "github.com/aanthord/pubsub-amqp/docs"
    httpSwagger "github.com/swaggo/http-swagger"
)

func main() {
    fmt.Println("Starting application...")

    if err := godotenv.Load(); err != nil {
        fmt.Printf("Error loading .env file: %v\n", err)
    } else {
        fmt.Println(".env file loaded successfully")
    }

    fmt.Println("Initializing configuration...")
    cfg, err := config.NewConfig()
    if err != nil {
        fmt.Printf("Failed to initialize config: %v\n", err)
        os.Exit(1)
    }
    fmt.Println("Configuration initialized successfully")
    defer cfg.TracerCloser.Close()

    fmt.Println("Initializing metrics...")
    metrics.Init()
    cfg.Logger.Info("Metrics initialized")
    fmt.Println("Metrics initialized")

    fmt.Println("Setting up router...")
    router := mux.NewRouter()

    router.Use(loggingMiddleware(cfg.Logger))
    router.Use(recoveryMiddleware(cfg.Logger))
    router.Use(tracingMiddleware())
    router.Use(metricsMiddleware)

    apiRouter := router.PathPrefix("/api/v1").Subrouter()
    apiRouter.HandleFunc("/publish/{topic}", handlers.NewPublishHandler(cfg.AMQPService, cfg.Logger).Handle).Methods("POST")
    apiRouter.HandleFunc("/subscribe/{topic}", handlers.NewSubscribeHandler(cfg.AMQPService, cfg.Logger).Handle).Methods("GET")
    apiRouter.HandleFunc("/uuid", handlers.NewUUIDHandler(cfg.UUIDService).Handle).Methods("GET")
    apiRouter.HandleFunc("/search", handlers.NewSearchHandler(cfg.SearchService).Handle).Methods("GET")

    router.HandleFunc("/healthz", healthCheckHandler).Methods("GET")
    router.Handle("/metrics", promhttp.Handler())
    router.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)
    fmt.Println("Router set up completed")

    fmt.Println("Setting up CORS...")
    corsOpts := cors.New(cors.Options{
        AllowedOrigins:   strings.Split(getEnv("CORS_ALLOWED_ORIGINS", "*"), ","),
        AllowedMethods:   strings.Split(getEnv("CORS_ALLOWED_METHODS", "GET,POST,PUT,DELETE,OPTIONS"), ","),
        AllowedHeaders:   strings.Split(getEnv("CORS_ALLOWED_HEADERS", "*"), ","),
        AllowCredentials: getEnvAsBool("CORS_ALLOW_CREDENTIALS", false),
        MaxAge:           getEnvAsInt("CORS_MAX_AGE", 300),
    })
    fmt.Println("CORS set up completed")

    srv := &http.Server{
        Addr:         ":" + getEnv("PORT", "8080"),
        Handler:      corsOpts.Handler(router),
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 15 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    g, ctx := errgroup.WithContext(context.Background())

    g.Go(func() error {
        cfg.Logger.Infow("Starting server", "port", getEnv("PORT", "8080"))
        fmt.Printf("Starting server on port %s\n", getEnv("PORT", "8080"))
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            return fmt.Errorf("server failed to start: %w", err)
        }
        return nil
    })

    g.Go(func() error {
        sigint := make(chan os.Signal, 1)
        signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
        select {
        case <-sigint:
            cfg.Logger.Info("Received interrupt signal, shutting down...")
            fmt.Println("Received interrupt signal, shutting down...")
        case <-ctx.Done():
            cfg.Logger.Info("Shutting down due to cancelled context...")
            fmt.Println("Shutting down due to cancelled context...")
        }

        shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
        defer cancel()

        if err := srv.Shutdown(shutdownCtx); err != nil {
            return fmt.Errorf("server forced to shutdown: %w", err)
        }

        cfg.Logger.Info("Server exited gracefully")
        fmt.Println("Server exited gracefully")
        return nil
    })

    fmt.Println("Server is now running. Press CTRL+C to shut down.")
    if err := g.Wait(); err != nil {
        cfg.Logger.Fatalw("Error during server lifecycle", "error", err)
        fmt.Printf("Error during server lifecycle: %v\n", err)
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

func tracingMiddleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            span, ctx := tracing.StartSpanFromContext(r.Context(), r.URL.Path, "")
            defer span.Finish()
            next.ServeHTTP(w, r.WithContext(ctx))
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

// @Summary Health check endpoint
// @Description Returns OK if the service is healthy
// @Tags health
// @Produce plain
// @Success 200 {string} string "OK"
// @Router /healthz [get]
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
    valStr := getEnv(key, "")
    if val, err := strconv.ParseBool(valStr); err == nil {
        return val
    }
    return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
    valStr := getEnv(key, "")
    if val, err := strconv.Atoi(valStr); err == nil {
        return val
    }
    return defaultValue
}

// respondWithJSON writes a JSON response to the http.ResponseWriter
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
    response, _ := json.Marshal(payload)
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    w.Write(response)
}

// respondWithXML writes an XML response to the http.ResponseWriter
func respondWithXML(w http.ResponseWriter, code int, payload interface{}) {
    response, _ := xml.Marshal(payload)
    w.Header().Set("Content-Type", "application/xml")
    w.WriteHeader(code)
    w.Write(response)
}

// respondWithError writes an error response (JSON or XML) to the http.ResponseWriter
func respondWithError(w http.ResponseWriter, r *http.Request, code int, message string) {
    if preferXML(r) {
        respondWithXML(w, code, ErrorResponse{Error: message})
    } else {
        respondWithJSON(w, code, ErrorResponse{Error: message})
    }
}

// respondWithData writes a success response (JSON or XML) to the http.ResponseWriter
func respondWithData(w http.ResponseWriter, r *http.Request, code int, payload interface{}) {
    if preferXML(r) {
        respondWithXML(w, code, payload)
    } else {
        respondWithJSON(w, code, payload)
    }
}

// preferXML checks if the client prefers XML over JSON
func preferXML(r *http.Request) bool {
    accept := r.Header.Get("Accept")
    return strings.Contains(accept, "application/xml") && !strings.Contains(accept, "application/json")
}

// ErrorResponse represents an error response
type ErrorResponse struct {
    Error string `json:"error" xml:"error"`
}
