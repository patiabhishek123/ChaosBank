package config

import (
	"os"
)

type Config struct {
	Port         string
	DatabaseURL  string
	KafkaBrokers string
}

func Load() *Config {
	return &Config{
		Port:         getEnv("PORT", "8081"),
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://chaosbank:chaosbank123@postgres:5432/chaosbank?sslmode=disable"),
		KafkaBrokers: getEnv("KAFKA_BROKERS", "kafka:29092"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
