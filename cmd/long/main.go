package main

import (
	"os"
	"strconv"

	"github.com/pphui8/long/auth"
	"github.com/pphui8/long/domain"
	"github.com/pphui8/long/logger"
	"github.com/pphui8/long/router"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

func main() {
	logger.Init("log/logs/app.log")
	defer logger.Sync()

	config, err := loadConfig()
	if err != nil {
		logger.Log.Fatal("Failed to load configuration", zap.Error(err))
	}
	redisAddr := config.Redis.Host + ":" + strconv.Itoa(config.Redis.Port)

	auth.InitRedis(redisAddr, "", 0)
	auth.InitDB(config.Postgres)

	logger.Log.Info("Starting Gin Web Server", zap.String("port", strconv.Itoa(config.App.Port)), zap.String("redis", redisAddr))

	r := router.Setup()

	if err := r.Run(":" + strconv.Itoa(config.App.Port)); err != nil {
		logger.Log.Fatal("Failed to start server", zap.Error(err))
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
