package domain

import (
	"errors"
	"strings"
)

type TransferRequest struct {
	From   string  `json:"from"`
	To     string  `json:"to"`
	Amount float64 `json:"amount"`
}

type TransferResponse struct {
	RequestID string  `json:"request_id"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	Amount    float64 `json:"amount"`
	Status    string  `json:"status"`
}

type ErrorResponse struct {
	RequestID string `json:"request_id"`
	Message   string `json:"message"`
	Code      string `json:"code"`
}

func (tr *TransferRequest) Validate() error {
	if tr == nil {
		return errors.New("transfer request is nil")
	}

	if strings.TrimSpace(tr.From) == "" {
		return errors.New("from field is required")
	}

	if strings.TrimSpace(tr.To) == "" {
		return errors.New("to field is required")
	}

	if tr.From == tr.To {
		return errors.New("from and to fields cannot be the same")
	}

	if tr.Amount <= 0 {
		return errors.New("amount must be greater than 0")
	}

	return nil
}
