package domain

import (
	"errors"
	"strings"
	"time"
)

type CreateAccountRequest struct {
	AccountNumber  string  `json:"account_number"`
	OwnerName      string  `json:"owner_name"`
	InitialBalance float64 `json:"initial_balance"`
	Currency       string  `json:"currency"`
}

type AccountSummary struct {
	ID            string    `json:"id"`
	AccountNumber string    `json:"account_number"`
	OwnerName     string    `json:"owner_name"`
	Balance       float64   `json:"balance"`
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

func (r *CreateAccountRequest) Validate() error {
	if r == nil {
		return errors.New("request is required")
	}
	if strings.TrimSpace(r.AccountNumber) == "" {
		return errors.New("account_number is required")
	}
	if len(strings.TrimSpace(r.AccountNumber)) > 20 {
		return errors.New("account_number must be <= 20 characters")
	}
	if strings.TrimSpace(r.OwnerName) == "" {
		return errors.New("owner_name is required")
	}
	if r.InitialBalance < 0 {
		return errors.New("initial_balance must be >= 0")
	}
	if strings.TrimSpace(r.Currency) != "" && len(strings.TrimSpace(r.Currency)) != 3 {
		return errors.New("currency must be a 3-letter code")
	}
	return nil
}
