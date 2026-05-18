package db

import (
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"github.com/pphui8/long/domain"
	"go.uber.org/zap"
)

func Init(cfg domain.PostgresConfig, log *zap.Logger) (*sql.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	log.Info("SUCCESS: Connected to PostgreSQL database", zap.String("host", cfg.Host), zap.String("dbname", cfg.DBName))
	return db, nil
}
