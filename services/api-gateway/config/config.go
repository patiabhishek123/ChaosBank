package config

import (
	"os"
)

type Config struct {
	Port            string
	TransactionSvcURL string
	WorkerSvcURL    string
}

func Load() *Config {
	return &Config{
		Port:            getEnv("PORT", "8080"),
		TransactionSvcURL: getEnv("TRANSACTION_SVC_URL", "http://transaction-service:8081"),
		WorkerSvcURL:    getEnv("WORKER_SVC_URL", "http://worker-service:8082"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}