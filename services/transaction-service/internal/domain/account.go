package domain

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Account represents a bank account with balance tracking
type Account struct {
	ID            string    `json:"id"`
	AccountNumber string    `json:"account_number"`
	OwnerName     string    `json:"owner_name"`
	Balance       string    `json:"balance"` // Use string for precise decimal handling (NUMERIC type)
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	Version       int       `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TransactionRecord represents a completed transaction in the ledger
type TransactionRecord struct {
	ID            string         `json:"id"`
	FromAccountID string         `json:"from_account_id"`
	ToAccountID   string         `json:"to_account_id"`
	Amount        string         `json:"amount"` // Use string for precise decimal handling (NUMERIC type)
	Currency      string         `json:"currency"`
	Status        string         `json:"status"`
	Description   string         `json:"description"`
	ReferenceCode string         `json:"reference_code"`
	FailureReason sql.NullString `json:"failure_reason"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	CompletedAt   *time.Time     `json:"completed_at"`
}

// LedgerEntry represents an immutable ledger entry for audit trail
type LedgerEntry struct {
	ID            string    `json:"id"`
	TransactionID string    `json:"transaction_id"`
	AccountID     string    `json:"account_id"`
	EntryType     string    `json:"entry_type"` // debit or credit
	Amount        string    `json:"amount"`
	BalanceBefore string    `json:"balance_before"`
	BalanceAfter  string    `json:"balance_after"`
	CreatedAt     time.Time `json:"created_at"`
}

// TransactionStatus enum for transaction lifecycle
type TransactionStatus string

const (
	TransactionPending    TransactionStatus = "pending"
	TransactionProcessing TransactionStatus = "processing"
	TransactionCompleted  TransactionStatus = "completed"
	TransactionFailed     TransactionStatus = "failed"
	TransactionReversed   TransactionStatus = "reversed"
)

// AccountStatus enum for account lifecycle
type AccountStatus string

const (
	AccountActive    AccountStatus = "active"
	AccountSuspended AccountStatus = "suspended"
	AccountClosed    AccountStatus = "closed"
)

// AccountRepository handles account persistence
type AccountRepository interface {
	GetByID(id string) (*Account, error)
	GetByAccountNumber(accountNumber string) (*Account, error)
	UpdateBalance(id string, newBalance string, expectedVersion int) error
	Create(account *Account) error
}

// TransactionRecordRepository handles transaction record persistence
type TransactionRecordRepository interface {
	Create(tx *TransactionRecord) error
	GetByID(id string) (*TransactionRecord, error)
	UpdateStatus(id string, status TransactionStatus, failureReason *string) error
	ListByAccount(accountID string, limit int, offset int) ([]*TransactionRecord, error)
}

// LedgerRepository handles immutable ledger entry persistence
type LedgerRepository interface {
	Create(entry *LedgerEntry) error
	GetByTransactionID(transactionID string) ([]*LedgerEntry, error)
	GetAccountHistory(accountID string, limit int, offset int) ([]*LedgerEntry, error)
}

// TransferServiceDomain orchestrates money transfers between accounts
type TransferServiceDomain interface {
	Transfer(ctx context.Context, fromAccountID, toAccountID string, amount string) (*TransactionRecord, error)
	GetTransaction(id string) (*TransactionRecord, error)
	GetAccountBalance(id string) (string, error)
}

// Validation errors
var (
	ErrInsufficientFunds    = errors.New("insufficient funds")
	ErrAccountNotFound      = errors.New("account not found")
	ErrAccountSuspended     = errors.New("account is suspended")
	ErrCurrencyMismatch     = errors.New("currency mismatch")
	ErrInvalidAmount        = errors.New("invalid amount")
	ErrOptimisticLockFailed = errors.New("optimistic lock failed, account was modified")
)

func (t TransactionStatus) IsValid() bool {
	switch t {
	case TransactionPending, TransactionProcessing, TransactionCompleted, TransactionFailed, TransactionReversed:
		return true
	}
	return false
}

func (a AccountStatus) IsValid() bool {
	switch a {
	case AccountActive, AccountSuspended, AccountClosed:
		return true
	}
	return false
}
