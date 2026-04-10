package repository

import (
	"database/sql"
	"errors"
	"time"

	"github.com/chaosbank/chaosbank/services/transaction-service/internal/domain"
	"github.com/lib/pq"
)

type PostgresIdempotencyRepository struct {
	db *sql.DB
}

func NewPostgresIdempotencyRepository(db *sql.DB) domain.IdempotencyRepository {
	return &PostgresIdempotencyRepository{db: db}
}

func (r *PostgresIdempotencyRepository) Create(key, requestHash string) (*domain.IdempotencyRecord, error) {
	record := &domain.IdempotencyRecord{
		Key:         key,
		RequestHash: requestHash,
		Status:      "pending",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	err := r.db.QueryRow(
		`INSERT INTO idempotency_records (key, request_hash, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		key, requestHash, "pending", record.CreatedAt, record.UpdatedAt,
	).Scan(&record.ID)

	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			// Unique constraint violation - key already exists
			return nil, &domain.ErrIdempotencyConflict{
				Message: "Idempotency key already exists",
			}
		}
		return nil, err
	}

	return record, nil
}

func (r *PostgresIdempotencyRepository) GetByKey(key string) (*domain.IdempotencyRecord, error) {
	record := &domain.IdempotencyRecord{}

	err := r.db.QueryRow(
		`SELECT id, key, request_hash, response, status, created_at, updated_at
		 FROM idempotency_records
		 WHERE key = $1`,
		key,
	).Scan(
		&record.ID,
		&record.Key,
		&record.RequestHash,
		&record.Response,
		&record.Status,
		&record.CreatedAt,
		&record.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return record, nil
}

func (r *PostgresIdempotencyRepository) UpdateResponse(key string, response string) error {
	result, err := r.db.Exec(
		`UPDATE idempotency_records
		 SET response = $1, status = $2, updated_at = $3
		 WHERE key = $4`,
		response,
		"completed",
		time.Now().UTC(),
		key,
	)

	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return errors.New("idempotency record not found")
	}

	return nil
}

func (r *PostgresIdempotencyRepository) CheckAndCreateIfNotExists(key, requestHash string) (*domain.IdempotencyRecord, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Try to get existing record
	var existingID, existingHash, existingResponse, existingStatus string
	var existingCreatedAt, existingUpdatedAt time.Time

	err = tx.QueryRow(
		`SELECT id, request_hash, response, status, created_at, updated_at
		 FROM idempotency_records
		 WHERE key = $1
		 FOR UPDATE`,
		key,
	).Scan(&existingID, &existingHash, &existingResponse, &existingStatus, &existingCreatedAt, &existingUpdatedAt)

	if err == nil {
		// Record exists
		if existingHash != requestHash {
			// Different request hash - conflict
			if errCommit := tx.Commit(); errCommit != nil {
				return nil, errCommit
			}
			return nil, &domain.ErrIdempotencyConflict{
				Message: "Idempotency key used with different request payload",
			}
		}

		// Same request - return existing record
		if errCommit := tx.Commit(); errCommit != nil {
			return nil, errCommit
		}

		return &domain.IdempotencyRecord{
			ID:          existingID,
			Key:         key,
			RequestHash: existingHash,
			Response:    existingResponse,
			Status:      existingStatus,
			CreatedAt:   existingCreatedAt,
			UpdatedAt:   existingUpdatedAt,
		}, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// Record doesn't exist - create it
	now := time.Now().UTC()
	var newID string

	err = tx.QueryRow(
		`INSERT INTO idempotency_records (key, request_hash, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		key, requestHash, "pending", now, now,
	).Scan(&newID)

	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			// Race condition: another request created the key
			if errRollback := tx.Rollback(); errRollback != nil {
				return nil, errRollback
			}
			// Retry the check
			return r.CheckAndCreateIfNotExists(key, requestHash)
		}
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &domain.IdempotencyRecord{
		ID:          newID,
		Key:         key,
		RequestHash: requestHash,
		Status:      "pending",
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}
