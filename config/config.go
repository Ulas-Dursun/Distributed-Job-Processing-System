package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	AppMode     string // "all", "api", "worker"
	ServerPort  string
	DatabaseURL string
	RedisURL    string
	WorkerCount int
	MaxRetries  int
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, reading from environment")
	}

	workerCount, err := strconv.Atoi(getEnv("WORKER_COUNT", "5"))
	if err != nil {
		workerCount = 5
	}

	maxRetries, err := strconv.Atoi(getEnv("MAX_RETRIES", "3"))
	if err != nil {
		maxRetries = 3
	}

	return &Config{
		AppMode:     getEnv("APP_MODE", "all"),
		ServerPort:  getEnv("SERVER_PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://queueuser:queuepass@localhost:5432/queueforge?sslmode=disable"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379"),
		WorkerCount: workerCount,
		MaxRetries:  maxRetries,
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
