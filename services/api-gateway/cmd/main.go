package main

import (
	"log"
	"net/http"

	"github.com/chaosbank/chaosbank/services/api-gateway/config"
	"github.com/chaosbank/chaosbank/services/api-gateway/internal/handler"
)

func main() {
	cfg := config.Load()

	h := handler.NewHandler()

	http.Handle("/", h.Router())

	log.Printf("API Gateway starting on port %s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, nil))
}