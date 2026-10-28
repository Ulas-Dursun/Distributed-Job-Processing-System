package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/ulasdursun/queueforge/internal/domain"
)

// Ensure JobRepository satisfies domain.JobRepository at compile time.
var _ domain.JobRepository = (*JobRepository)(nil)

type JobRepository struct {
	db *sql.DB
}

func NewJobRepository(db *sql.DB) *JobRepository {
	return &JobRepository{db: db}
}

// Create inserts a new job into the database.
func (r *JobRepository) Create(ctx context.Context, job *domain.Job) error {
	query := `
		INSERT INTO jobs (id, type, payload, status, attempts, max_retries, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := r.db.ExecContext(ctx, query,
		job.ID,
		string(job.Type),
		job.Payload,
		job.Status,
		job.Attempts,
		job.MaxRetries,
		job.CreatedAt,
		job.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}
	return nil
}

// GetByID fetches a single job by its UUID.
func (r *JobRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Job, error) {
	query := `
		SELECT id, type, payload, status, result, error_msg,
		       attempts, max_retries, created_at, updated_at
		FROM jobs
		WHERE id = $1`

	job := &domain.Job{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&job.ID,
		&job.Type,
		&job.Payload,
		&job.Status,
		&job.Result,
		&job.ErrorMsg,
		&job.Attempts,
		&job.MaxRetries,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrJobNotFound
		}
		return nil, fmt.Errorf("failed to get job: %w", err)
	}
	return job, nil
}

// List fetches jobs, optionally filtered by status, with pagination support.
func (r *JobRepository) List(ctx context.Context, status string, limit int, offset int) ([]*domain.Job, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT id, type, payload, status, result, error_msg,
		       attempts, max_retries, created_at, updated_at
		FROM jobs`

	args := []any{}
	argIdx := 1

	if status != "" {
		query += fmt.Sprintf(" WHERE status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*domain.Job
	for rows.Next() {
		job := &domain.Job{}
		if err := rows.Scan(
			&job.ID,
			&job.Type,
			&job.Payload,
			&job.Status,
			&job.Result,
			&job.ErrorMsg,
			&job.Attempts,
			&job.MaxRetries,
			&job.CreatedAt,
			&job.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan job: %w", err)
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// UpdateStatus updates the job status and timestamp.
func (r *JobRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.JobStatus) error {
	query := `
		UPDATE jobs
		SET status = $1, updated_at = $2
		WHERE id = $3`

	_, err := r.db.ExecContext(ctx, query, status, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}
	return nil
}

// UpdateAfterProcessing updates status, result or error, and increments attempts.
func (r *JobRepository) UpdateAfterProcessing(ctx context.Context, job *domain.Job) error {
	query := `
		UPDATE jobs
		SET status     = $1,
		    result     = $2,
		    error_msg  = $3,
		    attempts   = $4,
		    updated_at = $5
		WHERE id = $6`

	_, err := r.db.ExecContext(ctx, query,
		job.Status,
		job.Result,
		job.ErrorMsg,
		job.Attempts,
		time.Now(),
		job.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update job after processing: %w", err)
	}
	return nil
}
