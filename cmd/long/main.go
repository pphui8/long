package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/pphui8/long/auth"
	"github.com/pphui8/long/db"
	"github.com/pphui8/long/domain"
	"github.com/pphui8/long/handler"
	"github.com/pphui8/long/logger"
	"github.com/pphui8/long/provider"
	"github.com/pphui8/long/repository"
	"github.com/pphui8/long/router"
	"github.com/pphui8/long/service"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

const (
	serverReadHeaderTimeout = 5 * time.Second
	serverReadTimeout       = 15 * time.Second
	serverWriteTimeout      = 3 * time.Minute
	serverIdleTimeout       = 60 * time.Second
	serverShutdownTimeout   = 2*time.Minute + 15*time.Second
)

func main() {
	log := logger.Init("log/logs/app.log")
	defer logger.Sync(log)

	config, err := loadConfig()
	if err != nil {
		log.Fatal("Failed to load configuration", zap.Error(err))
	}
	redisAddr := config.Redis.Host + ":" + strconv.Itoa(config.Redis.Port)

	tokenStore, err := auth.InitRedis(redisAddr, config.Redis.Password, config.Redis.DB, log)
	if err != nil {
		log.Fatal("Failed to initialize Redis", zap.String("addr", redisAddr), zap.Int("db", config.Redis.DB), zap.Error(err))
	}
	log.Info("Initializing Database connection")
	dbConn, err := db.Init(config.Postgres, log)
	if err != nil {
		log.Fatal("Failed to initialize database", zap.String("host", config.Postgres.Host), zap.Int("port", config.Postgres.Port), zap.Error(err))
	}
	defer func() {
		if err := dbConn.Close(); err != nil {
			log.Error("Failed to close database", zap.Error(err))
		}
	}()
	defer func() {
		if err := tokenStore.Close(); err != nil {
			log.Error("Failed to close Redis", zap.Error(err))
		}
	}()

	userRepo := repository.NewUserRepository(dbConn, log)
	llmRepo := repository.NewLLMRepository(dbConn, log)
	modelRegistry := service.StaticModelRegistry{
		provider.GeminiProviderName: "gemini-3.1-flash-lite",
	}
	geminiModel, _ := modelRegistry.DefaultModel(provider.GeminiProviderName)
	chatProvider, err := provider.NewGeminiProvider(context.Background(), service.ProviderConfig{
		Name:   provider.GeminiProviderName,
		APIKey: os.Getenv("GEMINI_API"),
		Model:  geminiModel,
	})
	if err != nil {
		log.Fatal("Failed to initialize chat provider", zap.String("provider", provider.GeminiProviderName), zap.Error(err))
	}
	llmSvc, err := service.NewLLMService(llmRepo, chatProvider, log)
	if err != nil {
		log.Fatal("Failed to initialize LLM service", zap.Error(err))
	}

	app := &handler.App{
		DB:         dbConn,
		TokenStore: tokenStore,
		UserRepo:   userRepo,
		LLMService: llmSvc,
		Logger:     log,
	}

	r := router.Setup(app)
	server := &http.Server{
		Addr:              ":" + strconv.Itoa(config.App.Port),
		Handler:           r,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("Starting HTTP server",
			zap.String("addr", server.Addr),
			zap.String("redis", redisAddr),
			zap.Duration("read_header_timeout", server.ReadHeaderTimeout),
			zap.Duration("read_timeout", server.ReadTimeout),
			zap.Duration("write_timeout", server.WriteTimeout),
			zap.Duration("idle_timeout", server.IdleTimeout),
		)
		serverErr <- server.ListenAndServe()
	}()

	shutdownSignal, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("Failed to start HTTP server", zap.Error(err))
		}
		log.Info("HTTP server stopped")
		return
	case <-shutdownSignal.Done():
		log.Info("Shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("HTTP server graceful shutdown failed", zap.Error(err))
		if err := server.Close(); err != nil {
			log.Error("HTTP server forced close failed", zap.Error(err))
		}
		return
	}

	if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("HTTP server stopped unexpectedly", zap.Error(err))
	}
	log.Info("HTTP server stopped")
}

func loadConfig() (domain.Config, error) {
	var config domain.Config
	file, err := os.Open("env.yaml")
	if err != nil {
		return config, err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return config, err
	}

	return config, nil
}
