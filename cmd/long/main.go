package main

import (
	"context"
	"os"
	"strconv"

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

	userRepo := repository.NewUserRepository(dbConn)
	llmRepo := repository.NewLLMRepository(dbConn)
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
	llmSvc, err := service.NewLLMService(llmRepo, chatProvider)
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

	log.Info("Starting Gin Web Server", zap.String("port", strconv.Itoa(config.App.Port)), zap.String("redis", redisAddr))

	r := router.Setup(app)

	if err := r.Run(":" + strconv.Itoa(config.App.Port)); err != nil {
		log.Fatal("Failed to start server", zap.Error(err))
	}
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
