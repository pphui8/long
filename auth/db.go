package auth

import (
	"database/sql"
	"fmt"
	"github.com/pphui8/long/domain"
	"github.com/pphui8/long/logger"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

var DB *sql.DB

func InitDB(cfg domain.PostgresConfig) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		logger.Log.Fatal("Failed to open database connection", zap.Error(err))
	}

	if err := db.Ping(); err != nil {
		logger.Log.Fatal("Failed to ping database", zap.Error(err))
	}

	logger.Log.Info("Connected to PostgreSQL database", zap.String("dbname", cfg.DBName))
	DB = db
}
