package service

import (
	"os"
	"time"
)

type Config struct {
	ServiceName     string
	Port            string
	LogLevel        string
	ShutdownTimeout time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
}

func LoadConfig() *Config {
	return &Config{
		ServiceName:     getEnv("SERVICE_NAME", "chaosbank-service"),
		Port:            getEnv("PORT", "8080"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		ShutdownTimeout: getEnvAsDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
		ReadTimeout:     getEnvAsDuration("READ_TIMEOUT", 15*time.Second),
		WriteTimeout:    getEnvAsDuration("WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:     getEnvAsDuration("IDLE_TIMEOUT", 60*time.Second),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}
