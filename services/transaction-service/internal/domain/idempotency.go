package domain

import "time"

type IdempotencyRecord struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	RequestHash string    `json:"request_hash"`
	Response    string    `json:"response"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type IdempotencyRepository interface {
	// Create stores a new idempotency record with pending status
	Create(key, requestHash string) (*IdempotencyRecord, error)

	// GetByKey retrieves an existing idempotency record
	GetByKey(key string) (*IdempotencyRecord, error)

	// UpdateResponse updates the response and marks as completed
	UpdateResponse(key string, response string) error

	// CheckAndCreateIfNotExists attempts to get existing or create new
	CheckAndCreateIfNotExists(key, requestHash string) (*IdempotencyRecord, error)
}

// ErrIdempotencyKeyExists is returned when a key already exists with different request hash
type ErrIdempotencyConflict struct {
	Message string
}

func (e *ErrIdempotencyConflict) Error() string {
	return e.Message
}
