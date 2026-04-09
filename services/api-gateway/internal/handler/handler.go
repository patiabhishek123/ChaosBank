package handler

import (
	"net/http"

	"github.com/gorilla/mux"
)

type Handler struct {
	router *mux.Router
}

func NewHandler() *Handler {
	h := &Handler{
		router: mux.NewRouter(),
	}
	h.setupRoutes()
	return h
}

func (h *Handler) Router() *mux.Router {
	return h.router
}

func (h *Handler) setupRoutes() {
	h.router.HandleFunc("/health", h.healthCheck).Methods("GET")
	// Add more routes here
}

func (h *Handler) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}