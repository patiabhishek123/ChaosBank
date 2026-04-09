package main

import (
	"log"
	"net/http"

	"github.com/chaosbank/chaosbank/services/transaction-service/config"
	"github.com/chaosbank/chaosbank/services/transaction-service/internal/handler"
)

func main() {
	cfg := config.Load()

	h := handler.NewHandler()

	http.Handle("/", h.Router())

	log.Printf("Transaction Service starting on port %s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, nil))
}