package handler

import (
	"encoding/json"
	"net/http"

	"github.com/chaosbank/chaosbank/pkg/service"
	"github.com/chaosbank/chaosbank/pkg/util"
	"github.com/chaosbank/chaosbank/services/transaction-service/internal/domain"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	router chi.Router
	logger *service.Logger
}

func NewHandler(logger *service.Logger) *Handler {
	h := &Handler{
		router: chi.NewRouter(),
		logger: logger,
	}
	h.setupRoutes()
	return h
}

func (h *Handler) Router() chi.Router {
	return h.router
}

func (h *Handler) setupRoutes() {
	h.router.Post("/transfer", h.transfer)
}

func (h *Handler) transfer(w http.ResponseWriter, r *http.Request) {
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		h.writeError(w, http.StatusBadRequest, "Idempotency-Key", "Idempotency-Key header is required")
		return
	}

	if !util.ValidateRequestID(idempotencyKey) {
		h.writeError(w, http.StatusBadRequest, "Idempotency-Key", "Idempotency-Key must be a valid UUID")
		return
	}

	var req domain.TransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("handler.transfer.decode_error", map[string]interface{}{
			"request_id": idempotencyKey,
			"error":      err.Error(),
		})
		h.writeError(w, http.StatusBadRequest, "Idempotency-Key", "invalid JSON body")
		return
	}

	if err := req.Validate(); err != nil {
		h.logger.Warn("handler.transfer.validation_error", map[string]interface{}{
			"request_id": idempotencyKey,
			"error":      err.Error(),
		})
		h.writeError(w, http.StatusBadRequest, "Idempotency-Key", err.Error())
		return
	}

	h.logger.Info("handler.transfer.validated", map[string]interface{}{
		"request_id": idempotencyKey,
		"from":       req.From,
		"to":         req.To,
		"amount":     req.Amount,
	})

	resp := domain.TransferResponse{
		RequestID: idempotencyKey,
		From:      req.From,
		To:        req.To,
		Amount:    req.Amount,
		Status:    "accepted",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) writeError(w http.ResponseWriter, statusCode int, requestID, message string) {
	resp := domain.ErrorResponse{
		RequestID: requestID,
		Message:   message,
		Code:      http.StatusText(statusCode),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(resp)
}
