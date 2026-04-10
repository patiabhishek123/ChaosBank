package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/chaosbank/chaosbank/pkg/service"
	"github.com/chaosbank/chaosbank/pkg/util"
	"github.com/chaosbank/chaosbank/services/transaction-service/internal/domain"
)

func TestTransferSuccess(t *testing.T) {
	logger := service.NewLogger("info", os.Stdout)
	h := NewHandler(logger)

	reqID := util.GenerateRequestID()
	transferReq := domain.TransferRequest{
		From:   "userA",
		To:     "userB",
		Amount: 100.50,
	}

	body, _ := json.Marshal(transferReq)
	req := httptest.NewRequest("POST", "/transfer", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", reqID)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.transfer(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d", http.StatusAccepted, w.Code)
	}

	var resp domain.TransferResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.RequestID != reqID {
		t.Errorf("expected request ID %s, got %s", reqID, resp.RequestID)
	}

	if resp.Status != "accepted" {
		t.Errorf("expected status 'accepted', got '%s'", resp.Status)
	}
}

func TestTransferMissingIdempotencyKey(t *testing.T) {
	logger := service.NewLogger("info", os.Stdout)
	h := NewHandler(logger)

	transferReq := domain.TransferRequest{
		From:   "userA",
		To:     "userB",
		Amount: 100,
	}

	body, _ := json.Marshal(transferReq)
	req := httptest.NewRequest("POST", "/transfer", bytes.NewReader(body))

	w := httptest.NewRecorder()
	h.transfer(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var errResp domain.ErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)

	if errResp.Message != "Idempotency-Key header is required" {
		t.Errorf("expected 'Idempotency-Key header is required', got '%s'", errResp.Message)
	}
}

func TestTransferInvalidAmount(t *testing.T) {
	logger := service.NewLogger("info", os.Stdout)
	h := NewHandler(logger)

	reqID := util.GenerateRequestID()
	transferReq := domain.TransferRequest{
		From:   "userA",
		To:     "userB",
		Amount: 0,
	}

	body, _ := json.Marshal(transferReq)
	req := httptest.NewRequest("POST", "/transfer", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", reqID)

	w := httptest.NewRecorder()
	h.transfer(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestTransferSameFromTo(t *testing.T) {
	logger := service.NewLogger("info", os.Stdout)
	h := NewHandler(logger)

	reqID := util.GenerateRequestID()
	transferReq := domain.TransferRequest{
		From:   "userA",
		To:     "userA",
		Amount: 100,
	}

	body, _ := json.Marshal(transferReq)
	req := httptest.NewRequest("POST", "/transfer", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", reqID)

	w := httptest.NewRecorder()
	h.transfer(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}
