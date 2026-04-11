package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/chaosbank/chaosbank/pkg/service"
	"github.com/chaosbank/chaosbank/pkg/util"
	"github.com/chaosbank/chaosbank/services/transaction-service/internal/domain"
	"github.com/chaosbank/chaosbank/services/transaction-service/internal/kafka"
	"github.com/chaosbank/chaosbank/services/transaction-service/internal/repository"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	router                chi.Router
	logger                *service.Logger
	idempotencyRepository domain.IdempotencyRepository
	producer              *kafka.Producer
}

func NewHandler(logger *service.Logger, db *sql.DB, producer *kafka.Producer) *Handler {
	h := &Handler{
		router:                chi.NewRouter(),
		logger:                logger,
		idempotencyRepository: repository.NewPostgresIdempotencyRepository(db),
		producer:              producer,
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
		h.writeError(w, http.StatusBadRequest, idempotencyKey, "invalid JSON body")
		return
	}

	if err := req.Validate(); err != nil {
		h.logger.Warn("handler.transfer.validation_error", map[string]interface{}{
			"request_id": idempotencyKey,
			"error":      err.Error(),
		})
		h.writeError(w, http.StatusBadRequest, idempotencyKey, err.Error())
		return
	}

	// Hash the request for idempotency detection
	requestHash, err := util.HashRequest(req)
	if err != nil {
		h.logger.Error("handler.transfer.hash_error", map[string]interface{}{
			"request_id": idempotencyKey,
			"error":      err.Error(),
		})
		h.writeError(w, http.StatusInternalServerError, idempotencyKey, "internal server error")
		return
	}

	// Check and create idempotency record
	idempotencyRecord, err := h.idempotencyRepository.CheckAndCreateIfNotExists(idempotencyKey, requestHash)
	if err != nil {
		if conflictErr, ok := err.(*domain.ErrIdempotencyConflict); ok {
			h.logger.Warn("handler.transfer.idempotency_conflict", map[string]interface{}{
				"request_id": idempotencyKey,
				"error":      conflictErr.Error(),
			})
			h.writeError(w, http.StatusConflict, idempotencyKey, conflictErr.Error())
			return
		}

		h.logger.Error("handler.transfer.idempotency_error", map[string]interface{}{
			"request_id": idempotencyKey,
			"error":      err.Error(),
		})
		h.writeError(w, http.StatusInternalServerError, idempotencyKey, "internal server error")
		return
	}

	// If record already has a response, return it (idempotent response)
	if idempotencyRecord.Status == "completed" && idempotencyRecord.Response != "" {
		h.logger.Info("handler.transfer.returning_cached_response", map[string]interface{}{
			"request_id": idempotencyKey,
		})

		var cachedResp domain.TransferResponse
		if err := json.Unmarshal([]byte(idempotencyRecord.Response), &cachedResp); err != nil {
			h.logger.Error("handler.transfer.unmarshal_cached_error", map[string]interface{}{
				"request_id": idempotencyKey,
				"error":      err.Error(),
			})
			h.writeError(w, http.StatusInternalServerError, idempotencyKey, "internal server error")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(cachedResp)
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

	event := kafka.TransferEvent{
		From:   req.From,
		To:     req.To,
		Amount: req.Amount,
	}
	if h.producer != nil {
		if err := h.producer.ProduceTransferEvent(r.Context(), event); err != nil {
			h.logger.Error("handler.transfer.kafka_error", map[string]interface{}{
				"request_id": idempotencyKey,
				"error":      err.Error(),
			})
		}
	}

	// Store response in idempotency record
	respJSON, err := json.Marshal(resp)
	if err != nil {
		h.logger.Error("handler.transfer.response_marshal_error", map[string]interface{}{
			"request_id": idempotencyKey,
			"error":      err.Error(),
		})
		h.writeError(w, http.StatusInternalServerError, idempotencyKey, "internal server error")
		return
	}

	if err := h.idempotencyRepository.UpdateResponse(idempotencyKey, string(respJSON)); err != nil {
		h.logger.Error("handler.transfer.response_store_error", map[string]interface{}{
			"request_id": idempotencyKey,
			"error":      err.Error(),
		})
		h.writeError(w, http.StatusInternalServerError, idempotencyKey, "internal server error")
		return
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
