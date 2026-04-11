package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/chaosbank/chaosbank/pkg/service"
	"github.com/chaosbank/chaosbank/pkg/util"
	"github.com/chaosbank/chaosbank/services/transaction-service/internal/domain"
	"github.com/chaosbank/chaosbank/services/transaction-service/internal/kafka"
)

// MockIdempotencyRepository for testing
type MockIdempotencyRepository struct {
	records map[string]*domain.IdempotencyRecord
}

func NewMockIdempotencyRepository() *MockIdempotencyRepository {
	return &MockIdempotencyRepository{
		records: make(map[string]*domain.IdempotencyRecord),
	}
}

func (m *MockIdempotencyRepository) Create(key, requestHash string) (*domain.IdempotencyRecord, error) {
	if _, exists := m.records[key]; exists {
		return nil, &domain.ErrIdempotencyConflict{Message: "key already exists"}
	}
	record := &domain.IdempotencyRecord{
		ID:          util.GenerateRequestID(),
		Key:         key,
		RequestHash: requestHash,
		Status:      "pending",
	}
	m.records[key] = record
	return record, nil
}

func (m *MockIdempotencyRepository) GetByKey(key string) (*domain.IdempotencyRecord, error) {
	return m.records[key], nil
}

func (m *MockIdempotencyRepository) UpdateResponse(key string, response string) error {
	if record, exists := m.records[key]; exists {
		record.Response = response
		record.Status = "completed"
		return nil
	}
	return &domain.ErrIdempotencyConflict{Message: "record not found"}
}

func (m *MockIdempotencyRepository) CheckAndCreateIfNotExists(key, requestHash string) (*domain.IdempotencyRecord, error) {
	if record, exists := m.records[key]; exists {
		if record.RequestHash != requestHash {
			return nil, &domain.ErrIdempotencyConflict{Message: "different payload for same key"}
		}
		return record, nil
	}
	return m.Create(key, requestHash)
}

type MockProducer struct{}

func (p *MockProducer) ProduceTransferEvent(_ context.Context, _ kafka.TransferEvent) error {
	return nil
}

func (p *MockProducer) Close() error {
	return nil
}

func TestIdempotencySuccess(t *testing.T) {
	logger := service.NewLogger("info", os.Stdout)
	mockRepo := NewMockIdempotencyRepository()

	// Create handler with mock repository and mock producer
	h := &Handler{
		router:                service.NewRouter(),
		logger:                logger,
		idempotencyRepository: mockRepo,
		producer:              &MockProducer{},
	}
	h.setupRoutes()

	reqID := util.GenerateRequestID()
	transferReq := domain.TransferRequest{
		From:   "userA",
		To:     "userB",
		Amount: 100.50,
	}

	body, _ := json.Marshal(transferReq)
	req := httptest.NewRequest("POST", "/transfer", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", reqID)

	w := httptest.NewRecorder()
	h.transfer(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d", http.StatusAccepted, w.Code)
	}

	// Verify record was stored
	record, _ := mockRepo.GetByKey(reqID)
	if record == nil {
		t.Fatal("expected idempotency record to be created")
	}

	if record.Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", record.Status)
	}
}

func TestIdempotencyConflict(t *testing.T) {
	logger := service.NewLogger("info", os.Stdout)
	mockRepo := NewMockIdempotencyRepository()

	h := &Handler{
		router:                service.NewRouter(),
		logger:                logger,
		idempotencyRepository: mockRepo,
		producer:              &MockProducer{},
	}
	h.setupRoutes()

	reqID := util.GenerateRequestID()

	// First request
	transferReq1 := domain.TransferRequest{
		From:   "userA",
		To:     "userB",
		Amount: 100.50,
	}
	body1, _ := json.Marshal(transferReq1)
	req1 := httptest.NewRequest("POST", "/transfer", bytes.NewReader(body1))
	req1.Header.Set("Idempotency-Key", reqID)

	w1 := httptest.NewRecorder()
	h.transfer(w1, req1)

	if w1.Code != http.StatusAccepted {
		t.Fatalf("first request failed: %d", w1.Code)
	}

	// Second request with same key but different payload
	transferReq2 := domain.TransferRequest{
		From:   "userA",
		To:     "userC",
		Amount: 200.0,
	}
	body2, _ := json.Marshal(transferReq2)
	req2 := httptest.NewRequest("POST", "/transfer", bytes.NewReader(body2))
	req2.Header.Set("Idempotency-Key", reqID)

	w2 := httptest.NewRecorder()
	h.transfer(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Errorf("expected status %d (Conflict), got %d", http.StatusConflict, w2.Code)
	}

	var errResp domain.ErrorResponse
	json.NewDecoder(w2.Body).Decode(&errResp)

	if errResp.Message != "different payload for same key" && errResp.Message != "Idempotency key used with different request payload" {
		t.Errorf("unexpected error message: %s", errResp.Message)
	}
}

func TestIdempotencyCachedResponse(t *testing.T) {
	logger := service.NewLogger("info", os.Stdout)
	mockRepo := NewMockIdempotencyRepository()

	h := &Handler{
		router:                service.NewRouter(),
		logger:                logger,
		idempotencyRepository: mockRepo,
		producer:              &MockProducer{},
	}
	h.setupRoutes()

	reqID := util.GenerateRequestID()
	transferReq := domain.TransferRequest{
		From:   "userA",
		To:     "userB",
		Amount: 100.50,
	}

	body, _ := json.Marshal(transferReq)

	// First request
	req1 := httptest.NewRequest("POST", "/transfer", bytes.NewReader(body))
	req1.Header.Set("Idempotency-Key", reqID)

	w1 := httptest.NewRecorder()
	h.transfer(w1, req1)

	var resp1 domain.TransferResponse
	json.NewDecoder(w1.Body).Decode(&resp1)

	// Second request with same key and payload
	body2, _ := json.Marshal(transferReq)
	req2 := httptest.NewRequest("POST", "/transfer", bytes.NewReader(body2))
	req2.Header.Set("Idempotency-Key", reqID)

	w2 := httptest.NewRecorder()
	h.transfer(w2, req2)

	var resp2 domain.TransferResponse
	json.NewDecoder(w2.Body).Decode(&resp2)

	// Both responses should be identical
	if resp1.RequestID != resp2.RequestID {
		t.Errorf("expected same request ID, got %s vs %s", resp1.RequestID, resp2.RequestID)
	}

	if resp1.Status != resp2.Status {
		t.Errorf("expected same status, got %s vs %s", resp1.Status, resp2.Status)
	}
}
