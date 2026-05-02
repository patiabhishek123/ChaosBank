package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	db                    *sql.DB
	idempotencyRepository domain.IdempotencyRepository
	producer              kafka.EventProducer
}

func NewHandler(logger *service.Logger, db *sql.DB, producer kafka.EventProducer) *Handler {
	h := &Handler{
		router:                chi.NewRouter(),
		logger:                logger,
		db:                    db,
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
	h.router.Get("/accounts", h.listAccounts)
	h.router.Post("/accounts", h.createAccount)
	h.router.Post("/transfer", h.transfer)
}

func (h *Handler) createAccount(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "accounts", "invalid JSON body")
		return
	}

	if err := req.Validate(); err != nil {
		h.writeError(w, http.StatusBadRequest, "accounts", err.Error())
		return
	}

	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = "USD"
	}

	var account domain.AccountSummary
	err := h.db.QueryRow(`
		INSERT INTO accounts (account_number, owner_name, balance, currency, status)
		VALUES ($1, $2, $3, $4, 'active')
		RETURNING id, account_number, owner_name, balance, currency, status, created_at
	`, strings.TrimSpace(req.AccountNumber), strings.TrimSpace(req.OwnerName), req.InitialBalance, currency).Scan(
		&account.ID,
		&account.AccountNumber,
		&account.OwnerName,
		&account.Balance,
		&account.Currency,
		&account.Status,
		&account.CreatedAt,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "accounts_account_number_key") {
			h.writeError(w, http.StatusConflict, "accounts", "account number already exists")
			return
		}
		h.logger.Error("handler.accounts.create_error", map[string]interface{}{"error": err.Error()})
		h.writeError(w, http.StatusInternalServerError, "accounts", "failed to create account")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(account)
}

func (h *Handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 500 {
			limit = v
		}
	}

	rows, err := h.db.Query(`
		SELECT id, account_number, owner_name, balance, currency, status, created_at
		FROM accounts
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		h.logger.Error("handler.accounts.list_error", map[string]interface{}{"error": err.Error()})
		h.writeError(w, http.StatusInternalServerError, "accounts", "failed to list accounts")
		return
	}
	defer rows.Close()

	accounts := make([]domain.AccountSummary, 0)
	for rows.Next() {
		var a domain.AccountSummary
		if err := rows.Scan(&a.ID, &a.AccountNumber, &a.OwnerName, &a.Balance, &a.Currency, &a.Status, &a.CreatedAt); err != nil {
			h.logger.Error("handler.accounts.scan_error", map[string]interface{}{"error": err.Error()})
			h.writeError(w, http.StatusInternalServerError, "accounts", "failed to read account rows")
			return
		}
		accounts = append(accounts, a)
	}

	if err := rows.Err(); err != nil {
		h.logger.Error("handler.accounts.rows_error", map[string]interface{}{"error": err.Error()})
		h.writeError(w, http.StatusInternalServerError, "accounts", "failed to list accounts")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(accounts)
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

	event := kafka.TransferEvent{
		EventID:   idempotencyKey,
		From:      req.From,
		To:        req.To,
		Amount:    req.Amount,
		Timestamp: time.Now().UTC(),
	}

	if h.producer == nil {
		h.logger.Error("handler.transfer.kafka_not_configured", map[string]interface{}{
			"request_id": idempotencyKey,
		})
		h.writeError(w, http.StatusInternalServerError, idempotencyKey, "internal server error")
		return
	}

	if err := h.producer.ProduceTransferEvent(r.Context(), event); err != nil {
		h.logger.Error("handler.transfer.kafka_error", map[string]interface{}{
			"request_id": idempotencyKey,
			"error":      err.Error(),
		})
		h.writeError(w, http.StatusServiceUnavailable, idempotencyKey, "failed to enqueue transfer event")
		return
	}

	resp := domain.TransferResponse{
		RequestID: idempotencyKey,
		From:      req.From,
		To:        req.To,
		Amount:    req.Amount,
		Status:    "accepted",
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
