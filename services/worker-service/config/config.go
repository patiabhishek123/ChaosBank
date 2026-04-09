package config

import (
	"os"
)

type Config struct {
	KafkaBrokers string
	DatabaseURL  string
}

func Load() *Config {
	return &Config{
		KafkaBrokers: getEnv("KAFKA_BROKERS", "kafka:29092"),
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://chaosbank:chaosbank123@postgres:5432/chaosbank?sslmode=disable"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
