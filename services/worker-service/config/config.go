package config

import (
	"os"
	"strings"
)

type Config struct {
	KafkaBrokers        string
	KafkaGroupID        string
	ReplayFromBeginning bool
	ReplayEnabled       bool
	ReplayConfirmToken  string
	DatabaseURL         string
}

func Load() *Config {
	return &Config{
		KafkaBrokers:        getEnv("KAFKA_BROKERS", "kafka:29092"),
		KafkaGroupID:        getEnv("KAFKA_GROUP_ID", "worker-group"),
		ReplayFromBeginning: isTrue(getEnv("KAFKA_REPLAY_FROM_BEGINNING", "false")),
		ReplayEnabled:       isTrue(getEnv("REPLAY_ENABLED", "false")),
		ReplayConfirmToken:  getEnv("REPLAY_CONFIRM_TOKEN", "REPLAY_ALL_EVENTS"),
		DatabaseURL:         getEnv("DATABASE_URL", "postgres://chaosbank:chaosbank123@postgres:5432/chaosbank?sslmode=disable"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func isTrue(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "true")
}
