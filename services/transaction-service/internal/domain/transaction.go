package domain

import "time"

type Transaction struct {
	ID        string    `json:"id"`
	Amount    float64   `json:"amount"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type TransactionRepository interface {
	Create(tx *Transaction) error
	GetByID(id string) (*Transaction, error)
	UpdateStatus(id, status string) error
}

type TransactionUsecase interface {
	CreateTransaction(amount float64, from, to string) (*Transaction, error)
	ProcessTransaction(id string) error
}
